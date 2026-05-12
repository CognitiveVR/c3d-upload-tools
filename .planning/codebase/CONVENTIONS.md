# Coding Conventions

**Analysis Date:** 2026-05-12

This repository ships two parallel implementations of the Cognitive3D upload CLI:

1. **Bash scripts** at the repo root (`upload-scene.sh`, `upload-object.sh`, `upload-object-manifest.sh`, `list-objects.sh`) sharing helpers in `upload-utils.sh`.
2. **PowerShell module** under `C3DUploadTools/` (Public/Private function layout, `C3DUploadTools.psm1` + `C3DUploadTools.psd1`).

Both implementations are kept in deliberate functional parity. Conventions therefore have **shared cross-language rules** (CLI flags, env vars, settings.json schema) and **language-specific style rules**.

## Naming Patterns

### Files

**Bash (kebab-case `.sh`):**

- Executable entry points at repo root: `upload-scene.sh`, `upload-object.sh`, `upload-object-manifest.sh`, `list-objects.sh`
- Shared helper sourced via `source ./upload-utils.sh`: `upload-utils.sh`
- Test scripts under `test-scripts/`: `test-all.sh`, `test-utils.sh`, `test-env-workflow.sh`, `test-scene-upload-improvements.sh`, `test-object-upload-improvements.sh`

**PowerShell (one function per file, filename matches function exactly):**

- Public functions in `C3DUploadTools/Public/`: `Upload-C3DScene.ps1`, `Upload-C3DObject.ps1`, `Upload-C3DObjectManifest.ps1`, `Get-C3DObjects.ps1`, `Test-C3DUploads.ps1`
- Private helpers in `C3DUploadTools/Private/<Category>/`: `Write-C3DLog.ps1`, `New-C3DErrorRecord.ps1`, `Test-C3DApiKey.ps1`, `Test-C3DUuidFormat.ps1`, `Get-C3DApiUrl.ps1`, `Invoke-C3DApiRequest.ps1`, `Set-C3DRequestHeaders.ps1`, `New-C3DMultipartFormData.ps1`, `Send-C3DHttpRequest.ps1`, `Test-C3DFileSystem.ps1`, `Import-C3DEnvironment.ps1`, `Initialize-C3DModule.ps1`, `C3DClasses.ps1`, `New-C3DUploadSession.ps1`
- Module manifest/loader: `C3DUploadTools.psd1`, `C3DUploadTools.psm1`
- Test scripts in `C3DUploadTools/Tests/`: kebab-case `.ps1` for unit/integration (e.g. `test-scene-upload.ps1`) and `Verb-Noun.ps1` for runnable workflow scripts (`Test-EnvWorkflow.ps1`)

### Functions

**Bash — `snake_case`:** All helpers in `upload-utils.sh` use `snake_case`, e.g. `log_info`, `log_warn`, `log_error`, `log_debug`, `load_env_file`, `check_dependencies`, `validate_api_key`, `validate_uuid_format`, `validate_semantic_version`, `validate_file`, `validate_directory`, `validate_environment`, `get_api_base_url`, `get_scene_version`, `parse_http_response`, `handle_http_error`, `process_json_response`, `upload_screenshot`, `log_execution_time`. Test helpers in `test-scripts/test-utils.sh` follow the same pattern: `print_section`, `print_test`, `print_pass`, `print_fail`, `print_summary`, `extract_scene_id`, `check_success`, `check_scene_id_extracted`, `check_version_check`, `update_scene_timestamp`.

**PowerShell — `Verb-Noun` (PascalCase) with the `C3D` noun prefix:**

- Approved PowerShell verbs only (Upload-, Get-, Test-, New-, Set-, Send-, Invoke-, Write-, Import-, Initialize-, ConvertTo-).
- Public functions follow `<Verb>-C3D<Noun>` (e.g. `Upload-C3DScene`, `Get-C3DObjects`).
- Private functions follow the same scheme (e.g. `Write-C3DLog`, `New-C3DErrorRecord`, `Test-C3DApiKey`, `Get-C3DApiUrl`, `Set-C3DRequestHeaders`).
- Internal-only helpers that don't fit cleanly into Verb-Noun (e.g. `ConvertTo-C3DLowerUuid`, `New-C3DUuid`) still use Verb-Noun.

