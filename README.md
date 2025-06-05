# Cognitive3D Upload Tools

This repo contains a shell script (bash) for uploading scene files to the Cognitive3D platform.

## Requirements

* Bash (macOS / Linux / Windows Subsystem for Linux (WSL))
* `curl`
* `jq`

> Note: We have only tested these tools on macOS. Feedback welcome on your experience using them in Linux or Windows Subsystem for Linux (WSL). Open an Issue here or find us on our [Discord](https://discord.gg/x38sNUdDRH).

### Installation of required dependencies

Make sure the following tools are installed:

```bash
brew install jq curl         # macOS
sudo apt install jq curl     # Ubuntu/Debian
dnf install jq curl          # Fedora/RHEL
```

### Environment Variables

You must set your Cognitive3D Developer API key as an environment variable:

```bash
export C3D_DEVELOPER_API_KEY="your_api_key"
```

You can get your developer API key from the Cognitive3D web dashboard. Look for the "Manage developer key" option in the settings (gear icon) menu.

> Note: We strongly recommend you _do not_ store your developer API key in version control.

## Scene Upload Script

This script uploads a set of 3D scene files to the Cognitive3D platform using our API.

### Usage

```bash
./upload-scene.sh --scene_dir <scene_directory> [--env <prod|dev>] [--scene_id <scene_id_from_dashboard>]
```

#### Parameters

* `--scene_dir <scene_directory>` (required): Path to a folder containing:
  * `scene.bin`
  * `scene.gltf`
  * `screenshot.png`
  * `settings.json`
* `[--env <prod/dev>]` (optional): Either `prod` (default) or `dev`
* `[--scene_id <scene_id_from_dashboard>]` (optional): Scene ID to append to the API endpoint to upload a new version of an existing scene

### Example

For the first time you upload your scene you won't have a scene ID, so we don't pass that parameter. The first time you run this script, without a scene ID, it creates the scene and returns the new scene ID (output to the console at the end of the script.)

```bash
export C3D_DEVELOPER_API_KEY=<abc123xyz>
./upload-scene.sh --scene_dir ./TestScene --env prod
```

For subsequent (new) versions of the same scene, pass in your scene ID as the third parameter to the script. This will upload the scene assets again and the platform will auto-increment the scene version.

You can find the scene ID on the Cognitive3D dashboard on the Scenes page. Look for the "information" icon (letter 'i' in a circle) and hover over it.

```bash
export C3D_DEVELOPER_API_KEY=<abc123xyz>
./upload-scene.sh --scene_dir ./TestScene --env prod --scene_id my_scene_id
```

### Behavior

* Reads the SDK version from `sdk-version.txt` in the same directory as the script.
* Replaces the `sdkVersion` field inside `settings.json` file in the scene directory using `jq`.
* Uploads the four files to the correct API endpoint.
* Verifies API response and prints success or error.

### Help

To see usage information:

```bash
./upload-scene.sh --help
```

## Dynamic object uploader script

This Bash script uploads dynamic 3D object assets to the Cognitive3D platform, supporting GLTF + BIN files, optional textures, and thumbnail metadata. It supports both development and production environments.

Uploading a scene to the platform is required before you can upload any dynamic object models. The scene_id is a required parameter.

### Dynamic object uploader usage

```bash
./upload-object.sh \
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

#### Optional Parameters

* `--object_id`: If specified, uploads as a new version of an existing object.
* `--env`: Target environment (`prod` or `dev`). Defaults to `prod`.
* `--verbose`: Enables detailed logging.
* `--dry_run`: Prints the constructed `curl` command but skips execution.

### Dynamic object uploader environment variables

* `C3D_DEVELOPER_API_KEY`: Your Cognitive3D Developer API key (required).

### File Requirements

The following files must exist in the `--object_dir`:

* `<object_filename>.gltf`
* `<object_filename>.bin`
* (Optional, recommended) `cvr_object_thumbnail.png`, a representative screenshot of the object; used by the dashboard
* (Optional) Any additional `.png` textures used by the model

### Dynamic object uploader example

```bash
export C3D_DEVELOPER_API_KEY=<your-api-key>

./upload-object.sh \
  --scene_id <scene_id_goes_here> \
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

### Uploading the object manifest

After uploading the dynamic object asset files (mesh, textures) the `upload-object` script automatically uploads the dynamic object manifest file to display the objects on the Cognitive3D dashboard for your project and scene. You may want to modify the manifest and re-upload it with new values, such as starting position.

### Dynamic object manifest uploader usage

The dynamic object manifest for your scene and object is created automatically after successfully uploading the dynamic object assets. It will be in a file named `<scene_id>_object_manifest.json`.

```bash
./upload-object-manifest.sh \
  --scene_id <scene-uuid> \
  [--env dev|prod] \
  [--verbose] \
  [--dry_run]  # Use this to preview the `curl` command without executing it
```

#### Dynamic object manifest uploader required parameters

* `--scene_id`: The Scene ID UUID where the object will be uploaded.

#### Object manifest uploader optional parameters

* `--object_id`: If specified, uploads as a new version of an existing object.
* `--env`: Target environment (`prod` or `dev`). Defaults to `prod`.
* `--verbose`: Enables detailed logging.
* `--dry_run`: Prints the constructed `curl` command but skips execution.

### Dynamic object manifest uploader environment variables

* `C3D_DEVELOPER_API_KEY`: Your Cognitive3D Developer API key (required).

### Dynamic object manifest uploader file requirements

The following files must exist in the same directory where you are calling the script from:

* `<scene_id>_object_manifest.json`, which is automatically created after successfully uploading your dynamic object meshes

### Dynamic object manifest uploader example

```bash
export C3D_DEVELOPER_API_KEY=<your-api-key>

./upload-object-manifest.sh \
  --scene_id <scene_id_goes_here> \
  --env prod \
  --verbose
```

The reason we don't automatically upload the manifest after uploading the object assets is to allow you to modify the manifest JSON before uploading.

If you have any questions or problems using these scripts contact our customer support team using the Intercom button (public circle) on any Cognitive3D web page.
