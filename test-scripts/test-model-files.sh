#!/bin/bash

# test-model-files.sh
#
# Unit tests for upload-utils.sh::resolve_model_files, the rule both uploaders
# use to pick the model files for a scene or dynamic object: exactly one .glb,
# or the <base>.gltf + <base>.bin pair. Mixing the forms, or two .glb files, is
# rejected (cvr-se-upload returns a 400 for the same shapes).

set -euo pipefail

# Change to repo root (parent of test-scripts/)
cd "$(dirname "$0")/.."

source ./test-scripts/test-utils.sh

FIXTURE_ROOT=$(mktemp -d)
trap 'rm -rf "$FIXTURE_ROOT"' EXIT

# mk_dir <name> <file>... — creates a fixture directory holding the named files
mk_dir() {
  local dir="$FIXTURE_ROOT/$1"
  shift
  mkdir -p "$dir"
  local f
  for f in "$@"; do
    printf 'x' > "$dir/$f"
  done
  printf '%s' "$dir"
}

# run_resolver <dir> <base> <mode> — prints "<MODEL_FORMAT>|<MODEL_FORMS...>",
# or __FAILED__ when resolve_model_files returns non-zero. Runs in a fresh
# bash so globals never leak between cases.
run_resolver() {
  bash -c "
    source ./upload-utils.sh
    if ! resolve_model_files '$1' '$2' '$3' >/dev/null 2>&1; then
      printf '__FAILED__'
      exit 0
    fi
    printf '%s|%s' \"\$MODEL_FORMAT\" \"\${MODEL_FORMS[*]}\"
  "
}

assert_resolves() {
  local label="$1" dir="$2" base="$3" mode="$4" expected="$5"
  print_test "$TESTS_TOTAL" "$label"
  local actual
  actual=$(run_resolver "$dir" "$base" "$mode")
  if [[ "$actual" == "$expected" ]]; then
    print_pass "$label"
  else
    print_fail "$label — expected [$expected], got [$actual]"
  fi
}

print_section "MODEL FILE RESOLUTION TESTS"

# ---- scenes (mode any: a single .glb of any name, or scene.gltf + scene.bin)
D=$(mk_dir scene-pair scene.gltf scene.bin screenshot.png)
assert_resolves "scene: scene.gltf + scene.bin" "$D" scene any \
  "gltf|--form scene.bin=@$D/scene.bin --form scene.gltf=@$D/scene.gltf"

D=$(mk_dir scene-glb MyRoom.glb screenshot.png)
assert_resolves "scene: a single .glb of any name" "$D" scene any \
  "glb|--form MyRoom.glb=@$D/MyRoom.glb"

D=$(mk_dir scene-glb-upper Room.GLB)
assert_resolves "scene: .GLB extension matched case-insensitively" "$D" scene any \
  "glb|--form Room.GLB=@$D/Room.GLB"

D=$(mk_dir scene-two-glb one.glb two.glb)
assert_resolves "scene: two .glb files are rejected" "$D" scene any "__FAILED__"

D=$(mk_dir scene-mixed scene.glb scene.gltf scene.bin)
assert_resolves "scene: .glb beside scene.gltf + scene.bin is rejected" "$D" scene any "__FAILED__"

D=$(mk_dir scene-glb-bin scene.glb scene.bin)
assert_resolves "scene: .glb beside a stray scene.bin is rejected" "$D" scene any "__FAILED__"

D=$(mk_dir scene-half scene.gltf)
assert_resolves "scene: scene.gltf without scene.bin is rejected" "$D" scene any "__FAILED__"

D=$(mk_dir scene-none screenshot.png)
assert_resolves "scene: no model at all is rejected" "$D" scene any "__FAILED__"

# ---- objects (mode named: <base>.glb, or <base>.gltf + <base>.bin)
D=$(mk_dir obj-pair cube.gltf cube.bin cvr_object_thumbnail.png)
assert_resolves "object: cube.gltf + cube.bin" "$D" cube named \
  "gltf|--form cube.bin=@$D/cube.bin --form cube.gltf=@$D/cube.gltf"

D=$(mk_dir obj-glb cube.glb cvr_object_thumbnail.png)
assert_resolves "object: cube.glb" "$D" cube named "glb|--form cube.glb=@$D/cube.glb"

D=$(mk_dir obj-mixed cube.glb cube.gltf cube.bin)
assert_resolves "object: cube.glb beside cube.gltf + cube.bin is rejected" "$D" cube named "__FAILED__"

D=$(mk_dir obj-other-glb other.glb)
assert_resolves "object: a .glb with another base name is not the model" "$D" cube named "__FAILED__"

D=$(mk_dir obj-none cvr_object_thumbnail.png)
assert_resolves "object: no model at all is rejected" "$D" cube named "__FAILED__"

# ============================================================
# Summary
# ============================================================

print_section "Results"
echo "Tests passed: $TESTS_PASSED / $TESTS_TOTAL"
if [[ "$TESTS_FAILED" -gt 0 ]]; then
  echo "Tests failed: $TESTS_FAILED"
  exit 1
fi
