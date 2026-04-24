# molecule-ai-plugin-gh-identity

Injects per-agent identity into workspace env so every `gh` CLI call
carries agent attribution — without needing a distinct GitHub account per
agent.

## Problem

All agents in a Molecule fleet share one GitHub PAT. When an agent runs:

```
gh issue create --assignee @me ...
gh pr comment ...
```

`@me` resolves to the PAT owner (the CEO). Every issue, PR, and comment
gets attributed to one person, making audit impossible and flooding that
person's notifications. See [molecule-core#1957].

GitHub's identity model doesn't scale the way agent fleets do: creating N
machine users requires N emails + seats; creating N GitHub Apps requires
N manual UI round-trips. Neither is batch-generatable.

## Approach

Work around the identity model with a convention, enforced by a tiny
shell wrapper and this plugin's env injection.

1. Plugin injects per-workspace env: `MOLECULE_AGENT_ROLE`,
   `MOLECULE_OWNER`, `MOLECULE_ATTRIBUTION_BADGE`.
2. The workspace base image ships a `gh` wrapper (`/usr/local/bin/gh`)
   that reads `$MOLECULE_AGENT_ROLE` and:
   - prepends an attribution block to every `issue comment` / `pr
     comment` / `issue create --body` / `pr create --body`
   - rewrites `--assignee @me` to `--assignee $MOLECULE_OWNER` (or
     strips it entirely)
   - emits an audit line to `/var/log/molecule-gh.ndjson`
3. A `git` wrapper does the same for `Co-authored-by:` on commits.

The wrapper script is shipped embedded in the plugin (`wrapper.sh`) and
installed by each workspace-template's `install.sh` when the plugin is
active. Plugin → env injection; template → file write.

## What this plugin is NOT

- NOT a GitHub App installer. Existing `molecule-ai-plugin-github-app-auth`
  handles App-based auth; this plugin is additive and does not conflict.
- NOT a machine-user provisioning tool. There are no distinct GitHub
  identities; attribution is text-based.
- NOT a per-agent rate limiter or cost accounter (future work; see #1957
  follow-ups).

## Env vars injected

| Name | Source | Example |
|---|---|---|
| `MOLECULE_AGENT_ROLE` | workspace metadata (`role` field) | `PMM-Lead` |
| `MOLECULE_OWNER` | plugin config (role → owner map) | `HongmingWang-Rabbit` |
| `MOLECULE_ATTRIBUTION_BADGE` | computed | `🤖 [Agent: PMM-Lead · ws-a0689c35]` |
| `MOLECULE_GH_WRAPPER_SHA` | computed | hash of wrapper.sh for version pinning |

## Plugin manifest (v1)

This plugin ships as a v1 plugin (matching `molecule-ai-plugin-github-app-auth`).
Migration to [plugin-architecture-v2] happens in phase 6 of that plan.
The v1 shape here is intentionally structured so v2 migration is mostly a
manifest rename:

- `EnvMutator.MutateEnv` → v2's `contributions.env` + `hooks.env_refresh`
- Role→owner map in `config.yaml` → v2's `spec.config`
- Wrapper script shipping → v2's `contributions.files` (new axis)

## Install (v1)

Monorepo side:
```
manifest.json:plugins += {name: "gh-identity", repo: "Molecule-AI/molecule-ai-plugin-gh-identity", ref: "main"}
workspace-server/go.mod: require github.com/Molecule-AI/molecule-ai-plugin-gh-identity
workspace-server/cmd/server/main.go: pluginloader.BuildRegistry()
```

Env (operator):
```
MOLECULE_GH_IDENTITY_CONFIG_FILE=/path/to/config.yaml
```

## Config

```yaml
# config.yaml — role → owner map (used for `@me` rewrite)
roles:
  PMM-Lead:     { owner: HongmingWang-Rabbit }
  Dev-Lead:     { owner: HongmingWang-Rabbit }
  Research-Lead:{ owner: HongmingWang-Rabbit }
  default:      { owner: HongmingWang-Rabbit }
```

## Capabilities requested (v2 forward-compat)

When v2 enforcement lands, this plugin will declare:

- `workspace:env_inject` — required
- `workspace:file_write:/usr/local/bin/gh` — required (via template install.sh)
- `audit:emit` — required
- `network_egress:api.github.com` — required (wrapper makes API calls via real gh)

No broader capabilities. In particular: **no secret access** (PAT is
shared and platform-managed, not in plugin scope).

## Related

- molecule-core#1957 — agent identity collapse (this plugin's driver)
- molecule-core#1933 — GH_TOKEN refresh (separate concern; handled by
  github-app-auth plugin)
- internal `product/plugin-architecture-v2.md` — target arch for v2
  migration

[molecule-core#1957]: https://github.com/Molecule-AI/molecule-core/issues/1957
[plugin-architecture-v2]: https://github.com/Molecule-AI/internal/blob/main/product/plugin-architecture-v2.md
