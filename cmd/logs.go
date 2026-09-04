package cmd

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/jpvelasco/juggernaut/v5/internal/keychain"
	"github.com/jpvelasco/juggernaut/v5/internal/provider"
	"github.com/jpvelasco/juggernaut/v5/internal/redact"
	"github.com/jpvelasco/juggernaut/v5/internal/safepath"
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Export diagnostic bundles",
}

var logsExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Write a diagnostic zip (redacted by default)",
	Long: `Write a local diagnostic bundle for support.

By default the zip is redacted: tokens, account IDs, home-path usernames,
emails, hostnames, and LAN IPs are replaced with stable placeholders.
Pass --raw (or --include-secrets) only for private/self debugging.

The bundle includes juggernaut version, OS/arch, relevant env var names,
configured CLI names, and the same doctor/show data those commands print.
It does not copy the live keychain or settings.json into the default zip.`,
	RunE: runLogsExport,
}

var logsExportFlags struct {
	out            string
	raw            bool
	includeSecrets bool
}

var logsExportNow = time.Now

func init() {
	f := logsExportCmd.Flags()
	f.StringVar(&logsExportFlags.out, "out", "", "output zip path (default: juggernaut-logs-<timestamp>.zip in the current directory)")
	f.BoolVar(&logsExportFlags.raw, "raw", false, "include secrets (local/self use only; not the default)")
	f.BoolVar(&logsExportFlags.includeSecrets, "include-secrets", false, "alias for --raw")
	logsCmd.AddCommand(logsExportCmd)
	rootCmd.AddCommand(logsCmd)
}

func runLogsExport(_ *cobra.Command, _ []string) error {
	home, err := homeDir()
	if err != nil {
		return err
	}
	raw := logsExportFlags.raw || logsExportFlags.includeSecrets
	if raw {
		warnf("writing unredacted diagnostic bundle with secrets; keep it local and do not share")
	}
	files, err := buildDiagnosticFiles(home, raw)
	if err != nil {
		return err
	}
	path, err := resolveLogsOutPath(logsExportFlags.out)
	if err != nil {
		return err
	}
	if err := writeDiagnosticZip(path, files); err != nil {
		return fmt.Errorf("writing diagnostic bundle: %w", err)
	}
	kind := "redacted"
	if raw {
		kind = "unredacted"
	}
	fmt.Printf("Wrote %s diagnostic bundle to %s\n", kind, path)
	return nil
}

func defaultLogsFileName() string {
	return "juggernaut-logs-" + logsExportNow().UTC().Format("20060102T150405Z") + ".zip"
}

func resolveLogsOutPath(out string) (string, error) {
	if out == "" {
		out = defaultLogsFileName()
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return "", fmt.Errorf("resolving output path: %w", err)
	}
	if info, err := os.Stat(abs); err == nil && info.IsDir() {
		return "", fmt.Errorf("output path %q is a directory", abs)
	}
	base := filepath.Dir(abs)
	return safepath.JoinUnder(base, filepath.Base(abs))
}

func writeDiagnosticZip(path string, files map[string][]byte) error {
	data, err := zipBytes(files)
	if err != nil {
		return err
	}
	return safepath.WriteFile(filepath.Dir(path), path, data)
}

func zipBytes(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0o600)
		w, err := zw.CreateHeader(h)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		if _, err := w.Write(files[name]); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildDiagnosticFiles(home string, raw bool) (map[string][]byte, error) {
	report, err := collectDoctorReport(home, "claude", "")
	if err != nil {
		return nil, fmt.Errorf("collecting doctor output: %w", err)
	}
	// The bundle never uses show's field-level [redacted] masking: redact.String
	// is the single redactor (and is skipped entirely under --raw), so the raw
	// value is left intact for the pattern pass to handle.
	showOut, err := formatShow(collectShowResults(provider.MustGet("claude"), home, "", false), "", false)
	if err != nil {
		return nil, fmt.Errorf("collecting show output: %w", err)
	}

	doctorOut := report.String()
	if !raw {
		opts := redact.Options{Home: home}
		if token, err := keychain.Default().GetWithFallback(home); err == nil && token != "" {
			opts.Secrets = []string{token}
		}
		doctorOut = redact.String(doctorOut, opts)
		showOut = redact.String(showOut, opts)
	}

	privacy := "redacted"
	if raw {
		privacy = "raw"
	}
	return map[string][]byte{
		"manifest.txt":  []byte(fmt.Sprintf("privacy=%s\ncreated=%s\n", privacy, logsExportNow().UTC().Format(time.RFC3339))),
		"version.txt":   []byte(Version + "\n"),
		"runtime.txt":   []byte(fmt.Sprintf("os=%s\narch=%s\n", runtime.GOOS, runtime.GOARCH)),
		"env.txt":       []byte(formatRelevantEnv(raw)),
		"providers.txt": []byte(formatConfiguredProviders(home)),
		"doctor.txt":    []byte(doctorOut),
		"show.txt":      []byte(showOut),
	}, nil
}

func formatConfiguredProviders(home string) string {
	var configured []string
	for _, name := range provider.AllNames() {
		p := provider.MustGet(name)
		if providerConfigured(p, home) {
			configured = append(configured, name)
		}
	}
	var b strings.Builder
	b.WriteString("supported: " + provider.SupportedNames() + "\n")
	if len(configured) == 0 {
		b.WriteString("configured: (none)\n")
		return b.String()
	}
	b.WriteString("configured: " + strings.Join(configured, ", ") + "\n")
	return b.String()
}

func providerConfigured(p provider.Provider, home string) bool {
	for _, scope := range []string{"user", "project"} {
		data, err := readProviderConfig(p, home, scope)
		if err != nil || len(data) == 0 {
			continue
		}
		if p.OwnsConfig(data) {
			return true
		}
	}
	return false
}

func formatRelevantEnv(raw bool) string {
	names := relevantEnvNames()
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		value := os.Getenv(name)
		if !raw {
			value = redactedEnvValue(name)
		}
		fmt.Fprintf(&b, "%s=%s\n", name, value)
	}
	return b.String()
}

func relevantEnvNames() []string {
	seen := map[string]struct{}{}
	var names []string
	add := func(name string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if isDiagnosticEnv(name) {
			add(name)
		}
	}
	return names
}

func isDiagnosticEnv(name string) bool {
	switch {
	case strings.HasPrefix(name, "AWS_"),
		strings.HasPrefix(name, "ANTHROPIC_"),
		strings.HasPrefix(name, "CLAUDE_"),
		strings.HasPrefix(name, "BEDROCK_"),
		strings.HasPrefix(name, "JUGGERNAUT_"):
		return true
	}
	switch name {
	case "HOME", "USERPROFILE", "USERNAME", "USER", "LOGNAME",
		"HOSTNAME", "COMPUTERNAME", "PATH":
		return true
	}
	return false
}

func redactedEnvValue(name string) string {
	upper := strings.ToUpper(name)
	switch {
	case strings.Contains(upper, "TOKEN"),
		strings.Contains(upper, "SECRET"),
		strings.Contains(upper, "PASSWORD"),
		strings.Contains(upper, "CREDENTIAL"),
		strings.Contains(upper, "ACCESS_KEY"),
		strings.HasSuffix(upper, "_KEY"):
		return redact.Token
	case name == "HOME", name == "USERPROFILE":
		return redact.Home
	default:
		return "<redacted>"
	}
}
