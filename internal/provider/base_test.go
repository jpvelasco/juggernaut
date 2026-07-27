package provider

import (
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/juggernaut/v5/internal/bedrock"
)

// testProvider is a minimal Provider implementation for testing BaseProvider methods.
type testProvider struct {
	BaseProvider
}

func (t testProvider) ConfigPath(home, scope string) (string, error) {
	return home + "/.test/config.json", nil
}

func (t testProvider) OwnsConfig(data map[string]any) bool {
	return false
}

func (t testProvider) NativeManagedKeys() []string {
	return nil
}

func (t testProvider) DeepMergeKeys() []string {
	return nil
}

func (t testProvider) OwnedSubKeys() map[string][]string {
	return nil
}

func (t testProvider) BuildConfig(cfg *bedrock.Config, opts Options) (ConfigPlan, error) {
	return ConfigPlan{}, nil
}

func (t testProvider) LaunchSpec() LaunchSpec {
	return LaunchSpec{}
}

func newTestProvider(name, displayName, configFormat, binaryName string, caps []Capability) testProvider {
	return testProvider{
		BaseProvider: BaseProvider{
			name:         name,
			displayName:  displayName,
			configFormat: configFormat,
			binaryName:   binaryName,
			capabilities: caps,
		},
	}
}

// --- BaseProvider.Name() ---

func TestBaseProvider_Name(t *testing.T) {
	p := newTestProvider("mycli", "", "json", "mycli", nil)
	if got := p.Name(); got != "mycli" {
		t.Errorf("Name() = %q, want %q", got, "mycli")
	}
}

// --- BaseProvider.BinaryNames() ---

func TestBaseProvider_BinaryNames_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows")
	}
	p := newTestProvider("mycli", "", "json", "mycli", nil)
	got := p.BinaryNames()
	if len(got) != 1 || got[0] != "mycli" {
		t.Errorf("BinaryNames() = %v, want [mycli]", got)
	}
}

