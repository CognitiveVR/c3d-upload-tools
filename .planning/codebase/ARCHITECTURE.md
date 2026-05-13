<!-- refreshed: 2026-05-12 -->
# Architecture

**Snapshot date:** 2026-05-12. Line numbers and file references reflect codebase state at snapshot time and will drift as code changes. Future `/gsd-map-codebase` refreshes will regenerate this file; see `CONCERNS.md` for the audit trail of post-snapshot resolutions.

## System Overview

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                         User / Operator                                      │
│            (CLI args + .env file + environment variables)                    │
└──────────────────────────┬───────────────────────┬──────────────────────────┘
                           │                       │
                  macOS / Linux                Windows / Cross-platform
                           │                       │
                           ▼                       ▼
┌──────────────────────────────────┐  ┌────────────────────────────────────────┐
│  Bash entry-point scripts (root) │  │  PowerShell module                      │
│  `upload-scene.sh`               │  │  `C3DUploadTools/C3DUploadTools.psm1`   │
│  `upload-object.sh`              │  │  Public cmdlets:                        │
│  `upload-object-manifest.sh`     │  │   `Upload-C3DScene`                     │
│  `list-objects.sh`               │  │   `Upload-C3DObject`                    │
│                                  │  │   `Upload-C3DObjectManifest`            │
│                                  │  │   `Get-C3DObjects`                      │
│                                  │  │   `Test-C3DUploads`                     │
└──────────────┬───────────────────┘  └─────────────────┬──────────────────────┘
               │                                        │
               ▼                                        ▼
┌──────────────────────────────────┐  ┌────────────────────────────────────────┐
│  Shared bash utilities           │  │  PowerShell Private/ subsystems         │
│  `upload-utils.sh`               │  │   Core/      logging, env, classes      │
│   - load_env_file                │  │   Validation/  UUID, files, API key     │
│   - validate_*                   │  │   Api/        URLs, multipart, HTTP     │
│   - get_scene_version            │  │   Utilities/  upload session helpers    │
│   - parse_http_response          │  │  Classes (`C3DClasses.ps1`):            │
│   - handle_http_error            │  │   `C3DConfiguration`                    │
│   - upload_screenshot            │  │   `C3DUploadRequest`                    │
│  External deps: `curl`, `jq`,    │  │   `C3DApiResponse`                      │
│  `uuidgen`, `stat`, `sed`        │  │   `C3DUploadResult`                     │
│                                  │  │  HTTP: `System.Net.WebClient` (native)  │
└──────────────┬───────────────────┘  └─────────────────┬──────────────────────┘
               │                                        │
               └────────────────────┬───────────────────┘
                                    ▼
            ┌────────────────────────────────────────────────────┐
            │           Cognitive3D REST API                      │
            │  prod: `https://data.cognitive3d.com/v0`            │
            │  dev:  `https://data.c3ddev.com/v0`                 │
            │  Auth: `Authorization: APIKEY:DEVELOPER <key>`      │
            │  Endpoints:                                         │
            │   GET   /scenes/{sceneId}                           │
            │   POST  /scenes  or  /scenes/{sceneId}              │
            │   POST  /scenes/{sceneId}/screenshot?version=N      │
            │   POST  /objects/{sceneId}/{objectId}?version=N     │
            │   POST  /objects/{sceneId}?version=N (manifest)     │
            │   GET   /versions/{versionId}/objects               │
            └─────────────────────────┬──────────────────────────┘
                                      │
                                      ▼
            ┌────────────────────────────────────────────────────┐
            │   Local generated artifacts (repo root)             │
            │   `<sceneId>_object_manifest.json`                  │
            │   `<sceneId>_object_list.json`                      │
            │   `<sceneDir>/settings.json` (auto-generated)       │
            └────────────────────────────────────────────────────┘
