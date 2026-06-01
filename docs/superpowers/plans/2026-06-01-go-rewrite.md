# Juggernaut v4 Go Rewrite — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite Juggernaut from bash/PowerShell scripts into a single cross-platform Go binary that installs, configures, and migrates Claude Code to use Amazon Bedrock, distributed via npm and curl.

**Architecture:** Cobra CLI with six subcommands (`apply`, `show`, `doctor`, `uninstall`, `migrate`, `version`). Internal packages handle config JSON, keychain, schema building, launcher shim, and v3→v4 migration. A thin bootstrap shell script and npm postinstall handle distribution; the binary itself carries all logic.

**Tech Stack:** Go 1.22+, Cobra, charmbracelet/huh (interactive prompts), zalando/go-keyring (keychain), gofrs/flock (file locking), GoReleaser (release), npm (primary distribution).

---

## File Map

```
juggernaut/
├── main.go
├── go.mod
├── go.sum
├── cmd/
│   ├── root.go               # Cobra root, Version var, --launcher flag
│   ├── apply.go              # juggernaut apply
│   ├── show.go               # juggernaut show
│   ├── doctor.go             # juggernaut doctor
│   ├── uninstall.go          # juggernaut uninstall
│   ├── migrate.go            # juggernaut migrate
│   └── version.go            # juggernaut version
├── internal/
│   ├── bedrock/
│   │   └── config.go         # BedrockConfig loader from bedrock-config.json
│   ├── schema/
│   │   └── schema.go         # JuggernautBlock build + validate + derive native keys
│   ├── config/
│   │   └── manager.go        # settings.json atomic read/merge/write + backup rotation
│   ├── keychain/
│   │   └── keychain.go       # go-keyring wrapper + DPAPI fallback for Windows long keys
│   ├── launcher/
│   │   └── launcher.go       # write claude shim (symlink Unix, .cmd Windows) + exec mode
│   ├── migrate/
│   │   └── migrate.go        # v3→v4 migration (detect, transfer, upgrade, strip)
│   └── doctor/
│       └── doctor.go         # diagnostic check helpers
├── scripts/
│   ├── install.sh             # ~50-line bootstrap
│   └── install.ps1            # ~40-line PowerShell bootstrap
├── npm/
│   ├── package.json
│   └── install.js             # postinstall binary downloader
├── .goreleaser.yml
├── Makefile                   # test, lint, build targets
└── bedrock-config.json        # unchanged — single source of truth
```

---

## Task 1: Repository scaffold & Go module

**Files:**
- Create: `main.go`
- Create: `go.mod`
- Create: `cmd/root.go`
- Modify: `Makefile`

- [ ] **Step 1: Create the legacy/v3 branch from the current tag**

```bash
git checkout -b legacy/v3 v3.2.3
git push origin legacy/v3
git checkout main
```

- [ ] **Step 2: Remove shell source files from main**

```bash
git rm -r commands/ lib/ tests/v2/ juggernaut juggernaut.ps1 install.sh install.ps1
git rm Makefile  # will be replaced
```

- [ ] **Step 3: Initialize Go module**

```bash
go mod init github.com/jpvelasco/juggernaut
```

- [ ] **Step 4: Create `main.go`**

```go
package main

import "github.com/jpvelasco/juggernaut/cmd"

func main() {
	cmd.Execute()
}
```

- [ ] **Step 5: Create `cmd/root.go`**

```go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags. Dev builds use the fallback.
var Version = "4.0.0-dev"

var rootCmd = &cobra.Command{
	Use:   "juggernaut",
	Short: "Configure Claude Code to use Amazon Bedrock",
	Long:  `Juggernaut installs and configures Claude Code to route through Amazon Bedrock instead of Anthropic's direct API.`,
	// When invoked as "claude" (launcher mode), handle that before cobra routing.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