func TestBaseProvider_BinaryNames_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skipping on non-Windows")
	}
	p := newTestProvider("mycli", "", "json", "mycli", nil)
	got := p.BinaryNames()
	want := []string{"mycli.exe", "mycli.cmd", "mycli.bat"}
	if len(got) != len(want) {
		t.Fatalf("BinaryNames() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("BinaryNames()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// --- BaseProvider.ConfigFormatName() ---

func TestBaseProvider_ConfigFormatName(t *testing.T) {
	p := newTestProvider("mycli", "", "toml", "mycli", nil)
	if got := p.ConfigFormatName(); got != "toml" {
		t.Errorf("ConfigFormatName() = %q, want %q", got, "toml")
	}
}

// --- BaseProvider.ActivationMarkers() ---

func TestBaseProvider_ActivationMarkers_WithDisplayName(t *testing.T) {
	p := newTestProvider("mycli", "My CLI", "json", "mycli", nil)
	begin, end := p.ActivationMarkers()
	wantBegin := "# BEGIN: Juggernaut My CLI Activation"
	wantEnd := "# END: Juggernaut My CLI Activation"
	if begin != wantBegin {
		t.Errorf("begin = %q, want %q", begin, wantBegin)
	}
	if end != wantEnd {
		t.Errorf("end = %q, want %q", end, wantEnd)
	}
}

func TestBaseProvider_ActivationMarkers_WithoutDisplayName(t *testing.T) {
	p := newTestProvider("mycli", "", "json", "mycli", nil)
	begin, end := p.ActivationMarkers()
	wantBegin := "# BEGIN: Juggernaut Mycli Activation"
	wantEnd := "# END: Juggernaut Mycli Activation"
	if begin != wantBegin {
		t.Errorf("begin = %q, want %q", begin, wantBegin)
	}
	if end != wantEnd {
		t.Errorf("end = %q, want %q", end, wantEnd)
	}
}

// --- BaseProvider.Supports() ---

func TestBaseProvider_Supports_Matching(t *testing.T) {
	p := newTestProvider("mycli", "", "json", "mycli", []Capability{CapAutoMode, CapThinking})
	if !p.Supports(CapAutoMode) {
		t.Error("Supports(CapAutoMode) = false, want true")
	}
	if !p.Supports(CapThinking) {
		t.Error("Supports(CapThinking) = false, want true")
	}
}

func TestBaseProvider_Supports_NonMatching(t *testing.T) {
	p := newTestProvider("mycli", "", "json", "mycli", []Capability{CapAutoMode})
	if p.Supports(CapThinking) {
		t.Error("Supports(CapThinking) = true, want false")
	}
	if p.Supports(CapNativeAuth) {
		t.Error("Supports(CapNativeAuth) = true, want false")
	}
}

func TestBaseProvider_Supports_EmptyCapabilities(t *testing.T) {
	p := newTestProvider("mycli", "", "json", "mycli", nil)
	if p.Supports(CapAutoMode) {
		t.Error("Supports(CapAutoMode) = true with empty capabilities, want false")
	}
}

// --- BaseProvider.DisplayName() ---

func TestBaseProvider_DisplayName_Set(t *testing.T) {
	p := newTestProvider("mycli", "My Custom CLI", "json", "mycli", nil)
	if got := p.DisplayName(); got != "My Custom CLI" {
		t.Errorf("DisplayName() = %q, want %q", got, "My Custom CLI")
	}
}

func TestBaseProvider_DisplayName_Unset(t *testing.T) {
	p := newTestProvider("mycli", "", "json", "mycli", nil)
	got := p.DisplayName()
	if got != "Mycli" {
		t.Errorf("DisplayName() = %q, want %q (title-cased name)", got, "Mycli")
	}
}

// --- SupportedNames() ---

func TestSupportedNames_NonEmpty(t *testing.T) {
	got := SupportedNames()
	if got == "" {
		t.Error("SupportedNames() returned empty string, want non-empty")
	}
	// The default registry has claude, codex, opencode, grok.
	if !strings.Contains(got, "claude") {
		t.Errorf("SupportedNames() missing 'claude': %s", got)
	}
	if !strings.Contains(got, "codex") {
		t.Errorf("SupportedNames() missing 'codex': %s", got)
	}
}

// --- Register() ---

func TestRegister_AddsToRegistry(t *testing.T) {
	testName := "test-provider-" + t.Name()
	p := newTestProvider(testName, "TestProvider", "json", "testprovider", nil)

	Register(p)
	defer func() {
		// Clean up: remove the test provider from the global registry.
		delete(registry, testName)
	}()

	resolved, err := Get(testName)
	if err != nil {
		t.Fatalf("Get(%q) after Register: %v", testName, err)
	}
	if resolved.Name() != testName {
		t.Errorf("resolved provider Name() = %q, want %q", resolved.Name(), testName)
	}
}

// --- BaseProvider.SupportsModelWith() ---

func TestSupportsModelWith_PredicateAccepts(t *testing.T) {
	p := newTestProvider("test", "", "json", "test", nil)
	model := CatalogModel{ID: "anthropic.claude-5", Source: "foundation", Status: "ACTIVE", Availability: "AVAILABLE"}
	s := p.SupportsModelWith(model, func(m CatalogModel) ModelSupport {
		return ModelSupport{Supported: true, Reason: "native Claude model"}
	})
	if !s.Supported {
		t.Fatalf("expected supported, got reason: %s", s.Reason)
	}
	if s.Reason != "native Claude model" {
		t.Errorf("reason = %q, want 'native Claude model'", s.Reason)
	}
}

func TestSupportsModelWith_PredicateRejects(t *testing.T) {
	p := newTestProvider("test", "", "json", "test", nil)
	model := CatalogModel{ID: "xai.grok-4", Source: "mantle", Status: "ACTIVE", Availability: "AVAILABLE"}
	s := p.SupportsModelWith(model, func(m CatalogModel) ModelSupport {
		return ModelSupport{Supported: false, Reason: "not a Claude model"}
	})
	if s.Supported {
		t.Fatal("expected rejection")
	}
	if s.Reason != "not a Claude model" {
		t.Errorf("reason = %q, want 'not a Claude model'", s.Reason)
	}
}

func TestSupportsModelWith_InactiveModel(t *testing.T) {
	base := newTestProvider("test", "", "json", "test", nil)
	model := CatalogModel{ID: "old.model", Source: "foundation", Status: "LEGACY"}
	s := base.SupportsModelWith(model, func(m CatalogModel) ModelSupport {
		// Predicate should not be called for inactive models
		t.Fatal("predicate should not be called for inactive model")
		return ModelSupport{Supported: true}
	})
	if s.Supported {
		t.Fatal("expected rejection for inactive model")
	}
	if s.Reason != "model is not ACTIVE" {
		t.Errorf("reason = %q, want 'model is not ACTIVE'", s.Reason)
	}
}

func TestSupportsModelWith_UnavailableModel(t *testing.T) {
	base := newTestProvider("test", "", "json", "test", nil)
	model := CatalogModel{ID: "restricted.model", Source: "foundation", Status: "ACTIVE", Availability: "NOT_AVAILABLE"}
	s := base.SupportsModelWith(model, func(m CatalogModel) ModelSupport {
		t.Fatal("predicate should not be called for unavailable model")
		return ModelSupport{Supported: true}
	})
	if s.Supported {
		t.Fatal("expected rejection for unavailable model")
	}
	if s.Reason != "model is not available to this AWS account" {
		t.Errorf("reason = %q, want 'model is not available to this AWS account'", s.Reason)
	}
}

// --- BaseProvider.CatalogSources() ---

func TestCatalogSources_Empty(t *testing.T) {
	p := newTestProvider("test", "", "json", "test", nil)
	sources := p.CatalogSources()
	if sources != nil {
		t.Errorf("expected nil sources, got %v", sources)
	}
}

func TestCatalogSources_Set(t *testing.T) {
	p := newTestProvider("test", "", "json", "test", nil)
	p.catalogSources = []string{"foundation", "profile"}
	sources := p.CatalogSources()
	if len(sources) != 2 || sources[0] != "foundation" || sources[1] != "profile" {
		t.Errorf("sources = %v, want [foundation profile]", sources)
	}
}
