# External Integrations

**Analysis Date:** 2026-05-12

## APIs & External Services

This repo integrates with exactly one external service: the **Cognitive3D Data Platform REST API**. It is consumed by both bash (`curl`) and PowerShell (`System.Net.WebClient` / `System.Net.HttpWebRequest`) clients.

**Cognitive3D Data API:**
- Purpose: Upload and manage 3D scenes and dynamic objects for VR/XR analytics.
- Base URL (production): `https://data.cognitive3d.com/v0`
- Base URL (development): `https://data.c3ddev.com/v0`
- Version prefix: `v0` for every endpoint.
- SDK/Client: No vendor SDK — direct HTTP calls. The implementation is documented in code comments as mirroring the Unity SDK's reference behavior (e.g. `upload-scene.sh:5-37`, `upload-object.sh:5-27`, `upload-object-manifest.sh:5-22`).
- Auth: Custom `Authorization: APIKEY:DEVELOPER <key>` header (see Authentication section).
- Env var: `C3D_DEVELOPER_API_KEY`.

### Environment Selection

The `--env` flag (bash) or `-Environment` parameter (PowerShell) selects between `prod` and `dev`. Selection is centralized in:
- Bash: `get_api_base_url()` in `upload-utils.sh:150-166` — returns `https://data.cognitive3d.com/v0/${endpoint}` or `https://data.c3ddev.com/v0/${endpoint}`.
- PowerShell: `Get-C3DApiUrl` in `C3DUploadTools/Private/Api/Get-C3DApiUrl.ps1:48-58` — same URLs, returned as `"$baseUrl/$endpoint"`.
- PowerShell class fallback: `C3DConfiguration.EnvironmentUrls` in `C3DUploadTools/Private/Core/C3DClasses.ps1:7-10`.

Default environment is `prod` (overridable via `C3D_DEFAULT_ENVIRONMENT`). `list-objects.sh` additionally accepts `C3D_ENV` as an alternative env var (`list-objects.sh:96`).

### Endpoint Reference

All endpoints are scoped under `/v0/`. Path placeholders use `{sceneId}`, `{objectId}`, `{versionId}`, `{versionNumber}`.

| Method | Path | Purpose | Called From |
|--------|------|---------|-------------|
| `GET`  | `/v0/scenes/{sceneId}` | Fetch scene metadata + version array (`versions[].versionNumber`, `versions[].id`). Used as pre-upload version check. | `get_scene_version()` in `upload-utils.sh:112-147`; `Get-C3DObjects.ps1:148-156` |
| `POST` | `/v0/scenes` | Create new scene. Multipart upload (`scene.bin`, `scene.gltf`, `screenshot.png`, optional textures, `settings.json`). Returns HTTP 201 with plain-text scene UUID body. | `upload-scene.sh:341-357` (when no `--scene_id`); `Upload-C3DScene.ps1` |
| `POST` | `/v0/scenes/{sceneId}` | Update existing scene. Same multipart payload as create. Returns HTTP 200 with empty body. | `upload-scene.sh:341-357` (when `--scene_id` provided) |
| `POST` | `/v0/scenes/{sceneId}/screenshot?version={versionNumber}` | Upload scene thumbnail separately as multipart `screenshot=@file.png` after the main scene upload succeeds. | `upload_screenshot()` in `upload-utils.sh:335-388`; invoked from `upload-scene.sh:406` |
| `POST` | `/v0/objects/{sceneId}/{objectId}?version={versionNumber}` | Upload a dynamic object. Multipart payload of `{objectFilename}.gltf`, `{objectFilename}.bin`, `cvr_object_thumbnail.png`, optional textures. | `upload-object.sh:241-264`; `Upload-C3DObject.ps1` |
| `POST` | `/v0/objects/{sceneId}?version={versionNumber}` | Upload accumulated object manifest. `Content-Type: application/json`; body is the local `{sceneId}_object_manifest.json` file. | `upload-object-manifest.sh:126-167`; `Upload-C3DObjectManifest.ps1` |
| `GET`  | `/v0/versions/{versionId}/objects` | List all uploaded objects for a scene version. Response items contain `sdkId`, `meshName`, `name`, `scaleCustom`, `initialPosition`, `initialRotation`. | `list-objects.sh:154`; `Get-C3DObjects.ps1:167` |

All upload endpoints require `?version={versionNumber}` (resolved by the pre-upload `GET /v0/scenes/{sceneId}` call) to associate the upload with a specific scene version. References to Unity SDK source files are embedded in the bash script headers (`EditorCore.cs:453-578`, `ExportUtility.cs:367-550`, `CognitiveStatics.cs:46-69`).

### Success / Failure Conventions

