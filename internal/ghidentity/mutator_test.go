package ghidentity

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
)

func TestMutateEnv_NoRoleIsNoOp(t *testing.T) {
	m := &Mutator{Config: &Config{Roles: map[string]RoleConfig{
		"default": {Owner: "hongming"},
	}}}
	env := map[string]string{}
	if err := m.MutateEnv(context.Background(), "ws-abc", env); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(env) != 0 {
		t.Errorf("expected no mutation without role, got %v", env)
	}
}

func TestMutateEnv_InjectsAllFields(t *testing.T) {
	m := &Mutator{Config: &Config{Roles: map[string]RoleConfig{
		"PMM-Lead": {Owner: "hongming"},
	}}}
	env := map[string]string{"MOLECULE_AGENT_ROLE": "pmm-lead"}
	if err := m.MutateEnv(context.Background(), "ws-abcdef01-foo", env); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Role sanitized
	if got := env["MOLECULE_AGENT_ROLE"]; got != "Pmm-Lead" {
		t.Errorf("expected Pmm-Lead, got %q", got)
	}
	// Owner resolved via case-insensitive lookup: config key "PMM-Lead"
	// matches sanitized role "Pmm-Lead" because we lower-case both sides.
	if got := env["MOLECULE_OWNER"]; got != "hongming" {
		t.Errorf("expected owner=hongming, got %q", got)
	}
	// Workspace id passed through
	if env["MOLECULE_WORKSPACE_ID"] != "ws-abcdef01-foo" {
		t.Errorf("workspace id not set")
	}
	// Badge contains role + short id
	if !strings.Contains(env["MOLECULE_ATTRIBUTION_BADGE"], "Pmm-Lead") {
		t.Errorf("badge missing role: %q", env["MOLECULE_ATTRIBUTION_BADGE"])
	}
	if !strings.Contains(env["MOLECULE_ATTRIBUTION_BADGE"], "ws-abcdef01") {
		t.Errorf("badge missing short id: %q", env["MOLECULE_ATTRIBUTION_BADGE"])
	}
	// Wrapper base64 decodes back to wrapper.sh
	b64 := env["MOLECULE_GH_WRAPPER_B64"]
	if b64 == "" {
		t.Fatal("wrapper b64 not set")
	}
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(string(decoded), "#!/usr/bin/env bash") {
		t.Errorf("wrapper decode mismatch")
	}
	// Wrapper sha is short hex
	if len(env["MOLECULE_GH_WRAPPER_SHA"]) != 12 {
		t.Errorf("wrapper sha length: %d", len(env["MOLECULE_GH_WRAPPER_SHA"]))
	}
}

func TestMutateEnv_NilMapErrors(t *testing.T) {
	m := &Mutator{}
	if err := m.MutateEnv(context.Background(), "ws-1", nil); err == nil {
		t.Fatal("expected error on nil env map")
	}
}

func TestResolveOwner_FallbackChain(t *testing.T) {
	cfg := &Config{Roles: map[string]RoleConfig{
		"PMM":     {Owner: "alice"},
		"default": {Owner: "bob"},
	}}
	cases := []struct {
		role, want string
	}{
		{"PMM", "alice"},
		{"pmm", "alice"}, // case-insensitive
		{"Pmm", "alice"}, // case-insensitive (sanitized form)
		{"unknown-role", "bob"},
		{"", "bob"},
	}
	for _, c := range cases {
		if got := cfg.ResolveOwner(c.role); got != c.want {
			t.Errorf("role=%q: got %q want %q", c.role, got, c.want)
		}
	}
}

func TestResolveOwner_NoDefaultReturnsEmpty(t *testing.T) {
	cfg := &Config{Roles: map[string]RoleConfig{
		"PMM": {Owner: "alice"},
	}}
	if got := cfg.ResolveOwner("unknown"); got != "" {
		t.Errorf("expected empty for unknown role without default, got %q", got)
	}
}

func TestSanitizeRole(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"  pmm-lead ", "Pmm-Lead"},
		{"ResearchLead", "ResearchLead"},
		{"multi-part-role", "Multi-Part-Role"},
		{"  -starts-with-dash", "-Starts-With-Dash"}, // edge: preserved
	}
	for _, c := range cases {
		if got := SanitizeRole(c.in); got != c.want {
			t.Errorf("in=%q: got %q want %q", c.in, got, c.want)
		}
	}
}
