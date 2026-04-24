// Package ghidentity implements the workspace-server plugin that injects
// per-agent attribution env vars into workspace containers.
//
// See repo README for the "why" (molecule-core#1957 agent-identity
// collapse). This package contains the wiring; the behavioural logic
// lives in wrapper.sh which is shipped to the workspace via env.
package ghidentity

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config maps agent roles (set per-workspace via MOLECULE_AGENT_ROLE)
// to the human GitHub user who "owns" that role for purposes of @me
// rewriting. A single default entry covers roles not explicitly listed.
//
// Config is loaded once at platform boot from
// $MOLECULE_GH_IDENTITY_CONFIG_FILE. Missing file → use the DefaultOwner
// for all roles; plugin still works, just with blanket attribution.
type Config struct {
	Roles map[string]RoleConfig `yaml:"roles"`
}

// RoleConfig defines the per-role settings. Today: just the owner.
// Future fields (capability overrides, rate limits, per-role repo
// allowlists) slot in here without breaking the surface.
type RoleConfig struct {
	Owner string `yaml:"owner"`
}

// LoadConfig reads a YAML config file. Missing file is not an error —
// returns a Config with an empty Roles map and the caller falls through
// to DefaultOwner.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		return &Config{Roles: map[string]RoleConfig{}}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Roles: map[string]RoleConfig{}}, nil
		}
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	if cfg.Roles == nil {
		cfg.Roles = map[string]RoleConfig{}
	}
	return &cfg, nil
}

// ResolveOwner picks the GitHub owner for the given role. Unknown roles
// fall through to the "default" entry; if neither is set, returns "" so
// the wrapper strips --assignee @me entirely (correct behavior — better
// than assigning to the wrong person).
//
// Lookup is case-insensitive against the sanitized role form. The
// yaml config writer might use "PMM-Lead", "pmm-lead", or "Pmm-Lead"
// interchangeably — we accept all three by lower-casing both sides.
// "default" is treated literally (reserved key).
func (c *Config) ResolveOwner(role string) string {
	needle := strings.ToLower(role)
	for k, rc := range c.Roles {
		if k == "default" {
			continue
		}
		if strings.ToLower(k) == needle && rc.Owner != "" {
			return rc.Owner
		}
	}
	if rc, ok := c.Roles["default"]; ok {
		return rc.Owner
	}
	return ""
}

// SanitizeRole normalizes a role string for use in env vars / badges.
// Strips whitespace, upper-cases the first letter of each hyphen-
// separated segment so arbitrary user input ("  pmm-lead ") becomes a
// predictable string ("PMM-Lead") visible in attribution badges.
func SanitizeRole(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "-")
}