### Variables

**Bash — uppercase `SNAKE_CASE` for script-level vars, lowercase `snake_case` for `local`:**

- Configuration / argument capture vars at function scope are `UPPER_SNAKE_CASE`: `SCENE_DIRECTORY`, `SCENE_ID`, `SCENE_NAME`, `ENVIRONMENT`, `VERBOSE`, `DRY_RUN`, `OBJECT_FILENAME`, `OBJECT_ID`, `OBJECT_DIRECTORY`, `BIN_FILE`, `GLTF_FILE`, `JSON_FILE`, `SCREENSHOT_FILE`, `IMAGE_FORMS`, `IMAGE_COUNT`, `SDK_VERSION`, `FULL_SDK_VERSION`, `CURL_CMD`, `RESPONSE`, `HTTP_BODY`, `HTTP_STATUS` (see `upload-scene.sh:54-100`).
- Pure `local` working variables inside helpers use lowercase `snake_case`: `local env_file`, `local key`, `local value`, `local file_path`, `local file_size`, `local missing_deps`, `local start_time`, `local upload_duration` (see `upload-utils.sh`).
- Globals that are *intentionally* shared between helper and caller scripts are uppercase and documented as such: `HTTP_BODY`, `HTTP_STATUS`, `SCENE_VERSION_NUMBER`, `SCENE_VERSION_ID` (`upload-utils.sh:189-196`, `:130-131`).
- Always use `"${VAR:-default}"` for optional env-var-backed config: `ENVIRONMENT="${C3D_DEFAULT_ENVIRONMENT:-prod}"`, `SCENE_ID="${C3D_SCENE_ID:-}"`, `VERBOSE="${VERBOSE:-false}"`.

**PowerShell — `PascalCase` parameters, `camelCase` locals, `$script:` for module-scope state:**

- Parameters: `$SceneDirectory`, `$SceneName`, `$Environment`, `$SceneId`, `$DryRun`, `$Throw`, `$Uuid`, `$FieldName`, `$ApiKey`, `$Message`, `$Level`.
- Locals inside functions: `$startTime`, `$endTime`, `$apiUrl`, `$filePaths`, `$requiredFiles`, `$sdkVersion`, `$fullSdkVersion`, `$settingsContent`, `$uploadFiles`, `$additionalImages`, `$errorRecord`, `$timestamp`, `$logMessage`, `$colorMap`.
- Module-scope mutable state uses `$script:` scope: `$script:VerboseMode` (toggled by `-Verbose` in public functions; checked by `Write-C3DLog` in `C3DUploadTools/Private/Core/Write-C3DLog.ps1:46-49`).
- Environment variables are read as `$env:C3D_DEVELOPER_API_KEY`, `$env:C3D_SCENE_ID`, `$env:C3D_DEFAULT_ENVIRONMENT`.

### Types / Classes

**Bash:** no formal types; rely on shell variable conventions and `[[ ... =~ ... ]]` regex validation.

**PowerShell:** Custom classes are defined in `C3DUploadTools/Private/Core/C3DClasses.ps1` and use `PascalCase` class names, `PascalCase` properties, and explicit type annotations:

- `C3DConfiguration` — typed properties `[string] $ApiKey`, `[string] $DefaultEnvironment = 'prod'`, `[hashtable] $EnvironmentUrls`, `[int] $DefaultTimeoutSeconds = 300`. Provides constructors and methods (`GetApiUrl`, `IsValid`).
- `C3DUploadRequest` — request DTO; uses constructor overloads for file-upload vs. JSON request shapes; methods `AddFile`, `AddHeader`, `GetTotalFileSize`, `IsValid`, `GetRequestType`.
- `C3DApiResponse` — response DTO with `[int] $StatusCode`, `[string] $Body`, `[hashtable] $Headers`, `[bool] $Success`, `[datetime] $Timestamp`; helper methods `GetJsonBody`, `IsAuthenticationError`, `IsNotFound`, `IsRateLimited`, `IsServerError`, `ToString`.
- `C3DUploadResult` — aggregated result containing operation type, scene/object IDs, `[C3DApiResponse] $Response`, `[hashtable] $UploadedFiles`, `[long] $TotalBytesUploaded`, `[string[]] $NextSteps`.

