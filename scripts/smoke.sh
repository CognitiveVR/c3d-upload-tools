#!/usr/bin/env bash
# Live smoke test of cognitive3d-cli-upload against the dev or prod gateway.
# Usage: C3D_UPLOAD_LIVE=1 C3D_DEVELOPER_API_KEY="$(op read 'op://<vault>/<item>/credential')" scripts/smoke.sh
#        C3D_UPLOAD_LIVE=prod C3D_DEVELOPER_API_KEY="$(op read 'op://<vault>/<item>/credential')" scripts/smoke.sh --env prod
#        scripts/smoke.sh --dry-run-only   (no key, no network)
# Take the key from a command substitution or an exported variable, never a
# literal on the command line.
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/smoke.sh [--env dev|prod] [--dry-run-only]

Runs every command family against the chosen gateway (default dev) and
leaves the scene it creates in place. Requires C3D_DEVELOPER_API_KEY for that
environment, plus C3D_UPLOAD_LIVE=1 for dev or C3D_UPLOAD_LIVE=prod for prod.
--dry-run-only runs only the upload steps with --dry-run and needs neither.
COGNITIVE3D_CLI selects the binary (default ./cognitive3d-cli-upload, built
with make build when absent).
USAGE
}

dry_run_only=0
env=dev
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run-only) dry_run_only=1 ;;
    --env)
      [ $# -ge 2 ] || { usage >&2; exit 2; }
      env=$2
      shift
      ;;
    --env=*) env=${1#--env=} ;;
    -h | --help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
  shift
done
case "$env" in
  dev) live_token=1 ;;
  prod) live_token=prod ;;
  *) echo "smoke: --env must be dev or prod, not $env" >&2; exit 2 ;;
esac

if [ "$dry_run_only" -eq 0 ]; then
  missing=""
  [ -n "${C3D_DEVELOPER_API_KEY:-}" ] || missing="$missing C3D_DEVELOPER_API_KEY"
  [ "${C3D_UPLOAD_LIVE:-}" = "$live_token" ] || missing="$missing C3D_UPLOAD_LIVE=$live_token"
  if [ -n "$missing" ]; then
    echo "smoke: refusing to run against $env, missing:$missing" >&2
    echo "smoke: use --dry-run-only to test the script without a gateway" >&2
    exit 2
  fi
fi

command -v jq >/dev/null 2>&1 || { echo "smoke: jq is required" >&2; exit 2; }

repo_root=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo_root"
CLI=${COGNITIVE3D_CLI:-./cognitive3d-cli-upload}
if [ ! -x "$CLI" ]; then
  if [ -n "${COGNITIVE3D_CLI:-}" ]; then
    echo "smoke: COGNITIVE3D_CLI=$CLI is not an executable file" >&2
    exit 2
  fi
  make build >&2
