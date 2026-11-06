#!/usr/bin/env bash
# Live smoke test of cognitive3d-cli-upload against the dev gateway.
# Usage: C3D_UPLOAD_LIVE=1 C3D_DEVELOPER_API_KEY=... scripts/smoke-dev.sh
#        scripts/smoke-dev.sh --dry-run-only   (no key, no network)
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/smoke-dev.sh [--dry-run-only]

Runs every command family against the dev gateway and leaves the scene it
creates in place. Requires C3D_DEVELOPER_API_KEY and C3D_UPLOAD_LIVE=1.
--dry-run-only runs only the upload steps with --dry-run and needs neither.
COGNITIVE3D_CLI selects the binary (default ./cognitive3d-cli-upload, built
with make build when absent).
USAGE
}

dry_run_only=0
case "${1:-}" in
  "") ;;
  --dry-run-only) dry_run_only=1 ;;
  -h | --help) usage; exit 0 ;;
  *) usage >&2; exit 2 ;;
esac

if [ "$dry_run_only" -eq 0 ]; then
  missing=""
  [ -n "${C3D_DEVELOPER_API_KEY:-}" ] || missing="$missing C3D_DEVELOPER_API_KEY"
  [ "${C3D_UPLOAD_LIVE:-}" = "1" ] || missing="$missing C3D_UPLOAD_LIVE=1"
  if [ -n "$missing" ]; then
    echo "smoke-dev: refusing to run, missing:$missing" >&2
    echo "smoke-dev: use --dry-run-only to test the script without a gateway" >&2
    exit 2
  fi
fi

command -v jq >/dev/null 2>&1 || { echo "smoke-dev: jq is required" >&2; exit 2; }

repo_root=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo_root"
CLI=${COGNITIVE3D_CLI:-./cognitive3d-cli-upload}
if [ ! -x "$CLI" ]; then
  if [ -n "${COGNITIVE3D_CLI:-}" ]; then
    echo "smoke-dev: COGNITIVE3D_CLI=$CLI is not an executable file" >&2
    exit 2
  fi
  make build >&2
