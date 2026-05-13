# Technology Stack

**Snapshot date:** 2026-05-12. Line numbers and file references reflect codebase state at snapshot time and will drift as code changes. Future `/gsd-map-codebase` refreshes will regenerate this file; see `CONCERNS.md` for the audit trail of post-snapshot resolutions.

## Languages

**Primary:**
- Bash (POSIX shell + GNU/BSD bash extensions) — Cross-platform CLI scripts for macOS and Linux. Entry points: `upload-scene.sh`, `upload-object.sh`, `upload-object-manifest.sh`, `list-objects.sh`. Shared helpers in `upload-utils.sh`.
- PowerShell — Windows-first module providing the same functionality without external dependencies. Module root: `C3DUploadTools/`. Manifest: `C3DUploadTools/C3DUploadTools.psd1`. Loader: `C3DUploadTools/C3DUploadTools.psm1`.

**Secondary:**
- JSON — Used for the auto-generated `settings.json` (per-scene config: `scale`, `sceneName`, `sdkVersion`) and per-scene `<sceneId>_object_manifest.json` files (e.g. `0fedaede-3592-43b8-968b-87573e469620_object_manifest.json`).

## Runtime

**Environment:**
- Bash — System bash (uses `#!/bin/bash`, `set -e`, `set -u`, `[[ ]]` tests, arrays, `${var:-default}` expansions). Confirmed compatible with macOS (BSD `stat -f%z`) and Linux (GNU `stat -c%s`) via fallback logic in `upload-utils.sh:284`.
- PowerShell — `PowerShellVersion = '5.1'` and `CompatiblePSEditions = @('Desktop', 'Core')` declared in `C3DUploadTools/C3DUploadTools.psd1:16-19`. `Set-StrictMode -Version 3.0` and `$ErrorActionPreference = 'Stop'` enforced by `C3DUploadTools/C3DUploadTools.psm1:7-10`. Targets Windows PowerShell 5.1+ and PowerShell Core 7.x.

**Package Manager:**
- None. No `package.json`, `requirements.txt`, `Cargo.toml`, `go.mod`, or `pyproject.toml` is present.
- PowerShell module is loaded directly via `Import-Module ./C3DUploadTools -Force` (no PSGallery install required). The `.psd1` declares PSGallery metadata (`Tags`, `LicenseUri`, `ProjectUri`) for future publication.
- Lockfile: Not applicable.

## Frameworks

**Core:**
- None — Plain bash and plain PowerShell. No web framework, no ORM, no DI container.

**Testing:**
- Bash test scripts under `test-scripts/` (`test-all.sh`, `test-env-workflow.sh`, `test-object-upload-improvements.sh`, `test-scene-upload-improvements.sh`, `test-utils.sh`) — hand-rolled, no test framework. Invoked directly: `./test-all.sh <scene_id> <env>`.
- PowerShell test scripts under `C3DUploadTools/Tests/` (`test-module-structure.ps1`, `test-utilities-internal.ps1`, `test-scene-upload.ps1`, `test-object-upload.ps1`, `test-core-utilities.ps1`, `Test-EnvWorkflow.ps1`, `test-http-headers.ps1`) — hand-rolled, no Pester. Invoked via `pwsh -File <path>`.

**Build/Dev:**
- None. Scripts run in-place; the PowerShell module loads directly from source.

## Key Dependencies

**Critical (external CLI tools — bash side only):**
- `jq` — JSON manipulation. Required for all bash scripts. Checked at startup by `check_dependencies()` in `upload-utils.sh:80-97`; missing dependency exits with an error.
- `curl` — HTTP client for all REST calls. Required for all bash scripts. Checked alongside `jq` in `upload-utils.sh:80-97`. Specific flags used: `--silent`, `--write-out "\n%{http_code}"`, `--location`, `--form`, `--header`, `--data-binary`, `--globoff` (in `upload-object.sh:256` to disable URL globbing on object IDs).
- `uuidgen` — Used by `upload-object.sh:182,186` to generate object UUIDs when `--object_id` is not supplied. Standard on macOS and most Linux distros.
- Standard POSIX utilities: `stat`, `sed`, `basename`, `date`, `tr`, `head`, `tail`, `command`, `printf`, `cat`.