## Code Style

### Formatting

**Bash:**

- Shebang `#!/bin/bash` on every executable script.
- 2-space indentation.
- ANSI color helpers defined as constants (`COLOR_INFO`, `COLOR_WARN`, `COLOR_ERROR`, `COLOR_DEBUG`, `COLOR_RESET` in `upload-utils.sh:14-18`; a separate green/red/blue/yellow palette in `test-scripts/test-utils.sh:15-19`).
- No external linter is configured; conventions are enforced by code review.

**PowerShell:**

- 4-space indentation.
- Param block always uses `[CmdletBinding()]` (or `[CmdletBinding(SupportsShouldProcess)]` for state-changing public functions, e.g. `Upload-C3DScene.ps1:122`).
- Comment-based help (`<# .SYNOPSIS .DESCRIPTION .PARAMETER ... .EXAMPLE ... .NOTES .OUTPUTS .LINK #>`) precedes every public function and most private helpers.
- `Set-StrictMode -Version 3.0` and `$ErrorActionPreference = 'Stop'` at module load (`C3DUploadTools.psm1:7-10`); test scripts override with `Set-StrictMode -Version Latest` and `$ErrorActionPreference = 'Stop'`.
- `cspell.json` exists at repo root for spell-checking; no PSScriptAnalyzer config is checked in.

### Strict Mode / Safety Flags

**Bash — set early in `upload-utils.sh` and inherited by every sourcing script:**

```bash
set -e   # exit on first non-zero command (upload-utils.sh:8)
set -u   # error on unset variables  (upload-utils.sh:11)
```

Test scripts add both explicitly: `set -e; set -u` (`test-scripts/test-scene-upload-improvements.sh:13-14`, `test-scripts/test-env-workflow.sh:8` uses `set -euo pipefail`). Always reference optional vars as `"${VAR:-}"` because `set -u` is active.

**PowerShell:**

```powershell
Set-StrictMode -Version 3.0           # C3DUploadTools.psm1:7
$ErrorActionPreference = 'Stop'        # C3DUploadTools.psm1:10
```

## Argument & Parameter Conventions

### CLI Flag Style (Bash)

All bash CLIs use long-form `--parameter value` flags parsed with a `while [[ $# -gt 0 ]]; do case "$1" in ... esac done` loop. Standard flags shared across scripts:

- `--scene_dir <dir>`, `--scene_id <uuid>`, `--scene_name <name>`
- `--object_filename <name>`, `--object_dir <dir>`, `--object_id <uuid|name>`
- `--env <prod|dev>`
- `--verbose` (boolean; sets `VERBOSE=true`)
- `--dry_run` (boolean; sets `DRY_RUN=true`)
- `--help` / `-h`

Unknown args call `log_error "Unknown argument: $1"; exit 1` (`upload-scene.sh:111-113`). Boolean switches `shift` once; value flags `shift 2`. snake_case is used inside flag names (`--scene_id`, not `--scene-id`).

### Parameter Style (PowerShell)

Declarative validation via `[Parameter()]` and `[ValidateScript()]` / `[ValidateSet()]` attributes (`C3DUploadTools/Public/Upload-C3DScene.ps1:122-175`):