- `200 OK` — Scene/manifest updated; body is typically empty or JSON.
- `201 Created` — New scene; body is the plain-text scene UUID (parsed and trimmed via `tr -d '\n\r"' | sed ...` in `upload-scene.sh:376`).
- `401 Unauthorized` — Bad or expired API key. Body containing `"key expired"` triggers a specific remediation message (`handle_http_error()` in `upload-utils.sh:220-238`).
- `403 Forbidden` — API key lacks permission (`upload-utils.sh:240-244`).
- `404 Not Found` — Bad scene/object/version ID (`upload-utils.sh:245-249`).
- HTML error pages — Detected via `grep -qi "Internal Server Error\|Bad Request\|<html"` in `upload-utils.sh:208`; treated as transient server-side errors.

## Data Storage

**Databases:**
- None local. The Cognitive3D platform is the remote store.

**File Storage:**
- Local filesystem only — no S3 / GCS / Azure Blob integrations.
- Generated artifacts at repo root: `<sceneId>_object_manifest.json` (Unity-SDK-shaped manifest accumulated by `upload-object.sh:302-344`) and `<sceneId>_object_list.json` (raw API response from `list-objects.sh:177-178`). Both match the git-ignored pattern `*object*.json` (`.gitignore:2`).
- Per-scene `settings.json` files inside each scene directory (e.g. `scene-test/settings.json`); git-ignored via `**/settings.json` (`.gitignore:5`).

**Caching:**
- None.

## Authentication & Identity

**Auth Provider:**
- Cognitive3D developer API key — issued via the Cognitive3D dashboard (`Settings → Manage developer key`).
- Implementation: All requests carry the literal header `Authorization: APIKEY:DEVELOPER <key>`. This is **not** standard `Bearer` / `Basic` auth — the scheme token is `APIKEY:DEVELOPER` (with the colon, no space inside the scheme name), followed by a space and the raw key.

**Header construction sites:**
- Bash (`curl`): `--header "Authorization: APIKEY:DEVELOPER ${C3D_DEVELOPER_API_KEY}"` — in `upload-utils.sh:122,371`, `upload-scene.sh:342`, `upload-object.sh:257`, `upload-object-manifest.sh:166`, `list-objects.sh:131,159`.
- PowerShell (`WebClient` / `HttpWebRequest`): `$webClient.Headers.Add('Authorization', "APIKEY:DEVELOPER $ApiKey")` in `Send-C3DHttpRequest.ps1:78,143`.

**Key handling rules enforced by the code:**
- The key must be supplied via the `C3D_DEVELOPER_API_KEY` environment variable (or `.env` file, which is git-ignored).
- The key is never written to disk, never logged, and is redacted (`[REDACTED]`) in `--dry_run` output (`upload-scene.sh:291`, `upload-object-manifest.sh:148`).
- 401 responses prompt the user to rotate the key via the dashboard (`upload-utils.sh:220-238`).

**PowerShell-specific constraint:** `Invoke-WebRequest` rejects the `APIKEY:DEVELOPER` header format as malformed. The module works around this by using `System.Net.WebClient` (multipart) and `System.Net.HttpWebRequest` (other) directly, where `Headers.Add('Authorization', ...)` accepts any value. Documented at `CLAUDE.md` and in `Send-C3DHttpRequest.ps1:72-78`.

## Monitoring & Observability

**Error Tracking:**
- None. Errors are surfaced via stderr/stdout with color-coded `[ERROR]` lines (bash: `log_error()` in `upload-utils.sh:23`; PowerShell: `Write-C3DLog -Level Error` in `C3DUploadTools/Private/Core/Write-C3DLog.ps1`).

**Logs:**
- Bash: timestamped color-coded console logging (`[YYYY-MM-DD HH:MM:SS] [LEVEL] message`) via `log_info`, `log_warn`, `log_error`, `log_debug` in `upload-utils.sh:21-29`. `--verbose` enables `log_debug` output.
- PowerShell: `Write-C3DLog` + `Write-Progress` for upload progress bars (used in `Invoke-C3DApiRequest.ps1:116,122,131,137,153`).
- Per-operation timing captured via `date +%s` deltas in bash and `Get-Date` deltas in PowerShell (`Invoke-C3DApiRequest.ps1:93,161-162`).

## CI/CD & Deployment

**Hosting:**
- Not applicable — repo distributes shell scripts and a PowerShell module that are run locally by developers.

**CI Pipeline:**
- None detected. No `.github/workflows/`, `.gitlab-ci.yml`, `.circleci/`, `azure-pipelines.yml`, or similar configuration. Test scripts are run manually.

## Environment Configuration

**Required env vars:**
- `C3D_DEVELOPER_API_KEY` — Cognitive3D developer API key (required for every script). Validated by `validate_api_key()` in `upload-utils.sh:100-106` and `Test-C3DApiKey` in `C3DUploadTools/Private/Validation/Test-C3DApiKey.ps1`.