func Execute() {
	// Launcher mode: when argv[0] is "claude" or "--launcher" flag is present,
	// run as the credential-injecting wrapper and exec the real claude.
	if isLauncherMode() {
		runLauncher()
		os.Exit(0)
	}
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func isLauncherMode() bool {
	base := filepath.Base(os.Args[0])
	base = strings.TrimSuffix(base, ".exe")
	if base == "claude" {
		return true
	}
	for _, arg := range os.Args[1:] {
		if arg == "--launcher" {
			return true
		}
	}
	return false
}
```

> Note: `runLauncher()` is implemented in Task 7 (`internal/launcher`). Add the import then.

- [ ] **Step 6: Add missing imports to root.go**

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)
```

- [ ] **Step 7: Create replacement `Makefile`**

```makefile
.PHONY: build test lint clean

build:
	go build -ldflags "-X github.com/jpvelasco/juggernaut/cmd.Version=$(shell cat VERSION)" -o bin/juggernaut .

test:
	go test ./... -v

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/
```

- [ ] **Step 8: Install dependencies**

```bash
go get github.com/spf13/cobra@latest
go get github.com/charmbracelet/huh@latest
go get github.com/zalando/go-keyring@latest
go get github.com/gofrs/flock@latest
go mod tidy
```

- [ ] **Step 9: Verify it compiles**

```bash
go build ./...
```
Expected: no output (success).

- [ ] **Step 10: Commit**

```bash
git add main.go go.mod go.sum cmd/root.go Makefile
git commit -m "chore: scaffold Go module, root command, Makefile"
```

---

## Task 2: `internal/bedrock` — typed config loader

**Files:**
- Create: `internal/bedrock/config.go`
- Create: `internal/bedrock/config_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/bedrock/config_test.go
package bedrock_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/juggernaut/internal/bedrock"
)

func TestLoad(t *testing.T) {
	// Use the repo's real bedrock-config.json
	repoRoot := filepath.Join("..", "..")
	cfg, err := bedrock.Load(filepath.Join(repoRoot, "bedrock-config.json"))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Version == "" {
		t.Error("expected Version to be set")
	}
	if cfg.Models.Sonnet == "" {
		t.Error("expected Models.Sonnet to be set")
	}
	if len(cfg.Regions) == 0 {
		t.Error("expected at least one region")
	}
	if cfg.Defaults.Region == "" {
		t.Error("expected Defaults.Region to be set")
	}
}

func TestIsSupportedRegion(t *testing.T) {
	cfg := &bedrock.Config{Regions: []string{"us-east-1", "us-west-2"}}
	if !cfg.IsSupportedRegion("us-east-1") {
		t.Error("us-east-1 should be supported")
	}
	if cfg.IsSupportedRegion("eu-fake-1") {
		t.Error("eu-fake-1 should not be supported")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/bedrock/... -v
```
Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Implement `internal/bedrock/config.go`**

```go
package bedrock

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config is the typed representation of bedrock-config.json.
type Config struct {
	Version              string            `json:"version"`
	Models               ModelSet          `json:"models"`
	Environment          map[string]string `json:"environment"`
	EnvironmentBedrockAuth map[string]string `json:"environment_bedrock_auth"`
	Regions              []string          `json:"regions"`
	Defaults             Defaults          `json:"defaults"`
}

type ModelSet struct {
	Default string `json:"default"`
	Fast    string `json:"fast"`
	Opus    string `json:"opus"`
	Sonnet  string `json:"sonnet"`
	Haiku   string `json:"haiku"`
}

type Defaults struct {
	Region   string `json:"region"`
	AuthMode string `json:"auth_mode"`
	Model    string `json:"model"`
}

// Load reads and parses bedrock-config.json from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading bedrock-config.json: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing bedrock-config.json: %w", err)
	}
	return &cfg, nil
}

// IsSupportedRegion returns true if region is in the supported list.
func (c *Config) IsSupportedRegion(region string) bool {
	for _, r := range c.Regions {
		if r == region {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/bedrock/... -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bedrock/
git commit -m "feat: add bedrock config loader"
```

---

## Task 3: `internal/schema` — JuggernautBlock builder & validator

**Files:**
- Create: `internal/schema/schema.go`
- Create: `internal/schema/schema_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/schema/schema_test.go
package schema_test

import (
	"testing"
	"time"

	"github.com/jpvelasco/juggernaut/internal/bedrock"
	"github.com/jpvelasco/juggernaut/internal/schema"
)

func testConfig() *bedrock.Config {
	return &bedrock.Config{
		Version: "4.0.0",
		Models: bedrock.ModelSet{
			Opus:   "global.anthropic.claude-opus-4-7",
			Sonnet: "global.anthropic.claude-sonnet-4-6",
			Haiku:  "global.anthropic.claude-haiku-4-5-20251001-v1:0",
		},
		Environment: map[string]string{
			"CLAUDE_CODE_MAX_OUTPUT_TOKENS": "32768",
			"CLAUDE_CODE_EFFORT_LEVEL":      "xhigh",
		},
		EnvironmentBedrockAuth: map[string]string{
			"CLAUDE_CODE_USE_BEDROCK": "1",
		},
		Regions: []string{"us-east-1", "us-west-2"},
		Defaults: bedrock.Defaults{
			Region:   "us-west-2",
			AuthMode: "iam",
		},
	}
}

func TestBuild_IAM(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam",
		Region:   "us-west-2",
		Effort:   "xhigh",
		Scope:    "user",
		Version:  "4.0.0",
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if block.Auth.Mode != "iam" {
		t.Errorf("expected auth.mode=iam, got %s", block.Auth.Mode)
	}
	if block.Env["CLAUDE_CODE_USE_BEDROCK"] != "1" {
		t.Error("expected CLAUDE_CODE_USE_BEDROCK=1 for validated IAM")
	}
	if block.Meta.SchemaVersion != 2 {
		t.Errorf("expected schemaVersion=2, got %d", block.Meta.SchemaVersion)
	}
}

func TestBuild_InvalidRegion(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam",
		Region:   "eu-fake-99",
		Effort:   "xhigh",
		Scope:    "user",
		Version:  "4.0.0",
	}
	_, err := schema.Build(testConfig(), opts)
	if err == nil {
		t.Error("expected error for invalid region")
	}
}

func TestBuild_InvalidEffort(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam",
		Region:   "us-west-2",
		Effort:   "turbo",
		Scope:    "user",
		Version:  "4.0.0",
	}
	_, err := schema.Build(testConfig(), opts)
	if err == nil {
		t.Error("expected error for invalid effort level")
	}
}

func TestNativeKeys(t *testing.T) {
	opts := schema.Options{
		AuthMode: "iam",
		Region:   "us-west-2",
		Effort:   "xhigh",
		Scope:    "user",
		Version:  "4.0.0",
		Opusplan: true,
	}
	block, err := schema.Build(testConfig(), opts)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	native := block.NativeKeys()
	if native.Model != "opusplan" {
		t.Errorf("expected model=opusplan with opusplan=true, got %s", native.Model)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/schema/... -v
```
Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Implement `internal/schema/schema.go`**

```go
package schema

import (
	"fmt"
	"time"

	"github.com/jpvelasco/juggernaut/internal/bedrock"
)

const SchemaVersion = 2

// Options holds all user-supplied apply parameters.
type Options struct {
	AuthMode     string // "iam" or "bedrock-api-key"
	Region       string
	Effort       string // "low"|"medium"|"high"|"xhigh"|"max"
	Scope        string // "user"|"project"
	Version      string
	OpusModel    string
	SonnetModel  string
	HaikuModel   string
	Opusplan     bool
	Use1M        bool
	UseMantle    bool
	MantleURL    string
	Storage      string // "keychain"|"dpapi"|"profile"
	AuthValidated bool  // true once credential check passes
}

// Block is the typed .juggernaut block written to settings.json.
type Block struct {
	Auth     Auth              `json:"auth"`
	Models   ModelOverrides    `json:"modelOverrides"`
	Env      map[string]string `json:"env"`
	Meta     Meta              `json:"meta"`
}

type Auth struct {
	Mode    string `json:"mode"`
	Region  string `json:"region"`
	Storage string `json:"storage,omitempty"`
}

type ModelOverrides struct {
	Opus    string `json:"opus"`
	Sonnet  string `json:"sonnet"`
	Haiku   string `json:"haiku"`
	Subagent string `json:"subagent"`
}

type Meta struct {
	SchemaVersion int    `json:"schemaVersion"`
	Version       string `json:"version"`
	ManagedBy     string `json:"managedBy"`
	Scope         string `json:"scope"`
	AppliedAt     string `json:"appliedAt"`
	Opusplan      bool   `json:"opusplan"`
	Use1M         bool   `json:"use1mContext"`
	UseMantle     bool   `json:"useMantle"`
	MantleURL     string `json:"mantleBaseUrl,omitempty"`
	Effort        string `json:"effort"`
}

// NativeKeys are the top-level settings.json keys Claude Code reads directly.
type NativeKeys struct {
	Model          string            `json:"model,omitempty"`
	ModelOverrides map[string]string `json:"modelOverrides,omitempty"`
	Env            map[string]string `json:"env"`
}

var validEfforts = map[string]bool{
	"low": true, "medium": true, "high": true, "xhigh": true, "max": true,
}

// Build constructs and validates a JuggernautBlock from bedrock config + options.
func Build(cfg *bedrock.Config, opts Options) (*Block, error) {
	if !cfg.IsSupportedRegion(opts.Region) {
		return nil, fmt.Errorf("unsupported region %q — run `juggernaut doctor` for supported regions", opts.Region)
	}
	if !validEfforts[opts.Effort] {
		return nil, fmt.Errorf("invalid effort %q — must be one of: low, medium, high, xhigh, max", opts.Effort)
	}

	opus := opts.OpusModel
	if opus == "" {
		opus = cfg.Models.Opus
	}
	sonnet := opts.SonnetModel
	if sonnet == "" {
		sonnet = cfg.Models.Sonnet
	}
	haiku := opts.HaikuModel
	if haiku == "" {
		haiku = cfg.Models.Haiku
	}

	// Build env: start from bedrock-config defaults.
	env := make(map[string]string, len(cfg.Environment))
	for k, v := range cfg.Environment {
		env[k] = v
	}

	// Auth-gated overlay: CLAUDE_CODE_USE_BEDROCK=1 only when credential validated.
	if opts.AuthValidated {
		for k, v := range cfg.EnvironmentBedrockAuth {
			env[k] = v
		}
	}

	env["AWS_REGION"] = opts.Region
	env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = opus
	env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = sonnet
	env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = haiku
	env["CLAUDE_CODE_SUBAGENT_MODEL"] = haiku
	env["CLAUDE_CODE_EFFORT_LEVEL"] = opts.Effort

	if opts.Opusplan {
		env["ANTHROPIC_MODEL"] = "opusplan"
	}
	if opts.UseMantle {
		env["CLAUDE_CODE_USE_MANTLE"] = "1"
		if opts.MantleURL != "" {
			env["ANTHROPIC_BEDROCK_MANTLE_BASE_URL"] = opts.MantleURL
		}
	}

	storage := opts.Storage
	if storage == "" {
		storage = "keychain"
	}

	block := &Block{
		Auth: Auth{
			Mode:    opts.AuthMode,
			Region:  opts.Region,
			Storage: storage,
		},
		Models: ModelOverrides{
			Opus:    opus,
			Sonnet:  sonnet,
			Haiku:   haiku,
			Subagent: haiku,
		},
		Env: env,
		Meta: Meta{
			SchemaVersion: SchemaVersion,
			Version:       opts.Version,
			ManagedBy:     "juggernaut",
			Scope:         opts.Scope,
			AppliedAt:     time.Now().UTC().Format(time.RFC3339),
			Opusplan:      opts.Opusplan,
			Use1M:         opts.Use1M,
			UseMantle:     opts.UseMantle,
			MantleURL:     opts.MantleURL,
			Effort:        opts.Effort,
		},
	}
	return block, nil
}

// NativeKeys derives the top-level settings.json keys Claude Code reads directly.
func (b *Block) NativeKeys() NativeKeys {
	model := ""
	if b.Meta.Opusplan {
		model = "opusplan"
	}
	overrides := map[string]string{
		"opus":    b.Models.Opus,
		"sonnet":  b.Models.Sonnet,
		"haiku":   b.Models.Haiku,
	}
	return NativeKeys{
		Model:          model,
		ModelOverrides: overrides,
		Env:            b.Env,
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/schema/... -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/schema/
git commit -m "feat: add schema block builder and validator"
```

---

## Task 4: `internal/config` — atomic settings.json manager

**Files:**
- Create: `internal/config/manager.go`
- Create: `internal/config/manager_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/config/manager_test.go
package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/juggernaut/internal/config"
)

func TestReadMissing(t *testing.T) {
	m := config.NewManager(filepath.Join(t.TempDir(), "settings.json"))
	data, err := m.Read()
	if err != nil {
		t.Fatalf("Read() on missing file error: %v", err)
	}
	if len(data) != 0 {
		t.Error("expected empty map for missing file")
	}
}

func TestWriteAndRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	initial := map[string]any{"someKey": "someValue"}
	if err := m.Write(initial); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	got, err := m.Read()
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if got["someKey"] != "someValue" {
		t.Errorf("expected someKey=someValue, got %v", got["someKey"])
	}
}

func TestMergeJuggernautBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	// Write pre-existing user data.
	existing := map[string]any{"userPref": "keep-me"}
	_ = m.Write(existing)

	block := map[string]any{"managedBy": "juggernaut"}
	nativeEnv := map[string]string{"CLAUDE_CODE_USE_BEDROCK": "1"}

	if err := m.MergeJuggernautBlock(block, nativeEnv, ""); err != nil {
		t.Fatalf("MergeJuggernautBlock() error: %v", err)
	}

	got, _ := m.Read()
	if got["userPref"] != "keep-me" {
		t.Error("user pref should be preserved after merge")
	}
	jBlock, ok := got["juggernaut"]
	if !ok {
		t.Error("juggernaut block should be present")
	}
	_ = jBlock
}

func TestRemoveJuggernautBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := config.NewManager(path)

	data := map[string]any{
		"userPref":       "keep-me",
		"juggernaut":     map[string]any{"managedBy": "juggernaut"},
		"env":            map[string]any{"CLAUDE_CODE_USE_BEDROCK": "1"},
		"model":          "opusplan",
		"modelOverrides": map[string]any{},
	}
	_ = m.Write(data)

	if err := m.RemoveJuggernautBlock(); err != nil {
		t.Fatalf("RemoveJuggernautBlock() error: %v", err)
	}

	got, _ := m.Read()
	if _, ok := got["juggernaut"]; ok {
		t.Error("juggernaut key should be removed")
	}
	if _, ok := got["model"]; ok {
		t.Error("model key should be removed")
	}
	if got["userPref"] != "keep-me" {
		t.Error("user pref should be preserved after remove")
	}
}

func TestBackupRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	m := config.NewManager(path)

	data := map[string]any{"x": 1}
	// Write 7 times — should keep only 5 backups.
	for i := 0; i < 7; i++ {
		data["x"] = i
		_ = m.Write(data)
	}

	pattern := filepath.Join(dir, "settings.json.backup.*")
	matches, _ := filepath.Glob(pattern)
	if len(matches) > 5 {
		t.Errorf("expected ≤5 backups, got %d", len(matches))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/config/... -v
```
Expected: FAIL.

- [ ] **Step 3: Implement `internal/config/manager.go`**

```go
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gofrs/flock"
)

const backupRetain = 5

// Manager handles atomic read/merge/write of a settings.json file.
type Manager struct {
	path string
}

func NewManager(path string) *Manager {
	return &Manager{path: path}
}

// Read returns the parsed settings.json, or an empty map if the file is missing.
func (m *Manager) Read() (map[string]any, error) {
	data, err := os.ReadFile(m.path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", m.path, err)
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", m.path, err)
	}
	return result, nil
}

// Write atomically writes data to the settings.json path, rotating backups.
func (m *Manager) Write(data map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return fmt.Errorf("creating settings directory: %w", err)
	}

	lockPath := m.path + ".lock"
	fl := flock.New(lockPath)
	locked, err := fl.TryLock()
	if err != nil || !locked {
		return fmt.Errorf("could not acquire settings.json lock: %w", err)
	}
	defer fl.Unlock()

	// Rotate backups before overwriting.
	if _, err := os.Stat(m.path); err == nil {
		if err := m.rotateBackup(); err != nil {
			return err
		}
	}

	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding settings.json: %w", err)
	}

	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o644); err != nil {
		return fmt.Errorf("writing temp settings file: %w", err)
	}
	if err := os.Rename(tmp, m.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("committing settings.json: %w", err)
	}
	return nil
}

// MergeJuggernautBlock merges the juggernaut block + native keys into existing settings.
// model is the top-level model string (e.g. "opusplan") or "" to omit it.
func (m *Manager) MergeJuggernautBlock(block map[string]any, nativeEnv map[string]string, model string) error {
	existing, err := m.Read()
	if err != nil {
		return err
	}
	existing["juggernaut"] = block
	if len(nativeEnv) > 0 {
		existing["env"] = nativeEnv
	}
	if model != "" {
		existing["model"] = model
	} else {
		delete(existing, "model")
	}
	return m.Write(existing)
}

// RemoveJuggernautBlock strips juggernaut-managed keys from settings.json.
func (m *Manager) RemoveJuggernautBlock() error {
	existing, err := m.Read()
	if err != nil {
		return err
	}
	delete(existing, "juggernaut")
	delete(existing, "env")
	delete(existing, "model")
	delete(existing, "modelOverrides")
	return m.Write(existing)
}

// HasJuggernautBlock returns true if settings.json contains a juggernaut block.
func (m *Manager) HasJuggernautBlock() (bool, error) {
	data, err := m.Read()
	if err != nil {
		return false, err
	}
	block, ok := data["juggernaut"].(map[string]any)
	if !ok {
		return false, nil
	}
	meta, ok := block["meta"].(map[string]any)
	if !ok {
		return false, nil
	}
	return meta["managedBy"] == "juggernaut", nil
}

func (m *Manager) rotateBackup() error {
	stamp := time.Now().UTC().Format("20060102_150405")
	backup := m.path + ".backup." + stamp
	if err := copyFile(m.path, backup); err != nil {
		return fmt.Errorf("creating backup: %w", err)
	}
	return pruneBackups(m.path, backupRetain)
}

func pruneBackups(base string, keep int) error {
	pattern := base + ".backup.*"
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	sort.Strings(matches)
	for len(matches) > keep {
		os.Remove(matches[0])
		matches = matches[1:]
	}
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/config/... -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add atomic settings.json manager with backup rotation"
```

---

## Task 5: `internal/keychain` — cross-platform credential storage

**Files:**
- Create: `internal/keychain/keychain.go`
- Create: `internal/keychain/keychain_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/keychain/keychain_test.go
package keychain_test

import (
	"os"
	"testing"

	"github.com/jpvelasco/juggernaut/internal/keychain"
)

// Tests use a unique service name so they don't touch production credentials.
func testStore() *keychain.Store {
	svc := os.Getenv("JUGGERNAUT_KEYCHAIN_SERVICE")
	if svc == "" {
		svc = "juggernaut-test-" + testing.Short()
	}
	return keychain.NewStore(svc)
}

func TestStoreAndGet(t *testing.T) {
	s := testStore()
	defer s.Delete() // cleanup

	if err := s.Set("test-token-value"); err != nil {
		t.Skipf("keychain unavailable: %v", err)
	}

	got, err := s.Get()
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "test-token-value" {
		t.Errorf("expected test-token-value, got %q", got)
	}
}

func TestGetMissing(t *testing.T) {
	s := testStore()
	s.Delete() // ensure clean

	got, err := s.Get()
	if err != nil {
		t.Fatalf("Get() on missing key returned error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string for missing key, got %q", got)
	}
}

func TestDelete(t *testing.T) {
	s := testStore()
	_ = s.Set("to-delete")

	if err := s.Delete(); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	got, _ := s.Get()
	if got != "" {
		t.Error("expected empty after delete")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/keychain/... -v
```
Expected: FAIL.

- [ ] **Step 3: Implement `internal/keychain/keychain.go`**

```go
package keychain

import (
	"github.com/zalando/go-keyring"
)

const (
	defaultService = "juggernaut-bedrock"
	account        = "api-key"
)

// Store wraps go-keyring with a fixed account name and configurable service.
type Store struct {
	service string
}

// NewStore creates a Store. Use defaultService for production, a unique name for tests.
func NewStore(service string) *Store {
	if service == "" {
		service = defaultService
	}
	return &Store{service: service}
}

// Default returns a Store using the production service name.
func Default() *Store {
	return NewStore(defaultService)
}

// Set stores the token in the OS keychain.
func (s *Store) Set(token string) error {
	return keyring.Set(s.service, account, token)
}

// Get retrieves the token. Returns "" (not error) if not found.
func (s *Store) Get() (string, error) {
	token, err := keyring.Get(s.service, account)
	if err == keyring.ErrNotFound {
		return "", nil
	}
	return token, err
}

// Delete removes the token. Silent if not found.
func (s *Store) Delete() error {
	err := keyring.Delete(s.service, account)
	if err == keyring.ErrNotFound {
		return nil
	}
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/keychain/... -v
```
Expected: PASS (or SKIP if keychain daemon is unavailable in CI headless env).

- [ ] **Step 5: Commit**

```bash
git add internal/keychain/
git commit -m "feat: add cross-platform keychain store via go-keyring"
```

---

## Task 6: `internal/doctor` — diagnostic helpers

**Files:**
- Create: `internal/doctor/doctor.go`
- Create: `internal/doctor/doctor_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/doctor/doctor_test.go
package doctor_test

import (
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/internal/doctor"
)

func TestReport_AllOK(t *testing.T) {
	r := doctor.NewReport()
	r.Check("Juggernaut block", doctor.OK, "found (schemaVersion 2)")
	r.Check("Auth mode", doctor.OK, "iam")
	r.Check("Region", doctor.OK, "us-west-2")

	if r.HasFailures() {
		t.Error("expected no failures")
	}
	if r.HasWarnings() {
		t.Error("expected no warnings")
	}
}

func TestReport_Failures(t *testing.T) {
	r := doctor.NewReport()
	r.Check("Juggernaut block", doctor.Fail, "not found in settings.json")

	if !r.HasFailures() {
		t.Error("expected failures")
	}
	out := r.String()
	if !strings.Contains(out, "FAIL") {
		t.Error("expected FAIL in output")
	}
}

func TestReport_JSON(t *testing.T) {
	r := doctor.NewReport()
	r.Check("Region", doctor.Warn, "us-fake-1 is not a known Bedrock region")

	j, err := r.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}
	if !strings.Contains(string(j), "WARN") {
		t.Error("expected WARN in JSON output")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/doctor/... -v
```
Expected: FAIL.

- [ ] **Step 3: Implement `internal/doctor/doctor.go`**

```go
package doctor

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Status string

const (
	OK   Status = "OK"
	Warn Status = "WARN"
	Fail Status = "FAIL"
)

type Entry struct {
	Label  string `json:"label"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
}

