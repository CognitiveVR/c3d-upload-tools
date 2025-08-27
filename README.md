# Cognitive3D Upload Tools

A collection of bash scripts for uploading 3D scenes and dynamic objects to the Cognitive3D platform. These tools provide a complete workflow for managing scenes, objects, and their associated metadata through the Cognitive3D API.

## Overview

This repository contains the following scripts:

- **`upload-scene.sh`** - Upload scene files (GLTF, textures, settings)
- **`upload-object.sh`** - Upload dynamic 3D object assets
- **`upload-object-manifest.sh`** - Upload object manifest for dashboard display
- **`list-objects.sh`** - List objects associated with a scene
- **`test-all.sh`** - Run comprehensive tests for all upload functionality

## Quick Start

### 1. Install Dependencies

```bash
brew install jq curl         # macOS
sudo apt install jq curl     # Ubuntu/Debian
dnf install jq curl          # Fedora/RHEL
```

### 2. Set API Key

**Option A: Environment Variable (Traditional)**

```bash
export C3D_DEVELOPER_API_KEY="your_api_key"
```

**Option B: .env File (Recommended)**

```bash
# Copy the example file
cp .env.example .env

# Edit .env with your Developer API key
C3D_DEVELOPER_API_KEY=your_developer_api_key_here
```

Get your API key from the Cognitive3D dashboard: Settings (gear icon) → "Manage developer key"

> **Security**: The `.env` file is automatically ignored by git to keep your API key secure. Never commit API keys to version control.

### 3. Upload Workflow

**Traditional approach (with --scene_id parameters):**

```bash
# Step 1: Upload scene (first time - creates new scene)
./upload-scene.sh --scene_dir scene-test --env prod

# Note the scene_id returned from the above command

# Step 2: Upload objects
./upload-object.sh --scene_id YOUR_SCENE_ID --object_filename cube --object_dir object-test --env prod

# Step 3: Upload object manifest (displays objects in dashboard)
./upload-object-manifest.sh --scene_id YOUR_SCENE_ID --env prod
```

**Streamlined approach (with .env file):**

```bash
# Step 1: Set up .env file
cp .env.example .env
# Edit .env with your C3D_DEVELOPER_API_KEY

# Step 2: Upload scene to get scene ID
./upload-scene.sh --scene_dir scene-test --env prod
# Note the scene ID from response: e.g., 511b6159-cf6b-4de8-8cc4-3fe68132cafa

# Step 3: Add scene ID to .env file
echo "C3D_SCENE_ID=<your_scene_id_here>" >> .env

# Step 4: Upload objects (no --scene_id needed)
./upload-object.sh --object_filename cube --object_dir object-test --env prod

# Step 5: Upload object manifest (no --scene_id needed)
./upload-object-manifest.sh --env prod

# Step 6: List objects to verify (no --scene_id needed)
./list-objects.sh --env prod
```

**Alternative: Environment variable approach:**

```bash
# Upload scene and set scene ID manually
./upload-scene.sh --scene_dir scene-test --env prod
export C3D_SCENE_ID=YOUR_SCENE_ID_FROM_RESPONSE

# Use environment variable for subsequent operations
./upload-object.sh --object_filename cube --object_dir object-test --env prod
./upload-object-manifest.sh --env prod
./list-objects.sh --env prod
```

## Requirements

### Bash Scripts (Cross-platform)

- Bash (macOS / Linux / Windows Subsystem for Linux (WSL))
- `curl`
- `jq`

### PowerShell Module (Windows Native)

- PowerShell 5.1+ (Windows) or PowerShell Core 7.x+ (cross-platform)
- No external dependencies required (uses System.Net.WebClient for HTTP operations)

> **Note**: Bash scripts tested on macOS. PowerShell module tested on macOS PowerShell Core and provides full Windows compatibility using .NET Framework classes. Both implementations support `.env` file configuration and C3D_SCENE_ID environment variable fallback.

## Environment Variables

**Required**:

- `C3D_DEVELOPER_API_KEY` - Your Cognitive3D Developer API key

### Configuration Options

**1. .env File (Recommended)**

```bash
# Copy example and edit with your values
cp .env.example .env
```

**2. Environment Variables**

```bash
export C3D_DEVELOPER_API_KEY="your_api_key"
```

**3. PowerShell (Windows/Cross-platform)**

```powershell
# Import module (auto-loads .env file)
Import-Module ./C3DUploadTools

# Or set manually
$env:C3D_DEVELOPER_API_KEY = "your_api_key"

# For streamlined workflows, also set:
$env:C3D_SCENE_ID = "your_scene_id_here"

# PowerShell commands support C3D_SCENE_ID environment variable fallback
Upload-C3DObject -ObjectFilename cube -ObjectDirectory object-test -Environment prod
Upload-C3DObjectManifest -Environment prod
Get-C3DObjects -Environment prod
```

