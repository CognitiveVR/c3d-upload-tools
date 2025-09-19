# Cognitive3D Upload Tools

Upload 3D scenes and dynamic objects to the Cognitive3D platform with ease. Cross-platform tools supporting both Bash (macOS/Linux) and PowerShell (Windows) with streamlined environment variable workflows.

## Key Features

- **Complete Upload Workflow**: Scene files, dynamic objects, and manifests
- **Cross-Platform**: Bash scripts for macOS/Linux, PowerShell module for Windows
- **Environment Variables**: Streamlined workflows with `.env` file support
- **Enhanced Security**: Safe API key handling and input validation
- **Developer-Friendly**: Comprehensive logging, dry-run mode, and error guidance

## Quick Start

### 1. Get Your API Key

1. Go to [Cognitive3D Dashboard](https://app.cognitive3d.com/)
2. Settings (gear icon) → "Manage developer key"
3. Copy your Developer API key

### 2. Set Up Environment

```bash
# Copy and edit the configuration file
cp .env.example .env
# Edit .env and add your API key: C3D_DEVELOPER_API_KEY=your_api_key_here
```

### 3. Install Dependencies (Bash Only)

**macOS:** `brew install jq curl`
**Ubuntu/Debian:** `sudo apt install jq curl`
**Fedora/RHEL:** `dnf install jq curl`
**Windows:** Use PowerShell module (no dependencies required)

### 4. Upload Your First Scene

```bash
# Bash (macOS/Linux)
./upload-scene.sh --scene_dir scene-test --env prod

# PowerShell (Windows)
Import-Module ./C3DUploadTools
Upload-C3DScene -SceneDirectory scene-test -Environment prod
```

Save the returned Scene ID for object uploads, or add it to your `.env` file:
```bash
echo "C3D_SCENE_ID=your-scene-id-here" >> .env
```

### 5. Upload Objects (Optional)

```bash
# Bash - with scene ID in .env file
./upload-object.sh --object_filename cube --object_dir object-test --env prod
./upload-object-manifest.sh --env prod

# PowerShell - with scene ID in environment
Upload-C3DObject -ObjectFilename cube -ObjectDirectory object-test -Environment prod
Upload-C3DObjectManifest -Environment prod
```

## Platform Support

| Platform | Implementation | Dependencies | Status |
|----------|---------------|--------------|---------|
| **macOS/Linux** | Bash scripts | `jq`, `curl` | ✅ Fully tested |
| **Windows** | PowerShell module | None (uses .NET) | ✅ Native support |
| **WSL** | Bash scripts | `jq`, `curl` | ✅ Compatible |

## Configuration

### Environment Variables

Set these in your `.env` file or shell environment:

**Required:**
- `C3D_DEVELOPER_API_KEY` - Your Cognitive3D Developer API key

**Optional (for streamlined workflows):**
- `C3D_SCENE_ID` - Default scene ID to avoid passing --scene_id to every command
- `C3D_DEFAULT_ENVIRONMENT` - Default environment (`prod` or `dev`)

### Setup Options

**Option 1: .env File (Recommended)**
```bash
cp .env.example .env
# Edit .env with your values
```

**Option 2: Shell Environment**
```bash
export C3D_DEVELOPER_API_KEY="your_api_key"
export C3D_SCENE_ID="your_scene_id"  # optional
```

**Option 3: PowerShell**
```powershell
Import-Module ./C3DUploadTools  # Auto-loads .env file
# Or set manually:
$env:C3D_DEVELOPER_API_KEY = "your_api_key"
```

> **Security:** `.env` files are git-ignored automatically. Never commit API keys to version control.

## Command Reference

### Scene Upload

Upload 3D scene files (GLTF, textures, settings) to create or update a scene.

**Bash:**
```bash
./upload-scene.sh --scene_dir <directory> [--env prod|dev] [--scene_id <uuid>] [--verbose] [--dry_run]
```

**PowerShell:**
```powershell
Upload-C3DScene -SceneDirectory <directory> [-Environment prod|dev] [-SceneId <uuid>] [-Verbose] [-DryRun]
```

**Required Files in Scene Directory:**
- `scene.bin`, `scene.gltf`, `screenshot.png`, `settings.json`

**Examples:**
```bash
# Create new scene
./upload-scene.sh --scene_dir scene-test --env prod

# Update existing scene
./upload-scene.sh --scene_dir scene-test --scene_id "12345678-1234-1234-1234-123456789012" --env prod

# Preview changes (safe testing)
./upload-scene.sh --scene_dir scene-test --dry_run --verbose
```

### Object Upload

Upload dynamic 3D objects to an existing scene.

**Bash:**
```bash
./upload-object.sh --object_filename <name> --object_dir <directory> [--scene_id <uuid>] [--env prod|dev] [--verbose] [--dry_run]
```

**PowerShell:**
```powershell
Upload-C3DObject -ObjectFilename <name> -ObjectDirectory <directory> [-SceneId <uuid>] [-Environment prod|dev] [-Verbose] [-DryRun]
```

**Required Files in Object Directory:**
- `<filename>.gltf`, `<filename>.bin`
- `cvr_object_thumbnail.png` (optional, recommended)

**Examples:**
```bash
# Upload object (scene ID from environment)
./upload-object.sh --object_filename cube --object_dir object-test --env prod

# Upload with explicit scene ID
./upload-object.sh --object_filename cube --object_dir object-test --scene_id "your-scene-id" --env prod
```

### Object Manifest Upload

Upload object manifest to display objects in the dashboard.

**Bash:**
```bash
./upload-object-manifest.sh [--scene_id <uuid>] [--env prod|dev] [--verbose] [--dry_run]
```

**PowerShell:**
```powershell
Upload-C3DObjectManifest [-SceneId <uuid>] [-Environment prod|dev] [-Verbose] [-DryRun]
```

### List Objects

List all objects associated with a scene.

**Bash:**
```bash
./list-objects.sh [--scene_id <uuid>] --env <prod|dev> [--verbose]
```

**PowerShell:**
```powershell
Get-C3DObjects [-SceneId <uuid>] -Environment <prod|dev> [-Verbose]
```

### Universal Options

| Option | Description |
|--------|-------------|
| `--env` / `-Environment` | Target environment: `prod` (production) or `dev` (development) |
| `--scene_id` / `-SceneId` | Scene UUID (optional if set in environment variables) |
| `--verbose` / `-Verbose` | Enable detailed logging and debug information |
| `--dry_run` / `-DryRun` | Preview operations without executing (safe testing mode) |

**Security Features:**
- Automatic input validation (UUID format, file sizes, SDK versions)
- Safe file operations with backup/rollback
- API key validation with helpful error messages
- 100MB file size limit with clear warnings

## Testing & Validation

### Complete Workflow Test

Test the entire upload workflow with sample assets.

**Bash:**
```bash
./test-env-workflow.sh --env prod [--verbose] [--dry_run]
```

**PowerShell:**
```powershell
./Test-EnvWorkflow.ps1 -Environment prod [-Verbose] [-DryRun]
```

**Automated Test Steps:**
1. Set up temporary environment configuration
2. Upload test scene and capture scene ID
3. Upload test objects (cube, lantern)
4. Upload object manifests
5. Verify objects are listed correctly
6. Clean up temporary files

### Individual Component Testing

**Test scene upload:**
```bash
./upload-scene.sh --scene_dir scene-test --env dev --dry_run --verbose
```

**Test object upload:**
```bash
./upload-object.sh --object_filename cube --object_dir object-test --env dev --dry_run
```

### Legacy Test Script

For existing workflows:
```bash
./test-all.sh [scene_id] [env]  # Use after manual scene upload
```

## Workflow Examples

### Complete Upload Workflow

**For New Projects:**
```bash
# 1. Set up configuration
cp .env.example .env
# Edit .env with your API key

# 2. Upload scene
./upload-scene.sh --scene_dir your-scene --env prod
# Save the returned Scene ID

# 3. Add Scene ID to environment (optional but recommended)
echo "C3D_SCENE_ID=your-scene-id-here" >> .env

# 4. Upload objects (as needed)
./upload-object.sh --object_filename object1 --object_dir your-objects --env prod
./upload-object.sh --object_filename object2 --object_dir your-objects --env prod

# 5. Upload manifest to display in dashboard
./upload-object-manifest.sh --env prod

# 6. Verify objects
./list-objects.sh --env prod
```

**For Existing Projects:**
```bash
# Update existing scene
./upload-scene.sh --scene_dir updated-scene --scene_id "existing-uuid" --env prod

# Add new objects to existing scene
./upload-object.sh --object_filename new-object --object_dir objects --env prod
./upload-object-manifest.sh --env prod
```

## Troubleshooting

### Common Setup Issues

| Issue | Solution |
|-------|----------|
| **"jq: command not found"** | Install dependencies: `brew install jq curl` (macOS) or `sudo apt install jq curl` (Ubuntu) |
| **"API key not set"** | Set `C3D_DEVELOPER_API_KEY` in `.env` file or environment |
| **"Invalid scene_id format"** | Scene IDs must be valid UUIDs: `12345678-1234-1234-1234-123456789012` |
| **PowerShell execution policy** | Run `Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser` |

### Common Upload Issues

| Issue | Solution |
|-------|----------|
| **"Scene not found" (objects)** | Upload a scene first; objects require an existing scene |
| **"Manifest file not found"** | Object manifests auto-generate after successful object upload |
| **File size errors** | Individual files cannot exceed 100MB; check with `--verbose` |
| **HTTP 401 "Key expired"** | Generate new API key from Dashboard → Settings → "Manage developer key" |
| **HTTP 403 "Forbidden"** | Verify API key permissions and scene ownership |

### Debug and Testing

**Use dry-run mode for safe testing:**
```bash
./upload-scene.sh --scene_dir scene-test --dry_run --verbose
```

**Enable verbose logging:**
```bash
./upload-object.sh --object_filename cube --object_dir object-test --verbose
```

**Check file contents and structure:**
```bash
# Verify scene directory structure
ls -la scene-test/
# Should contain: scene.bin, scene.gltf, screenshot.png, settings.json

# Verify object directory structure
ls -la object-test/
# Should contain: <filename>.gltf, <filename>.bin, optional textures
```

### Windows-Specific Issues

**PowerShell module not loading:**
```powershell
# Ensure you're in the correct directory
Import-Module ./C3DUploadTools -Force
```

**.env file not recognized:**
```powershell
# PowerShell automatically loads .env files when importing the module
# Alternatively, set variables manually:
$env:C3D_DEVELOPER_API_KEY = "your_api_key"
```

### Getting Help

**Built-in Help:**
- `./upload-scene.sh --help` - Show usage information
- `--verbose` flag - Enable detailed logging
- `--dry_run` flag - Preview operations safely

**Support Channels:**
- [GitHub Issues](https://github.com/cognitive3d/c3d-upload-tools/issues) - Bug reports and feature requests
- [Discord Community](https://discord.gg/x38sNUdDRH) - Community support and discussion
- [Cognitive3D Support](https://cognitive3d.com/support) - Official technical support

## Project Structure

```
c3d-upload-tools/
├── upload-scene.sh           # Scene upload (Bash)
├── upload-object.sh          # Object upload (Bash)
├── upload-object-manifest.sh # Manifest upload (Bash)
├── list-objects.sh           # List objects (Bash)
├── test-*.sh                 # Test scripts (Bash)
├── C3DUploadTools/           # PowerShell module
│   ├── C3DUploadTools.psd1   # Module manifest
│   ├── Public/               # User-facing functions
│   └── Private/              # Internal utilities
├── scene-test/               # Sample scene assets
├── object-test/              # Sample object assets
├── .env.example              # Configuration template
└── sdk-version.txt           # Current SDK version
```

**Sample Assets Included:**
- `scene-test/` - Complete scene for testing uploads
- `object-test/` - Dynamic object (cube) for testing
- `lantern-test/` - Additional object example

## License

This project is licensed under the MIT License. See the LICENSE file for details.