```powershell
[CmdletBinding(SupportsShouldProcess)]
param(
    [Parameter(Mandatory, Position = 0, HelpMessage = "Path to scene directory ...")]
    [ValidateScript({
        if (-not (Test-Path $_ -PathType Container)) { throw "Scene directory does not exist: $_" }
        # ... required-file and size checks ...
        $true
    })]
    [string]$SceneDirectory,

    [Parameter(HelpMessage = "Target environment: 'prod' or 'dev'")]
    [ValidateSet('prod', 'dev')]
    [string]$Environment = $(if ($env:C3D_DEFAULT_ENVIRONMENT) { $env:C3D_DEFAULT_ENVIRONMENT } else { 'prod' }),

    [Parameter(HelpMessage = "Optional UUID of existing scene to update")]
    [ValidateScript({
        if ([string]::IsNullOrWhiteSpace($_)) { return $true }
        if ($_ -notmatch '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$') {
            throw "Invalid UUID format for SceneId: '$_'. Expected format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
        }
        $true
    })]
    [string]$SceneId,

    [Parameter(HelpMessage = "Preview operations without executing them")]
    [switch]$DryRun
)
```

Rules:

- Mandatory parameters declared via `[Parameter(Mandatory, ...)]`, never via custom logic.
- Enumerated values via `[ValidateSet(...)]` (e.g. `'prod','dev'`, `'Info','Warn','Error','Debug'`).
- Complex validation (file existence, size limits, UUID format, JSON shape) via `[ValidateScript({ ... })]` blocks that `throw` user-readable error messages.
- Boolean flags use `[switch]` (e.g. `[switch]$DryRun`, `[switch]$Throw`, `[switch]$NoNewline`).
- Defaults that depend on environment variables are computed inline: `= $(if ($env:C3D_DEFAULT_ENVIRONMENT) { ... } else { 'prod' })`.
- Each parameter carries a `HelpMessage` for interactive prompts.

## Import Organization

### Bash — `source` at top of every script

```bash
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/upload-utils.sh"
load_env_file
```

This pattern appears at the top of every executable bash script (`upload-scene.sh:40-44`, `upload-object.sh:30-34`, `upload-object-manifest.sh:24-29`, `list-objects.sh:8-12`). Test scripts `cd "$(dirname "$0")/.."` first to anchor to the repo root, then `source test-scripts/test-utils.sh` and/or `source ./upload-utils.sh`.

### PowerShell — auto-discovery in `.psm1`

`C3DUploadTools.psm1` dot-sources every `.ps1` under `Private/` (recursive) first, then everything under `Public/`, then auto-exports Public function names via `Export-ModuleMember`. New helpers are added simply by dropping a `.ps1` into the appropriate `Private/<Category>/` subdirectory — the loader picks them up. After loading, the module calls `Import-C3DEnvironment` to auto-load `.env` (`C3DUploadTools.psm1:52`).

## Logging

### Bash — `log_info` / `log_warn` / `log_error` / `log_debug`

Defined once in `upload-utils.sh:21-24`:

```bash
log_info()  { echo -e "${COLOR_INFO}[$(date '+%Y-%m-%d %H:%M:%S')] [INFO] $1${COLOR_RESET}"; }
log_warn()  { echo -e "${COLOR_WARN}[$(date '+%Y-%m-%d %H:%M:%S')] [WARN] $1${COLOR_RESET}"; }
log_error() { echo -e "${COLOR_ERROR}[$(date '+%Y-%m-%d %H:%M:%S')] [ERROR] $1${COLOR_RESET}"; }
log_debug() { if [ "${VERBOSE:-false}" = true ]; then echo -e "${COLOR_DEBUG}[$(date '+%Y-%m-%d %H:%M:%S')] [DEBUG] $1${COLOR_RESET}"; fi; }
```

Rules:

- Always use the helpers; never use bare `echo` for status output. (`echo` is reserved for the JSON/scene-ID stdout payload that consumers parse — see scene-ID emission in `upload-scene.sh:384`.)
- Format is **always** `[YYYY-MM-DD HH:MM:SS] [LEVEL] message`.
- Debug messages are gated on `VERBOSE=true`.
- `log_verbose` is a backward-compatibility alias forwarding to `log_debug` (`upload-utils.sh:27-29`).
- Errors typically follow with a `handle_http_error` call or `exit 1`.

### PowerShell — `Write-C3DLog`

