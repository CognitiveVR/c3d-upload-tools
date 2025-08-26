# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

### Testing
- `./test-all.sh <scene_id> <env>` - Run comprehensive tests for scene and object uploads (requires scene_id from previous upload)

### Scene Operations
- `./upload-scene.sh --scene_dir <dir> [--env prod|dev] [--scene_id <id>] [--verbose] [--dry_run]` - Upload scene files with enhanced security and validation
- `./upload-scene.sh --help` - Show usage information

### Object Operations  
- `./upload-object.sh [--scene_id <id>] --object_filename <name> --object_dir <dir> [--object_id <id>] [--env prod|dev] [--verbose] [--dry_run]` - Upload dynamic 3D objects
- `./upload-object-manifest.sh [--scene_id <id>] [--env prod|dev] [--verbose] [--dry_run]` - Upload object manifest after object upload
- `./list-objects.sh [--scene_id <id>] --env <env> --verbose` - List objects for a scene

### Dependencies
- Requires `jq` and `curl` to be installed
- Set `C3D_DEVELOPER_API_KEY` environment variable or use `.env` file (recommended)
- Optional: Set `C3D_SCENE_ID` environment variable to avoid --scene_id parameters

## Architecture

### File Structure
- **Shell scripts**: Main upload functionality in bash scripts with standardized logging, error handling, and API interaction patterns
- **Test directories**: `scene-test/`, `object-test/`, `lantern-test/` contain sample assets for testing uploads
- **Generated files**: `<scene_id>_object_manifest.json` files are created automatically after object uploads

### Key Components
- `sdk-version.txt`: Contains current SDK version (0.2.0) that gets injected into settings.json during scene uploads
- `settings.json`: Scene configuration with scale, sceneName, and sdkVersion fields (automatically backed up before modification)
- **API endpoints**: Support both prod (cognitive3d.com) and dev (c3ddev.com) environments

### Upload Workflow
1. Upload scene first to get scene_id
2. Upload objects using scene_id (can be set in .env file for convenience)
3. Upload object manifest to display objects in dashboard
4. Object manifests are automatically generated but can be manually edited before upload

### Environment Variable Support (SDK-191) ✅
**Streamlined Workflow with .env:**
- Set `C3D_SCENE_ID` in `.env` file to avoid --scene_id parameters
- All object operations (upload-object.sh, upload-object-manifest.sh, list-objects.sh) support environment variable fallback
- PowerShell module automatically loads .env file during import
- Tested with real API uploads in both dev and prod environments

### Common Patterns
- All scripts use consistent argument parsing with `--parameter value` format
- Standardized logging with color-coded output (INFO/WARN/ERROR/DEBUG) and timestamps
- Comprehensive error checking for dependencies and required parameters
- Support for `--verbose` and `--dry_run` flags across scripts

## Enhanced upload-scene.sh Features

### Security & Reliability
- Secure API key handling without local storage exposure
- Safe file operations with automatic backup and rollback mechanisms
- Comprehensive input validation (UUID format, semantic versioning, file size limits)
- Cross-platform compatibility (macOS and Linux)

### Advanced Error Handling
- Specific guidance for HTTP 401 "Key expired" errors with step-by-step resolution
- Authentication troubleshooting for general 401 errors
- Clear messages for 403 (forbidden) and 404 (not found) errors
- Actionable error messages that guide users to solutions

### Validation Features
- Scene ID must be valid UUID format: `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`
- SDK version must follow semantic versioning: `x.y.z`
- File size validation with 100MB limit per file
- Early validation prevents costly operations with invalid data

### Logging & Monitoring
- Timestamped logs with format: `[YYYY-MM-DD HH:MM:SS] [LEVEL] message`
- Upload timing metrics and performance monitoring
- File size reporting in debug mode
- Color-coded output for different log levels

### Safe Testing
- `--dry_run` mode shows exact operations without execution
- Previews curl commands with redacted API keys
- Displays file inventory with sizes before upload
- Validates all inputs before making any changes

### Best Practices for Development
- Always test with `--dry_run` first before real uploads
- Use `--verbose` for detailed debugging information
- Check file sizes if uploads are slow or failing
- Rotate API keys immediately when receiving 401 errors
- Validate scene_id format from dashboard before use

## PowerShell Implementation (Windows Compatibility)

### Status: In Progress ✅
The repository is being enhanced with PowerShell equivalents to provide native Windows support while maintaining cross-platform compatibility.

### Completed Components

#### Phase 1: Module Structure (SDK-182) ✅
- **C3DUploadTools PowerShell Module**: Complete module structure with proper manifest
- **Cross-platform compatibility**: Tested on macOS PowerShell 7.5.2, compatible with Windows PowerShell 5.1+
- **Function organization**: Public functions for user interface, Private functions for utilities
- **Module loading**: Dynamic function discovery and export system