fi
case "$CLI" in
  /*) ;;
  *) CLI="$repo_root/${CLI#./}" ;;
esac
[ -x "$CLI" ] || { echo "smoke: binary $CLI not found" >&2; exit 2; }

assets="$repo_root"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"

if [ "$dry_run_only" -eq 1 ]; then
  steps=(
    "scene upload (new scene)"
    "scene upload (textured glTF scene as a new version)"
    "object upload cube (gltf + bin)"
    "object upload box (glb)"
    "ego upload head (gltf + bin)"
    "ego upload controller_left (glb)"
  )
  scene_id="00000000-0000-4000-8000-000000000000"
else
  steps=(
    "auth verify"
    "scene upload (new scene)"
    "scene upload (textured glTF scene as a new version)"
    "object upload cube (gltf + bin)"
    "object upload box (glb)"
    "edit manifest"
    "object manifest upload"
    "object list --write-manifest"
    "object upload cube again (no local manifest)"
    "ego upload head (gltf + bin)"
    "ego upload controller_left (glb)"
    "ego list"
    "ego delete head"
    "ego delete controller_left"
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

# run_cli <args...>: runs the CLI against the chosen environment with JSON output; OUT holds stdout.
run_cli() {
  : >"$errfile"
  if ! OUT=$("$CLI" "$@" --env "$env" --format json 2>"$errfile"); then
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
  local request_path
  request_path=$(field .path)
  done_step "(dry run: $request_path)"
}

# Scene uploads write settings.json into the scene directory, so both scenes
# are copies and testdata stays untouched.
new_scene="$work/new-scene"
mkdir "$new_scene"
cp "$assets"/testdata/scene-glb/* "$new_scene"/
rm -f "$new_scene/settings.json"
textured="$work/textured"
mkdir "$textured"
cp "$assets"/testdata/scene-gltf/* "$textured"/
rm -f "$textured/settings.json"

scene_name="cli-smoke $(date -u +%Y%m%dT%H%M%SZ)"

if [ "$dry_run_only" -eq 1 ]; then
  run_cli scene upload --scene-dir "$new_scene" --scene-name "$scene_name" --dry-run
  check_dry_run
  run_cli scene upload --scene-dir "$textured" --scene-id "$scene_id" --dry-run
  check_dry_run
  run_cli object upload --scene-id "$scene_id" --object-dir "$assets/testdata/object-gltf" --object-filename cube --dry-run
  check_dry_run
  run_cli object upload --scene-id "$scene_id" --object-dir "$assets/testdata/object-glb" --object-filename box --dry-run
  check_dry_run
  run_cli ego upload --scene-id "$scene_id" --ego-dir "$assets/testdata/ego-head" --slot head --dry-run
  check_dry_run
  run_cli ego upload --scene-id "$scene_id" --ego-dir "$assets/testdata/ego-controller-glb" --slot controller_left --dry-run
  check_dry_run
  echo "smoke: dry-run-only finished; nothing was sent"
  exit 0
fi

run_cli auth verify
done_step "project=$(printf '%s' "$OUT" | jq -r '.projectName // "?"') environment=$(printf '%s' "$OUT" | jq -r '.environment // "?"')"

run_cli scene upload --scene-dir "$new_scene" --scene-name "$scene_name"
scene_id=$(field .sceneId)
[ "$(printf '%s' "$OUT" | jq -r '.created')" = "true" ] || fail "expected a new scene, got: $OUT"
version=$(field .versionNumber)
done_step "scene=$scene_id version=$version"

run_cli scene upload --scene-dir "$textured" --scene-id "$scene_id"
version=$(field .versionNumber)
[ "$version" = "2" ] || fail "expected versionNumber 2, got $version"
done_step "scene=$scene_id version=$version"

run_cli object upload --scene-id "$scene_id" --object-dir "$assets/testdata/object-gltf" --object-filename cube
cube_id=$(field .objectId)
mesh=$(field .mesh)
version=$(field .versionNumber)
done_step "object=$cube_id mesh=$mesh version=$version"

run_cli object upload --scene-id "$scene_id" --object-dir "$assets/testdata/object-glb" --object-filename box
object_id=$(field .objectId)
mesh=$(field .mesh)
version=$(field .versionNumber)
done_step "object=$object_id mesh=$mesh version=$version"

manifest="${scene_id}_object_manifest.json"
cube_scaled='[.objects[] | select(.mesh == "cube" and .scaleCustom == [2,2,2])] | length == 1'
[ -f "$manifest" ] || fail "manifest $manifest was not written"
jq '(.objects[] | select(.mesh == "cube") | .scaleCustom) = [2,2,2]' "$manifest" >"$manifest.tmp" || fail "could not edit $manifest"
mv "$manifest.tmp" "$manifest"
jq -e "$cube_scaled" "$manifest" >/dev/null || fail "cube scaleCustom edit did not stick"
done_step "cube scaleCustom=[2,2,2]"

run_cli object manifest upload --scene-id "$scene_id"
objects=$(field .objectCount)
version=$(field .versionNumber)
done_step "objects=$objects version=$version"

rm -f "$manifest"
run_cli object list --scene-id "$scene_id" --write-manifest
listed=$(printf '%s' "$OUT" | jq 'length')
jq -e "$cube_scaled" "$manifest" >/dev/null || fail "round trip lost the cube scaleCustom [2,2,2]: $(jq -c '.objects[] | select(.mesh == "cube")' "$manifest" || true)"
done_step "round trip kept cube scaleCustom=[2,2,2] (listed $listed objects)"

# A re-upload from a checkout without the manifest must find the version's
# object for the mesh, send no entry, and copy its transforms into the local file.
rm -f "$manifest"
run_cli object upload --scene-id "$scene_id" --object-dir "$assets/testdata/object-gltf" --object-filename cube
object_id=$(field .objectId)
[ "$object_id" = "$cube_id" ] || fail "re-upload used id $object_id, expected the version's $cube_id"
[ "$(printf '%s' "$OUT" | jq -r '.manifestEntryExisting')" = "true" ] || fail "re-upload should report manifestEntryExisting true: $OUT"
jq -e "$cube_scaled" "$manifest" >/dev/null || fail "re-upload did not seed the local entry from the version: $(jq -c '.objects[] | select(.mesh == "cube")' "$manifest" || true)"
run_cli object list --scene-id "$scene_id"
relisted=$(printf '%s' "$OUT" | jq 'length')
[ "$relisted" = "$listed" ] || fail "re-upload changed the object count from $listed to $relisted"
done_step "object=$object_id reused, entry kept, $relisted objects"

run_cli ego upload --scene-id "$scene_id" --ego-dir "$assets/testdata/ego-head" --slot head --wait
slot=$(field .slot)
files=$(field .fileCount)
optimized=$(printf '%s' "$OUT" | jq -r '.optimized')
done_step "slot=$slot files=$files optimized=$optimized"

run_cli ego upload --scene-id "$scene_id" --ego-dir "$assets/testdata/ego-controller-glb" --slot controller_left --wait
slot=$(field .slot)
files=$(field .fileCount)
optimized=$(printf '%s' "$OUT" | jq -r '.optimized')
done_step "slot=$slot files=$files optimized=$optimized"

run_cli ego list --scene-id "$scene_id"
printf '%s' "$OUT" | jq -e '.egos | map(select(.slot == "head")) | length == 1' >/dev/null || fail "head slot missing from: $OUT"
printf '%s' "$OUT" | jq -e '.egos | map(select(.slot == "controller_left")) | length == 1' >/dev/null || fail "controller_left slot missing from: $OUT"
done_step "head and controller_left slots present"

run_cli ego delete --scene-id "$scene_id" --slot head --yes
[ "$(printf '%s' "$OUT" | jq -r '.deleted')" = "true" ] || fail "slot not deleted: $OUT"
done_step "deleted=true"

run_cli ego delete --scene-id "$scene_id" --slot controller_left --yes
[ "$(printf '%s' "$OUT" | jq -r '.deleted')" = "true" ] || fail "slot not deleted: $OUT"
done_step "deleted=true"

echo
echo "smoke: all ${#steps[@]} steps passed on $env"
echo "smoke: scene $scene_id (\"$scene_name\") on $env is left in place for inspection in the dashboard"
