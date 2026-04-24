# Known Issues

## Wrapper ships as base64 env var (migrate to v2 files axis)

**State**: accepted trade-off for v1, tracked for v2 migration.

**What**: `MutateEnv` base64-encodes `wrapper.sh` into `MOLECULE_GH_WRAPPER_B64`.
Each workspace template's `install.sh` decodes and writes it to
`/usr/local/bin/gh`.

**Why not a direct file write?** The platform's `provisionhook` interface
only has `EnvMutator` today — there's no `FileMutator` or
`contributions.files` surface. Inventing one here would couple this
plugin to a core-monorepo API change.

**Consequences**:
- Every workspace template (hermes, claude-code, langgraph, etc.) needs
  a ~10-line `install.sh` snippet to decode + install the wrapper. The
  plugin ships the canonical snippet; template authors paste it in.
- Wrapper size is capped by env var limits (EC2 user-data ~16KB total;
  wrapper is ~5KB after base64, plenty of headroom).
- Wrapper updates propagate via plugin version bump, but require a
  workspace RESTART to take effect (new user-data writes the new
  wrapper). Not hot-reloadable in v1.

**Migration target**: [plugin-architecture-v2][v2], phase 6 — the
unified contribution manifest adds `contributions.files` as an explicit
axis. At that point:
- Plugin declares the file write in YAML manifest, not Go code.
- Platform's v2 provisioner handles the file write.
- Templates drop their install-snippet.
- Grade-A hot reload becomes possible (platform can re-emit the file
  without a workspace restart).

[v2]: https://github.com/Molecule-AI/internal/blob/main/product/plugin-architecture-v2.md

---

## Role is read from env map, not workspace metadata

**State**: requires a small monorepo-side change.

**What**: `Mutator.MutateEnv` expects `env["MOLECULE_AGENT_ROLE"]` to
already be populated by the provisioner. The provisioner does NOT do
this today — workspace metadata's `role` field is not propagated into
the env map before mutators run.

**Why not read workspace metadata directly in the plugin?** The
`EnvMutator` interface deliberately gives plugins a narrow view — they
get the env map, the workspace ID, and nothing else. Passing the full
workspace struct would let plugins read secrets / plan / parent
relationships the plugin has no business caring about.

**Fix**: a small monorepo PR (~3 lines in
`workspace-server/internal/handlers/workspace_provision.go`) populates
`env["MOLECULE_AGENT_ROLE"]` from the workspace row's `role` column
before calling the mutator chain. Tracked in the companion monorepo PR.

Until that lands, the plugin is safe — absent the env var, it no-ops
and the wrapper script falls back to pass-through.

---

## Wrapper heuristics miss non-trivial argv shapes

**State**: accepted; works for 95% of agent gh calls.

**What**: `wrapper.sh` parses argv to detect publishing commands by
matching "first non-flag token + second non-flag token" against a
hardcoded list (`issue create`, `pr comment`, etc.). This misses:

- `gh api` calls constructing issues/PRs via raw REST — no `--body`
  flag to intercept.
- Custom `gh alias` expansions (an alias like `gh post` expanding to
  `gh issue create` won't be recognized — we see `gh post`, not
  `gh issue create`).
- Flag ordering oddities where the verb appears after global flags
  the wrapper doesn't know about (unlikely but possible).

**Consequences**: some agent actions bypass attribution. The audit log
still captures them (every invocation is logged regardless of
rewrite), so this is a visibility gap, not a correctness gap.

**Fix**: when/if this becomes common, migrate to wrapping the gh Go
binary directly (gh exposes a Go-plugin extension model) rather than
shell-argv rewriting. Not planned for v1.

---

## Audit log grows unbounded

**State**: accepted; needs rotation in workspace base image.

**What**: Every wrapper invocation appends one NDJSON line to
`/var/log/molecule-gh.ndjson`. No rotation, no size limit.

**Why no rotation in the plugin?** Log rotation is a workspace-host
concern, not a plugin concern. The workspace base image's logrotate
config should cover `/var/log/molecule-gh.ndjson` the same way it
covers other `/var/log/*.ndjson`.

**Fix**: ensure logrotate config in workspace base image includes this
file. Follow-up issue in the monorepo.