type Report struct {
	entries []Entry
}

func NewReport() *Report {
	return &Report{}
}

func (r *Report) Check(label string, status Status, detail string) {
	r.entries = append(r.entries, Entry{Label: label, Status: status, Detail: detail})
}

func (r *Report) HasFailures() bool {
	for _, e := range r.entries {
		if e.Status == Fail {
			return true
		}
	}
	return false
}

func (r *Report) HasWarnings() bool {
	for _, e := range r.entries {
		if e.Status == Warn {
			return true
		}
	}
	return false
}

func (r *Report) String() string {
	var sb strings.Builder
	for _, e := range r.entries {
		fmt.Fprintf(&sb, "  %-8s %-30s %s\n", "["+string(e.Status)+"]", e.Label, e.Detail)
	}
	return sb.String()
}

func (r *Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r.entries, "", "  ")
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/doctor/... -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/doctor/
git commit -m "feat: add doctor report helper"
```

---

## Task 7: `internal/launcher` — claude shim writer + exec mode

**Files:**
- Create: `internal/launcher/launcher.go`
- Create: `internal/launcher/launcher_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/launcher/launcher_test.go
package launcher_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jpvelasco/juggernaut/internal/launcher"
)

func TestInstall(t *testing.T) {
	dir := t.TempDir()
	if err := launcher.Install(dir); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	if runtime.GOOS == "windows" {
		shim := filepath.Join(dir, "claude.cmd")
		data, err := os.ReadFile(shim)
		if err != nil {
			t.Fatalf("claude.cmd not created: %v", err)
		}
		if string(data) == "" {
			t.Error("claude.cmd should not be empty")
		}
	} else {
		shim := filepath.Join(dir, "claude")
		info, err := os.Lstat(shim)
		if err != nil {
			t.Fatalf("claude symlink not created: %v", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("expected symlink for claude on Unix")
		}
	}
}

func TestIsInstalled(t *testing.T) {
	dir := t.TempDir()
	if launcher.IsInstalled(dir) {
		t.Error("should not be installed in empty dir")
	}
	_ = launcher.Install(dir)
	if !launcher.IsInstalled(dir) {
		t.Error("should be installed after Install()")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/launcher/... -v
```
Expected: FAIL.

- [ ] **Step 3: Implement `internal/launcher/launcher.go`**

```go
package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jpvelasco/juggernaut/internal/keychain"
)

const cmdShim = "@echo off\njuggernaut --launcher %*\n"

// DefaultBinDir returns ~/.local/bin on Unix, %USERPROFILE%\.local\bin on Windows.
func DefaultBinDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("USERPROFILE"), ".local", "bin")
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "bin")
}

