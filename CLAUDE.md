# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

### Testing
- `./test-all.sh <scene_id> <env>` - Run comprehensive tests for scene and object uploads (requires scene_id from previous upload)

### Scene Operations
- `./upload-scene.sh --scene_dir <dir> [--env prod|dev] [--scene_id <id>] [--verbose] [--dry_run]` - Upload scene files with enhanced security and validation
- `./upload-scene.sh --help` - Show usage information

### Object Operations  
- `./upload-object.sh --scene_id <id> --object_filename <name> --object_dir <dir> [--object_id <id>] [--env prod|dev] [--verbose] [--dry_run]` - Upload dynamic 3D objects
- `./upload-object-manifest.sh --scene_id <id> [--env prod|dev] [--verbose] [--dry_run]` - Upload object manifest after object upload
- `./list-objects.sh --scene_id <id> --env <env> --verbose` - List objects for a scene

### Dependencies
- Requires `jq` and `curl` to be installed
- Set `C3D_DEVELOPER_API_KEY` environment variable before running scripts

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
2. Upload objects using scene_id 
3. Upload object manifest to display objects in dashboard
4. Object manifests are automatically generated but can be manually edited before upload

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