Defined in `C3DUploadTools/Private/Core/Write-C3DLog.ps1`. Matches the bash format exactly:

```powershell
Write-C3DLog -Message "Starting scene upload process" -Level Info
Write-C3DLog -Message "Invalid file format detected"   -Level Warn
Write-C3DLog -Message "Upload failed with HTTP 401"    -Level Error
Write-C3DLog -Message "File size: 1.2MB"                -Level Debug
```

- `-Level` is `[ValidateSet('Info','Warn','Error','Debug')]`.
- Timestamp format `yyyy-MM-dd HH:mm:ss` matches bash exactly.
- Color map matches bash COLOR_* constants (Cyan/Yellow/Red/Gray).
- Debug messages only emit when `$script:VerboseMode` is `$true`. Public functions opt in by checking `$PSBoundParameters.ContainsKey('Verbose')` and setting `$script:VerboseMode = $true` (`Upload-C3DScene.ps1:182-184`).
- Never use raw `Write-Host` for status messages — always go through `Write-C3DLog`. `Write-Host` with colors is acceptable inside `-DryRun` previews where multi-line formatted output is shown (`Upload-C3DScene.ps1:313-318`, `:336-348`).

## Error Handling

### Bash

- `set -e` aborts on any unhandled non-zero exit. Anywhere a non-fatal failure is acceptable, capture the exit explicitly: `if ! some_command; then ... fi` or use `|| true`.
- Validation helpers (`validate_uuid_format`, `validate_semantic_version`, `validate_file`, `validate_directory`, `validate_environment`, `validate_api_key`) `log_error` and `exit 1` on failure rather than returning.
- HTTP errors funnel through `handle_http_error <status> <body> <operation>` (`upload-utils.sh:199-257`), which has dedicated cases for 401 (with key-expired sub-detection), 403, 404, and a fallback that echoes the body. HTML error pages (server-side 500s returning HTML) are detected up front via `grep -qi "Internal Server Error\|Bad Request\|<html"` and short-circuit.
- `HTTP_STATUS` / `HTTP_BODY` are exported globals populated by `parse_http_response` so the caller can inspect them after every `curl` call.

### PowerShell

- Errors are structured `[System.Management.Automation.ErrorRecord]` objects built by `New-C3DErrorRecord` (`C3DUploadTools/Private/Core/New-C3DErrorRecord.ps1`). Always provide an `ErrorId`, an `[ErrorCategory]`, a `TargetObject`, the inner exception, and a `RecommendedAction` string. Public functions throw via `$PSCmdlet.ThrowTerminatingError($errorRecord)`.
- Category mapping by HTTP status (in `Invoke-C3DWithErrorHandling`, same file):
  - 401 → `AuthenticationError`
  - 403 → `PermissionDenied`
  - 404 → `ObjectNotFound`
  - 429 → `LimitsExceeded`
  - 500/502/503/504/408 → retryable; sleep `$RetryDelaySeconds` and retry up to `$RetryCount` times
  - default → `ConnectionError`
- Public functions wrap their main body in `try` with typed `catch` clauses for `[System.Net.WebException]`, `[System.IO.IOException]`, `[System.ArgumentException]`, `[System.UnauthorizedAccessException]`, then a generic `catch` (`Upload-C3DScene.ps1:413-428`). Each typed catch logs via `Write-C3DLog -Level Error` and re-throws as a `New-C3DErrorRecord`.
- Validation helpers (`Test-C3DApiKey`, `Test-C3DUuidFormat`, `Test-C3DDirectory`, `Test-C3DFile`, `Test-C3DEnvironment`) accept a `-Throw` switch: return `$true`/`$false` by default, throw on `-Throw`. Public functions call them with `-Throw`.

## Shared API Key Handling

API authentication is identical across both implementations.

**Variable:** `C3D_DEVELOPER_API_KEY`

**Sources, in precedence order:**