```

## Component Responsibilities

| Component | Responsibility | File |
|-----------|----------------|------|
| Scene uploader (bash) | Multipart POST of `scene.bin`/`scene.gltf`/`screenshot.png`/`settings.json`; auto-generate `settings.json`; separate screenshot POST | `upload-scene.sh` |
| Object uploader (bash) | Per-object multipart POST; accumulate local `<sceneId>_object_manifest.json`; idempotent object IDs | `upload-object.sh` |
| Manifest uploader (bash) | JSON POST of accumulated manifest with `?version=` parameter | `upload-object-manifest.sh` |
| Scene object lister (bash) | GET scene → extract latest `versionId` → GET objects, persist `<sceneId>_object_list.json` and reshape into manifest format | `list-objects.sh` |
| Shared bash library | Logging, env loading, dependency check, UUID/semver/file/dir validation, HTTP response parsing, HTTP error handling, screenshot upload | `upload-utils.sh` |
| PowerShell module loader | Dot-source all `Private/**/*.ps1` and `Public/**/*.ps1`; export public functions; auto-load `.env` | `C3DUploadTools/C3DUploadTools.psm1` |
| PowerShell manifest | Module metadata, declared exports, PS 5.1 + Core compatibility | `C3DUploadTools/C3DUploadTools.psd1` |
| PowerShell classes | `C3DConfiguration`, `C3DUploadRequest`, `C3DApiResponse`, `C3DUploadResult` plus `New-C3DConfiguration` factory | `C3DUploadTools/Private/Core/C3DClasses.ps1` |
| PowerShell env loader | Parse `.env` and `Set-Item env:` without overriding existing variables | `C3DUploadTools/Private/Core/Import-C3DEnvironment.ps1` |
| PowerShell HTTP client | `System.Net.WebClient`-based multipart POST that bypasses `Invoke-WebRequest` Authorization-header validation | `C3DUploadTools/Private/Api/Send-C3DHttpRequest.ps1` |
| PowerShell multipart builder | Hand-rolled boundary construction for binary-safe multipart form data | `C3DUploadTools/Private/Api/New-C3DMultipartFormData.ps1` |
| PowerShell request orchestrator | Combine URL, headers, multipart body, and response into `C3DApiResponse` | `C3DUploadTools/Private/Api/Invoke-C3DApiRequest.ps1` |

## Pattern Overview

**Overall:** Dual parallel CLI/SDK implementations sharing the same Cognitive3D REST contract — flat bash scripts for POSIX shells, structured PowerShell module for Windows.

**Key Characteristics:**

- Two independent implementations, no shared runtime; only the wire protocol and `.env` schema are common.
- Stateless scripts/cmdlets: every invocation re-loads `.env`, re-resolves scene version, and POSTs.
- Local filesystem is the only persistent state between invocations (`<sceneId>_object_manifest.json` accumulates objects across `upload-object` calls).
- All operations are HTTPS multipart/JSON POSTs against `data.cognitive3d.com` (prod) or `data.c3ddev.com` (dev); auth header is the literal string `APIKEY:DEVELOPER <key>` (non-standard scheme).
- Bash side shells out to `curl`, `jq`, `uuidgen`, `stat`; PowerShell side is dependency-free, relying on .NET BCL.

## Layers

**Entry layer (CLI/cmdlet):**

- Purpose: parse arguments, load `.env`, validate inputs, drive the workflow.
- Locations: repo-root `*.sh`; `C3DUploadTools/Public/*.ps1`.
- Contains: argument parsing, top-level orchestration, usage/help text.
- Depends on: shared utilities / private functions.
- Used by: human operator and `test-scripts/*.sh` / `C3DUploadTools/Tests/*.ps1`.

**Shared utilities / Private layer:**

- Purpose: logging, validation, HTTP, multipart construction, error mapping.
- Locations: `upload-utils.sh`; `C3DUploadTools/Private/{Core,Validation,Api,Utilities}/`.
- Contains: pure helpers, classes, the actual HTTP client.
- Depends on: external CLIs (`curl`, `jq`) for bash; .NET `System.Net.WebClient` and `ConvertFrom-Json` for PowerShell.
- Used by: entry layer.

**Generated artifact layer:**

- Purpose: persist scene/object state locally between invocations.
- Location: repo root.
- Contains: `<sceneId>_object_manifest.json` (built by `upload-object.sh` / `Upload-C3DObject`); `<sceneId>_object_list.json` (built by `list-objects.sh`); per-scene-dir `settings.json` (built by `upload-scene.sh` / `Upload-C3DScene`).
- Depends on: scene ID returned by the API.
- Used by: subsequent `upload-object-manifest.sh` / `Upload-C3DObjectManifest` calls.

## Data Flow

### Primary Request Path — scene → objects → manifest

1. Operator runs `./upload-scene.sh --scene_dir <dir> --scene_name <name> [--env dev|prod]` (`upload-scene.sh:53` onward).
2. Shared utilities load `.env`, validate UUID/semver/files, and check `curl`/`jq` (`upload-utils.sh:80`, `upload-utils.sh:178`).
3. If `--scene_id` provided, `get_scene_version` issues `GET /v0/scenes/{sceneId}` and extracts latest `versionNumber`/`id` (`upload-utils.sh:112`).
4. `settings.json` is generated or patched in place with `sdkVersion = cli-bash-v<sdk-version.txt>` and `sceneName` (`upload-scene.sh:243-272`).
5. Multipart POST to `/v0/scenes` (new) or `/v0/scenes/{sceneId}` (update) with `scene.bin`, `scene.gltf`, `screenshot.png`, additional textures, `settings.json` (`upload-scene.sh:341-357`).
6. On HTTP 201, response body is the new scene ID; printed both human-readable and on stdout for capture (`upload-scene.sh:371-387`).
7. `upload_screenshot` performs a second POST to `/v0/scenes/{sceneId}/screenshot?version=N` (`upload-utils.sh:335`).
8. Operator (or test harness) then loops `./upload-object.sh --scene_id <id> --object_filename <name> --object_dir <dir>` for each object (`upload-object.sh:48` onward).
9. Each object upload re-fetches the scene version, POSTs to `/v0/objects/{sceneId}/{objectId}?version=N`, and merges a new entry into `<sceneId>_object_manifest.json` via `jq` (`upload-object.sh:198-347`).
10. Object IDs are stable: when `--object_id` is omitted, the script reuses any existing manifest entry whose `mesh` matches `OBJECT_FILENAME`, otherwise generates a UUID via `uuidgen` (`upload-object.sh:172-189`).
11. Finally `./upload-object-manifest.sh` POSTs the accumulated JSON file to `/v0/objects/{sceneId}?version=N` (`upload-object-manifest.sh:125-167`).

### PowerShell Flow

1. `Import-Module ./C3DUploadTools -Force` dot-sources every file under `Private/` and `Public/`, then exports public cmdlets and calls `Import-C3DEnvironment` to load `.env` (`C3DUploadTools/C3DUploadTools.psm1:15-52`).
2. A public cmdlet (e.g., `Upload-C3DScene`) validates parameters with `[ValidateScript()]`, constructs a `C3DConfiguration` via `New-C3DConfiguration` (`C3DUploadTools/Private/Core/C3DClasses.ps1:257`), and resolves the API URL with `Get-C3DApiUrl`.
3. `New-C3DMultipartFormData` builds a byte-array body with a manually generated boundary.
4. `Send-C3DHttpRequest` (with `-UseWebClient`) attaches the `APIKEY:DEVELOPER <key>` header and POSTs (`C3DUploadTools/Private/Api/Send-C3DHttpRequest.ps1`).
5. `Invoke-C3DApiRequest` wraps the raw response into `C3DApiResponse`, surfacing `StatusCode`, `Body`, `TimingMs`, and `Success`.
6. The cmdlet maps the response into a `C3DUploadResult` and emits it; downstream cmdlets (`Upload-C3DObject`, `Upload-C3DObjectManifest`) follow the same pipeline.

### State Management

- Scene version is **always re-fetched from the API** before object and manifest uploads — never cached locally.
- The manifest file at `<sceneId>_object_manifest.json` is the only persisted state between runs; it is treated as authoritative for object-ID reuse.
- `.env` is loaded on every script invocation; existing process env vars always win over `.env` values.

## Key Abstractions

**Scene workflow (bash + PowerShell):**

- Purpose: model "upload scene → upload N objects → upload manifest" as three serial steps that can each be retried independently.
- Examples: `upload-scene.sh`, `upload-object.sh`, `upload-object-manifest.sh`; `C3DUploadTools/Public/Upload-C3DScene.ps1`, `Upload-C3DObject.ps1`, `Upload-C3DObjectManifest.ps1`.
- Pattern: each step is idempotent given a stable scene ID and stable object IDs.

**Shared bash helpers (`upload-utils.sh`):**

- Purpose: avoid duplication between the four bash entry scripts.
- Functions: `load_env_file`, `check_dependencies`, `validate_api_key`, `get_scene_version`, `get_api_base_url`, `validate_environment`, `validate_uuid_format`, `parse_http_response`, `handle_http_error`, `log_execution_time`, `validate_file`, `validate_directory`, `process_json_response`, `validate_semantic_version`, `upload_screenshot`.
- Pattern: every entry script does `SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)" && source "$SCRIPT_DIR/upload-utils.sh"` then `load_env_file`.

**PowerShell classes (`C3DUploadTools/Private/Core/C3DClasses.ps1`):**

- `C3DConfiguration` — holds API key, environment URLs (`prod` and `dev`), default scene ID, timeout, user agent; provides `GetApiUrl()` and `IsValid()`.
- `C3DUploadRequest` — bundles URI, method, files, headers, content type, body; `AddFile()`, `AddHeader()`, `GetTotalFileSize()`, `GetRequestType()`.
- `C3DApiResponse` — wraps status code, body, timing, success flag; `GetJsonBody()`, `IsAuthenticationError()`, `IsNotFound()`, `IsRateLimited()`, `IsServerError()`.
- `C3DUploadResult` — operation type + `C3DApiResponse` + uploaded-file summary + next-step hints; `GetSummary()`.

**Generated manifest format:**

- File: `<sceneId>_object_manifest.json` at repo root (gitignored via `*object*.json`).
- Shape: `{ "objects": [ { "id": "<uuid>", "mesh": "<filename>", "name": "<filename>", "scaleCustom": [1,1,1], "initialPosition": [0,0,0], "initialRotation": [0,0,0,1] } ] }` (`upload-object.sh:307-316`).

## Entry Points

**Bash CLI:**

- `upload-scene.sh` — scene upload + screenshot upload; required `--scene_dir`, `--scene_name` (new scenes only).
- `upload-object.sh` — per-object upload + manifest accumulation; required `--object_filename`, `--object_dir`; `--scene_id` falls back to `C3D_SCENE_ID`.
- `upload-object-manifest.sh` — manifest POST; `--scene_id` falls back to `C3D_SCENE_ID`.
- `list-objects.sh` — read-only listing; writes `<sceneId>_object_list.json` and `<sceneId>_object_manifest.json`.
- `test-scripts/test-all.sh` — composite test runner that `cd`s to repo root and invokes the upload scripts in sequence.

**PowerShell cmdlets (exported from `C3DUploadTools.psd1`):**

- `Upload-C3DScene` (`C3DUploadTools/Public/Upload-C3DScene.ps1`)
- `Upload-C3DObject` (`C3DUploadTools/Public/Upload-C3DObject.ps1`)
- `Upload-C3DObjectManifest` (`C3DUploadTools/Public/Upload-C3DObjectManifest.ps1`)
- `Get-C3DObjects` (`C3DUploadTools/Public/Get-C3DObjects.ps1`)
- `Test-C3DUploads` (`C3DUploadTools/Public/Test-C3DUploads.ps1`, currently a placeholder)

**Triggers:**

- Manual operator invocation from a terminal.
- `test-scripts/*.sh` and `C3DUploadTools/Tests/*.ps1` for regression testing.

## Architectural Constraints

- **Threading:** Single-threaded throughout; bash scripts run sequentially, PowerShell cmdlets are synchronous; no background jobs or parallel uploads.
- **Global state:**
  - Bash: `set -e` and `set -u` in `upload-utils.sh`; globals `HTTP_STATUS`, `HTTP_BODY`, `SCENE_VERSION_NUMBER`, `SCENE_VERSION_ID` exported by helper functions (`upload-utils.sh:127-135`, `upload-utils.sh:189-196`).
  - PowerShell: module-level dot-sourcing means all `Private/` functions and classes share the module scope; no module-level mutable singletons beyond `Set-StrictMode -Version 3.0` and `$ErrorActionPreference = 'Stop'`.
- **External dependencies (bash):** `curl`, `jq`, `uuidgen`, `stat`, `sed`. Missing tools cause `check_dependencies` to abort (`upload-utils.sh:80`).
- **External dependencies (PowerShell):** none beyond PS 5.1+ / PS Core 7.x; HTTP goes through `System.Net.WebClient` because `Invoke-WebRequest` rejects the non-standard `APIKEY:DEVELOPER` auth scheme.
- **File size limit:** 100 MB per file, enforced by `validate_file` (`upload-utils.sh:270`).
- **Required scene files:** `scene.bin`, `scene.gltf`, `screenshot.png`; `settings.json` is generated.
- **Required object files:** `<filename>.gltf`, `<filename>.bin`, `cvr_object_thumbnail.png` (must be exactly this PNG name).
- **Scene ID format:** must match UUID regex `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$` (`upload-utils.sh:178`).
- **SDK version format:** must match semver `x.y.z` (`upload-utils.sh:321`); prefixed with `cli-bash-v` or `cli-powershell-v` before being written to `settings.json`.
- **Generated artifacts in repo root:** `*object*.json` is gitignored; per-scene `settings.json` is gitignored.
- **No persistent secrets in repo:** `.env*` patterns are gitignored; `.env.example` documents the schema.
- **Circular imports:** none — bash files form a tree from each entry script into `upload-utils.sh`; PowerShell module dot-sources Private before Public (`C3DUploadTools/C3DUploadTools.psm1:17-39`).

## Anti-Patterns

### Re-implementing upload logic in a new bash script

**What happens:** A new operation is added as a new top-level `*.sh` file that re-defines logging, env loading, dependency checking, and HTTP parsing.
**Why it's wrong:** Divergence with `upload-utils.sh` produces inconsistent error messages, breaks `.env` precedence, and skips UUID/semver validation. `list-objects.sh` already shows partial drift (it defines its own `log`/`debug` helpers in `list-objects.sh:21-31` and inlines URL construction in `list-objects.sh:112-119` instead of using `get_api_base_url`).
**Do this instead:** `source "$SCRIPT_DIR/upload-utils.sh"`, `load_env_file`, and call `check_dependencies`, `validate_api_key`, `validate_uuid_format`, `validate_environment`, `get_api_base_url`, `parse_http_response`, and `handle_http_error` from the shared library — as `upload-scene.sh:39-41`, `upload-object.sh:30-31`, and `upload-object-manifest.sh:25-26` do.

### Bypassing the PowerShell HTTP helper

**What happens:** A new cmdlet calls `Invoke-WebRequest` or `Invoke-RestMethod` directly with an `Authorization: APIKEY:DEVELOPER <key>` header.
**Why it's wrong:** `Invoke-WebRequest` validates the Authorization header against RFC 7235 and rejects the non-standard `APIKEY:DEVELOPER` scheme, so the call fails before leaving the box.
**Do this instead:** Build the request through `New-C3DMultipartFormData` and `Send-C3DHttpRequest` with `-UseWebClient`, the way `Upload-C3DObject` and `Upload-C3DObjectManifest` do (`C3DUploadTools/Private/Api/Send-C3DHttpRequest.ps1`).

### Generating object IDs without checking the manifest

**What happens:** A re-upload assigns a fresh UUID to an object even though `<sceneId>_object_manifest.json` already has an entry for that mesh.
**Why it's wrong:** The Cognitive3D dashboard treats it as a new object, orphaning the previous entry and breaking dashboard continuity.
**Do this instead:** Reuse the manifest's existing `id` for the same `mesh` name (`upload-object.sh:172-189`). New PowerShell flows should adopt the same lookup.

### Skipping the pre-upload scene-version GET

**What happens:** A caller POSTs directly to `/v0/objects/{sceneId}/{objectId}` or `/v0/objects/{sceneId}` without `?version=N`.
**Why it's wrong:** The API associates uploads with whatever the server-side "current" version is, which can race with concurrent scene updates and cause objects to land on the wrong version.
**Do this instead:** Always call `get_scene_version` (bash) before object/manifest uploads (`upload-object.sh:200-214`, `upload-object-manifest.sh:108-122`); use the returned `SCENE_VERSION_NUMBER` in the URL query string.

### Committing generated artifacts or `.env` files

**What happens:** `<sceneId>_object_manifest.json`, per-scene `settings.json`, or `.env*` files get committed.
**Why it's wrong:** Manifests are operator-specific; `settings.json` is regenerated on every upload; `.env*` contain `C3D_DEVELOPER_API_KEY`.
**Do this instead:** Rely on `.gitignore` patterns `*object*.json`, `**/settings.json`, `.env*`.

## Error Handling

**Strategy:** Centralized HTTP-error mapping in `upload-utils.sh:handle_http_error` and PowerShell `New-C3DErrorRecord`.

**Patterns:**

- HTML responses (containing `Internal Server Error`, `Bad Request`, or `<html`) are detected and reported as server-side errors rather than parsed as API responses (`upload-utils.sh:208-217`).
- HTTP 401 with body containing `key expired` produces step-by-step dashboard-rotation instructions (`upload-utils.sh:220-238`).
- HTTP 401 (other), 403, 404 each have dedicated remediation guidance.
- Non-2xx outside the above prints the raw body and exits non-zero.
- `set -e` + `set -u` in `upload-utils.sh` aborts on any unhandled command failure or unbound variable.
- PowerShell side wraps results in `C3DApiResponse` with `IsAuthenticationError()`, `IsNotFound()`, `IsRateLimited()`, `IsServerError()` predicates (`C3DUploadTools/Private/Core/C3DClasses.ps1:160-178`).

## Cross-Cutting Concerns

**Logging:**

- Bash: ANSI-colored `log_info`, `log_warn`, `log_error`, `log_debug` with `[YYYY-MM-DD HH:MM:SS] [LEVEL]` timestamps (`upload-utils.sh:21-24`). `log_debug` only fires when `VERBOSE=true`.
- PowerShell: `Write-C3DLog` in `C3DUploadTools/Private/Core/Write-C3DLog.ps1`, called with `-Level Info|Warn|Error|Debug`.

**Validation:**

- Bash: `validate_directory`, `validate_file`, `validate_environment`, `validate_uuid_format`, `validate_semantic_version`, `validate_api_key`.
- PowerShell: `Test-C3DApiKey`, `Test-C3DFileSystem`, `Test-C3DUuidFormat`; plus `[Parameter(Mandatory)]` and `[ValidateScript()]` attributes at the cmdlet boundary.

**Authentication:**

- Single header: `Authorization: APIKEY:DEVELOPER <C3D_DEVELOPER_API_KEY>`.
- API key is read from `.env` or process env; never logged. Bash dry-run substitutes `[REDACTED]` in printed `curl` commands (`upload-scene.sh:291`, `upload-object-manifest.sh:148`).

**Versioning:**

- `sdk-version.txt` at repo root contains semver (currently `1.1.0`).
- Bash prefixes with `cli-bash-v`; PowerShell with `cli-powershell-v`; the prefixed value is written to `settings.json` under `sdkVersion`.

---

*Architecture analysis: 2026-05-12*
