# Migrating to 2.0

Version 2.0 replaces the bash scripts and the PowerShell module with the `cognitive3d-cli-upload` binary. The 1.x tools keep working until then.

This guide is an outline and is filled in as each command lands.

## Invocation mapping

| 1.x invocation | 2.0 command |
| --- | --- |
| `upload-scene.sh` | `cognitive3d-cli-upload scene upload` |
| `upload-object.sh` | `cognitive3d-cli-upload object upload` |
| `upload-object-manifest.sh` | `cognitive3d-cli-upload object manifest upload` |
| `list-objects.sh` | `cognitive3d-cli-upload object list` |
| `Upload-C3DScene` | `cognitive3d-cli-upload scene upload` |
| `Upload-C3DObject` | `cognitive3d-cli-upload object upload` |
| `Upload-C3DObjectManifest` | `cognitive3d-cli-upload object manifest upload` |
| `Get-C3DObjects` | `cognitive3d-cli-upload object list` |

Legacy flag spellings such as `--scene_dir` and `--dry_run` are accepted by the new binary.

## Behavior changes

- Logs move to stderr.
- `object list` no longer writes files unless asked.
- `object upload` no longer auto-uploads the manifest (the PowerShell behavior) and keeps the transforms of an existing manifest entry.
- Object textures include jpg, jpeg and webp on every platform.
- The screenshot is sent inside the scene upload.
- Dry run is offline and keyless.
- `sdkVersion` becomes `cli-go-v<version>`.
- A model that references a texture the CLI would not upload is rejected instead of uploaded broken.

## Flags and environment variables

Not yet written.

## Exit codes

Not yet written.