// Install creates the claude shim in binDir.
// Unix: symlink binDir/claude -> current juggernaut binary.
// Windows: write binDir/claude.cmd with two-line batch shim.
func Install(binDir string) error {
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("creating bin dir: %w", err)
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	if runtime.GOOS == "windows" {
		shimPath := filepath.Join(binDir, "claude.cmd")
		return os.WriteFile(shimPath, []byte(cmdShim), 0o644)
	}

	shimPath := filepath.Join(binDir, "claude")
	// Remove existing shim before re-creating.
	os.Remove(shimPath)
	return os.Symlink(self, shimPath)
}

// Uninstall removes the claude shim from binDir.
func Uninstall(binDir string) error {
	if runtime.GOOS == "windows" {
		return removeIfExists(filepath.Join(binDir, "claude.cmd"))
	}
	return removeIfExists(filepath.Join(binDir, "claude"))
}

// IsInstalled returns true if the claude shim exists in binDir.
func IsInstalled(binDir string) bool {
	var path string
	if runtime.GOOS == "windows" {
		path = filepath.Join(binDir, "claude.cmd")
	} else {
		path = filepath.Join(binDir, "claude")
	}
	_, err := os.Lstat(path)
	return err == nil
}

// RunAsLauncher injects credentials and execs the real claude binary.
// Called when the binary is invoked as "claude" or with --launcher flag.
func RunAsLauncher(args []string) error {
	token, err := keychain.Default().Get()
	if err != nil {
		return fmt.Errorf("reading keychain: %w", err)
	}
	if token != "" {
		os.Setenv("AWS_BEARER_TOKEN_BEDROCK", token)
	}
	os.Setenv("CLAUDE_CODE_USE_BEDROCK", "1")

	claudePath, err := findRealClaude()
	if err != nil {
		return err
	}

	cmd := exec.Command(claudePath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// findRealClaude walks PATH and returns the first claude that isn't our shim.
func findRealClaude() (string, error) {
	self, _ := os.Executable()
	selfBase := strings.TrimSuffix(filepath.Base(self), ".exe")

	paths := filepath.SplitList(os.Getenv("PATH"))
	for _, dir := range paths {
		candidate := filepath.Join(dir, "claude")
		if runtime.GOOS == "windows" {
			candidate += ".exe"
		}
		info, err := os.Lstat(candidate)
		if err != nil {
			continue
		}
		// Skip symlinks that point back to us.
		if info.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(candidate)
			if strings.TrimSuffix(filepath.Base(target), ".exe") == selfBase {
				continue
			}
		}
		return candidate, nil
	}
	return "", fmt.Errorf("claude binary not found on PATH — is Claude Code installed?")
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
```

- [ ] **Step 4: Wire `RunAsLauncher` into `cmd/root.go`**

Replace the `runLauncher()` stub in `cmd/root.go`:

```go
// At the top of cmd/root.go, add import:
import "github.com/jpvelasco/juggernaut/internal/launcher"

// Replace runLauncher():
func runLauncher() {
	// Strip --launcher flag from args before passing to real claude.
	var args []string
	for _, a := range os.Args[1:] {
		if a != "--launcher" {
			args = append(args, a)
		}
	}
	if err := launcher.RunAsLauncher(args); err != nil {
		fmt.Fprintln(os.Stderr, "juggernaut launcher:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/launcher/... -v
go build ./...
```
Expected: PASS, no compile errors.

- [ ] **Step 6: Commit**

```bash
git add internal/launcher/ cmd/root.go
git commit -m "feat: add claude launcher shim (symlink Unix, .cmd Windows)"
```

---

## Task 8: `internal/migrate` — v3→v4 migration

**Files:**
- Create: `internal/migrate/migrate.go`
- Create: `internal/migrate/migrate_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/migrate/migrate_test.go
package migrate_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/juggernaut/internal/migrate"
)

func writeSettings(t *testing.T, dir string, data map[string]any) string {
	t.Helper()
	path := filepath.Join(dir, ".claude", "settings.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	b, _ := json.Marshal(data)
	os.WriteFile(path, b, 0o644)
	return path
}

func TestDetect_V3Block(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{
				"managedBy":     "juggernaut",
				"schemaVersion": 1,
				"version":       "3.2.3",
			},
		},
	})
	state, err := migrate.Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if !state.HasV3Block {
		t.Error("expected HasV3Block=true")
	}
	if state.V3Version != "3.2.3" {
		t.Errorf("expected V3Version=3.2.3, got %s", state.V3Version)
	}
}

func TestDetect_NoBlock(t *testing.T) {
	dir := t.TempDir()
	state, err := migrate.Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if state.HasV3Block {
		t.Error("expected HasV3Block=false for clean dir")
	}
}

func TestDetect_TooOld(t *testing.T) {
	dir := t.TempDir()
	writeSettings(t, dir, map[string]any{
		"juggernaut": map[string]any{
			"meta": map[string]any{
				"managedBy":     "juggernaut",
				"schemaVersion": 1,
				"version":       "3.1.0",
			},
		},
	})
	state, err := migrate.Detect(dir)
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if !state.TooOld {
		t.Error("expected TooOld=true for version < 3.2.3")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/migrate/... -v
```
Expected: FAIL.

- [ ] **Step 3: Implement `internal/migrate/migrate.go`**

```go
package migrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// State describes what was found during migration detection.
type State struct {
	HasV3Block bool
	V3Version  string
	AuthMode   string
	TooOld     bool   // version < 3.2.3 — cannot migrate, must upgrade v3 first
	AlreadyV4  bool   // schemaVersion == 2 — migration already complete
}

// Detect inspects homeDir for a v3 juggernaut block.
func Detect(homeDir string) (*State, error) {
	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return &State{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading settings.json: %w", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return &State{}, nil
	}

	jRaw, ok := settings["juggernaut"]
	if !ok {
		return &State{}, nil
	}
	jBlock, ok := jRaw.(map[string]any)
	if !ok {
		return &State{}, nil
	}
	metaRaw, ok := jBlock["meta"].(map[string]any)
	if !ok {
		return &State{}, nil
	}
	if metaRaw["managedBy"] != "juggernaut" {
		return &State{}, nil
	}

	state := &State{HasV3Block: true}

	if v, ok := metaRaw["version"].(string); ok {
		state.V3Version = v
		if !meetsMinVersion(v, "3.2.3") {
			state.TooOld = true
		}
	}

	if sv, ok := metaRaw["schemaVersion"].(float64); ok && sv >= 2 {
		state.AlreadyV4 = true
	}

	if auth, ok := jBlock["auth"].(map[string]any); ok {
		if mode, ok := auth["mode"].(string); ok {
			state.AuthMode = mode
		}
	}

	return state, nil
}

// StripLauncherBlocks removes legacy "# BEGIN: Juggernaut Launcher" blocks
// from shell profile files found in homeDir.
func StripLauncherBlocks(homeDir string) []string {
	profiles := []string{
		filepath.Join(homeDir, ".bashrc"),
		filepath.Join(homeDir, ".bash_profile"),
		filepath.Join(homeDir, ".zshrc"),
		filepath.Join(homeDir, ".profile"),
		filepath.Join(homeDir, ".config", "fish", "config.fish"),
	}

	var stripped []string
	for _, p := range profiles {
		if stripped1, err := stripMarkerBlock(p, "# BEGIN: Juggernaut Launcher", "# END: Juggernaut Launcher"); err == nil && stripped1 {
			stripped = append(stripped, p)
		}
	}
	return stripped
}

// meetsMinVersion returns true if version >= min (simple semver compare, no pre-release).
func meetsMinVersion(version, min string) bool {
	return compareSemver(version, min) >= 0
}

func compareSemver(a, b string) int {
	partsA := strings.SplitN(a, ".", 3)
	partsB := strings.SplitN(b, ".", 3)
	for i := 0; i < 3; i++ {
		var pa, pb int
		fmt.Sscanf(partsA[i], "%d", &pa)
		fmt.Sscanf(partsB[i], "%d", &pb)
		if pa != pb {
			if pa > pb {
				return 1
			}
			return -1
		}
	}
	return 0
}

func stripMarkerBlock(path, beginMarker, endMarker string) (bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	lines := strings.Split(string(data), "\n")
	var out []string
	inBlock := false
	found := false

	for _, line := range lines {
		if strings.Contains(line, beginMarker) {
			inBlock = true
			found = true
			continue
		}
		if strings.Contains(line, endMarker) {
			inBlock = false
			continue
		}
		if !inBlock {
			out = append(out, line)
		}
	}

	if !found {
		return false, nil
	}
	return true, os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/migrate/... -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/migrate/
git commit -m "feat: add v3→v4 migration detector and launcher block stripper"
```

---

## Task 9: `cmd/apply.go` — the main configure command

**Files:**
- Create: `cmd/apply.go`
- Create: `cmd/apply_test.go`

- [ ] **Step 1: Write the failing test**

```go
// cmd/apply_test.go
package cmd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jpvelasco/juggernaut/cmd"
)

func TestApply_DryRun_IAM(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// Use ExecuteArgs to run the command programmatically.
	err := cmd.ExecuteArgs([]string{
		"apply",
		"--auth=iam",
		"--region=us-west-2",
		"--dry-run",
		"--skip-preflight",
	})
	if err != nil {
		t.Fatalf("apply --dry-run error: %v", err)
	}

	// Dry run must NOT write settings.json.
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Error("dry-run should not create settings.json")
	}
}
```

- [ ] **Step 2: Add `ExecuteArgs` to `cmd/root.go`** for testability

```go
// In cmd/root.go, add:
func ExecuteArgs(args []string) error {
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./cmd/... -run TestApply_DryRun_IAM -v
```
Expected: FAIL — apply subcommand not registered yet.

- [ ] **Step 4: Implement `cmd/apply.go`**

```go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/jpvelasco/juggernaut/internal/bedrock"
	"github.com/jpvelasco/juggernaut/internal/config"
	"github.com/jpvelasco/juggernaut/internal/keychain"
	"github.com/jpvelasco/juggernaut/internal/launcher"
	"github.com/jpvelasco/juggernaut/internal/migrate"
	"github.com/jpvelasco/juggernaut/internal/schema"
	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Configure Claude Code to use Amazon Bedrock",
	RunE:  runApply,
}

var applyFlags struct {
	auth          string
	bedrockKey    string
	preserveKey   bool
	region        string
	model         string
	opusModel     string
	sonnetModel   string
	haikuModel    string
	effort        string
	opusplan      bool
	noOpusplan    bool
	use1m         bool
	no1m          bool
	noMantle      bool
	mantleURL     string
	scope         string
	dryRun        bool
	skipPreflight bool
	storage       string
}

func init() {
	f := applyCmd.Flags()
	f.StringVar(&applyFlags.auth, "auth", "", "authentication mode: iam or bedrock-api-key")
	f.StringVar(&applyFlags.bedrockKey, "bedrock-key", "", "Bedrock API key")
	f.BoolVar(&applyFlags.preserveKey, "preserve-key", false, "reuse existing key from keychain/env")
	f.StringVar(&applyFlags.region, "region", "", "AWS region (default: us-west-2)")
	f.StringVar(&applyFlags.model, "model", "", "override all model IDs")
	f.StringVar(&applyFlags.opusModel, "opus-model", "", "override Opus model ID")
	f.StringVar(&applyFlags.sonnetModel, "sonnet-model", "", "override Sonnet model ID")
	f.StringVar(&applyFlags.haikuModel, "haiku-model", "", "override Haiku model ID")
	f.StringVar(&applyFlags.effort, "effort", "xhigh", "effort level: low|medium|high|xhigh|max")
	f.BoolVar(&applyFlags.opusplan, "opusplan", false, "route planning to Opus, execution to Sonnet")
	f.BoolVar(&applyFlags.noOpusplan, "no-opusplan", false, "disable opusplan")
	f.BoolVar(&applyFlags.use1m, "1m-context", true, "enable 1M token context")
	f.BoolVar(&applyFlags.no1m, "no-1m-context", false, "disable 1M token context")
	f.BoolVar(&applyFlags.noMantle, "no-mantle", false, "disable Mantle routing")
	f.StringVar(&applyFlags.mantleURL, "mantle-url", "", "custom Mantle base URL")
	f.StringVar(&applyFlags.scope, "scope", "user", "settings scope: user or project")
	f.BoolVar(&applyFlags.dryRun, "dry-run", false, "preview without writing")
	f.BoolVar(&applyFlags.skipPreflight, "skip-preflight", false, "skip dependency checks")
	f.StringVar(&applyFlags.storage, "storage", "keychain", "credential storage: keychain|dpapi|profile")

	rootCmd.AddCommand(applyCmd)
}

func runApply(cmd *cobra.Command, args []string) error {
	homeDir := homeDir()

	// Load bedrock config.
	cfgPath := bedrockConfigPath()
	bCfg, err := bedrock.Load(cfgPath)
	if err != nil {
		return err
	}

	// Run migration if v3 block detected.
	if err := runMigrationIfNeeded(homeDir, bCfg); err != nil {
		return err
	}

	// Interactive prompts if required fields are missing and no existing config.
	authMode, region, opusplan, err := resolveApplyInputs(homeDir, bCfg)
	if err != nil {
		return err
	}

	// Resolve credential.
	token, err := resolveCredential(authMode)
	if err != nil {
		return err
	}

	opts := schema.Options{
		AuthMode:      authMode,
		Region:        region,
		Effort:        applyFlags.effort,
		Scope:         applyFlags.scope,
		Version:       Version,
		OpusModel:     applyFlags.opusModel,
		SonnetModel:   applyFlags.sonnetModel,
		HaikuModel:    applyFlags.haikuModel,
		Opusplan:      opusplan,
		Use1M:         !applyFlags.no1m,
		UseMantle:     !applyFlags.noMantle,
		MantleURL:     applyFlags.mantleURL,
		Storage:       applyFlags.storage,
		AuthValidated: true,
	}

	block, err := schema.Build(bCfg, opts)
	if err != nil {
		return err
	}
	native := block.NativeKeys()

	if applyFlags.dryRun {
		fmt.Println("Dry run — no changes written.")
		fmt.Printf("Would write juggernaut block to %s\n", settingsPath(homeDir, applyFlags.scope))
		return nil
	}

	// Store token in keychain.
	if authMode == "bedrock-api-key" && token != "" {
		if err := keychain.Default().Set(token); err != nil {
			return fmt.Errorf("storing API key: %w", err)
		}
	}

	// Write settings.json.
	mgr := config.NewManager(settingsPath(homeDir, applyFlags.scope))

	blockMap, err := toMap(block)
	if err != nil {
		return err
	}
	if err := mgr.MergeJuggernautBlock(blockMap, native.Env, native.Model); err != nil {
		return err
	}

	// Install claude launcher shim.
	binDir := launcher.DefaultBinDir()
	if !launcher.IsInstalled(binDir) {
		if err := launcher.Install(binDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not install claude shim: %v\n", err)
		} else {
			fmt.Printf("  ✓ Installed claude shim → %s\n", binDir)
		}
	}

	fmt.Println("Configuration written successfully.")
	return nil
}

func resolveApplyInputs(homeDir string, bCfg *bedrock.Config) (authMode, region string, opusplan bool, err error) {
	authMode = applyFlags.auth
	region = applyFlags.region
	if region == "" {
		region = bCfg.Defaults.Region
	}
	opusplan = applyFlags.opusplan

	// Non-interactive if all required fields provided.
	if authMode != "" {
		return
	}

	// Check for existing config — no prompt needed if already configured.
	mgr := config.NewManager(settingsPath(homeDir, applyFlags.scope))
	has, _ := mgr.HasJuggernautBlock()
	if has {
		// Re-apply with existing defaults — read auth from existing block.
		authMode = bCfg.Defaults.AuthMode
		return
	}

	// Interactive first-run prompt.
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Authentication method").
				Options(
					huh.NewOption("IAM / SSO (recommended for organizations)", "iam"),
					huh.NewOption("Bedrock API key", "bedrock-api-key"),
				).
				Value(&authMode),
			huh.NewInput().
				Title("AWS region").
				Placeholder(bCfg.Defaults.Region).
				Value(&region),
			huh.NewConfirm().
				Title("Enable opusplan? (routes planning to Opus 4.8, execution to Sonnet 4.6)").
				Value(&opusplan),
		),
	)
	err = form.Run()
	return
}

func resolveCredential(authMode string) (string, error) {
	if authMode != "bedrock-api-key" {
		return "", nil
	}
	if applyFlags.bedrockKey != "" {
		return applyFlags.bedrockKey, nil
	}
	if applyFlags.preserveKey {
		token, err := keychain.Default().Get()
		if err != nil {
			return "", fmt.Errorf("reading existing key: %w", err)
		}
		if token != "" {
			return token, nil
		}
	}
	// Interactive key prompt.
	var token string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Bedrock API key").
				EchoMode(huh.EchoModePassword).
				Value(&token),
		),
	)
	if err := form.Run(); err != nil {
		return "", err
	}
	return token, nil
}

