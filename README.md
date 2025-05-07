# Cognitive3D Upload Tools

This repo contains a shell script (bash) for uploading scene files to the Cognitive3D platform.

## Scene Upload Script

This script uploads a set of 3D scene files to the Cognitive3D API.

### Requirements

* Bash (macOS/Linux)
* `curl`
* `jq`

### Installation

Make sure the following tools are installed:

```bash
brew install jq curl         # macOS
sudo apt install jq curl     # Ubuntu/Debian
dnf install jq curl          # Fedora/RHEL
```

### Usage

```bash
./scene-upload.sh <scene_directory> [environment] [scene_id]
```

#### Parameters

* `<scene_directory>` (required): Path to a folder containing:

  * `scene.bin`
  * `scene.gltf`
  * `screenshot.png`
  * `settings.json`

* `[environment]` (optional): Either `prod` (default) or `dev`

* `[scene_id]` (optional): Scene ID to append to the API endpoint to upload a new version of an existing scene

#### Environment Variables

You must set your Cognitive3D developer API key as an environment variable:

```bash
export C3D_DEVELOPER_API_KEY="your_api_key"
```

You can get your developer API key from the Cognitive3D web dashboard. Look for the "Manage developer key" option in the settings (gear icon) menu.

> Note: We strongly recommend you _do not_ store your developer API key in version control.

### Example

For the first time you upload your scene you won't have a scene ID, so we don't pass that parameter. The first time you run this script, without a scene ID, it creates the scene and returns the new scene ID (output to the console at the end of the script.)

```bash
export C3D_DEVELOPER_API_KEY="abc123xyz"
./scene-upload.sh ./TestScene prod
```

For subsequent (new) versions of the same scene, pass in your scene ID as the third parameter to the script. This will upload the scene assets again and the platform will auto-increment the scene version.

You can find the scene ID on the Cognitive3D dashboard on the Scenes page. Look for the "information" icon (letter 'i' in a circle) and hover over it.

```bash
export C3D_DEVELOPER_API_KEY="abc123xyz"
./scene-upload.sh ./TestScene prod my-scene-id
```

### Behavior

* Reads the SDK version from `sdk-version.txt` in the same directory as the script.
* Replaces the `sdkVersion` field inside `settings.json` using `jq`.
* Uploads the four files to the correct API endpoint.
* Verifies API response and prints success or error.

### Help

To see usage information:

```bash
./scene-upload.sh --help
```

## Dynamic object uploader script

This Bash script uploads dynamic 3D object assets to the Cognitive3D platform, supporting GLTF + BIN files, optional textures, and thumbnail metadata. It supports both development and production environments.

### Dynamic object uploader requirements

* Bash 4+
* `curl`
* A Cognitive3D Developer API key (set as an environment variable)

### Dynamic object uploader usage

```bash
./upload-dynamic-object.sh \
  --scene_id <scene-uuid> \
  --object_filename <object-name> \
  --object_dir <path-to-object-directory> \
  [--object_id <existing-object-id>] \
  [--env dev|prod] \
  [--verbose] \
  [--dry_run]
```

#### Required Parameters

* `--scene_id`: The Scene ID UUID where the object will be uploaded.
* `--object_filename`: The base filename (no extension) of the object, used to find `.gltf` and `.bin` files.
* `--object_dir`: The directory containing the object files.
* `--object_id`: If specified, uploads as a new version of an existing object.

#### Optional Parameters

* `--env`: Target environment (`prod` or `dev`). Defaults to `prod`.
* `--verbose`: Enables detailed logging.
* `--dry_run`: Prints the constructed `curl` command but skips execution.

### Dynamic object uploader environment variables

* `C3D_DEVELOPER_API_KEY`: Your Cognitive3D Developer API key (required).

### File Requirements

The following files must exist in the `--object_dir`:

* `<object_filename>.gltf`
* `<object_filename>.bin`
* (Optional, recommended) `cvr_object_thumbnail.png`
* (Optional) Any additional `.png` textures used by the model

### Dynamic object uploader example

```bash
export C3D_DEVELOPER_API_KEY="your-api-key"

./object-upload.sh \
  --scene_id scene_id_goes_here \
  --object_filename cube \
  --object_dir object-test \
  --env prod \
  --object_id cube
  --verbose
```

### Exit Codes

* `0`: Success
* `1`: Missing argument or setup error
* Non-zero: Returned by `curl` if upload fails

### Logging

* `--verbose` prints detailed steps
* `--dry_run` shows the `curl` command without executing it
* Colored output highlights info, warnings, errors, and debug details

### Examples

```bash
./object-upload.sh \
  --scene_id fea64809-2a44-4b0c-acc1-a66f371521a8 \
  --env dev \
  --object_dir lantern-test \
  --object_filename Lantern \
  --object_id Lantern
```
