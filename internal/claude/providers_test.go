package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadProviderRouterWithoutFallbackDoesNotReadConfig(t *testing.T) {
	router, err := LoadProviderRouter("anthropic", nil, filepath.Join(t.TempDir(), "missing.json"), []string{"PATH=test"})
	if err != nil {
		t.Fatalf("LoadProviderRouter: %v", err)
	}
	route := router.Current()
	if route.Name != "anthropic" || !route.Primary || route.Environment != nil {
		t.Fatalf("primary route = %#v", route)
	}
}

func TestLoadProviderRouterBuildsIsolatedFallbackEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.json")
	content := `{
  "Kimi": {
    "ANTHROPIC_BASE_URL": "https://example.invalid",
    "ANTHROPIC_AUTH_TOKEN": "fallback-token",
    "ANTHROPIC_API_KEY": null,
    "CLAUDE_CODE_EFFORT_LEVEL": "high",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  }
}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	base := []string{"PATH=bin", "ANTHROPIC_API_KEY=primary-key", "ANTHROPIC_MODEL=primary-model", "ANTHROPIC_CUSTOM_PRIMARY=must-not-leak", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=0", "UNRELATED=keep"}
	router, err := LoadProviderRouter("anthropic", []string{"kimi"}, path, base)
	if err != nil {
		t.Fatalf("LoadProviderRouter: %v", err)
	}
	fallback, ok := router.Failover("anthropic")
	if !ok {
		t.Fatal("expected fallback route")
	}
	if fallback.Name != "Kimi" || fallback.Primary {
		t.Fatalf("fallback route = %#v", fallback)
	}
	env := testEnvironmentMap(fallback.Environment)
	for key, want := range map[string]string{
		"PATH": "bin", "UNRELATED": "keep",
		"ANTHROPIC_BASE_URL":                       "https://example.invalid",
		"ANTHROPIC_AUTH_TOKEN":                     "fallback-token",
		"CLAUDE_CODE_EFFORT_LEVEL":                 "high",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
	} {
		if env[key] != want {
			t.Errorf("%s = %q, want %q", key, env[key], want)
		}
	}
	for _, removed := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_MODEL", "ANTHROPIC_CUSTOM_PRIMARY"} {
		if _, exists := env[removed]; exists {
			t.Errorf("fallback retained primary variable %s", removed)
		}
	}
}

func TestLoadProviderRouterRejectsUnsafeVariablesWithoutLeakingValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	const secret = "must-not-appear"
	if err := os.WriteFile(path, []byte(`{"bad":{"PATH":"`+secret+`"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadProviderRouter("anthropic", []string{"bad"}, path, []string{"PATH=bin"})
	if err == nil || !strings.Contains(err.Error(), "unsupported environment variable") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("configuration error leaked a provider value")
	}
}

func TestLoadProviderRouterRejectsCaseInsensitiveDuplicateProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "providers.json")
	if err := os.WriteFile(path, []byte(`{"Kimi":{},"kimi":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadProviderRouter("anthropic", []string{"kimi"}, path, nil)
	if err == nil || !strings.Contains(err.Error(), "differ only by case") {
		t.Fatalf("error = %v", err)
	}
}

func TestProviderRouterSharesStickyFailover(t *testing.T) {
	router := NewProviderRouter([]ProviderRoute{{Name: "anthropic", Primary: true}, {Name: "kimi"}, {Name: "minimax"}})
	kimi, ok := router.Failover("anthropic")
	if !ok || kimi.Name != "kimi" {
		t.Fatalf("first failover = %#v, %v", kimi, ok)
	}
	fromStaleWorker, ok := router.Failover("anthropic")
	if !ok || fromStaleWorker.Name != "kimi" {
		t.Fatalf("stale worker failover = %#v, %v", fromStaleWorker, ok)
	}
	minimax, ok := router.Failover("kimi")
	if !ok || minimax.Name != "minimax" {
		t.Fatalf("second failover = %#v, %v", minimax, ok)
	}
	if _, ok := router.Failover("minimax"); ok {
		t.Fatal("expected all routes exhausted")
	}
}

func testEnvironmentMap(entries []string) map[string]string {
	result := make(map[string]string, len(entries))
	for _, entry := range entries {
		key, value, found := strings.Cut(entry, "=")
		if found {
			result[strings.ToUpper(key)] = value
		}
	}
	return result
}