func runMigrationIfNeeded(homeDir string, bCfg *bedrock.Config) error {
	state, err := migrate.Detect(homeDir)
	if err != nil || !state.HasV3Block || state.AlreadyV4 {
		return err
	}
	if state.TooOld {
		return fmt.Errorf(
			"legacy version detected (pre-v3.2.3). Please upgrade to v3.2.3 first:\n"+
				"  curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/v3.2.3/install.sh | bash\n"+
				"Then re-run: juggernaut apply",
		)
	}

	fmt.Printf("Existing Juggernaut configuration detected (v%s, %s auth).\n", state.V3Version, state.AuthMode)
	fmt.Println("Migrating to Juggernaut v4...")

	// Transfer bearer token.
	if state.AuthMode == "bedrock-api-key" {
		token, err := keychain.Default().Get()
		if err == nil && token != "" {
			if err := keychain.Default().Set(token); err != nil {
				fmt.Fprintln(os.Stderr, "  Warning: could not transfer bearer token:", err)
			} else {
				fmt.Println("  ✓ Bearer token transferred to go-keyring")
			}
		}
	}

	// Strip legacy launcher blocks.
	stripped := migrate.StripLauncherBlocks(homeDir)
	for _, p := range stripped {
		fmt.Printf("  ✓ Removed legacy launcher block from %s\n", p)
	}

	fmt.Println("Migration complete. No credentials were re-entered.")
	return nil
}

