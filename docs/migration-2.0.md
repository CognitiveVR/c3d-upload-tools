# Migrating to 2.0

Version 2.0 replaces the bash scripts and the PowerShell module with the `cognitive3d-cli-upload` binary. The 1.x tools keep working in 2.0 and print a deprecation banner; they are removed in 3.0.

## Invocation mapping

| 1.x invocation | 2.0 command |
| --- | --- |
| `./upload-scene.sh --scene_dir D --scene_name N` | `cognitive3d-cli-upload scene upload --scene-dir D --scene-name N` |
| `./upload-scene.sh --scene_dir D --scene_id ID` | `cognitive3d-cli-upload scene upload --scene-dir D --scene-id ID` |
| `./upload-object.sh --object_dir D --object_filename F --scene_id ID` | `cognitive3d-cli-upload object upload --object-dir D --object-filename F --scene-id ID` |
| `./upload-object.sh ... --object_id OID` | `cognitive3d-cli-upload object upload ... --object-id OID` |
| `./upload-object-manifest.sh --scene_id ID` | `cognitive3d-cli-upload object manifest upload --scene-id ID` |
| `./list-objects.sh --scene_id ID` | `cognitive3d-cli-upload object list --scene-id ID` |
| `./list-objects.sh ... --verbose` | `cognitive3d-cli-upload object list --scene-id ID --verbose` |
| any script with `--env prod\|dev` | the same `--env prod\|dev` |
| any script with `--dry_run` | `--dry-run` (the underscore spelling also works) |
| `Upload-C3DScene -SceneDirectory D -SceneName N [-SceneId ID]` | `cognitive3d-cli-upload scene upload --scene-dir D --scene-name N [--scene-id ID]` |
| `Upload-C3DObject -ObjectDirectory D -ObjectFilename F [-SceneId ID]` | `cognitive3d-cli-upload object upload --object-dir D --object-filename F [--scene-id ID]` |
| `Upload-C3DObjectManifest [-SceneId ID]` | `cognitive3d-cli-upload object manifest upload [--scene-id ID]` |
| `Get-C3DObjects [-SceneId ID]` | `cognitive3d-cli-upload object list [--scene-id ID]` |
| `Get-C3DObjects -OutputFile F` | `cognitive3d-cli-upload object list --write-list`, or redirect stdout with `--format json` |
| `Get-C3DObjects -FormatAsManifest` | `cognitive3d-cli-upload object list --write-manifest` |
| any module function with `-Environment E` or `-DryRun` | `--env E` or `--dry-run` |

Commands with no 1.x equivalent: `auth verify`, `scene list`, `scene show`, `scene screenshot`, `ego upload`, `ego list`, `ego delete`, `version` and `completion`.

The API key and scene id come from the same places as before (`C3D_DEVELOPER_API_KEY`, `C3D_SCENE_ID`, a `.env` file in the working directory), plus `~/.cognitive3d/upload.yaml`. Precedence is flag, environment variable, `.env`, then the config file.

Legacy flag spellings such as `--scene_id`, `--scene_dir` and `--dry_run` are accepted as aliases of the dashed flags, because the root command normalizes underscores to dashes.

## Behavior changes

- Logs move to stderr. stdout carries only the result, so output can be piped or parsed.
- `object list` no longer writes files unless asked, with `--write-manifest` (`<scene id>_object_manifest.json`) or `--write-list` (`<scene id>_object_list.json`). The manifest it writes keeps each object's scale, position and rotation.
- `object upload` no longer auto-uploads the manifest (the PowerShell behavior); run `object manifest upload`, or pass `--upload-manifest`. It keeps the transforms of an existing manifest entry for the same mesh.
- Object textures include jpg, jpeg and webp on every platform, in addition to png.
- The screenshot is sent inside the scene upload. `scene screenshot` remains for replacing it later.
- Dry run is offline and keyless: it makes no network calls and needs no API key.
- `sdkVersion` in `settings.json` becomes `cli-go-v<version>`.
- A model that references a texture the CLI would not upload is rejected instead of uploaded broken.
- Scene `settings.json` is written into the scene directory only after the upload succeeds.
- Size thresholds replace the old 100 MB per-file limit: a file of 500 MB or more and a total of 2 GiB or more produce a warning, and a total of 3 GiB or more is refused (exit 4).
- Symlinks in an upload directory are not uploaded.
- A new scene version is attributed by reading the latest version before the upload and waiting for the next one to appear. If it cannot be told apart from another concurrent upload, the CLI exits 1 and names no version.
- `ego upload` takes a directory of one model plus its images and an optional `ego.json`; `ego list` shows each slot's `files` count and `optimized` flag; `--wait` polls for up to 120 seconds and reports `optimized`, which small models may never become.

## Flags and environment variables

Persistent flags on every command: `--env`, `--gateway-url`, `--api-key`, `--format text|json`, `--timeout`, `--verbose`, `--quiet` and `--config`. Upload commands add `--dry-run`.

The binary reads `C3D_DEVELOPER_API_KEY`, `C3D_SCENE_ID`, `C3D_DEFAULT_ENVIRONMENT`, `C3D_ENV`, `C3D_UPLOAD_GATEWAY_URL` and `NO_COLOR`. `--scene-id` falls back to `C3D_SCENE_ID` wherever it appears.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success |
| 1 | General failure |
| 2 | Authentication failure |
| 3 | Not found or not accessible |
| 4 | Validation or usage error |
