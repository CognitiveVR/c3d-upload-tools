#!/bin/bash

# test-env-loader.sh
#
# Unit tests for upload-utils.sh::load_env_file's value-parsing behavior.
# Specifically covers the SDK-500 quote-stripping fix: bash and PowerShell
# must agree on how to parse quoted .env values, otherwise users hit a
# silent 401 on bash (because the literal quotes get sent in the
# Authorization header) while PowerShell works fine on the same .env file.

set -euo pipefail

# Change to repo root (parent of test-scripts/)
cd "$(dirname "$0")/.."

source ./test-scripts/test-utils.sh

# load_env_file refuses to overwrite already-set variables — every test
# needs to run in a fresh bash subshell so prior assignments don't leak.
# Helper: run load_env_file against a fixture and print the value of one variable.
run_loader() {
  local fixture="$1"
  local var_name="$2"
  bash -c "
    set -e
    source ./upload-utils.sh
    load_env_file '$fixture' >/dev/null 2>&1
    printf '%s' \"\${$var_name}\"
  "
}

# Helper: assert exported value matches expected exactly.
assert_value() {
  local label="$1"
  local fixture="$2"
  local var_name="$3"
  local expected="$4"

  print_test "$TESTS_TOTAL" "$label"
  local actual
  actual=$(run_loader "$fixture" "$var_name")
  if [[ "$actual" == "$expected" ]]; then
    print_pass "$label (got [$actual])"
  else
    print_fail "$label — expected [$expected], got [$actual]"
  fi
}

# ============================================================
# Fixture construction
# ============================================================

FIXTURE=$(mktemp)
trap 'rm -f "$FIXTURE"' EXIT

cat > "$FIXTURE" <<'EOF'
# A comment line at top
BARE=abc123
DOUBLE_QUOTED="def456"
SINGLE_QUOTED='ghi789'
EMPTY_BARE=
EMPTY_DOUBLE=""
EMPTY_SINGLE=''
EMBEDDED_EQUALS="key=val=more"
EMBEDDED_SPACES="hello world"
LEADING_TRAILING_SPACES="  spaces  "
DOUBLE_INNER_SINGLE="it's quoted"
SINGLE_INNER_DOUBLE='he said "hi"'
   INDENTED_KEY=indented_value
# Comment in the middle
ASYMMETRIC_DOUBLE_LEFT="oops
ASYMMETRIC_DOUBLE_RIGHT=oops"
ASYMMETRIC_MIXED="foo'
NUMERIC_QUOTED="42"
EOF

# ============================================================
# Tests
# ============================================================

print_section "load_env_file value parsing (SDK-500 quote-stripping)"

# Happy-path: bare, no quotes
assert_value "bare value (no quotes)"             "$FIXTURE" BARE                       "abc123"

# Quote stripping
assert_value "double-quoted value"                "$FIXTURE" DOUBLE_QUOTED              "def456"
assert_value "single-quoted value"                "$FIXTURE" SINGLE_QUOTED              "ghi789"
assert_value "double-quoted numeric"              "$FIXTURE" NUMERIC_QUOTED             "42"

# Empty value edge cases
assert_value "bare empty"                         "$FIXTURE" EMPTY_BARE                 ""
assert_value "double-quoted empty"                "$FIXTURE" EMPTY_DOUBLE               ""
assert_value "single-quoted empty"                "$FIXTURE" EMPTY_SINGLE               ""

# Embedded characters preserved inside quotes
assert_value "embedded equals signs"              "$FIXTURE" EMBEDDED_EQUALS            "key=val=more"
assert_value "embedded spaces"                    "$FIXTURE" EMBEDDED_SPACES            "hello world"
assert_value "preserved inner spaces (no trim)"   "$FIXTURE" LEADING_TRAILING_SPACES    "  spaces  "

# Mixed inner quotes preserved
assert_value "double-quoted with inner single"    "$FIXTURE" DOUBLE_INNER_SINGLE        "it's quoted"
assert_value "single-quoted with inner double"    "$FIXTURE" SINGLE_INNER_DOUBLE        'he said "hi"'

# Indented key still parses (existing trim behavior)
assert_value "leading-whitespace key"             "$FIXTURE" INDENTED_KEY               "indented_value"

# Asymmetric / malformed quotes — do NOT strip; preserve literal
assert_value "left-only double quote (preserved)" "$FIXTURE" ASYMMETRIC_DOUBLE_LEFT     '"oops'
assert_value "right-only double quote (preserved)" "$FIXTURE" ASYMMETRIC_DOUBLE_RIGHT   'oops"'
assert_value "asymmetric mixed quotes (preserved)" "$FIXTURE" ASYMMETRIC_MIXED          "\"foo'"

# ============================================================
# Summary
# ============================================================

print_section "Results"
echo "Tests passed: $TESTS_PASSED / $TESTS_TOTAL"
if [[ "$TESTS_FAILED" -gt 0 ]]; then
  echo "Tests failed: $TESTS_FAILED"
  exit 1
fi