#### Phase 1.2: Core Utilities (SDK-183) ✅
- **Logging System** (`Write-C3DLog`): Timestamped, color-coded logging matching bash functionality
- **API Key Management** (`Test-C3DApiKey`, `Get-C3DApiKey`): Enhanced validation with cross-platform instructions
- **Environment Management** (`Get-C3DApiUrl`, `Test-C3DEnvironment`): URL generation for prod/dev environments
- **UUID Operations** (`Test-C3DUuidFormat`, `ConvertTo-C3DLowerUuid`, `New-C3DUuid`): Complete UUID validation and generation
- **HTTP Engine** (`Invoke-C3DApiRequest`): Native PowerShell replacement for curl with progress indicators
- **File System** (`Test-C3DDirectory`, `Test-C3DFile`, `Backup-C3DFile`): Comprehensive file validation and backup system
- **Module Initialization**: Strict mode equivalent to bash `set -e` and `set -u`

### PowerShell Commands (Available Now)

#### Testing PowerShell Module Structure
- `pwsh -File test-module-structure.ps1` - Comprehensive module structure validation
- `pwsh -File test-utilities-internal.ps1` - Internal utility function testing

#### Module Usage
```powershell
# Import the module
Import-Module ./C3DUploadTools -Force

# Available functions (placeholders, ready for implementation)
Upload-C3DScene -SceneDirectory <path> [-Environment prod|dev] [-SceneId <uuid>] [-DryRun] [-Verbose]
Upload-C3DObject -SceneId <uuid> -ObjectFilename <name> -ObjectDirectory <path> [-Environment prod|dev] [-DryRun]
Upload-C3DObjectManifest -SceneId <uuid> [-Environment prod|dev] [-DryRun]
Get-C3DObjects -SceneId <uuid> [-Environment prod|dev]
Test-C3DUploads -SceneId <uuid> [-Environment prod|dev]
```

### PowerShell Advantages Over Bash

| Feature | Bash (current) | PowerShell (new) |
|---------|----------------|------------------|
| **Dependencies** | Requires `jq` and `curl` | No external dependencies |
| **JSON Processing** | External `jq` command | Native `ConvertFrom-Json` |
| **HTTP Requests** | External `curl` command | Native `Invoke-WebRequest` with progress |
| **Error Handling** | Exit codes + manual parsing | Rich exception objects with detailed context |
| **Parameter Validation** | Manual validation functions | Declarative `[Parameter()]` attributes |
| **File Operations** | External commands (`cp`, `mv`) | Native PowerShell cmdlets |
| **Cross-platform Paths** | Manual path handling | Automatic path normalization |
| **Progress Indicators** | Basic text output | Native progress bars for uploads |
| **Windows Integration** | Limited Windows support | Native Windows PowerShell support |

### Implementation Roadmap

#### Next Phase: Scene Upload (SDK-184) 🔄
Convert `upload-scene.sh` functionality to `Upload-C3DScene.ps1`:
- Settings.json backup/rollback with PowerShell file operations
- SDK version injection from sdk-version.txt
- Native JSON manipulation replacing jq
- Progress indicators for large file uploads
- Enhanced parameter validation

#### Future Phases: Object Operations (SDK-185, SDK-186)
- `Upload-C3DObject.ps1`: Object upload with multipart form support
- `Upload-C3DObjectManifest.ps1`: Manifest generation and upload
- `Get-C3DObjects.ps1`: Object listing with formatted output
- `Test-C3DUploads.ps1`: Comprehensive testing workflow

#### Windows Enhancements (SDK-187)
- Windows Credential Manager integration for secure API key storage
- Registry-based configuration for user preferences
- Windows Toast notifications for upload completion
- PowerShell Gallery publishing preparation

### Current File Structure
```
C3DUploadTools/                    # PowerShell Module
├── C3DUploadTools.psd1           # Module manifest ✅
├── C3DUploadTools.psm1           # Module loader ✅
├── Public/                       # User-facing functions
│   ├── Upload-C3DScene.ps1       # 🔄 Ready for implementation
│   ├── Upload-C3DObject.ps1      # 📋 Planned
│   └── [other functions]         # 📋 Planned
└── Private/                      # Core utilities ✅ COMPLETE
    ├── Write-C3DLog.ps1          # Logging system
    ├── Test-C3DApiKey.ps1        # API key validation
    ├── Get-C3DApiUrl.ps1         # URL generation
    ├── Test-C3DUuidFormat.ps1    # UUID operations
    ├── Invoke-C3DApiRequest.ps1  # HTTP engine
    ├── Test-C3DFileSystem.ps1    # File operations
    └── Initialize-C3DModule.ps1  # Module initialization

# Testing Infrastructure ✅
test-module-structure.ps1          # Module structure validation
test-utilities-internal.ps1        # Internal function testing
```

### Compatibility
- **Bash Scripts**: Continue to work as before (no breaking changes)
- **PowerShell Module**: Available for Windows users and cross-platform PowerShell users
- **Testing**: Both bash and PowerShell implementations thoroughly tested on macOS