1. Already-exported environment variable (e.g. `export C3D_DEVELOPER_API_KEY=...` or `$env:C3D_DEVELOPER_API_KEY=...`).
2. `.env` file in the working directory, loaded automatically:
   - Bash: `load_env_file` in `upload-utils.sh:32-77` parses `KEY=value`, skips blank lines and lines beginning with `#`, validates keys against `^[A-Za-z_][A-Za-z0-9_]*$`, and never overwrites an already-set variable.
   - PowerShell: `Import-C3DEnvironment` in `C3DUploadTools/Private/Core/Import-C3DEnvironment.ps1` does the same and is invoked from `C3DUploadTools.psm1:52` on module import. Quoted values are unquoted; already-set env vars are preserved.

**Validation:**

- Bash: `validate_api_key` (`upload-utils.sh:100-106`) fails fast with `log_error` + `exit 1` when unset.
- PowerShell: `Test-C3DApiKey` (`C3DUploadTools/Private/Validation/Test-C3DApiKey.ps1`) checks for empty, minimum length 10, and rejects known placeholder strings (`your_api_key`, `test`, `example`, `placeholder`, `change_me`). `Get-C3DApiKey` calls `Test-C3DApiKey -Throw` then returns the value.

**Usage on the wire:** The key is sent as `Authorization: APIKEY:DEVELOPER <key>` (not `Bearer`). Bash builds the header directly in each `curl` call (e.g. `upload-scene.sh:342`); PowerShell builds it in `Set-C3DRequestHeaders` because `Invoke-WebRequest` rejects the custom format — see CLAUDE.md "PowerShell HTTP Client Implementation".

**Security:**

- API key never logged or echoed. Dry-run output redacts it explicitly: `Authorization: APIKEY:DEVELOPER [REDACTED]` (`upload-scene.sh:291`, `Upload-C3DScene.ps1:352`).
- `.env`, `.env.*`, `.env.backup`, `.env.sample.dev`, `.env.sample.prod`, `.env.webxr*` are all `.gitignore`d.
- Sample env files (`.env.example`, `.env.sample.dev`, `.env.sample.prod`) contain placeholder values only.

## Settings.json Auto-Generation

Both implementations generate or patch `<scene_dir>/settings.json` immediately before upload. The file always contains at least:

```json
{
  "scale": 1,
  "sceneName": "<scene name>",
  "sdkVersion": "cli-bash-v<sdkVersion>"      // bash
  "sdkVersion": "cli-powershell-v<sdkVersion>" // powershell
}
```

**SDK version source:** `sdk-version.txt` at the repo root (currently `1.1.0`). Validated against `^[0-9]+\.[0-9]+\.[0-9]+$` (semantic versioning) in both languages:

- Bash: `validate_semantic_version` (`upload-utils.sh:321-330`), wrapped to `cli-bash-v${SDK_VERSION}` (`upload-scene.sh:213-219`).
- PowerShell: inline regex `$sdkVersion -notmatch '^\d+\.\d+\.\d+$'`, wrapped to `cli-powershell-v$sdkVersion` (`Upload-C3DScene.ps1:267-278`).

**Update vs. create semantics:**

- **Bash** (`upload-scene.sh:242-272`): if `settings.json` already exists, the script preserves all existing fields and patches only `sdkVersion` and `sceneName` via `jq '.sdkVersion = $sdkVersion | .sceneName = $sceneName'`. If absent, generates a minimal `{scale: 1, sceneName, sdkVersion}` document with `jq -n`. When `--scene_name` is omitted on an update, the existing `sceneName` is preserved (or defaults to `"Scene"` if missing).
- **PowerShell** (`Upload-C3DScene.ps1:285-319`): warns if an existing `settings.json` is present, then overwrites with a freshly-built `@{ scale = 1; sceneName = $settingsSceneName; sdkVersion = $fullSdkVersion } | ConvertTo-Json -Depth 10`. Defaults `sceneName` to `"Scene"` when `-SceneName` is empty.

**Dry-run:** Both implementations print the would-be contents of `settings.json` and skip the file write entirely when `--dry_run` / `-DryRun` is set.

**Git:** `**/settings.json` is `.gitignore`d (it is regenerated by every upload).

## Validation Conventions (Shared Behavior)

