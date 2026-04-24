#!/usr/bin/env bash
# molecule-gh wrapper — intercepts `gh` calls to inject agent attribution
# and kill the @me-collapses-to-CEO anti-pattern (molecule-core#1957).
#
# Installed at /usr/local/bin/gh ahead of the real gh binary at /usr/bin/gh
# by the workspace template's install.sh. Both `gh` and `git` (a separate
# wrapper calling this for `git commit` trailers) read the same env.
#
# The wrapper is opt-in: if $MOLECULE_AGENT_ROLE is unset, we pass through
# unchanged. Workspaces without the plugin behave exactly like today.
#
# Audit log is append-only NDJSON at /var/log/molecule-gh.ndjson. Each
# invocation emits one line with role, workspace id, argv, and exit code.
# Log readers: PM triage, post-incident forensics, and the audit:emit
# capability if v2 formalizes it.

set -uo pipefail

# Find the real gh — must NOT be this script.
real_gh() {
  local p
  for p in /usr/bin/gh /opt/gh/bin/gh /usr/local/bin/gh.real; do
    if [ -x "$p" ] && [ "$p" != "/usr/local/bin/gh" ]; then
      echo "$p"
      return 0
    fi
  done
  # Fall back to PATH hunt, skipping this wrapper by path.
  local self="${BASH_SOURCE[0]}"
  local cand
  while IFS= read -r cand; do
    if [ -x "$cand" ] && [ "$cand" != "$self" ]; then
      echo "$cand"
      return 0
    fi
  done < <(command -v -a gh 2>/dev/null | grep -v "^$self$")
  return 1
}

audit_emit() {
  local rc="$1"; shift
  local log_file="/var/log/molecule-gh.ndjson"
  # Quote argv via python's json for safety (shell arg quoting is a trap).
  # Timestamp comes from _MOLECULE_GH_TS exported by the caller.
  python3 - "$@" <<PYEOF 2>/dev/null >> "$log_file" || true
import json, sys, os
argv = sys.argv[1:]
rec = {
  "ts": os.environ.get("_MOLECULE_GH_TS"),
  "role": os.environ.get("MOLECULE_AGENT_ROLE",""),
  "workspace_id": os.environ.get("MOLECULE_WORKSPACE_ID",""),
  "owner": os.environ.get("MOLECULE_OWNER",""),
  "rc": int(os.environ.get("_MOLECULE_GH_RC","0")),
  "argv": argv,
}
print(json.dumps(rec))
PYEOF
}

# Short-circuit: plugin disabled → pure passthrough.
if [ -z "${MOLECULE_AGENT_ROLE:-}" ]; then
  exec "$(real_gh)" "$@"
fi

BADGE="${MOLECULE_ATTRIBUTION_BADGE:-🤖 [Agent: ${MOLECULE_AGENT_ROLE}]}"
OWNER="${MOLECULE_OWNER:-}"

# Rewrite argv:
#   1. If we see --assignee @me, replace with the human owner (or drop).
#   2. If we see --body <text> on a command that publishes to github,
#      prepend BADGE + two newlines to <text>. Only rewrites once per
#      invocation, to stay idempotent.
#
# The set of publishing commands is small and well-known — we explicitly
# enumerate them rather than rewriting every --body (e.g. `gh release
# view --body-length` would be mis-matched on a loose grep).
PUBLISH_VERBS=(
  "issue create"
  "issue comment"
  "issue edit"
  "pr create"
  "pr comment"
  "pr edit"
  "pr review"
  "release create"
  "release edit"
  "discussion create"
)

argv=("$@")
n=${#argv[@]}

# Detect which verb is being invoked by joining the first 2 non-flag tokens.
# `gh <subcmd> <verb> [flags]` — we just need to know if this is a
# publishing verb.
first=""; second=""
for ((i=0; i<n; i++)); do
  tok="${argv[$i]}"
  [[ "$tok" == -* ]] && continue
  if [ -z "$first" ]; then first="$tok"
  elif [ -z "$second" ]; then second="$tok"; break
  fi
done
verb="$first $second"

is_publish=0
for pv in "${PUBLISH_VERBS[@]}"; do
  if [ "$pv" = "$verb" ]; then is_publish=1; break; fi
done

# Rewrite @me and --body.
new_argv=()
body_rewritten=0
skip=0
for ((i=0; i<n; i++)); do
  if [ "$skip" = "1" ]; then skip=0; continue; fi
  tok="${argv[$i]}"
  case "$tok" in
    --assignee)
      next="${argv[$((i+1))]:-}"
      if [ "$next" = "@me" ]; then
        if [ -n "$OWNER" ]; then
          new_argv+=("--assignee" "$OWNER")
        fi
        # If no OWNER configured, drop the flag entirely.
        skip=1
        continue
      fi
      ;;
    --assignee=@me)
      if [ -n "$OWNER" ]; then
        new_argv+=("--assignee=$OWNER")
      fi
      continue
      ;;
    --body)
      if [ "$is_publish" = "1" ] && [ "$body_rewritten" = "0" ]; then
        next="${argv[$((i+1))]:-}"
        new_argv+=("--body" "${BADGE}"$'\n\n'"${next}")
        skip=1
        body_rewritten=1
        continue
      fi
      ;;
    --body=*)
      if [ "$is_publish" = "1" ] && [ "$body_rewritten" = "0" ]; then
        body="${tok#--body=}"
        new_argv+=("--body=${BADGE}"$'\n\n'"${body}")
        body_rewritten=1
        continue
      fi
      ;;
  esac
  new_argv+=("$tok")
done

# If publishing with no --body provided, we don't add one — the real gh
# will either prompt ($EDITOR) or fail, same as today. We don't want to
# turn a "you forgot --body" error into "we silently posted a badge-only
# comment."

GH=$(real_gh) || { echo "molecule-gh: cannot find real gh binary" >&2; exit 127; }

_MOLECULE_GH_TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  "$GH" "${new_argv[@]}"
rc=$?

_MOLECULE_GH_RC=$rc audit_emit "$rc" "${new_argv[@]}"
exit $rc