func settingsPath(homeDir, scope string) string {
	if scope == "project" {
		return filepath.Join(".", ".claude", "settings.json")
	}
	return filepath.Join(homeDir, ".claude", "settings.json")
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE")
}

func bedrockConfigPath() string {
	// Look for bedrock-config.json next to the binary, then CWD.
	self, _ := os.Executable()
	candidate := filepath.Join(filepath.Dir(self), "bedrock-config.json")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return "bedrock-config.json"
}

func toMap(v any) (map[string]any, error) {
	import "encoding/json"
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	return m, json.Unmarshal(b, &m)
}
```

> Note: Fix the inline import in `toMap` — move `"encoding/json"` to the file's import block.

- [ ] **Step 5: Fix `toMap` — move json import to file-level imports**

The `toMap` function's inline import is a placeholder to illustrate intent. In the actual file, `encoding/json` should be in the top-level import block alongside the other imports. Remove the inline `import "encoding/json"` line inside `toMap` and ensure `"encoding/json"` appears in the file's imports.

- [ ] **Step 6: Run tests**

```bash
go test ./cmd/... -run TestApply_DryRun_IAM -v
go build ./...
```
Expected: PASS, no compile errors.

- [ ] **Step 7: Commit**

```bash
git add cmd/apply.go cmd/apply_test.go cmd/root.go
git commit -m "feat: add apply command with interactive first-run and migration"
```

---

## Task 10: `cmd/show.go`, `cmd/doctor.go`, `cmd/version.go`

**Files:**
- Create: `cmd/show.go`
- Create: `cmd/doctor.go`
- Create: `cmd/version.go`
- Create: `cmd/uninstall.go`
- Create: `cmd/migrate.go`

- [ ] **Step 1: Implement `cmd/version.go`**

```go
package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var versionJSON bool

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print Juggernaut version",
	RunE: func(cmd *cobra.Command, args []string) error {
		if versionJSON {
			out, _ := json.Marshal(map[string]string{"version": Version})
			fmt.Println(string(out))
			return nil
		}
		fmt.Println(Version)
		return nil
	},
}

func init() {
	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "output as JSON")
	rootCmd.AddCommand(versionCmd)
}
```

- [ ] **Step 2: Implement `cmd/show.go`**

```go
package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/jpvelasco/juggernaut/internal/config"
	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Display current Juggernaut configuration",
	RunE:  runShow,
}

var showFlags struct {
	scope    string
	jsonOut  bool
}

func init() {
	showCmd.Flags().StringVar(&showFlags.scope, "scope", "", "show only user or project scope")
	showCmd.Flags().BoolVar(&showFlags.jsonOut, "json", false, "output as JSON")
	rootCmd.AddCommand(showCmd)
}

func runShow(cmd *cobra.Command, args []string) error {
	home := homeDir()
	scopes := []string{"user", "project"}
	if showFlags.scope != "" {
		scopes = []string{showFlags.scope}
	}

	results := map[string]any{}
	for _, scope := range scopes {
		mgr := config.NewManager(settingsPath(home, scope))
		data, err := mgr.Read()
		if err != nil {
			continue
		}
		results[scope] = data["juggernaut"]
	}

	if showFlags.jsonOut {
		out, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	for scope, block := range results {
		fmt.Printf("=== %s scope ===\n", scope)
		if block == nil {
			fmt.Println("  (not configured)")
			continue
		}
		out, _ := json.MarshalIndent(block, "", "  ")
		fmt.Println(string(out))
	}
	return nil
}
```

- [ ] **Step 3: Implement `cmd/doctor.go`**

```go
package cmd

import (
	"fmt"

	"github.com/jpvelasco/juggernaut/internal/bedrock"
	"github.com/jpvelasco/juggernaut/internal/config"
	"github.com/jpvelasco/juggernaut/internal/doctor"
	"github.com/jpvelasco/juggernaut/internal/keychain"
	"github.com/jpvelasco/juggernaut/internal/launcher"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Verify Juggernaut configuration and credentials",
	RunE:  runDoctor,
}

var doctorFlags struct {
	scope   string
	jsonOut bool
}

func init() {
	doctorCmd.Flags().StringVar(&doctorFlags.scope, "scope", "", "check only user or project scope")
	doctorCmd.Flags().BoolVar(&doctorFlags.jsonOut, "json", false, "output as JSON")
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	home := homeDir()
	r := doctor.NewReport()

	bCfg, err := bedrock.Load(bedrockConfigPath())
	if err != nil {
		r.Check("bedrock-config.json", doctor.Fail, err.Error())
	} else {
		r.Check("bedrock-config.json", doctor.OK, "loaded (v"+bCfg.Version+")")
	}

	// Check settings.json block.
	mgr := config.NewManager(settingsPath(home, "user"))
	has, err := mgr.HasJuggernautBlock()
	if err != nil {
		r.Check("settings.json", doctor.Fail, err.Error())
	} else if !has {
		r.Check("settings.json", doctor.Fail, "juggernaut block not found — run `juggernaut apply`")
	} else {
		r.Check("settings.json", doctor.OK, "juggernaut block present")
	}

	// Check keychain.
	token, err := keychain.Default().Get()
	if err != nil {
		r.Check("keychain", doctor.Warn, "error reading: "+err.Error())
	} else if token == "" {
		r.Check("keychain", doctor.OK, "no bearer token (IAM auth)")
	} else {
		r.Check("keychain", doctor.OK, "bearer token found")
	}

	// Check launcher shim.
	binDir := launcher.DefaultBinDir()
	if launcher.IsInstalled(binDir) {
		r.Check("claude shim", doctor.OK, binDir)
	} else {
		r.Check("claude shim", doctor.Warn, "not installed — run `juggernaut apply` to install")
	}

	if doctorFlags.jsonOut {
		out, _ := r.JSON()
		fmt.Println(string(out))
	} else {
		fmt.Print(r.String())
	}

	if r.HasFailures() {
		return fmt.Errorf("doctor found failures — see above")
	}
	return nil
}
```

- [ ] **Step 4: Implement `cmd/uninstall.go`**

```go
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/jpvelasco/juggernaut/internal/config"
	"github.com/jpvelasco/juggernaut/internal/keychain"
	"github.com/jpvelasco/juggernaut/internal/launcher"
	"github.com/jpvelasco/juggernaut/internal/migrate"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove Juggernaut configuration",
	RunE:  runUninstall,
}

var uninstallFlags struct {
	scope  string
	full   bool
	force  bool
	dryRun bool
}

func init() {
	f := uninstallCmd.Flags()
	f.StringVar(&uninstallFlags.scope, "scope", "", "remove only user or project scope")
	f.BoolVar(&uninstallFlags.full, "full", false, "also remove claude shim")
	f.BoolVarP(&uninstallFlags.force, "force", "f", false, "skip confirmation prompt")
	f.BoolVar(&uninstallFlags.dryRun, "dry-run", false, "preview without removing")
	rootCmd.AddCommand(uninstallCmd)
}

func runUninstall(cmd *cobra.Command, args []string) error {
	home := homeDir()

	if !uninstallFlags.force && !uninstallFlags.dryRun {
		fmt.Print("Remove Juggernaut configuration? [y/N] ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		if !strings.EqualFold(strings.TrimSpace(scanner.Text()), "y") {
			fmt.Println("Aborted.")
			return nil
		}
	}

	scopes := []string{"user", "project"}
	if uninstallFlags.scope != "" {
		scopes = []string{uninstallFlags.scope}
	}

	for _, scope := range scopes {
		mgr := config.NewManager(settingsPath(home, scope))
		has, _ := mgr.HasJuggernautBlock()
		if !has {
			continue
		}
		if uninstallFlags.dryRun {
			fmt.Printf("Would remove juggernaut block from %s settings.json\n", scope)
			continue
		}
		if err := mgr.RemoveJuggernautBlock(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not remove %s block: %v\n", scope, err)
		} else {
			fmt.Printf("  ✓ Removed juggernaut block from %s settings.json\n", scope)
		}
	}

	if !uninstallFlags.dryRun {
		if err := keychain.Default().Delete(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not remove keychain entry: %v\n", err)
		} else {
			fmt.Println("  ✓ Removed bearer token from keychain")
		}
	}

	if uninstallFlags.full {
		binDir := launcher.DefaultBinDir()
		if uninstallFlags.dryRun {
			fmt.Printf("Would remove claude shim from %s\n", binDir)
		} else {
			_ = launcher.Uninstall(binDir)
			fmt.Println("  ✓ Removed claude shim")
			stripped := migrate.StripLauncherBlocks(home)
			for _, p := range stripped {
				fmt.Printf("  ✓ Removed legacy launcher block from %s\n", p)
			}
		}
	}

	if !uninstallFlags.dryRun {
		fmt.Println("Uninstall complete.")
	}
	return nil
}
```

- [ ] **Step 5: Implement `cmd/migrate.go`**

```go
package cmd