> **Security Note**: The `.env` file is automatically excluded from git. Never commit API keys to version control.

## Scene Upload Script

Uploads a set of 3D scene files to the Cognitive3D platform.

### Usage

```bash
./upload-scene.sh --scene_dir <scene_directory> [--env <prod|dev>] [--scene_id <scene_id>] [--verbose] [--dry_run]
```

### Parameters

**Required:**

- `--scene_dir <scene_directory>` - Path to folder containing:
  - `scene.bin`
  - `scene.gltf`
  - `screenshot.png`
  - `settings.json`

**Optional:**

- `--env <prod|dev>` - Target environment. Defaults to `prod`
- `--scene_id <scene_id>` - Scene ID for uploading new version of existing scene (must be valid UUID format)
- `--verbose` - Enable detailed logging with debug information and file sizes
- `--dry_run` - Preview operations without executing them (safe testing mode)

### Examples

**First upload (creates new scene):**

```bash
export C3D_DEVELOPER_API_KEY="abc123xyz"
./upload-scene.sh --scene_dir ./scene-test --env prod
```

**Update existing scene:**

```bash
./upload-scene.sh --scene_dir ./scene-test --env prod --scene_id my_scene_id
```

**Test with dry run (safe preview):**

```bash
./upload-scene.sh --scene_dir ./scene-test --env prod --dry_run --verbose
```

### Features

**Enhanced Security & Reliability:**

- Secure API key handling without local storage
- Safe file operations with automatic backup and rollback
- Comprehensive input validation (UUID format, file sizes, SDK version)

**Advanced Logging & Monitoring:**

- Timestamped, color-coded logging (INFO, WARN, ERROR, DEBUG)
- Upload timing and performance metrics
- File size validation and reporting (100MB limit per file)

**Smart Error Handling:**

- Specific guidance for common errors (401 key expired, 403 forbidden, 404 not found)
- Step-by-step instructions for API key rotation
- Clear troubleshooting steps for authentication issues

**Safe Testing:**

- `--dry_run` mode previews all operations without execution
- Shows exact curl commands and file operations
- Validates inputs before making any changes

### Behavior

- Validates all inputs (scene_id UUID format, SDK version, file sizes)
- Creates backup of `settings.json` before modification
- Reads SDK version from `sdk-version.txt` and updates `settings.json`
- Uploads all four files to the API endpoint with timing metrics
- Returns scene ID for new scenes with next-step guidance

### Help

```bash
./upload-scene.sh --help
```

## Dynamic Object Upload Script

Uploads dynamic 3D object assets to the Cognitive3D platform. **Requires a scene to be uploaded first.**

### Usage

```bash
./upload-object.sh \
  [--scene_id <scene-uuid>] \
  --object_filename <object-name> \
  --object_dir <path-to-object-directory> \
  [--object_id <existing-object-id>] \
  [--env <prod|dev>] \
  [--verbose] \
  [--dry_run]
```

### Parameters

**Required:**

- `--scene_id` - Scene ID UUID where object will be uploaded (or set C3D_SCENE_ID environment variable)
- `--object_filename` - Base filename (no extension) for `.gltf` and `.bin` files
- `--object_dir` - Directory containing object files

**Optional:**

- `--object_id` - Upload as new version of existing object
- `--env` - Target environment (`prod` or `dev`). Defaults to `prod`
- `--verbose` - Enable detailed logging
- `--dry_run` - Show `curl` command without executing

### File Requirements

Must exist in `--object_dir`:

- `<object_filename>.gltf`
- `<object_filename>.bin`
- `cvr_object_thumbnail.png` (optional, recommended) - Object thumbnail for dashboard
- Additional `.png` textures (optional) - Any textures used by the model

### Example

```bash
export C3D_DEVELOPER_API_KEY="your-api-key"

./upload-object.sh \
  --scene_id "your-scene-id-here" \
  --object_filename cube \
  --object_dir object-test \
  --env prod \
  --object_id cube \
  --verbose
```

### Exit Codes

- `0` - Success
- `1` - Missing argument or setup error
- Non-zero - `curl` upload failure

## Object Manifest Upload Script

Uploads object manifest to display objects in the Cognitive3D dashboard. Run after uploading object assets.

### Usage

```bash
./upload-object-manifest.sh \
  [--scene_id <scene-uuid>] \
  [--env <prod|dev>] \
  [--verbose] \
  [--dry_run]
```

### Parameters

**Required:**

- `--scene_id` - Scene ID UUID (or set C3D_SCENE_ID environment variable)

**Optional:**

