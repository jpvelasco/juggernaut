package activation

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// --- containsPathCI ---

func TestContainsPathCI_Found(t *testing.T) {
	paths := []string{"/usr/bin", "/usr/local/bin", "/opt/bin"}
	if !containsPathCI(paths, "/usr/local/bin") {
		t.Error("expected /usr/local/bin to be found")
	}
}

func TestContainsPathCI_NotFound(t *testing.T) {
	paths := []string{"/usr/bin", "/usr/local/bin"}
	if containsPathCI(paths, "/opt/bin") {
		t.Error("/opt/bin should not be found")
	}
}

func TestContainsPathCI_EmptyList(t *testing.T) {
	if containsPathCI(nil, "/usr/bin") {
		t.Error("nil list should not contain anything")
	}
	if containsPathCI([]string{}, "/usr/bin") {
		t.Error("empty list should not contain anything")
	}
}

func TestContainsPathCI_CaseSensitiveOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping case-sensitivity test on Windows")
	}
	paths := []string{"/User/Bin"}
	if containsPathCI(paths, "/user/bin") {
		t.Error("should be case-sensitive on non-Windows")
	}
	if containsPathCI(paths, "/User/Bin") {
		// OK — exact match.
	} else {
		t.Error("exact match should still be found")
	}
}

func TestContainsPathCI_CaseInsensitiveOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skipping case-insensitivity test on non-Windows")
	}
	paths := []string{`C:\User\Bin`}
	if !containsPathCI(paths, `c:\user\bin`) {
		t.Error("should be case-insensitive on Windows")
	}
}

// --- deduplicatePathsCI ---

func TestDeduplicatePathsCI_NoDuplicates(t *testing.T) {
	paths := []string{"/a", "/b", "/c"}
	got := deduplicatePathsCI(paths)
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	if got[0] != "/a" || got[1] != "/b" || got[2] != "/c" {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestDeduplicatePathsCI_WithDuplicates(t *testing.T) {
	paths := []string{"/a", "/b", "/a", "/c", "/b"}
	got := deduplicatePathsCI(paths)
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d: %v", len(got), got)
	}
	if got[0] != "/a" || got[1] != "/b" || got[2] != "/c" {
		t.Errorf("expected [/a /b /c], got %v", got)
	}
}

func TestDeduplicatePathsCI_EmptyInput(t *testing.T) {
	if got := deduplicatePathsCI(nil); len(got) != 0 {
		t.Errorf("nil input should produce empty slice, got %v", got)
	}
	if got := deduplicatePathsCI([]string{}); len(got) != 0 {
		t.Errorf("empty input should produce empty slice, got %v", got)
	}
}

func TestDeduplicatePathsCI_AllSame(t *testing.T) {
	paths := []string{"/a", "/a", "/a", "/a"}
	got := deduplicatePathsCI(paths)
	if len(got) != 1 || got[0] != "/a" {
		t.Errorf("expected [/a], got %v", got)
	}
}

func TestDeduplicatePathsCI_CaseInsensitiveOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skipping case-insensitive dedup test on non-Windows")
	}
	paths := []string{`C:\Foo`, `c:\foo`, `C:\FOO`}
	got := deduplicatePathsCI(paths)
	if len(got) != 1 {
		t.Errorf("expected 1 unique entry on Windows, got %d: %v", len(got), got)
	}
}

func TestDeduplicatePathsCI_CaseSensitiveOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping case-sensitivity dedup test on Windows")
	}
	paths := []string{`/Foo`, `/foo`}
	got := deduplicatePathsCI(paths)
	if len(got) != 2 {
		t.Errorf("expected 2 entries on non-Windows (case-sensitive), got %d: %v", len(got), got)
	}
}

// --- validateAndCanonicalizePath ---

func TestValidateAndCanonicalizePath_EmptyInput(t *testing.T) {
	if got := validateAndCanonicalizePath("", ""); got != "" {
		t.Errorf("empty input should produce empty output, got %q", got)
	}
}

func TestValidateAndCanonicalizePath_WhitespaceOnly(t *testing.T) {
	for _, input := range []string{" ", "   ", "\t", "\n"} {
		if got := validateAndCanonicalizePath(input, ""); got != "" {
			t.Errorf("whitespace-only input %q should produce empty output, got %q", input, got)
		}
	}
}

func TestValidateAndCanonicalizePath_Dot(t *testing.T) {
	if got := validateAndCanonicalizePath(".", ""); got != "" {
		t.Errorf(". should produce empty output, got %q", got)
	}
}

func TestValidateAndCanonicalizePath_ValidAbsolute(t *testing.T) {
	p := filepath.Join("usr", "local", "bin")
	abs := filepath.Clean(p)
	got := validateAndCanonicalizePath(p, "")
	if got != abs {
		t.Errorf("expected %q, got %q", abs, got)
	}
}

func TestValidateAndCanonicalizePath_UnderBaseDir(t *testing.T) {
	base := filepath.Join("home", "user")
	child := filepath.Join(base, "Documents")
	got := validateAndCanonicalizePath(child, base)
	if got != child {
		t.Errorf("expected %q, got %q", child, got)
	}
}

func TestValidateAndCanonicalizePath_EscapingBaseDir(t *testing.T) {
	base := filepath.Join("home", "user")
	escape := filepath.Join(base, "..", "etc", "passwd")
	got := validateAndCanonicalizePath(escape, base)
	if got != "" {
		t.Errorf("path escaping baseDir should be rejected, got %q", got)
	}
}

func TestValidateAndCanonicalizePath_DirectDotDotEscape(t *testing.T) {
	base := filepath.Join("home", "user")
	got := validateAndCanonicalizePath("..", base)
	if got != "" {
		t.Errorf("'..' relative to baseDir should be rejected, got %q", got)
	}
}

func TestValidateAndCanonicalizePath_EmptyBaseDir(t *testing.T) {
	// With empty baseDir, containment is not enforced — just clean.
	p := filepath.Join("..", "etc", "passwd")
	got := validateAndCanonicalizePath(p, "")
	expected := filepath.Clean(p)
	if got != expected {
		t.Errorf("expected %q (no containment check), got %q", expected, got)
	}
}

func TestValidateAndCanonicalizePath_TrailingSeparators(t *testing.T) {
	sep := string(filepath.Separator)
	raw := "/usr/local/bin" + sep + sep
	got := validateAndCanonicalizePath(raw, "")
	expected := "/usr/local/bin"
	if runtime.GOOS != "windows" {
		// On POSIX, forward slashes are canonical.
		if got != expected {
			t.Errorf("expected %q, got %q", expected, got)
		}
	} else {
		// On Windows, an absolute path starting with / is relative to the
		// current drive root, so Clean may produce something different.
		// Just verify trailing separators are stripped.
		if strings.HasSuffix(got, sep) || strings.HasSuffix(got, "/") {
			t.Errorf("trailing separators not cleaned: %q", got)
		}
	}
}

func TestValidateAndCanonicalizePath_TrailingWhitespace(t *testing.T) {
	got := validateAndCanonicalizePath("  /usr/bin  ", "")
	expected := "/usr/bin"
	if runtime.GOOS != "windows" {
		if got != expected {
			t.Errorf("expected %q, got %q", expected, got)
		}
	} else {
		if strings.HasSuffix(got, " ") || strings.HasPrefix(got, " ") {
			t.Errorf("whitespace not trimmed: %q", got)
		}
	}
}