import (
	"fmt"
	"os"

	"github.com/jpvelasco/juggernaut/internal/bedrock"
	"github.com/jpvelasco/juggernaut/internal/keychain"
	"github.com/jpvelasco/juggernaut/internal/launcher"
	"github.com/jpvelasco/juggernaut/internal/migrate"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate from Juggernaut v3 (shell) to v4 (Go)",
	RunE:  runMigrate,
}

var migrateDryRun bool

func init() {
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "preview without making changes")
	rootCmd.AddCommand(migrateCmd)
}

func runMigrate(cmd *cobra.Command, args []string) error {
	home := homeDir()

	state, err := migrate.Detect(home)
	if err != nil {
		return err
	}

	if !state.HasV3Block {
		fmt.Println("No legacy Juggernaut v3 configuration found.")
		return nil
	}

	if state.AlreadyV4 {
		fmt.Println("Already on Juggernaut v4. Nothing to migrate.")
		return nil
	}

	if state.TooOld {
		return fmt.Errorf(
			"legacy version detected (pre-v3.2.3). Please upgrade to v3.2.3 first:\n"+
				"  curl -fsSL https://raw.githubusercontent.com/jpvelasco/juggernaut/v3.2.3/install.sh | bash\n"+
				"Then re-run: juggernaut migrate",
		)
	}

	fmt.Printf("Found Juggernaut v%s configuration (%s auth).\n", state.V3Version, state.AuthMode)

	if migrateDryRun {
		fmt.Println("\nWould migrate:")
		if state.AuthMode == "bedrock-api-key" {
			fmt.Println("  • Transfer bearer token from keychain → go-keyring")
		}
		fmt.Println("  • Upgrade settings.json schema v1 → v2")
		fmt.Printf("  • Install claude shim → %s\n", launcher.DefaultBinDir())
		fmt.Println("  • Strip legacy shell launcher blocks from shell profiles")
		fmt.Println("\nRun without --dry-run to apply.")
		return nil
	}

	fmt.Println("Migrating to Juggernaut v4...")

	if state.AuthMode == "bedrock-api-key" {
		token, err := keychain.Default().Get()
		if err == nil && token != "" {
			if err := keychain.Default().Set(token); err != nil {
				fmt.Fprintln(os.Stderr, "  Warning: could not transfer bearer token:", err)
			} else {
				fmt.Println("  ✓ Bearer token transferred to go-keyring")
			}
		}
	}

	binDir := launcher.DefaultBinDir()
	if err := launcher.Install(binDir); err != nil {
		fmt.Fprintln(os.Stderr, "  Warning: could not install claude shim:", err)
	} else {
		fmt.Printf("  ✓ Installed claude shim → %s\n", binDir)
	}

	stripped := migrate.StripLauncherBlocks(home)
	for _, p := range stripped {
		fmt.Printf("  ✓ Removed legacy launcher block from %s\n", p)
	}

	_, err = bedrock.Load(bedrockConfigPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, "  Warning: could not load bedrock-config.json:", err)
	}

	fmt.Println("\nMigration complete. No credentials were re-entered.")
	fmt.Println("Run `juggernaut apply` to refresh your configuration with v4 settings.")
	return nil
}
```

- [ ] **Step 6: Build and smoke test**

```bash
go build -o bin/juggernaut .
./bin/juggernaut version
./bin/juggernaut --help
./bin/juggernaut apply --help
./bin/juggernaut doctor --help
```
Expected: version prints, all commands appear in help output.

- [ ] **Step 7: Run all tests**

```bash
go test ./... -v
```
Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add cmd/
git commit -m "feat: add show, doctor, version, uninstall, migrate commands"
```

---

## Task 11: `.goreleaser.yml` and version sync CI

**Files:**
- Create: `.goreleaser.yml`
- Modify: `.github/workflows/test.yml`

- [ ] **Step 1: Create `.goreleaser.yml`**

```yaml
# .goreleaser.yml
version: 2

before:
  hooks:
    - go mod tidy

builds:
  - id: juggernaut
    main: .
    binary: juggernaut
    ldflags:
      - -s -w -X github.com/jpvelasco/juggernaut/cmd.Version={{.Version}}
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    ignore:
      - goos: windows
        goarch: arm64

archives:
  - id: juggernaut
    builds: [juggernaut]
    format_overrides:
      - goos: windows
        format: zip
    name_template: "juggernaut_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: "checksums.txt"

release:
  github:
    owner: jpvelasco
    name: juggernaut
  name_template: "v{{.Version}}"
```

- [ ] **Step 2: Create `.github/workflows/test.yml`**

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
      - name: Verify version sync
        run: |
          FILE_VERSION=$(cat VERSION)
          JSON_VERSION=$(jq -r '.version' bedrock-config.json)
          GO_VERSION=$(grep 'var Version' cmd/root.go | grep -oP '"[^"]+"' | tr -d '"')
          if [ "$FILE_VERSION" != "$JSON_VERSION" ] || [ "$FILE_VERSION" != "$GO_VERSION" ]; then
            echo "Version mismatch: VERSION=$FILE_VERSION bedrock-config.json=$JSON_VERSION cmd/root.go=$GO_VERSION"
            exit 1
          fi
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: latest

  test:
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
      - run: go test ./... -v
```

- [ ] **Step 3: Commit**

```bash
git add .goreleaser.yml .github/workflows/test.yml
git commit -m "chore: add GoReleaser config and CI workflow"
```

---

## Task 12: Bootstrap scripts

**Files:**
- Create: `scripts/install.sh`
- Create: `scripts/install.ps1`

- [ ] **Step 1: Create `scripts/install.sh`**

```bash
#!/usr/bin/env bash
set -euo pipefail

REPO="jpvelasco/juggernaut"
BIN_DIR="${HOME}/.local/bin"
VERSION="${JUGGERNAUT_VERSION:-latest}"

detect_platform() {
  local os arch
  case "$(uname -s)" in
    Linux)  os="linux" ;;
    Darwin) os="darwin" ;;
    *)      echo "Unsupported OS: $(uname -s)" >&2; exit 1 ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) echo "Unsupported arch: $(uname -m)" >&2; exit 1 ;;
  esac
  echo "${os}_${arch}"
}

get_latest_version() {
  curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | sed 's/.*"v\([^"]*\)".*/\1/'
}

main() {
  if [ "$VERSION" = "latest" ]; then
    VERSION=$(get_latest_version)
  fi

  local platform
  platform=$(detect_platform)
  local ext="tar.gz"
  local archive="juggernaut_${platform}.${ext}"
  local url="https://github.com/${REPO}/releases/download/v${VERSION}/${archive}"
  local checksum_url="https://github.com/${REPO}/releases/download/v${VERSION}/checksums.txt"

  echo "Installing Juggernaut v${VERSION} (${platform})..."

  local tmp
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT

  curl -fsSL "$url" -o "${tmp}/${archive}"
  curl -fsSL "$checksum_url" -o "${tmp}/checksums.txt"

  # Verify checksum.
  (cd "$tmp" && grep "$archive" checksums.txt | sha256sum -c -)

  mkdir -p "$BIN_DIR"
  tar -xzf "${tmp}/${archive}" -C "$tmp"
  mv "${tmp}/juggernaut" "${BIN_DIR}/juggernaut"
  chmod +x "${BIN_DIR}/juggernaut"

  echo "Juggernaut v${VERSION} installed to ${BIN_DIR}/juggernaut"
  echo "Run: juggernaut apply"
}

main "$@"
```

- [ ] **Step 2: Create `scripts/install.ps1`**

```powershell
param(
  [string]$Version = "latest"
)

$Repo = "jpvelasco/juggernaut"
$BinDir = "$env:USERPROFILE\.local\bin"

function Get-LatestVersion {
  $release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
  return $release.tag_name -replace '^v', ''
}

if ($Version -eq "latest") {
  $Version = Get-LatestVersion
}

$Archive = "juggernaut_windows_amd64.zip"
$Url = "https://github.com/$Repo/releases/download/v$Version/$Archive"
$ChecksumUrl = "https://github.com/$Repo/releases/download/v$Version/checksums.txt"

Write-Host "Installing Juggernaut v$Version (windows_amd64)..."

$Tmp = New-TemporaryFile | ForEach-Object { Remove-Item $_; New-Item -ItemType Directory -Path $_ }
try {
  Invoke-WebRequest $Url -OutFile "$Tmp\$Archive"
  Invoke-WebRequest $ChecksumUrl -OutFile "$Tmp\checksums.txt"

  # Verify checksum.
  $Expected = (Get-Content "$Tmp\checksums.txt" | Where-Object { $_ -match $Archive }) -split '\s+' | Select-Object -First 1
  $Actual = (Get-FileHash "$Tmp\$Archive" -Algorithm SHA256).Hash.ToLower()
  if ($Actual -ne $Expected) {
    Write-Error "Checksum mismatch"; exit 1
  }

  New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
  Expand-Archive "$Tmp\$Archive" -DestinationPath $Tmp -Force
  Move-Item "$Tmp\juggernaut.exe" "$BinDir\juggernaut.exe" -Force
} finally {
  Remove-Item -Recurse -Force $Tmp
}