| Concern | Bash helper (`upload-utils.sh`) | PowerShell helper |
|---------|----------------------------------|-------------------|
| UUID format | `validate_uuid_format` line 178 — regex `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$` | `Test-C3DUuidFormat` in `Private/Validation/Test-C3DUuidFormat.ps1` (case-insensitive); `ConvertTo-C3DLowerUuid` normalizes case |
| Semver | `validate_semantic_version` line 321 — regex `^[0-9]+\.[0-9]+\.[0-9]+$` | inline regex in `Upload-C3DScene.ps1:273` |
| File size cap (100 MB) | `validate_file` line 270 (default `max_size_mb=100`) — BSD `stat -f%z` then GNU `stat -c%s` fallback | `[ValidateScript]` block + `Test-C3DFile -MaxSizeBytes 100MB` in `Private/Validation/Test-C3DFileSystem.ps1` |
| Directory exists | `validate_directory` line 296 | `Test-C3DDirectory -RequiredFiles` checks both dir and required-file membership |
| Environment value | `validate_environment` line 169 — must be `prod` or `dev` | `[ValidateSet('prod','dev')]` attribute |
| API key | `validate_api_key` line 100 | `Test-C3DApiKey` / `Get-C3DApiKey` |
| Required files | inline checks per script (e.g. `scene.bin`, `scene.gltf`, `screenshot.png`, `cvr_object_thumbnail.png`) | declared in `[ValidateScript]` block of public function |

Every validation failure exits non-zero (bash) or throws a terminating error (PowerShell). Validation runs **before** any network call.

## API Conventions

### Base URLs

- **prod:** `https://data.cognitive3d.com/v0/<endpoint>`
- **dev:** `https://data.c3ddev.com/v0/<endpoint>`

Bash: `get_api_base_url <env> <endpoint>` (`upload-utils.sh:150-166`, default endpoint `scenes`).
PowerShell: `Get-C3DApiUrl -Environment <env> -EndpointType <endpoint>` (`Private/Api/Get-C3DApiUrl.ps1`) plus `[C3DConfiguration]::GetApiUrl()`.

### Endpoints

| Operation | Method + path |
|-----------|---------------|
| Get/create scene version | `GET /v0/scenes/{sceneId}` |
| Create scene | `POST /v0/scenes` |
| Update scene | `POST /v0/scenes/{sceneId}` |
| Upload screenshot | `POST /v0/scenes/{sceneId}/screenshot?version={versionNumber}` |
| Upload object | `POST /v0/objects/{sceneId}/{objectId}?version={versionNumber}` |
| Upload object manifest | `POST /v0/objects/{sceneId}?version={versionNumber}` |
| List objects | `GET /v0/objects/{sceneId}` |

Convention: when updating versioned content (objects, manifests, screenshots) always read the current scene version first via `get_scene_version` / equivalent, then pass `?version={versionNumber}` on the upload. The Unity SDK reference comments in each script (`upload-scene.sh:5-37`, `upload-object.sh:5-27`, `upload-object-manifest.sh:5-22`) document the exact Unity files this mirrors.

### Multipart Uploads

- Bash uses `curl --form 'name=@file'`, assembled as an array `CURL_CMD=(curl ...)` to preserve argument boundaries (`upload-scene.sh:341-357`).
- PowerShell builds multipart bodies manually using `System.Net.WebClient` (not `Invoke-WebRequest`) because PowerShell's web cmdlet rejects the `APIKEY:DEVELOPER` Authorization scheme. Helpers: `Invoke-C3DApiRequest`, `Send-C3DHttpRequest`, `New-C3DMultipartFormData`, `Set-C3DRequestHeaders` under `C3DUploadTools/Private/Api/`.

## Comments

### When to Comment

- File header block at the top of every script/module describing purpose, API flow, and Unity SDK file:line references (see `upload-scene.sh:1-37`, `upload-object.sh:1-27`, `upload-object-manifest.sh:1-22`).
- Section banners using `# === HEADING ===` or `# ----------` separate major regions inside long scripts.
- Single-line `# ...` comments explain *why* (not what) above non-obvious logic, especially around regex patterns and Unity-parity decisions.
- PowerShell functions always carry comment-based help (`<# .SYNOPSIS ... #>`).