- `--env` - Target environment (`prod` or `dev`). Defaults to `prod`
- `--verbose` - Enable detailed logging
- `--dry_run` - Show `curl` command without executing

### File Requirements

Must exist in current directory:

- `<scene_id>_object_manifest.json` - Auto-generated after object upload

### Example

```bash
./upload-object-manifest.sh \
  --scene_id "your-scene-id-here" \
  --env prod \
  --verbose
```

> **Note**: The manifest is auto-generated but can be manually edited before upload to modify object properties like starting position.

## List Objects Script

Lists all dynamic objects associated with a scene.

### Usage

```bash
./list-objects.sh [--scene_id <scene_id>] --env <prod|dev> [--verbose] [--debug]
```

### Parameters

**Required:**

- `--scene_id` - Scene ID UUID (or set C3D_SCENE_ID environment variable)
- `--env` - Target environment (`prod` or `dev`)

**Optional:**

- `--verbose` - Enable detailed logging
- `--debug` - Enable debug output

### Example

```bash
./list-objects.sh --scene_id "your-scene-id-here" --env prod --verbose
```

## Test Scripts

### Environment Workflow Test Scripts

Test the complete upload workflow with environment-specific configuration.

**Bash:**

```bash
./test-env-workflow.sh --env <prod|dev> [--verbose] [--dry_run]
```

**PowerShell:**

```powershell
./Test-EnvWorkflow.ps1 -Environment <prod|dev> [-Verbose] [-DryRun]
```

These scripts automatically:

1. Copy `.env.sample.<env>` to `.env`
2. Upload test scene to get scene ID
3. Add scene ID to `.env` file
4. Test object upload using C3D_SCENE_ID environment variable
5. Upload object manifest and list objects
6. Clean up temporary `.env` file

### Legacy Test Script

Comprehensive testing script that runs the complete upload workflow.

### Usage

```bash
./test-all.sh [scene_id] [env]
```

### Parameters

- `scene_id` - Existing scene ID (run scene upload first to get this, or set C3D_SCENE_ID environment variable)
- `env` - Target environment (`prod` or `dev`), defaults to `prod`

### Example

**Traditional approach:**

```bash
# First upload a scene to get scene_id
./upload-scene.sh --scene_dir scene-test --env prod

# Use returned scene_id for testing
./test-all.sh "your-scene-id-here" prod
```

**Environment variable approach:**

```bash
# Option 1: Set in .env file
echo "C3D_SCENE_ID=your-scene-id-here" >> .env
./test-all.sh

# Option 2: Set manually in shell
export C3D_SCENE_ID="your-scene-id-here"
./test-all.sh
```

> **Tested**: This workflow has been verified with both dev and prod environments using real API uploads.

### Test Workflow

1. Uploads scene (new version)
2. Uploads cube object
3. Uploads object manifest
4. Uploads Lantern object
5. Uploads updated object manifest
6. Lists all objects

## Logging and Output

All scripts support:

- **Colored output** - Info (blue), warnings (yellow), errors (red), debug (cyan)
- **Verbose mode** - `--verbose` flag shows detailed steps
- **Dry run mode** - `--dry_run` flag shows commands without executing
- **Consistent exit codes** - `0` for success, `1` for setup errors, other codes for API failures

## Troubleshooting

### Common Issues

**"jq: command not found"**

```bash
# Install jq using your package manager
brew install jq              # macOS
sudo apt install jq          # Ubuntu/Debian
```

**"API key not set"**

```bash
# Set the environment variable
export C3D_DEVELOPER_API_KEY="your_api_key"
```

**"Scene not found" when uploading objects**

- Objects require an existing scene. Upload a scene first and use the returned scene_id

**Manifest file not found**

- Object manifests are auto-generated after successful object upload
- Check that object upload completed successfully before uploading manifest

**"Your developer API key has expired" (HTTP 401)**

- Follow the step-by-step instructions provided by the script
- Generate a new key from Dashboard → Settings → 'Manage developer key'
- Update environment variable: `export C3D_DEVELOPER_API_KEY="your_new_key"`

**"Invalid scene_id format" error**

- Scene IDs must be in UUID format: `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`
- Check the scene ID from your dashboard or previous upload response

**File size errors**

- Individual files cannot exceed 100MB
- Use `--verbose` flag to see actual file sizes
- Compress or optimize large assets before upload

### Getting Help

- Use `--help` flag on any script for usage information
- Use `--verbose` flag to see detailed execution steps
- Use `--dry_run` flag to preview API calls without executing

### Support

For questions or issues:

- Open an issue in this repository
- Join our [Discord](https://discord.gg/x38sNUdDRH)
- Contact support via the Intercom button on any Cognitive3D web page