Write-Host "Juggernaut v$Version installed to $BinDir\juggernaut.exe"
Write-Host "Run: juggernaut apply"
```

- [ ] **Step 3: Make install.sh executable and commit**

```bash
chmod +x scripts/install.sh
git add scripts/
git commit -m "feat: add bootstrap install scripts (sh + ps1)"
```

---

## Task 13: npm package

**Files:**
- Create: `npm/package.json`
- Create: `npm/install.js`

- [ ] **Step 1: Create `npm/package.json`**

```json
{
  "name": "juggernaut",
  "version": "4.0.0",
  "description": "Configure Claude Code to use Amazon Bedrock",
  "bin": {
    "juggernaut": "./bin/juggernaut"
  },
  "scripts": {
    "postinstall": "node install.js"
  },
  "os": ["darwin", "linux", "win32"],
  "cpu": ["x64", "arm64"],
  "keywords": ["claude", "bedrock", "aws", "anthropic"],
  "license": "MIT",
  "repository": {
    "type": "git",
    "url": "https://github.com/jpvelasco/juggernaut"
  }
}
```

- [ ] **Step 2: Create `npm/install.js`**

```js
const { execSync } = require("child_process");
const https = require("https");
const fs = require("fs");
const path = require("path");
const crypto = require("crypto");
const { createGunzip } = require("zlib");
const tar = require("tar"); // Note: add "tar" to dependencies in package.json

const REPO = "jpvelasco/juggernaut";
const BIN_DIR = path.join(__dirname, "bin");

function getPlatform() {
  const os = process.platform;
  const arch = process.arch;
  const osMap = { darwin: "darwin", linux: "linux", win32: "windows" };
  const archMap = { x64: "amd64", arm64: "arm64" };
  if (!osMap[os] || !archMap[arch]) {
    throw new Error(`Unsupported platform: ${os}/${arch}`);
  }
  return `${osMap[os]}_${archMap[arch]}`;
}

async function getLatestVersion() {
  return new Promise((resolve, reject) => {
    https.get(
      `https://api.github.com/repos/${REPO}/releases/latest`,
      { headers: { "User-Agent": "juggernaut-npm-installer" } },
      (res) => {
        let data = "";
        res.on("data", (chunk) => (data += chunk));
        res.on("end", () => {
          const release = JSON.parse(data);
          resolve(release.tag_name.replace(/^v/, ""));
        });
      }
    ).on("error", reject);
  });
}

async function download(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    https.get(url, { headers: { "User-Agent": "juggernaut-npm-installer" } }, (res) => {
      if (res.statusCode === 302 || res.statusCode === 301) {
        return download(res.headers.location, dest).then(resolve).catch(reject);
      }
      res.pipe(file);
      file.on("finish", () => { file.close(); resolve(); });
    }).on("error", reject);
  });
}

async function main() {
  const version = await getLatestVersion();
  const platform = getPlatform();
  const ext = platform.startsWith("windows") ? "zip" : "tar.gz";
  const archive = `juggernaut_${platform}.${ext}`;
  const baseUrl = `https://github.com/${REPO}/releases/download/v${version}`;

  const tmp = fs.mkdtempSync(path.join(require("os").tmpdir(), "juggernaut-"));
  const archivePath = path.join(tmp, archive);

  console.log(`Downloading Juggernaut v${version} (${platform})...`);
  await download(`${baseUrl}/${archive}`, archivePath);
  await download(`${baseUrl}/checksums.txt`, path.join(tmp, "checksums.txt"));

  // Verify checksum.
  const checksums = fs.readFileSync(path.join(tmp, "checksums.txt"), "utf8");
  const line = checksums.split("\n").find((l) => l.includes(archive));
  if (!line) throw new Error("Checksum not found for " + archive);
  const expected = line.split(/\s+/)[0];
  const actual = crypto.createHash("sha256").update(fs.readFileSync(archivePath)).digest("hex");
  if (actual !== expected) throw new Error("Checksum mismatch");

  fs.mkdirSync(BIN_DIR, { recursive: true });

  if (ext === "zip") {
    // Windows: use adm-zip or built-in
    const AdmZip = require("adm-zip");
    const zip = new AdmZip(archivePath);
    zip.extractEntryTo("juggernaut.exe", BIN_DIR, false, true);
  } else {
    await tar.x({ file: archivePath, cwd: BIN_DIR, strip: 0 });
    fs.chmodSync(path.join(BIN_DIR, "juggernaut"), 0o755);
  }

  console.log(`Juggernaut v${version} installed. Run: juggernaut apply`);
  fs.rmSync(tmp, { recursive: true, force: true });
}

main().catch((err) => {
  console.error("Installation failed:", err.message);
  process.exit(1);
});
```

> Note: Add `"tar": "^6.0.0"` and `"adm-zip": "^0.5.0"` to `npm/package.json` dependencies.

- [ ] **Step 3: Update `npm/package.json` with dependencies**

```json
{
  "name": "juggernaut",
  "version": "4.0.0",
  "description": "Configure Claude Code to use Amazon Bedrock",
  "bin": {
    "juggernaut": "./bin/juggernaut"
  },
  "scripts": {
    "postinstall": "node install.js"
  },
  "dependencies": {
    "tar": "^6.0.0",
    "adm-zip": "^0.5.0"
  },
  "os": ["darwin", "linux", "win32"],
  "cpu": ["x64", "arm64"],
  "keywords": ["claude", "bedrock", "aws", "anthropic"],
  "license": "MIT",
  "repository": {
    "type": "git",
    "url": "https://github.com/jpvelasco/juggernaut"
  }
}
```

- [ ] **Step 4: Commit**

```bash
git add npm/
git commit -m "feat: add npm package with binary downloader"
```

---

## Task 14: Update `VERSION` and `bedrock-config.json` to v4.0.0

**Files:**
- Modify: `VERSION`
- Modify: `bedrock-config.json`
- Modify: `cmd/root.go` (Version var)

- [ ] **Step 1: Bump VERSION**

```bash
echo "4.0.0" > VERSION
```

- [ ] **Step 2: Update bedrock-config.json version field**

Change `"version": "3.2.3"` to `"version": "4.0.0"` in `bedrock-config.json`.

- [ ] **Step 3: Update cmd/root.go Version var**

```go
var Version = "4.0.0-dev"
```
(GoReleaser overrides this at build time via ldflags; the fallback is for local dev.)

- [ ] **Step 4: Verify version sync passes**

```bash
FILE_VERSION=$(cat VERSION)
JSON_VERSION=$(jq -r '.version' bedrock-config.json)
echo "VERSION=$FILE_VERSION bedrock-config.json=$JSON_VERSION"
# Both should be 4.0.0
```

- [ ] **Step 5: Run all tests**

```bash
go test ./... -v
go build ./...
```
Expected: all PASS, binary builds.

- [ ] **Step 6: Commit**

```bash
git add VERSION bedrock-config.json cmd/root.go
git commit -m "chore: bump version to 4.0.0"
```

---

## Task 15: Final integration smoke test + CHANGELOG

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Build release binary locally**

```bash
make build
```

- [ ] **Step 2: Smoke test apply --dry-run**

```bash
./bin/juggernaut apply --auth=iam --region=us-west-2 --dry-run
```
Expected: prints dry-run preview, exits 0.

- [ ] **Step 3: Smoke test version**

```bash
./bin/juggernaut version
./bin/juggernaut version --json
```
Expected: `4.0.0-dev` (or similar), JSON object.

- [ ] **Step 4: Smoke test doctor**

```bash
./bin/juggernaut doctor
```
Expected: runs checks, reports FAIL for missing block (expected on clean machine), exits non-zero.

- [ ] **Step 5: Smoke test migrate --dry-run on clean machine**

```bash
./bin/juggernaut migrate --dry-run
```
Expected: "No legacy Juggernaut v3 configuration found."

- [ ] **Step 6: Add CHANGELOG entry**

At the top of `CHANGELOG.md`, add:

```markdown
## [4.0.0] — unreleased

### Changed
- Full rewrite from bash/PowerShell to Go — single cross-platform binary
- Launcher is now a symlink (Unix) or `.cmd` shim (Windows) — no shell profile modification required
- Keychain handled via go-keyring — no platform-specific shell commands

### Added
- `juggernaut migrate` — explicit v3→v4 migration command
- Automatic migration detection on first run of any command
- Interactive first-run prompts when `apply` is run without flags (via charmbracelet/huh)
- `--json` flag on `show`, `doctor`, `version` for machine-readable output
- npm distribution: `npm install -g juggernaut`
- GoReleaser: pre-built binaries for linux/darwin/windows × amd64/arm64

### Removed
- Bash and PowerShell source scripts (moved to `legacy/v3` branch)
- jq dependency (no longer required)

### Migration
Upgrade from v3.2.3: install v4 binary, then run `juggernaut migrate` or simply `juggernaut apply`.
Pre-v3.2.3 installations must upgrade to v3.2.3 first.
```

- [ ] **Step 7: Final commit**

```bash
git add CHANGELOG.md
git commit -m "docs: add v4.0.0 changelog entry"
```

---

## Self-Review Notes

- **Spec coverage:** All spec sections covered — repo structure (Task 1), bedrock loader (Task 2), schema (Task 3), config manager (Task 4), keychain (Task 5), doctor (Task 6), launcher/shim (Task 7), migration (Task 8), all commands (Tasks 9–10), GoReleaser (Task 11), bootstrap scripts (Task 12), npm (Task 13), version bump (Task 14), smoke test (Task 15).
- **Type consistency:** `schema.Options` used in Task 3 tests and Task 9 apply command — field names consistent. `config.Manager` interface consistent across Tasks 4, 9, 10. `keychain.Store` API consistent across Tasks 5, 7, 9, 10.
- **The `toMap` inline import bug** is flagged explicitly in Task 9 Step 5 — fix before committing.
- **bedrock-config.json path resolution** in `bedrockConfigPath()` (Task 9) looks next to the binary first, then CWD. This correctly handles both installed use and dev builds.