### Help Output (`--help`)

- Bash: `--help|-h` case branches print usage, file requirements, environment variable docs, and worked examples directly (e.g. `upload-object.sh:78-118` uses a here-doc `cat <<'EOF'`).
- PowerShell: relies on `Get-Help <Function-Name>` powered by the comment-based help. Each public function provides `.EXAMPLE` blocks demonstrating: minimal usage, advanced usage with optional params, dry-run preview, and a complete workflow chain to the next cmdlet (`Upload-C3DScene.ps1:37-75`).

## Function Design

### Bash

- Helpers in `upload-utils.sh` follow single-responsibility (one validation, one HTTP parse, one URL builder, etc.).
- Public entry-point scripts wrap their logic in a `main()` function called at the end with `main "$@"` (`upload-scene.sh:53,427`; same in object and manifest scripts).
- `local` is used inside helpers and `main` for any new variable that should not leak (`local start_time`, `local upload_duration`, ...).
- Helpers usually `exit 1` on validation failure (terminal); helpers that may legitimately fail return `0`/`1` and let the caller decide (e.g. `get_scene_version` returns 1 on HTTP 404, see line 139).

### PowerShell

- One function per file under `Public/` or `Private/<Category>/` (Core, Validation, Api, Utilities).
- Public functions are kept focused on orchestration; heavy lifting is delegated to `Private/` helpers.
- Functions use `[CmdletBinding()]` so they get common parameters (`-Verbose`, `-ErrorAction`, etc.) and proper `$PSCmdlet` access for `ThrowTerminatingError`.
- State-modifying public functions add `SupportsShouldProcess` to honor `-WhatIf` / `-Confirm` (`Upload-C3DScene.ps1:122`).
- Functions return either a typed object (`[C3DApiResponse]`, `[C3DUploadResult]`) or `$true`/`$false` for predicates. No bare hashtables.

## Module Design

### Bash

- `upload-utils.sh` is the single shared library. Adding a new shell helper means appending a function to it and re-sourcing in scripts that need it.
- `test-scripts/test-utils.sh` is the parallel shared library for tests (colors, pass/fail counters, scene-ID extraction, timestamp updater).

### PowerShell

- `C3DUploadTools.psm1` is the only loader; never `Import-Module` private helpers directly from outside the module.
- Both `FunctionsToExport` in `C3DUploadTools.psd1` and `Export-ModuleMember` in `C3DUploadTools.psm1` are kept in sync. Public functions are: `Upload-C3DScene`, `Upload-C3DObject`, `Upload-C3DObjectManifest`, `Get-C3DObjects`, `Test-C3DUploads`.
- Module manifest declares `PowerShellVersion = '5.1'` and `CompatiblePSEditions = @('Desktop','Core')` so it works on Windows PowerShell 5.1+, PowerShell 7.x, macOS, and Linux.
- Private helpers are loaded recursively from `Private/`, so new categories can be added as new subdirectories (currently `Core/`, `Validation/`, `Api/`, `Utilities/`) without touching the loader.

## Cross-Platform Considerations

- Bash uses `stat -f%z "$file" 2>/dev/null || stat -c%s "$file" 2>/dev/null` to support both macOS (BSD) and Linux (GNU) without branching (`upload-utils.sh:284`).
- PowerShell uses `Join-Path`, `Get-Item`, `Test-Path` (never string concatenation) so paths normalize on Windows, macOS, and Linux. The `[ValidateScript]` UUID regex is case-insensitive; lowercase normalization happens in `ConvertTo-C3DLowerUuid` before each request.
- Test scripts under `test-scripts/` and `C3DUploadTools/Tests/` all `cd "$(dirname "$0")/.."` (bash) or rely on `$PSScriptRoot` (PowerShell) so they can be invoked from anywhere.

---

*Convention analysis: 2026-05-12*
