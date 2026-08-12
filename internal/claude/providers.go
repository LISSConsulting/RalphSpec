package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const defaultProviderConfigFile = ".claude/providers.json"

var providerVariablePattern = regexp.MustCompile(`^(?:ANTHROPIC|CLAUDE_CODE)_[A-Z0-9_]+$`)

// ProviderRoute is one Claude-compatible provider environment. The primary
// route inherits Ralph's startup environment; fallback routes receive an
// isolated environment built from the named provider profile.
type ProviderRoute struct {
	Name        string
	Environment []string
	Primary     bool
}

// ProviderRouter tracks the active provider for one Ralph run. It is safe to
// share between parallel worktree agents so an exhausted route is retired once.
type ProviderRouter struct {
	mu        sync.Mutex
	routes    []ProviderRoute
	active    int
	exhausted map[string]struct{}
}

// LoadProviderRouter loads named fallback profiles using the same configuration
// shape and variable allowlist as the AIProvider PowerShell module.
func LoadProviderRouter(primary string, fallbacks []string, configFile string, baseEnv []string) (*ProviderRouter, error) {
	primary = strings.TrimSpace(primary)
	if primary == "" {
		primary = "anthropic"
	}
	routes := []ProviderRoute{{Name: primary, Primary: true}}
	if len(fallbacks) == 0 {
		return NewProviderRouter(routes), nil
	}

	path, err := resolveProviderConfigFile(configFile)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Claude provider configuration %s: %w", path, err)
	}
	var rawProfiles map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawProfiles); err != nil {
		return nil, fmt.Errorf("decode Claude provider configuration %s: %w", path, err)
	}

	profiles := make(map[string]providerProfile, len(rawProfiles))
	for profileName, variables := range rawProfiles {
		keyName := strings.ToLower(strings.TrimSpace(profileName))
		if keyName == "" {
			return nil, fmt.Errorf("claude provider configuration contains an empty provider name")
		}
		if existing, duplicate := profiles[keyName]; duplicate {
			return nil, fmt.Errorf("claude provider configuration defines provider names %q and %q that differ only by case", existing.name, profileName)
		}
		profile := providerProfile{name: profileName, variables: make(map[string]*string, len(variables))}
		for name, raw := range variables {
			if !allowedProviderVariable(name) {
				return nil, fmt.Errorf("provider %q contains unsupported environment variable %q", profileName, name)
			}
			key := strings.ToUpper(name)
			if string(raw) == "null" {
				profile.variables[key] = nil
				continue
			}
			var value string
			if err := json.Unmarshal(raw, &value); err != nil {
				return nil, fmt.Errorf("provider %q variable %q must be a string or null", profileName, name)
			}
			profile.variables[key] = &value
		}
		profiles[keyName] = profile
	}
	if baseEnv == nil {
		baseEnv = os.Environ()
	}
	for _, requested := range fallbacks {
		profile, ok := profiles[strings.ToLower(strings.TrimSpace(requested))]
		if !ok {
			return nil, fmt.Errorf("claude fallback provider %q is not defined in %s", requested, path)
		}
		routes = append(routes, ProviderRoute{
			Name:        profile.name,
			Environment: providerEnvironment(baseEnv, profile.variables),
		})
	}
	return NewProviderRouter(routes), nil
}

type providerProfile struct {
	name      string
	variables map[string]*string
}

// NewProviderRouter constructs a router from already-resolved routes.
func NewProviderRouter(routes []ProviderRoute) *ProviderRouter {
	cloned := make([]ProviderRoute, len(routes))
	copy(cloned, routes)
	return &ProviderRouter{routes: cloned, exhausted: make(map[string]struct{})}
}

// Current returns the active non-exhausted route.
func (r *ProviderRouter) Current() ProviderRoute {
	if r == nil {
		return ProviderRoute{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.routes) == 0 {
		return ProviderRoute{}
	}
	if r.active >= len(r.routes) {
		return r.routes[len(r.routes)-1]
	}
	return r.routes[r.active]
}

// Failover retires failed and returns the next available route. If another
// worker already advanced the shared router, that active route is returned.
func (r *ProviderRouter) Failover(failed string) (ProviderRoute, bool) {
	if r == nil {
		return ProviderRoute{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.routes) == 0 {
		return ProviderRoute{}, false
	}
	r.exhausted[strings.ToLower(failed)] = struct{}{}
	for r.active < len(r.routes) {
		current := r.routes[r.active]
		if _, unavailable := r.exhausted[strings.ToLower(current.Name)]; !unavailable {
			return current, !strings.EqualFold(current.Name, failed)
		}
		r.active++
	}
	return ProviderRoute{}, false
}

func resolveProviderConfigFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for Claude provider configuration: %w", err)
	}
	if path == "" {
		return filepath.Join(home, filepath.FromSlash(defaultProviderConfigFile)), nil
	}
	if path == "~" {
		return home, nil
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		path = filepath.Join(home, path[2:])
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve Claude provider configuration %s: %w", path, err)
	}
	return absolute, nil
}

func allowedProviderVariable(name string) bool {
	return providerVariablePattern.MatchString(name) || name == "ENABLE_TOOL_SEARCH"
}
func providerEnvironment(base []string, values map[string]*string) []string {
	env := make([]string, 0, len(base)+len(values))
	for _, entry := range base {
		key := entry
		if index := strings.IndexByte(entry, '='); index >= 0 {
			key = entry[:index]
		}
		if !allowedProviderVariable(strings.ToUpper(key)) {
			env = append(env, entry)
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := values[key]
		if value != nil {
			env = append(env, key+"="+*value)
		}
	}
	return env
}
