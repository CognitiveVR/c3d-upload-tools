# Cognitive3D Upload Tools

`cognitive3d-cli-upload` is a single command-line program for Windows, macOS and Linux that uploads 3D scenes, dynamic objects and egocentric (session replay) models to the Cognitive3D platform. It checks your files locally before it sends anything, works in scripts and CI, and can print machine-readable JSON.

## Installation

### macOS

```bash
brew tap cognitivevr/tap && brew install --cask cognitive3d-cli-upload
```

### Windows

1. Download `cognitive3d-cli-upload_<version>_Windows_x86_64.zip` (or `_arm64.zip`) and `cognitive3d-cli-upload_<version>_checksums.txt` from the [GitHub release](https://github.com/CognitiveVR/c3d-upload-tools/releases).
2. Compare the zip's sha256 with its line in the checksums file, in PowerShell with `Get-FileHash` or in a command prompt with `certutil`:

   ```powershell
   Get-FileHash .\cognitive3d-cli-upload_<version>_Windows_x86_64.zip -Algorithm SHA256
   ```

   ```bat
   certutil -hashfile cognitive3d-cli-upload_<version>_Windows_x86_64.zip SHA256
   ```

3. Extract the zip and add the folder holding `cognitive3d-cli-upload.exe` to your `PATH`.

The Windows binary is unsigned in 2.0.0, so the first launch shows a SmartScreen "unknown publisher" prompt. That is expected: choose "More info", then "Run anyway".

### Linux

Download `cognitive3d-cli-upload_<version>_Linux_x86_64.tar.gz` (or `_arm64.tar.gz`) and the checksums file from the release, then verify and extract:

```bash
sha256sum --ignore-missing -c cognitive3d-cli-upload_<version>_checksums.txt
sudo tar -xzf cognitive3d-cli-upload_<version>_Linux_x86_64.tar.gz -C /usr/local/bin cognitive3d-cli-upload
```

### Build from source

Requires Go 1.25 or newer.

```bash
make build
./cognitive3d-cli-upload version
```

## Quick start

Create a developer API key in the Cognitive3D dashboard under Settings, "Manage developer key", and export it:

```bash
export C3D_DEVELOPER_API_KEY=your_api_key
```

Check the key and see which project it belongs to:

```bash
cognitive3d-cli-upload auth verify
```

Upload a scene. The directory holds one `.glb` (or `scene.gltf` plus `scene.bin`), its textures and `screenshot.png`. A new scene needs a name; pass `--scene-id` to add a new version to an existing scene instead:

```bash
cognitive3d-cli-upload scene upload --scene-dir ./my-scene --scene-name "My Scene"
```

A scene id set in `C3D_SCENE_ID`, `.env` or the config file also counts, so `scene upload` then adds a new version to that scene and says so on stderr. Unset it to create a new scene.

Upload a dynamic object. The directory holds `<name>.glb` (or `<name>.gltf` plus `<name>.bin`), its textures and `cvr_object_thumbnail.png`. The object is merged into `<scene id>_object_manifest.json` in the current directory:

```bash
cognitive3d-cli-upload object upload --scene-id <scene-id> --object-dir ./my-object --object-filename cube
```

Pass `--manifest <path>` to merge into a different manifest file, and `--upload-manifest` to upload the manifest right after the object. Otherwise upload the manifest as a separate step so the objects appear in the dashboard:

```bash
cognitive3d-cli-upload object manifest upload --scene-id <scene-id>
```

List the objects of the latest version. `--write-manifest` saves the list as `<scene id>_object_manifest.json`, replacing that file, and `--write-list` saves the raw list as `<scene id>_object_list.json`:

```bash
cognitive3d-cli-upload object list --scene-id <scene-id>
```

Upload an egocentric model into the `head` slot and wait for it to be optimized:

```bash
cognitive3d-cli-upload ego upload --scene-id <scene-id> --slot head --ego-dir ./my-head --wait
```

Every upload command accepts `--dry-run`, which validates the files and prints the request plan without contacting the gateway:

```bash
cognitive3d-cli-upload scene upload --scene-dir ./my-scene --scene-name "My Scene" --dry-run
```

Replace the screenshot of an existing version with `--file` (the latest version unless `--version` is given):

```bash
cognitive3d-cli-upload scene screenshot --scene-id <scene-id> --file screenshot.png
```

Empty an egocentric slot, reverting session replay to the bundled model; `--yes` skips the confirmation prompt:

```bash
cognitive3d-cli-upload ego delete --scene-id <scene-id> --slot head --yes
```

Other commands: `scene list`, `scene show`, `ego list`, `version` and `completion`. Run any command with `--help` for its flags.

## What goes in each directory

| Command | Required files | Optional files |
| --- | --- | --- |
| `scene upload` | One `.glb` of any name, or `scene.gltf` plus `scene.bin`; `screenshot.png` | Textures (`.png`, `.jpg`, `.jpeg`, `.webp`), `settings.json` |
| `object upload` | `<name>.glb`, or `<name>.gltf` plus `<name>.bin`; `cvr_object_thumbnail.png` | Textures (`.png`, `.jpg`, `.jpeg`, `.webp`) |
| `ego upload` | The model and its `.bin` files | `.png`, `.jpg` and `.webp` images, one `ego.json` |

`settings.json` is built in memory from the existing file and the scene name, sent with the upload, and written into the scene directory only after the upload succeeds. Its `sdkVersion` is `cli-go-v<version>`.

The CLI warns about any file of 500 MB or more and about an upload of 2 GiB or more, and refuses an upload over 3 GiB, the gateway's limit per request. A large upload can legitimately take a long time because the gateway converts every file before it answers; there is no client-side response timeout unless you set `--timeout`.

## Configuration

Every setting is resolved in this order, first match wins:

1. The command-line flag.
2. The environment variable.
3. A `.env` file in the working directory.
4. `~/.cognitive3d/upload.yaml` (or the file named by `--config`).

| Setting | Flag | Variable |
| --- | --- | --- |
| Developer API key | `--api-key` | `C3D_DEVELOPER_API_KEY` |
| Scene id | `--scene-id` | `C3D_SCENE_ID` |
| Environment, `prod` or `dev` | `--env` | `C3D_DEFAULT_ENVIRONMENT`, then `C3D_ENV` |
| Gateway base URL override, including the API version path, for example `https://data.c3ddev.com/v0` | `--gateway-url` | `C3D_UPLOAD_GATEWAY_URL` |
| Color off | none | `NO_COLOR` |

The environment defaults to `prod`. The config file holds `default_environment`, plus `developer_api_key` and `scene_id` under `production:` and `development:`:

```yaml
default_environment: prod
production:
  developer_api_key: your_prod_key
  scene_id: your_scene_id
development:
  developer_api_key: your_dev_key
```

The key is required only when a command is about to talk to the gateway, so `--dry-run` and local validation work without one. Keep the key out of version control; `.env` is git-ignored here.

Flags also accept the old underscore spellings, so `--scene_dir` and `--dry_run` still work in existing scripts.

## Output and exit codes

stdout carries only the result of the command. Logs, warnings and errors go to stderr as `[YYYY-MM-DD HH:MM:SS] [LEVEL] message`, so output can be piped safely. `--verbose` adds debug lines and `--quiet` hides informational ones.

With `--format json`, a command prints exactly one JSON object or array on stdout, and stderr carries one JSON record per line. An error record looks like `{"level":"error","message":"...","error":"<code>","hint":"...","status_code":401}`; `hint` holds the steps to fix it or the command to check with, and is left out when there is none.

| Code | Meaning |
| --- | --- |
| 0 | Success. |
| 1 | General failure: server error, network error, interrupted upload, ambiguous scene version. |
| 2 | Authentication failure: missing, invalid or expired key, or 403. |
| 3 | Not found: the scene or resource does not exist or is outside the key's project. |
| 4 | Validation or usage error: bad flags, a missing file, a model that fails the checks, an upload over 3 GiB. |

`--timeout <duration>` (for example `--timeout 2h`) bounds a whole command. Ctrl-C cancels it cleanly.

## Scene versions

Each `scene upload` to an existing scene creates a new version. The CLI reads the latest version number before the upload and waits for the next one to appear, and prints that number as the new version. If more than one new version appears while the upload runs, it cannot tell which one is its own, so it lists the candidate numbers, prints no version number, and exits 1; check with `scene show`. If the new version is not visible within 30 seconds, the command also exits 1 and prints the `scene show` command to check; `settings.json` has already been written by then.

Objects, manifests and egos belong to a version. After a new version, upload them again; the CLI warns about this each time. Commands that take `--version` use the latest version when it is omitted.

## Egocentric models

`ego upload` fills one slot (`head`, `controller_left`, `controller_right` or a custom key) from a directory of a model, its `.png`, `.jpg` and `.webp` images and an optional `ego.json`. `ego list` shows each slot with its file count and whether it is optimized, and `ego delete` empties a slot so session replay falls back to the bundled model.

`--wait` polls every 5 seconds for up to 120 seconds and reports whether the gateway marked the model optimized. Running out of time is not an error: small models may never be marked optimized, so a `false` answer is normal for them. `--timeout` bounds the whole command, including the wait, so a long `--wait` needs a longer `--timeout` or none.

## Troubleshooting

Expired key (exit 2): the CLI prints the rotation steps. Open the dashboard, go to Settings, "Manage developer key", generate a new key and update `C3D_DEVELOPER_API_KEY`. Run `cognitive3d-cli-upload auth verify` to confirm it.

"The upload may still have completed": the connection dropped, or the command timed out or was interrupted, after the files were sent. The gateway may have finished the job. Check with `cognitive3d-cli-upload scene show --scene-id <scene-id>` (or `object list`) before retrying, because a retry creates another version.

Scene not found (exit 3): the gateway answers an unknown scene and a scene from another project the same way. Both the plain id and the `p<projectId>-<uuid>` form are accepted. Run `cognitive3d-cli-upload scene list` to see the scenes this key can reach, and `auth verify --scene-id <id>` to test one id. Check that the key belongs to the right project and `--env`.

Model check failures (exit 4): textures must be `.png`, `.jpg`, `.jpeg` or `.webp`. Other files in the directory are skipped with a warning naming each one, and the upload is refused if the model references a skipped file or a file that is not in the directory. A `.glb` must be valid glTF 2.0 with no external references, and a scene directory holds either one `.glb` or `scene.gltf` plus `scene.bin`, never both. Symlinks are not uploaded.

## Legacy tools

The bash scripts and the PowerShell module still ship in this release but are deprecated: they print a banner and are removed in 3.0. See the [migration guide](docs/migration-2.0.md) for the command and flag mapping.