**Optional env vars:**
- `C3D_SCENE_ID` — Default scene UUID, eliminates the need for `--scene_id` / `-SceneId`. Consumed by `upload-object.sh:43`, `upload-object-manifest.sh:34`, `list-objects.sh:88`, `Get-C3DObjects.ps1:100`. Format validated by `validate_uuid_format()` in `upload-utils.sh:178-187` (regex `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`).
- `C3D_DEFAULT_ENVIRONMENT` — Default `--env` value (`prod` or `dev`). Defaults to `prod`.
- `C3D_ENV` — Alternative environment var, only consumed by `list-objects.sh:96`.
- `C3D_VERBOSE`, `C3D_MAX_FILE_SIZE_MB`, `C3D_SCENE_VERSION_ID` — Declared in `.env.example` but not yet referenced by runtime code.

**Secrets location:**
- `.env` file at repo root (git-ignored). Template at `.env.example`. The `.gitignore` (`.gitignore:8-16`) blocks `.env`, `.env.dev`, `.env.prod`, `.env.backup`, `.env.sample.prod`, `.env.sample.dev`, `.env.webxr*`, and a catch-all `.env*`.

## Webhooks & Callbacks

**Incoming:**
- None. This repo is a one-way client of the Cognitive3D API; nothing listens for callbacks.

**Outgoing:**
- None beyond the synchronous REST calls listed in the Endpoint Reference table.

## File Format Support

The Cognitive3D API accepts a specific set of file formats; the scripts enforce these locally before transmission.

### Scene Uploads (`POST /v0/scenes`, `POST /v0/scenes/{sceneId}`)

Required files (collected from `--scene_dir`):
- `scene.gltf` — glTF 2.0 scene definition. Validated by `validate_file()` (`upload-scene.sh:172-174`). 100 MB max.
- `scene.bin` — Binary buffer companion to the glTF. Same validation.
- `screenshot.png` — Required scene preview thumbnail. Validated at `upload-scene.sh:177-178`. Uploaded as `--form 'screenshot.png=@<file>'` and **also** uploaded separately via `POST /v0/scenes/{sceneId}/screenshot?version={n}` (`upload-utils.sh:335-388`).
- `settings.json` — Auto-generated by `upload-scene.sh:242-272` if missing; on update, existing fields are preserved and only `sdkVersion` + `sceneName` are patched.

Optional texture files (auto-discovered in `--scene_dir`):
- `*.png`, `*.jpg`, `*.jpeg`, `*.webp` — Glob loop at `upload-scene.sh:183-198`. Each file is added as `--form '<basename>=@<path>'`. `screenshot.png` is excluded from this loop because it's handled separately.

### Object Uploads (`POST /v0/objects/{sceneId}/{objectId}`)

Required files (collected from `--object_dir`):
- `{ObjectFilename}.gltf` — glTF object geometry/materials. Validated at `upload-object.sh:222`.
- `{ObjectFilename}.bin` — Binary buffer. Validated at `upload-object.sh:223`.
- `cvr_object_thumbnail.png` — **PNG only** (filename is literal). Required object preview. Validated at `upload-object.sh:225`.

Optional texture files (auto-discovered):
- `*.png`, `*.jpg`, `*.jpeg`, `*.webp` — Glob loop at `upload-object.sh:229-238`. Each file is added as `--form '<basename>=@<path>'`. The literal `cvr_object_thumbnail.png` is excluded from the texture loop because it's posted as its own field.

### Manifest Upload (`POST /v0/objects/{sceneId}`)

- `Content-Type: application/json`.
- Body is the file `<sceneId>_object_manifest.json` (sent via `curl --data-binary @...` at `upload-object-manifest.sh:167`).
- Shape: `{ "objects": [{ "id": <uuid>, "mesh": <name>, "name": <name>, "scaleCustom": [x,y,z], "initialPosition": [x,y,z], "initialRotation": [x,y,z,w] }, ...] }` — accumulated by `upload-object.sh:307-344`.

### Local File Size Limit

All file validations use a 100 MB ceiling (`validate_file()` in `upload-utils.sh:270-293`, default `max_size_mb=100`). Exceeding the limit aborts the upload before any network call.

### Multipart Encoding

- Bash: delegated to `curl --form`, which builds RFC 7578 multipart payloads with auto-generated boundaries.
- PowerShell: built manually in `New-C3DMultipartFormData.ps1` and `New-C3DSingleFileFormData.ps1`. Boundary format: `----C3DUploadBoundary<Guid-no-dashes>`. Each part uses `Content-Type: application/octet-stream` regardless of source file MIME type.

---

*Integration audit: 2026-05-12*
