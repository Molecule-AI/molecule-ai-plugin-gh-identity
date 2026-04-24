package ghidentity

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// Mutator implements the monorepo's provisionhook.EnvMutator interface.
// Exported so cmd/server (monorepo side) can register it at platform
// boot and the provisioner calls MutateEnv per-workspace.
type Mutator struct {
	Config *Config
}

// Name satisfies provisionhook.EnvMutator. Appears in log lines and
// metrics; keep stable — matches the plugin's manifest name.
func (m *Mutator) Name() string { return "gh-identity" }

// MutateEnv is the per-workspace entry point. It reads the workspace's
// declared role (passed in via the env map pre-populated by the
// provisioner — see "Role resolution" below) and injects:
//
//   - MOLECULE_AGENT_ROLE       — sanitized role string
//   - MOLECULE_OWNER            — GitHub user to rewrite @me to
//   - MOLECULE_WORKSPACE_ID     — for the audit log
//   - MOLECULE_ATTRIBUTION_BADGE — the markdown badge the wrapper prepends
//   - MOLECULE_GH_WRAPPER_B64   — base64'd wrapper.sh; template decodes
//   - MOLECULE_GH_WRAPPER_SHA   — hash of wrapper.sh for version pinning
//
// ## Role resolution
//
// The role is expected in env["MOLECULE_AGENT_ROLE"] already — the
// workspace-server's provisionWorkspace reads workspace metadata (the
// `role` field on the workspace row) and sets it BEFORE calling
// mutators. If it's unset we skip silently; the wrapper script falls
// back to pass-through mode in that case so nothing breaks.
//
// This mutator never returns an error for policy reasons: a missing
// config file OR an unknown role must NOT block workspace boot. The
// wrapper passes through gracefully when env is absent.
func (m *Mutator) MutateEnv(ctx context.Context, workspaceID string, env map[string]string) error {
	if env == nil {
		return fmt.Errorf("gh-identity: env map is nil")
	}
	rawRole := env["MOLECULE_AGENT_ROLE"]
	role := SanitizeRole(rawRole)
	if role == "" {
		// No role declared → plugin is a no-op for this workspace.
		// Leave env alone; wrapper falls back to pass-through.
		return nil
	}

	owner := ""
	if m.Config != nil {
		owner = m.Config.ResolveOwner(role)
	}

	env["MOLECULE_AGENT_ROLE"] = role
	env["MOLECULE_OWNER"] = owner
	env["MOLECULE_WORKSPACE_ID"] = workspaceID
	env["MOLECULE_ATTRIBUTION_BADGE"] = fmt.Sprintf("🤖 [Agent: %s · %s]", role, shortID(workspaceID))

	// Ship the wrapper as base64 so the template's install.sh can
	// decode + write without dealing with newline-embedded strings in
	// cloud-init user-data.
	env["MOLECULE_GH_WRAPPER_B64"] = base64.StdEncoding.EncodeToString([]byte(WrapperScript))
	h := sha256.Sum256([]byte(WrapperScript))
	env["MOLECULE_GH_WRAPPER_SHA"] = hex.EncodeToString(h[:])[:12]

	return nil
}

// shortID returns a human-readable tag for the workspace, used in the
// attribution badge. Workspace IDs are UUIDs (e.g. d3605ef2-f7d6-…), so
// we take the first 8 hex chars and prefix "ws-" → "ws-d3605ef2".
// Idempotent: strips any pre-existing "ws-" to avoid "ws-ws-…" if a
// caller happens to pass an already-prefixed id (some test fixtures do).
func shortID(id string) string {
	id = strings.TrimPrefix(id, "ws-")
	if len(id) >= 8 {
		return "ws-" + id[:8]
	}
	return "ws-" + id
}