**Critical (PowerShell side — all native .NET, no external):**
- `System.Net.WebClient` — Used for multipart uploads (`Send-C3DHttpRequest.ps1:77`). Chosen because PowerShell's `Invoke-WebRequest` rejects the non-standard `APIKEY:DEVELOPER` Authorization header format.
- `System.Net.HttpWebRequest` — Used for non-multipart JSON/GET requests (`Send-C3DHttpRequest.ps1:141`).
- `System.Net.HttpRequestHeader` enum — Used to set `User-Agent` on `WebClient` (which is a restricted .NET header that cannot be set via `Headers.Add()`); see `Set-C3DRequestHeaders.ps1:28-43`.
- `System.IO.File` — `ReadAllBytes` for streaming file bodies into multipart payloads (`New-C3DMultipartFormData.ps1:82`).
- `System.Text.Encoding`, `System.Guid` — Multipart boundary construction.
- Native cmdlets: `ConvertFrom-Json`, `ConvertTo-Json`, `Get-Item`, `Test-Path`, `Get-Content`, `Out-File`, `Write-Progress`.

**Infrastructure:**
- None (no databases, no message queues, no caching layer).

## Configuration

**Environment:**
- `.env` file at repo root (git-ignored via `.gitignore:8-16`, including `.env`, `.env.dev`, `.env.prod`, `.env.backup`, `.env*`). Template at `.env.example`.
- Bash loads `.env` via `load_env_file()` in `upload-utils.sh:32-77` — line-by-line parser that respects existing env vars (does not overwrite) and validates key format `^[A-Za-z_][A-Za-z0-9_]*$`.
- PowerShell loads `.env` via `Import-C3DEnvironment` in `C3DUploadTools/Private/Core/Import-C3DEnvironment.ps1`, called automatically on module import (`C3DUploadTools.psm1:52`). Supports surrounding quotes; preserves existing env vars.

**Key configs required:**
- `C3D_DEVELOPER_API_KEY` (required) — Cognitive3D developer API key. Validated by `validate_api_key()` in `upload-utils.sh:100-106` and `Test-C3DApiKey` in `C3DUploadTools/Private/Validation/Test-C3DApiKey.ps1`.
- `C3D_SCENE_ID` (optional) — Default scene UUID; lets users omit `--scene_id` / `-SceneId` on object operations. Consumed in `upload-object.sh:43`, `upload-object-manifest.sh:34`, `list-objects.sh:88`, and `Get-C3DObjects.ps1:100`.
- `C3D_DEFAULT_ENVIRONMENT` (optional, default `prod`) — Default `--env` value. Used in `upload-scene.sh:56`, `upload-object.sh:39`, `upload-object-manifest.sh:35`.
- `C3D_ENV` (optional) — Alternative environment variable name supported only by `list-objects.sh:96`.
- `C3D_VERBOSE`, `C3D_MAX_FILE_SIZE_MB`, `C3D_SCENE_VERSION_ID` — Documented in `.env.example` but not actively consumed by any script (placeholders for future use).

**Build:**
- No build step. Bash scripts are made executable directly (`chmod +x`). PowerShell module is loaded with `Import-Module ./C3DUploadTools -Force`.

## Platform Requirements

**Development:**
- macOS or Linux for bash scripts. Requires `jq` and `curl` on PATH.
- Windows / macOS / Linux for PowerShell module. Requires PowerShell 5.1+ (Windows PowerShell) or PowerShell 7.x (PowerShell Core).
- A valid Cognitive3D developer API key from the Cognitive3D dashboard (`Settings → Manage developer key`).

**Production:**
- Not deployed as a service — this is a developer-tooling repo. "Production" means running scripts against the live Cognitive3D API (`data.cognitive3d.com`). The `dev` environment (`data.c3ddev.com`) exists for testing.

## Repo Configuration Files

