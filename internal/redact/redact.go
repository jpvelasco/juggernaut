// Package redact strips secrets and identifying values from diagnostic text.
package redact

import (
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// Stable placeholders. Empty strings are never used as replacements.
const (
	Token       = "<token>"
	AccountID   = "<account-id>"
	Home        = "<home>"
	Email       = "<email>"
	Hostname    = "<hostname>"
	IP          = "<ip>"
	ARN         = "<arn>"
	AccessKeyID = "<access-key-id>"
)

// Options controls extra values to scrub in addition to built-in patterns.
type Options struct {
	Home     string
	Secrets  []string
	Hostname string
}

var (
	reAccessKeyID = regexp.MustCompile(`(?:AKIA|ASIA|AROA|AIDA)[A-Z0-9]{16}`)
	reAccountID   = regexp.MustCompile(`\b\d{12}\b`)
	reEmail       = regexp.MustCompile(`(?i)[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}`)
	reARN         = regexp.MustCompile(`arn:aws[\w-]*:[^\s"'\\]+`)
	reBearer      = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._\-+=/]+`)
	reAuthHeader  = regexp.MustCompile(`(?i)\bauthorization\s*[:=]\s*\S.*`)
	reBedrockKey  = regexp.MustCompile(`bedrock-api-key-[A-Za-z0-9+/=_\-]+`)
	reABSK        = regexp.MustCompile(`\bABSK[A-Za-z0-9+/=_\-]{8,}`)
	reJWT         = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)
	reSecretKey   = regexp.MustCompile(`(?i)(aws_secret_access_key\s*[=:]\s*)([A-Za-z0-9/+=]{40})`)
	reSessionTok  = regexp.MustCompile(`(?i)(aws_session_token\s*[=:]\s*)(\S+)`)
	reIPv4        = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4]\d|[01]?\d?\d)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d?\d)\b`)
	reHomeUser    = regexp.MustCompile(`(?i)([/\\](?:Users|home)[/\\])([^/\\]+)`)
)

// String replaces secrets, account identifiers, and home-path usernames with
// stable placeholders. The input is unchanged when empty.
func String(s string, opts Options) string {
	if s == "" {
		return s
	}
	s = replaceSecrets(s, opts.Secrets)
	s = reBearer.ReplaceAllString(s, "Bearer "+Token)
	s = reAuthHeader.ReplaceAllString(s, "Authorization: "+Token)
	s = reBedrockKey.ReplaceAllString(s, Token)
	s = reABSK.ReplaceAllString(s, Token)
	s = reJWT.ReplaceAllString(s, Token)
	s = reSecretKey.ReplaceAllString(s, "${1}"+Token)
	s = reSessionTok.ReplaceAllString(s, "${1}"+Token)
	s = reAccessKeyID.ReplaceAllString(s, AccessKeyID)
	s = reARN.ReplaceAllString(s, ARN)
	s = reEmail.ReplaceAllString(s, Email)
	s = replaceHome(s, opts.Home)
	s = replaceHostname(s, opts.Hostname)
	s = redactPrivateIPs(s)
	s = reAccountID.ReplaceAllString(s, AccountID)
	return s
}

func replaceSecrets(s string, secrets []string) string {
	vals := uniqueNonEmpty(secrets)
	sort.Slice(vals, func(i, j int) bool { return len(vals[i]) > len(vals[j]) })
	for _, secret := range vals {
		s = strings.ReplaceAll(s, secret, Token)
	}
	return s
}

func replaceHome(s, home string) string {
	for _, variant := range homeVariants(home) {
		s = replaceInsensitive(s, variant, Home)
	}
	return reHomeUser.ReplaceAllString(s, "${1}"+Home)
}

func replaceHostname(s, host string) string {
	if host == "" {
		host, _ = os.Hostname()
	}
	host = strings.TrimSpace(host)
	if host == "" || strings.EqualFold(host, "localhost") {
		return s
	}
	s = replaceInsensitive(s, host, Hostname)
	if short, _, ok := strings.Cut(host, "."); ok && len(short) > 1 && !strings.EqualFold(short, "localhost") {
		s = replaceInsensitive(s, short, Hostname)
	}
	return s
}

func redactPrivateIPs(s string) string {
	return reIPv4.ReplaceAllStringFunc(s, func(match string) string {
		ip := net.ParseIP(match)
		if ip == nil {
			return match
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return IP
		}
		return match
	})
}

func homeVariants(home string) []string {
	real, _ := os.UserHomeDir()
	seen := map[string]struct{}{}
	var out []string
	add := func(v string) {
		v = strings.TrimRight(v, `/\`)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	for _, raw := range []string{home, os.Getenv("HOME"), os.Getenv("USERPROFILE"), real} {
		if raw == "" {
			continue
		}
		clean := filepath.Clean(raw)
		add(clean)
		add(filepath.ToSlash(clean))
		if runtime.GOOS == "windows" {
			add(strings.ReplaceAll(clean, `\`, `\\`))
			add(strings.ReplaceAll(clean, `\`, `/`))
		}
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

func replaceInsensitive(s, old, neu string) string {
	if old == "" || neu == "" {
		return s
	}
	re, err := regexp.Compile(`(?i)` + regexp.QuoteMeta(old))
	if err != nil {
		return strings.ReplaceAll(s, old, neu)
	}
	return re.ReplaceAllString(s, neu)
}

func uniqueNonEmpty(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, v := range in {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