fi
case "$CLI" in
  /*) ;;
  *) CLI="$repo_root/${CLI#./}" ;;
esac
[ -x "$CLI" ] || { echo "smoke-dev: binary $CLI not found" >&2; exit 2; }

assets="$repo_root"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"

if [ "$dry_run_only" -eq 1 ]; then
  steps=(
    "scene upload (new scene)"
    "scene upload (SciFiHelmet as a new version)"
    "object upload cube"
    "object upload lantern"
    "ego upload head"
  )
  scene_id="00000000-0000-4000-8000-000000000000"
else
  steps=(
    "auth verify"
    "scene upload (new scene)"
    "scene upload (SciFiHelmet as a new version)"
    "object upload cube"
    "object upload lantern"
    "edit manifest"
    "object manifest upload"
    "object list --write-manifest"
    "ego upload head"
    "ego list"
    "ego delete head"
  )
  scene_id=""
fi
step_index=0
OUT=""
errfile="$work/stderr.txt"
: >"$errfile"

# fail [detail]: reports the current step, the CLI's stderr and the steps never reached.
fail() {
  echo "FAILED: ${steps[$step_index]}" >&2
  if [ -s "$errfile" ]; then cat "$errfile" >&2; fi
  if [ -n "${1:-}" ]; then echo "$1" >&2; fi
  echo "pending steps:" >&2
  local i
  for ((i = step_index + 1; i < ${#steps[@]}; i++)); do
    echo "  - ${steps[$i]}" >&2
  done
  exit 1
}

# run_cli <args...>: runs the CLI against dev with JSON output; OUT holds stdout.
run_cli() {
  : >"$errfile"
  if ! OUT=$("$CLI" "$@" --env dev --format json 2>"$errfile"); then
    fail "exit code was not 0"
  fi
}

# field <jq filter>: prints the value, failing the step when it is null or empty.
field() {
  local value
  value=$(printf '%s' "$OUT" | jq -r "$1 // empty") || fail "output is not JSON: $OUT"
  [ -n "$value" ] || fail "missing $1 in output: $OUT"
  printf '%s' "$value"
}

done_step() {
  echo "ok: ${steps[$step_index]} $*"
  step_index=$((step_index + 1))
}

# check_dry_run: a dry run prints the request it would send, with a path.
check_dry_run() {
  done_step "(dry run: $(field .path))"
}

# SciFiHelmet as a scene directory: scene.gltf + scene.bin, no settings.json.
helmet="$work/helmet"
mkdir "$helmet"
cp "$assets"/SciFiHelmet/* "$helmet"/
rm -f "$helmet/settings.json" "$helmet/README.md"
if [ -f "$helmet/SciFiHelmet.gltf" ]; then
  mv -f "$helmet/SciFiHelmet.gltf" "$helmet/scene.gltf"
  mv -f "$helmet/SciFiHelmet.bin" "$helmet/scene.bin"
fi
jq '.buffers[0].uri = "scene.bin"' "$helmet/scene.gltf" >"$helmet/scene.gltf.tmp"
mv "$helmet/scene.gltf.tmp" "$helmet/scene.gltf"

scene_name="cli-smoke $(date -u +%Y%m%dT%H%M%SZ)"

if [ "$dry_run_only" -eq 1 ]; then
  run_cli scene upload --scene-dir "$assets/scene-test-glb" --scene-name "$scene_name" --dry-run
  check_dry_run
  run_cli scene upload --scene-dir "$helmet" --scene-id "$scene_id" --dry-run
  check_dry_run
  run_cli object upload --scene-id "$scene_id" --object-dir "$assets/object-test" --object-filename cube --dry-run
  check_dry_run
  run_cli object upload --scene-id "$scene_id" --object-dir "$assets/object-test-glb" --object-filename lantern --dry-run
  check_dry_run
  run_cli ego upload --scene-id "$scene_id" --ego-dir "$assets/testdata/ego-head" --slot head --dry-run
  check_dry_run
  echo "smoke-dev: dry-run-only finished; nothing was sent"
  exit 0
fi

run_cli auth verify
done_step "project=$(printf '%s' "$OUT" | jq -r '.projectName // "?"') environment=$(printf '%s' "$OUT" | jq -r '.environment // "?"')"

run_cli scene upload --scene-dir "$assets/scene-test-glb" --scene-name "$scene_name"
scene_id=$(field .sceneId)
[ "$(printf '%s' "$OUT" | jq -r '.created')" = "true" ] || fail "expected a new scene, got: $OUT"
done_step "scene=$scene_id version=$(field .versionNumber)"

run_cli scene upload --scene-dir "$helmet" --scene-id "$scene_id"
version=$(field .versionNumber)
[ "$version" = "2" ] || fail "expected versionNumber 2, got $version"
done_step "scene=$scene_id version=$version"

run_cli object upload --scene-id "$scene_id" --object-dir "$assets/object-test" --object-filename cube
done_step "object=$(field .objectId) mesh=$(field .mesh) version=$(field .versionNumber)"

run_cli object upload --scene-id "$scene_id" --object-dir "$assets/object-test-glb" --object-filename lantern
done_step "object=$(field .objectId) mesh=$(field .mesh) version=$(field .versionNumber)"

manifest="${scene_id}_object_manifest.json"
cube_scaled='[.objects[] | select(.mesh == "cube" and .scaleCustom == [2,2,2])] | length == 1'
[ -f "$manifest" ] || fail "manifest $manifest was not written"
jq '(.objects[] | select(.mesh == "cube") | .scaleCustom) = [2,2,2]' "$manifest" >"$manifest.tmp" || fail "could not edit $manifest"
mv "$manifest.tmp" "$manifest"
jq -e "$cube_scaled" "$manifest" >/dev/null || fail "cube scaleCustom edit did not stick"
done_step "cube scaleCustom=[2,2,2]"

run_cli object manifest upload --scene-id "$scene_id"
done_step "objects=$(field .objectCount) version=$(field .versionNumber)"

run_cli object list --scene-id "$scene_id" --write-manifest
listed=$(printf '%s' "$OUT" | jq 'length')
jq -e "$cube_scaled" "$manifest" >/dev/null || fail "round trip lost the cube scaleCustom [2,2,2]: $(jq -c '.objects[] | select(.mesh == "cube")' "$manifest" || true)"
done_step "round trip kept cube scaleCustom=[2,2,2] (listed $listed objects)"

run_cli ego upload --scene-id "$scene_id" --ego-dir "$assets/testdata/ego-head" --slot head --wait
done_step "slot=$(field .slot) files=$(field .fileCount) optimized=$(printf '%s' "$OUT" | jq -r '.optimized')"

run_cli ego list --scene-id "$scene_id"
printf '%s' "$OUT" | jq -e '.egos | map(select(.slot == "head")) | length == 1' >/dev/null || fail "head slot missing from: $OUT"
done_step "head slot present"

run_cli ego delete --scene-id "$scene_id" --slot head --yes
[ "$(printf '%s' "$OUT" | jq -r '.deleted')" = "true" ] || fail "slot not deleted: $OUT"
done_step "deleted=true"

echo
echo "smoke-dev: all ${#steps[@]} steps passed"
echo "smoke-dev: scene $scene_id (\"$scene_name\") on dev is left in place for inspection in the dashboard"