- `sdk-version.txt` (1 line, current value `1.1.0`) — Single-line semantic version consumed by both bash (`upload-scene.sh:207-220`) and the PowerShell `Upload-C3DScene.ps1`. The script prefixes it with `cli-bash-v` or `cli-powershell-v` before injecting into `settings.json` (e.g. `cli-bash-v1.1.0`, `cli-powershell-v1.1.0`). Validated against `^[0-9]+\.[0-9]+\.[0-9]+$` by `validate_semantic_version()` in `upload-utils.sh:321-330`.
- `settings.json` (auto-generated per scene directory; **not** committed — git-ignored at `.gitignore:5` as `**/settings.json`). Generated by `upload-scene.sh:262-272` with shape `{ "scale": 1, "sceneName": <name>, "sdkVersion": <prefixed> }`. On updates, existing fields are preserved and only `sdkVersion` + `sceneName` are patched via `jq` (`upload-scene.sh:252-261`). Sample: `scene-test/settings.json`.
- `cspell.json` — Code Spell Checker config (VS Code extension). `language: en-ca`. Custom dictionary includes `APIKEY`, `globoff`, `gltf`, `visualstudio`. Flagged words: `hte`.
- `.gitattributes` — Single line: `* text=auto` (LF normalization).
- `.gitignore` — Excludes `.DS_Store`, `*object*.json` (per-scene manifest/list outputs), `**/settings.json`, and the entire `.env*` family.
- `.claude/settings.local.json` — Local Claude Code settings (not part of the application).

## PowerShell Module Manifest

`C3DUploadTools/C3DUploadTools.psd1` declares:
- `RootModule = 'C3DUploadTools.psm1'`
- `ModuleVersion = '1.0.0'`
- `GUID = 'f4e6d8c2-1a3b-4e5f-8c7d-2e9f1a6b3c4d'`
- `Author = 'Cognitive3D'`, `CompanyName = 'Cognitive3D'`
- `FunctionsToExport`: `Upload-C3DScene`, `Upload-C3DObject`, `Upload-C3DObjectManifest`, `Get-C3DObjects`, `Test-C3DUploads`
- `CmdletsToExport = @()`, `VariablesToExport = @()`, `AliasesToExport = @()`
- PSData tags: `Cognitive3D`, `VR`, `Analytics`, `Upload`, `Scene`, `Objects`, `Cross-Platform`
- `ProjectUri = 'https://github.com/cognitive3d/c3d-upload-tools'`
- `HelpInfoURI = 'https://docs.cognitive3d.com/upload-tools/powershell'`

The `.psm1` (`C3DUploadTools/C3DUploadTools.psm1`) dot-sources all `Private/**/*.ps1` then all `Public/**/*.ps1`, then calls `Export-ModuleMember -Function <discovered names>` (dynamic discovery, overriding the manifest's static export list) and finally invokes `Import-C3DEnvironment` to auto-load `.env`.

### Module Directory Layout

```
C3DUploadTools/
├── C3DUploadTools.psd1           # Manifest
├── C3DUploadTools.psm1           # Loader (dot-sources Private/, then Public/)
├── Public/                       # 5 exported functions
│   ├── Upload-C3DScene.ps1
│   ├── Upload-C3DObject.ps1
│   ├── Upload-C3DObjectManifest.ps1
│   ├── Get-C3DObjects.ps1
│   └── Test-C3DUploads.ps1       # Placeholder
├── Private/
│   ├── Core/                     # Logging, classes, .env loader, error records
│   │   ├── C3DClasses.ps1        # C3DConfiguration, C3DUploadRequest, C3DApiResponse, C3DUploadResult
│   │   ├── Import-C3DEnvironment.ps1
│   │   ├── Initialize-C3DModule.ps1
│   │   ├── New-C3DErrorRecord.ps1
│   │   └── Write-C3DLog.ps1
│   ├── Api/                      # HTTP layer
│   │   ├── Get-C3DApiUrl.ps1
│   │   ├── Invoke-C3DApiRequest.ps1
│   │   ├── New-C3DMultipartFormData.ps1
│   │   ├── Send-C3DHttpRequest.ps1
│   │   └── Set-C3DRequestHeaders.ps1
│   ├── Validation/
│   │   ├── Test-C3DApiKey.ps1
│   │   ├── Test-C3DFileSystem.ps1
│   │   └── Test-C3DUuidFormat.ps1
│   └── Utilities/
│       └── New-C3DUploadSession.ps1
└── Tests/                        # Hand-rolled validation scripts
```

---

*Stack analysis: 2026-05-12*
