#!/usr/bin/env bash
# test-wrapper.sh — offline unit tests for wrapper.sh argv rewriting.
#
# We can't run the wrapper end-to-end without a real gh binary, so
# these tests stub gh to echo its argv and check the wrapper mutates
# correctly. Exercises the contract in wrapper.sh top-of-file.
#
# Run: bash scripts/test-wrapper.sh

set -u

HERE="$(cd "$(dirname "$0")" && pwd)"
WRAPPER="$HERE/../internal/ghidentity/wrapper.sh"

if [ ! -f "$WRAPPER" ]; then
  echo "FAIL: wrapper.sh not found at $WRAPPER" >&2
  exit 2
fi

TMP=$(mktemp -d)
# shellcheck disable=SC2064  # $TMP must expand now — it's what we want to clean
trap "rm -rf $TMP" EXIT

# Stub gh: echoes its argv, one per line prefixed with a marker.
mkdir -p "$TMP/bin"
cat > "$TMP/bin/gh" <<'STUB'
#!/usr/bin/env bash
for arg in "$@"; do
  printf 'ARG<%s>\n' "$arg"
done
STUB
chmod +x "$TMP/bin/gh"

# The wrapper looks for /usr/bin/gh first; on macOS/linux CI that
# either is or isn't real gh. We redirect by symlinking our stub
# into a search path the wrapper checks, and prepending $TMP/bin
# to PATH for the command-v fallback.
ln -sf "$TMP/bin/gh" "$TMP/bin/gh.real"

PASS=0
FAIL=0

# Run wrapper with a controlled env and capture output.
# Takes argv for the wrapper directly; caller sets env vars inline.
run_wrapper() {
  (
    export MOLECULE_AGENT_ROLE="$MOLECULE_AGENT_ROLE"
    export MOLECULE_OWNER="${MOLECULE_OWNER:-}"
    export MOLECULE_ATTRIBUTION_BADGE="${MOLECULE_ATTRIBUTION_BADGE:-}"
    export MOLECULE_WORKSPACE_ID="${MOLECULE_WORKSPACE_ID:-ws-test}"
    # Force wrapper to find our stub by prepending a fake /usr/bin path.
    # The wrapper checks /usr/bin/gh first — on CI that might be the real
    # gh. For test predictability we use the PATH fallback by ensuring
    # /usr/bin/gh does not exist IN OUR TEST ENV via sandboxing. Simplest:
    # patch the wrapper via sed to point at our stub.
    sed "s|/usr/bin/gh|$TMP/bin/gh|g; s|/opt/gh/bin/gh|$TMP/bin/gh.real|g" "$WRAPPER" > "$TMP/wrapper-patched.sh"
    chmod +x "$TMP/wrapper-patched.sh"
    bash "$TMP/wrapper-patched.sh" "$@" 2>&1
  )
}

assert_contains() {
  local label="$1" needle="$2" haystack="$3"
  if echo "$haystack" | grep -qF "$needle"; then
    echo "  PASS  $label"
    PASS=$((PASS+1))
  else
    echo "  FAIL  $label"
    echo "    looking for: $needle"
    echo "    in: $haystack" | head -c 400
    echo ""
    FAIL=$((FAIL+1))
  fi
}

assert_not_contains() {
  local label="$1" needle="$2" haystack="$3"
  if echo "$haystack" | grep -qF "$needle"; then
    echo "  FAIL  $label (unexpectedly contained: $needle)"
    FAIL=$((FAIL+1))
  else
    echo "  PASS  $label"
    PASS=$((PASS+1))
  fi
}

echo "== wrapper.sh =="

# === Test 1: no MOLECULE_AGENT_ROLE → pure passthrough ===
MOLECULE_AGENT_ROLE="" out=$(run_wrapper issue create --body "hello")
assert_contains "no role → passthrough, preserves --body" "ARG<--body>" "$out"
assert_contains "no role → passthrough, preserves hello verbatim" "ARG<hello>" "$out"
assert_not_contains "no role → no badge injected" "🤖" "$out"

# === Test 2: role set + issue create → badge prepended to --body ===
MOLECULE_AGENT_ROLE=PMM-Lead \
  MOLECULE_OWNER=hongming \
  MOLECULE_ATTRIBUTION_BADGE="🤖 [Agent: PMM-Lead · ws-abc]" \
  out=$(run_wrapper issue create --body "hello")
assert_contains "badge-prepend: badge present" "🤖 [Agent: PMM-Lead · ws-abc]" "$out"
assert_contains "badge-prepend: original body preserved" "hello" "$out"

# === Test 3: --assignee @me → rewritten to OWNER ===
MOLECULE_AGENT_ROLE=PMM \
  MOLECULE_OWNER=alice \
  MOLECULE_ATTRIBUTION_BADGE="🤖 [Agent: PMM]" \
  out=$(run_wrapper issue create --assignee @me --body "hi")
assert_contains "assignee-rewrite: new owner injected" "ARG<alice>" "$out"
assert_not_contains "assignee-rewrite: @me stripped" "ARG<@me>" "$out"

# === Test 4: --assignee=@me (equals form) → rewritten ===
MOLECULE_AGENT_ROLE=PMM \
  MOLECULE_OWNER=alice \
  MOLECULE_ATTRIBUTION_BADGE="🤖" \
  out=$(run_wrapper issue create --assignee=@me --body "hi")
assert_contains "assignee-equals-form: rewritten" "ARG<--assignee=alice>" "$out"

# === Test 5: non-publish verb (`gh repo view`) → body untouched even if present ===
MOLECULE_AGENT_ROLE=PMM \
  MOLECULE_OWNER=alice \
  MOLECULE_ATTRIBUTION_BADGE="🤖 PMM" \
  out=$(run_wrapper repo view --json body)
assert_not_contains "non-publish: no badge injection" "🤖 PMM" "$out"

# === Test 6: publish with no --body → NO synthetic body added ===
MOLECULE_AGENT_ROLE=PMM \
  MOLECULE_OWNER=alice \
  MOLECULE_ATTRIBUTION_BADGE="🤖 PMM" \
  out=$(run_wrapper issue create --title "foo")
assert_not_contains "no-body: wrapper does not synth a --body" "ARG<--body>" "$out"

# === Test 7: --assignee @me with no OWNER → flag dropped entirely ===
MOLECULE_AGENT_ROLE=PMM \
  MOLECULE_OWNER="" \
  MOLECULE_ATTRIBUTION_BADGE="🤖" \
  out=$(run_wrapper issue create --assignee @me --body "x")
assert_not_contains "assignee-drop: @me dropped when no owner" "ARG<@me>" "$out"
assert_not_contains "assignee-drop: --assignee flag dropped too" "ARG<--assignee>" "$out"

echo
echo "== results: $PASS passed, $FAIL failed =="
[ "$FAIL" -eq 0 ]
