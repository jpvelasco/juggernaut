package config

import "testing"

// TestFormatByName resolves format names to implementations without the provider
// package needing to import config internals (breaks the would-be import cycle:
// provider returns a NAME string, cmd/ calls FormatByName to get the format).
func TestFormatByName(t *testing.T) {
	cases := map[string]string{"json": "json", "toml": "toml"}
	for name, want := range cases {
		f, err := FormatByName(name)
		if err != nil {
			t.Errorf("FormatByName(%q): %v", name, err)
			continue
		}
		if f.Name() != want {
			t.Errorf("FormatByName(%q).Name() = %q, want %q", name, f.Name(), want)
		}
	}
}

// TestFormatByName_Default treats empty as JSON (back-compat: existing callers).
func TestFormatByName_Default(t *testing.T) {
	f, err := FormatByName("")
	if err != nil {
		t.Fatalf("FormatByName(\"\"): %v", err)
	}
	if f.Name() != "json" {
		t.Errorf("empty format = %q, want json", f.Name())
	}
}

// TestFormatByName_Unknown errors on an unrecognized format.
func TestFormatByName_Unknown(t *testing.T) {
	if _, err := FormatByName("yaml"); err == nil {
		t.Error("expected error for unknown format 'yaml'")
	}
}
