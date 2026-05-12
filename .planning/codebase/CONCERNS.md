# Codebase Concerns

**Analysis Date:** 2026-05-12 (snapshot at the time of `/gsd-map-codebase`)

> **Status legend** — items resolved or newly tracked after this snapshot was taken are annotated inline:
>
> - ✅ **Resolved by PR #N** — fixed on `develop`.
> - 🎫 **Tracked as SDK-NNN** — Linear backlog ticket exists.
> - ⚠️ **Partial / superseded** — some aspects resolved, others remain open.
>
> Future `/gsd-map-codebase` refreshes will regenerate this file from scratch; the annotations preserve the audit trail for what got fixed and when.

## Tech Debt

**PowerShell HTTP path forced to legacy `System.Net.WebClient`:**

- Issue: PowerShell's modern `Invoke-WebRequest` / `Invoke-RestMethod` cmdlets strictly validate the `Authorization` header and reject the Cognitive3D API's `APIKEY:DEVELOPER <token>` format, so the module falls back to `System.Net.WebClient` (multipart) and `System.Net.HttpWebRequest` (JSON) — both .NET types Microsoft has marked obsolete in favor of `HttpClient`.
- Files: `C3DUploadTools/Private/Api/Send-C3DHttpRequest.ps1:77-131` (WebClient branch), `C3DUploadTools/Private/Api/Send-C3DHttpRequest.ps1:133-223` (HttpWebRequest branch), `C3DUploadTools/Private/Api/Set-C3DRequestHeaders.ps1:1-45` (works around .NET's "restricted header" behavior for User-Agent on both client types).
- Impact: Two parallel HTTP code paths to maintain, restricted-header bugs (e.g., commits `5a1d618`, `7d39d11`, `3b0ada1` on `fix/user-agent-header-windows`), no native streaming/chunked uploads, no `HttpClient` connection pooling, and any future .NET deprecation removes the workaround surface entirely.
- Fix approach: Either (a) get the Cognitive3D API to accept a header value that satisfies RFC 7235 (e.g., custom scheme like `Authorization: DEVELOPER <token>` or `X-C3D-API-Key`) so `Invoke-RestMethod` works, or (b) port `Send-C3DHttpRequest` to `System.Net.Http.HttpClient` which does not validate the Authorization header value the way the high-level PowerShell cmdlets do.

**Manual multipart body construction in PowerShell:**

- Issue: `New-C3DMultipartFormData` builds the body by hand with `$bodyBytes = @()` and `$bodyBytes += $headerBytes; $bodyBytes += $fileBytes; $bodyBytes += $crlfBytes`. PowerShell array `+=` reallocates a new array on every append — O(n²) memory + CPU for large multi-MB payloads. The entire file is also held in memory via `[System.IO.File]::ReadAllBytes` with no streaming.
- Files: `C3DUploadTools/Private/Api/New-C3DMultipartFormData.ps1:63-97` (multi-file), `C3DUploadTools/Private/Api/New-C3DMultipartFormData.ps1:160-178` (single-file).
- Impact: Approaching the 100 MB per-file cap means hundreds of MB of garbage allocations during the build phase. Will be slow and may exhaust memory on 32-bit Windows PowerShell 5.1.
- Fix approach: Use `[System.IO.MemoryStream]` and `Write()` the chunks, or — better — switch to `System.Net.Http.HttpClient` + `MultipartFormDataContent` + `StreamContent` so files are read streaming.

**Bash ↔ PowerShell duplication:**

- Issue: Every operation has two independent implementations (`upload-scene.sh` ↔ `Upload-C3DScene.ps1`, `upload-object.sh` ↔ `Upload-C3DObject.ps1`, `upload-object-manifest.sh` ↔ `Upload-C3DObjectManifest.ps1`, `list-objects.sh` ↔ `Get-C3DObjects.ps1`). API contract changes (endpoint paths, version query string, response shape, manifest schema) must be applied in both places by hand.
- Files: `upload-scene.sh:1-427` vs `C3DUploadTools/Public/Upload-C3DScene.ps1:1-428`; `upload-object.sh:1-362` vs `C3DUploadTools/Public/Upload-C3DObject.ps1:1-431`; `upload-object-manifest.sh:1-187` vs `C3DUploadTools/Public/Upload-C3DObjectManifest.ps1:1-170`; `list-objects.sh:1-191` vs `C3DUploadTools/Public/Get-C3DObjects.ps1:1-211`.
- Impact: Drift risk. Already observed — the SDK version prefix string differs by implementation (`cli-bash-v<version>` vs `cli-powershell-v<version>` — `upload-scene.sh:219`, `Upload-C3DScene.ps1:278`), which is intentional but emblematic of the parallel-maintenance burden.
- Fix approach: Either pick a single language (PowerShell Core 7 is cross-platform) and deprecate the other, or extract a tiny shared CLI in a portable language (Python, Go, Node).

**SDK version managed by hand in `sdk-version.txt`:** ⚠️ **Drift resolved by PR #13 (`4299306`), 2026-05-12. Automation still open.**

- Issue: Version string sits in a plain text file (`sdk-version.txt`, currently `1.1.0`) read by both bash (`upload-scene.sh:213`) and PowerShell (`Upload-C3DScene.ps1:262-275`). No automation bumps it on release, no CI gate ensures it matches the `ModuleVersion` in the `.psd1`, no git tag enforcement.
- Files: `sdk-version.txt`, `upload-scene.sh:206-220`, `C3DUploadTools/Public/Upload-C3DScene.ps1:261-279`, `C3DUploadTools/C3DUploadTools.psd1:4` (`ModuleVersion = '1.0.0'` — already drifted from `sdk-version.txt`'s `1.1.0`).
- Impact: Confirmed drift between `ModuleVersion = '1.0.0'` (psd1) and `sdk-version.txt = 1.1.0`. Telemetry uploaded with `sdkVersion = "cli-powershell-v1.1.0"` while module advertises 1.0.0 to PowerShell Gallery.
- Fix approach: Single source of truth — generate `ModuleVersion` in the psd1 at build time from `sdk-version.txt`, or vice versa. Add a CI check that fails if they disagree.
- **Update 2026-05-12:** Manual sync applied in PR #13 (commit `4299306`) — psd1 `ModuleVersion` bumped to `1.1.0` to match `sdk-version.txt`, with a comment pinning the two together. The underlying "no automation / no CI gate" concern remains open until CI exists (see "No CI/CD" below).

**100 MB per-file limit hard-coded in multiple places:**

- Issue: The 100 MB cap appears as a literal in five+ locations. `.env.example:32` advertises `C3D_MAX_FILE_SIZE_MB=100` as configurable, but neither the bash nor the PowerShell code reads that variable.
- Files: `upload-utils.sh:272` (`local max_size_mb="${2:-100}"`), `upload-scene.sh:173,178,194` (calls `validate_file "$file" 100`), `C3DUploadTools/Public/Upload-C3DScene.ps1:141,145,147,226,244,246`, `C3DUploadTools/Public/Upload-C3DObject.ps1:263-265`, `C3DUploadTools/Private/Validation/Test-C3DFileSystem.ps1:131,158` (`[long]$MaxSizeBytes = 100MB`).
- Impact: Large scenes (high-res textures, dense meshes) hit the limit with no override mechanism. Documented `C3D_MAX_FILE_SIZE_MB` env var is dead code. No chunking, so any single file >100 MB fails outright at validation rather than being split or streamed.
- Fix approach: Wire `C3D_MAX_FILE_SIZE_MB` through to both stacks, and add a `--max_file_size_mb` flag. Long-term: implement chunked/resumable uploads if the Cognitive3D API supports them (verify with backend team).

**Stale generated artifacts littering working tree:**

- Issue: 18 generated UUID-named files at the repo root (`<sceneId>_object_manifest.json` and `<sceneId>_object_list.json`, e.g., `09f1abb4-a42c-417d-ba01-0a49bb35688f_object_manifest.json` and 17 siblings dating Aug 2025 – Jan 2026).
- Files: Repo root — pattern `*_object_manifest.json` / `*_object_list.json` (18 files via `ls *_object_*.json | wc -l`). Note: gitignore at `.gitignore:2` (`*object*.json`) correctly excludes them from version control, so they are working-tree clutter only, not committed.
- Impact: Cosmetic noise in `ls` output, accidental tab-completion targets, confusion about which manifest is "current," scripts always write to repo root rather than an output directory.
- Fix approach: Have `upload-object.sh`/`Upload-C3DObject.ps1` and `list-objects.sh`/`Get-C3DObjects.ps1` write to a configurable output directory (e.g., `./out/` or `--output_dir`). Add a `clean` target / `Clear-C3DGeneratedFiles` helper. Optionally extend `.gitignore` patterns to be more specific.

**`list-objects.sh` reimplements logging and dependency checks instead of using `upload-utils.sh`:** ✅ **Resolved by PR #14 (`832e55c`), 2026-05-12.**

- Issue: `list-objects.sh` sources `upload-utils.sh` (line 9) but then re-defines its own `log()`, `debug()`, `usage()` functions (lines 21-46) and re-implements its own dependency check (lines 49-57) and HTTP parsing (lines 134-135, 163-164) rather than using `log_info`, `log_debug`, `check_dependencies`, `parse_http_response` from the shared utils.
- Files: `list-objects.sh:21-46,49-57,127-179`.
- Impact: Inconsistent log formatting (no timestamps, no color, no `[LEVEL]` prefix), no 401-expired-key guidance, no HTML-error-page detection — divergent UX vs the other bash scripts.
- Fix approach: Delete the local helpers in `list-objects.sh` and call the shared `log_info`/`log_debug`/`check_dependencies`/`handle_http_error`/`parse_http_response`.
- **Update 2026-05-12:** Done in PR #14 (commit `832e55c`). Net `−77/+35` LOC. Also dropped the dead `--debug` flag and added `validate_uuid_format` / `validate_environment` / `validate_api_key` at entry. PR #14 also folded in a `validate_api_key` cleanup (commit `3271d1e`) that demotes the success-case `log_info` to `log_debug` — quieter stdout for piping across all four bash scripts.

**`Test-C3DUploads` is a placeholder, but exported as a public function:** ✅ **Resolved by PR #13 (`4de40bc`), 2026-05-12.**

- Issue: `Test-C3DUploads` is exported from the module manifest (`C3DUploadTools.psd1:27`) but its body is `throw "Not implemented yet - placeholder for module structure testing"`.
- Files: `C3DUploadTools/Public/Test-C3DUploads.ps1:1-22`, `C3DUploadTools/C3DUploadTools.psd1:22-28`.
- Impact: Anyone discovering the module sees a function in `Get-Command -Module C3DUploadTools` that immediately throws. Looks broken to end users.
- Fix approach: Either implement it (a real smoke test using fixtures from `scene-test/`, `object-test/`, `lantern-test/`) or remove it from `FunctionsToExport` until Pester tests land.
- **Update 2026-05-12:** Removed from `FunctionsToExport` in PR #13. Source file kept in `Public/` so re-adding is just "fill in the body + re-export." Follow-up [SDK-497](https://linear.app/cognitive3d/issue/SDK-497/) tracks tightening the psm1/psd1 export contract so placeholder files in `Public/` can't accidentally leak through.

## Known Bugs

**`Get-C3DObjects` reads `.Content` from the standardized response, which only has `.Body`:** ✅ **Resolved by PR #11 (`7c5c48b`), 2026-05-12.**

- Symptoms: `Get-C3DObjects` will fail to parse the response and report empty/null data, because `ConvertTo-C3DApiResponse` (`Send-C3DHttpRequest.ps1:305-313`) emits a `PSCustomObject` with `Body` (not `Content`), but `Get-C3DObjects.ps1:153,156,172,175` reads `$sceneResponse.Content` and `$objectsResponse.Content`.
- Files: `C3DUploadTools/Public/Get-C3DObjects.ps1:153`, `:156`, `:172`, `:175`.
- Trigger: Any call to `Get-C3DObjects`. The recent commit `3b0ada1 "Fix StatusDescription use-after-dispose and Content→Body property mismatch"` addressed this in `Upload-C3DObjectManifest.ps1` (now uses `.Body` correctly) but missed the same bug in `Get-C3DObjects.ps1`.
- Workaround: Run `list-objects.sh` instead until fixed. Replace `.Content` with `.Body` in the four call sites.
- **Update 2026-05-12:** Fixed in PR #11 (commit `7c5c48b`). PR #11 also folded in a Codex-flagged sibling fix: commit `e7a0923` (cherry-pick of `3b0ada1`) ensured `Upload-C3DObjectManifest.ps1` also reads `.Body`, plus commit `b6e1f61` fixed a latent `-or` (boolean) bug in `ConvertTo-C3DApiResponse` that was silently emitting `.Error = $true` instead of the actual error string. Test coverage for this whole class of bug is now tracked in [SDK-496](https://linear.app/cognitive3d/issue/SDK-496/).

## Security Considerations

**API key handling depends on `.env` discipline; no automated secret-scanning hook:**

- Risk: `.env` files in the repo root contain `C3D_DEVELOPER_API_KEY`. The repo correctly gitignores `.env`, `.env.*`, `.env.backup`, etc. (`.gitignore:6-13`), and `.env.example` has placeholder text only, but there is no pre-commit hook (no `.pre-commit-config.yaml`, no `.git/hooks/pre-commit` template) to prevent an accidental `git add -f .env` or to scan diffs for `APIKEY:DEVELOPER ...` strings.
- Files: `.env`, `.env.backup`, `.env.2026-01-21`, `.env.sample.dev`, `.env.sample.prod`, `.env.webxr.threejs.prod` (all present locally, all gitignored), `.gitignore:6-13`.
- Current mitigation: `.gitignore` patterns; documentation in `.env.example:81-90` and `upload-utils.sh:255` advising key rotation on 401.
- Recommendations: Add a `gitleaks` / `detect-secrets` pre-commit hook; add a CI job (once CI exists) that scans diffs for the `APIKEY:DEVELOPER` literal. Also note: the API key appears in `curl` command lines and could leak into shell history if a user copy-pastes the rendered `--dry_run` output of `upload-scene.sh:289-300` — it is redacted there, but real (non-dry-run) curl invocations expose the key to anything reading `/proc/<pid>/cmdline` on Linux. Consider passing the key via `--header @-` from stdin or via `CURLOPT_HEADERFUNCTION`.

**Multiple stale `.env` snapshots in working tree:**

- Risk: `.env.backup` (Aug 2025), `.env.2026-01-21` (Jan 2026), `.env.webxr.threejs.prod` (Aug 2025) — each is a full snapshot of a working `.env` (3.4-3.5 KB each, gitignored). They sit in the working tree indefinitely with real keys.
- Files: `.env`, `.env.backup`, `.env.2026-01-21`, `.env.webxr.threejs.prod`.
- Current mitigation: All gitignored.
- Recommendations: Move backups out of the repo (to `~/.config/c3d/` or a password manager). Document key rotation cadence.

## Performance Bottlenecks

**Quadratic byte-array growth in multipart construction (see Tech Debt):**

- Problem: O(n²) reallocation when building multipart bodies; full file read into memory.
- Files: `C3DUploadTools/Private/Api/New-C3DMultipartFormData.ps1:63-97,160-178`.
- Cause: PowerShell `$arr += $more` reallocates on each call.
- Improvement path: `MemoryStream`-based assembly, or switch to `HttpClient` + `MultipartFormDataContent`.

**Bash `validate_file` shells out to `stat` per file:**

- Problem: Each file size check runs `stat -f%z` or `stat -c%s`. Minor in absolute terms but visible per upload.
- Files: `upload-utils.sh:281-289`.
- Cause: External process invocation per file.
- Improvement path: Acceptable for current scale; flag only if scenes grow to many dozens of files.

## Fragile Areas

**Fragile API contracts hard-coded across two stacks:**

- Files: `upload-utils.sh:121-123,150-166`, `upload-object.sh:243-260`, `upload-object-manifest.sh:127-167`, `list-objects.sh:128-159,182-191`, `C3DUploadTools/Private/Api/Get-C3DApiUrl.ps1`, `C3DUploadTools/Private/Api/Send-C3DHttpRequest.ps1:78,143`.
- Why fragile: The exact wire-level details are scattered, undocumented as a single contract, and duplicated:
  - **Authorization header format:** `APIKEY:DEVELOPER <token>` is a non-standard scheme (contains `:` mid-scheme, no space-separated `<scheme> <credentials>` per RFC 7235). This is exactly why `Invoke-WebRequest` rejects it. Any client library that validates RFC 7235 will reject it. Tracked across 11 sites; hard-coded as a literal string everywhere.
  - **Multipart field naming convention:** Field name **equals** filename (e.g., `--form "scene.bin=@$BIN_FILE"`, `--form "screenshot.png=@$SCREENSHOT_FILE"`, `--form "${OBJECT_FILENAME}.bin=@$BIN_FILE"` — `upload-scene.sh:343-345`, `upload-object.sh:258-260`). If the server changes to expect a generic `file` field name, this breaks.
  - **Multipart boundary handling:** PowerShell builds its own boundary (`----C3DUploadBoundary<guid>` — `New-C3DMultipartFormData.ps1:38`) with a single `\r\n` between parts; bash relies on `curl --form` to construct boundaries. If the server is sensitive to boundary format or expects no trailing CRLF after the closing boundary, the two stacks behave differently.
  - **Response shape — HTTP 201 plain-text scene ID:** `upload-scene.sh:371-385` and `Upload-C3DScene.ps1:386-396` both special-case HTTP 201 → plain-text body containing the new scene UUID. If the API ever wraps it in JSON, both stacks break.
  - **Response shape — `versions[-1]` / `max_by(versionNumber)` to pick latest:** `upload-utils.sh:130-131` uses `.versions[-1]` (jq's "last element"), while `list-objects.sh:144` uses `max_by(.versionNumber)` and `Get-C3DObjects.ps1:157` uses `Sort-Object versionNumber | Select-Object -Last 1`. Three different strategies for the same task — if the API ever returns versions in non-chronological order, only the latter two will work correctly.
  - **JSON field names** baked into queries: `versionNumber`, `id`, `sdkId`, `meshName`, `name`, `scaleCustom`, `initialPosition`, `initialRotation` are referenced as string literals in jq filters and `ConvertFrom-Json` accessors across both stacks. Any rename on the server requires touching ~6 files.
  - **HTML error detection by content sniffing:** `upload-utils.sh:208` greps the response body for `"Internal Server Error|Bad Request|<html"` to detect HTML error pages. If the server changes the wording, the special handling silently disappears and the user sees a raw HTML dump.
- Safe modification: Treat any change to the above as a coordinated cross-stack PR. Add a contract doc (e.g., `docs/API-CONTRACT.md`) listing each field and the files that depend on it.
- Test coverage: No automated coverage. The only thing that catches drift today is `./test-all.sh` run manually against a live API.

**Mixed test runner styles in PowerShell tests:** 🎫 **Tracked as [SDK-496](https://linear.app/cognitive3d/issue/SDK-496/) (Pester adoption + Send-C3DHttpRequest coverage).**

- Files: `C3DUploadTools/Tests/test-http-headers.ps1`, `test-core-utilities.ps1`, `test-scene-upload.ps1`, `test-module-structure.ps1`, `test-object-upload.ps1`, `test-utilities-internal.ps1`, `Test-EnvWorkflow.ps1`.
- Why fragile: All seven scripts are hand-rolled `try/catch` style scripts that `exit 1` on failure — no Pester, no shared assertion vocabulary, no result aggregation. CLAUDE.md acknowledges Pester as remaining work for PowerShell Gallery publication.
- Safe modification: Hard to add new tests without picking a side. Suggest migrating one file at a time to Pester 5.x.
- Test coverage: Unknown coverage %; nothing measures it.
- **Update 2026-05-12:** A separate but related bug — `test-module-structure.ps1` Test 6's `-contains` check that produces a spurious red ❌ on every run — is tracked in [SDK-498](https://linear.app/cognitive3d/issue/SDK-498/).

## Scaling Limits

**Per-file 100 MB cap (see Tech Debt section).**

- Current capacity: 100 MB per file, no overall request-size cap stated.
- Limit: Files larger than 100 MB are rejected client-side. No chunking.
- Scaling path: Either raise the cap (verify server-side limits with C3D backend team) or implement chunked / multipart-resumable uploads.

**Whole-file in-memory load (PowerShell):**

- Current capacity: Files up to ~RAM/2 will work; beyond that, OOM.
- Limit: `[System.IO.File]::ReadAllBytes` at `New-C3DMultipartFormData.ps1:82,161` + array `+=` doubling means peak memory ~3-4x the file size.
- Scaling path: Stream via `HttpClient` + `StreamContent`.

## Dependencies at Risk

**`curl` and `jq` on bash side:**

- Risk: External binary dependencies, version-skew differences in `jq` filter syntax (`.versions[-1]` requires jq 1.5+; OK in practice).
- Impact: Hard error at startup via `check_dependencies` (`upload-utils.sh:80-97`) if missing. Not a silent failure.
- Migration plan: PowerShell stack already removed both. Bash stack would need to rewrite jq filters in pure bash or Python to drop jq.

**`uuidgen` on bash side:**

- Risk: Used at `upload-object.sh:182,186` but not listed in `check_dependencies`. Missing on minimal containers (busybox).
- Impact: Cryptic error on systems without `uuid-runtime` package.
- Migration plan: Add `uuidgen` to `check_dependencies`, or fall back to `/proc/sys/kernel/random/uuid` (Linux) / `python3 -c 'import uuid; print(uuid.uuid4())'`.

**.NET Framework 4.x for Windows PowerShell 5.1:**

- Risk: Module declares `PowerShellVersion = '5.1'` and `CompatiblePSEditions = @('Desktop', 'Core')` (`C3DUploadTools.psd1:16-19`). `System.Net.WebClient` and `System.Net.HttpWebRequest` are obsolete in .NET 6+ — Microsoft still ships them, but new APIs (`HttpClient`) are preferred. They will not be removed near-term but new TLS features land first in `HttpClient`.
- Impact: TLS 1.3 / HTTP/2 support eventually lags.
- Migration plan: Drop Windows PowerShell 5.1, target PowerShell 7+, switch to `HttpClient`.

## Missing Critical Features

**No CI/CD:**

- Problem: No `.github/` directory exists. All tests are invoked manually via `./test-all.sh`, `pwsh -File C3DUploadTools/Tests/test-*.ps1`. No PR gating, no scheduled regression run against the dev environment, no module-publishing automation.
- Blocks: Regression detection on the parallel bash/PowerShell stacks; the version-drift between `sdk-version.txt` and `ModuleVersion`; secret-scanning; lint of `.ps1`/`.sh`; publishing to PowerShell Gallery.

**No Pester test suite:**

- Problem: Listed in CLAUDE.md as "Remaining: Pester test suite for automated testing" and "95% Ready for Publication." The seven `Tests/test-*.ps1` files are bespoke scripts, not Pester `Describe`/`It` tests.
- Blocks: PowerShell Gallery publishing best-practice; coverage reporting; integration with CI runners that have built-in Pester support.

**No structured release process:** ⚠️ **LICENSE + LicenseUri resolved by PR #12, 2026-05-12. CHANGELOG and tagging still open.**

- Problem: No `CHANGELOG.md`, no `LICENSE` file in the repo root (psd1 line 42 references `LICENSE` but it is absent — confirmed via `git ls-files | grep -i license` returning nothing), no release tagging convention visible.
- Blocks: PowerShell Gallery publishing requires a `LicenseUri` that resolves. Currently points at `https://github.com/cognitive3d/c3d-upload-tools/blob/main/LICENSE` — 404.
- **Update 2026-05-12:** PR #12 added the LICENSE file (Cognitive3D SDK Software License, copied verbatim from `cvr-sdk-unity`), corrected the org-slug typo in `LicenseUri`/`ProjectUri` (`cognitive3d` → `CognitiveVR`), removed a contradicting "MIT License" reference from README, and normalized curly quotes in the LICENSE to ASCII. PSGallery publish path is now unblocked. PR #13 (`a36590e`) also replaced the stale "Initial release" `ReleaseNotes` with a real 1.1.0 changelog inside the psd1 — partial CHANGELOG substitute until a real `CHANGELOG.md` exists.

**No `C3D_MAX_FILE_SIZE_MB` wiring (documented but inert):**

- Problem: Documented at `.env.example:29-32` but no code reads `$env:C3D_MAX_FILE_SIZE_MB` or `${C3D_MAX_FILE_SIZE_MB}`.
- Blocks: Users who want to enforce stricter limits or who hit the 100 MB ceiling have no escape hatch.

## Test Coverage Gaps

**No automated tests for the PowerShell HTTP/multipart path:** 🎫 **Tracked as [SDK-496](https://linear.app/cognitive3d/issue/SDK-496/).**

- What's not tested: End-to-end `Upload-C3DScene`/`Upload-C3DObject`/`Upload-C3DObjectManifest` against a recorded or mocked HTTP endpoint. `test-http-headers.ps1` covers `Set-C3DRequestHeaders` in isolation (added in commit `7d39d11`), but the actual `Send-C3DHttpRequest` happy-path and error-path branches are not unit-tested.
- Files: `C3DUploadTools/Tests/` (no `test-http-request.ps1` or `test-send-c3d-http.ps1`); `C3DUploadTools/Private/Api/Send-C3DHttpRequest.ps1:72-223` (both `UseWebClient` branch and `WebRequest` branch).
- Risk: The recent `StatusDescription use-after-dispose` bug (`3b0ada1`) and `Content→Body` mismatch shipped to users before being caught manually. Same class of bug could regress unnoticed.
- Priority: High.
- **Update 2026-05-12:** SDK-496 (5 points, ~1.5 days) scopes Pester-based coverage of both branches × {200, 4xx, 5xx, transport error}, plus the `ConvertTo-C3DApiResponse` normalization layer. Suggested approach: `System.Net.HttpListener` fixture instead of mocks, so real serialization bugs are caught.

**`Get-C3DObjects` `Content`/`Body` bug is live and untested:** ✅ **Bug resolved by PR #11 (`7c5c48b`), 2026-05-12. Test gap tracked as [SDK-496](https://linear.app/cognitive3d/issue/SDK-496/).**

- What's not tested: That `Get-C3DObjects` actually parses the returned JSON. No test exercises the `$objectsData = $objectsResponse.Content | ConvertFrom-Json` line.
- Files: `C3DUploadTools/Public/Get-C3DObjects.ps1:156,175`.
- Risk: Listed under Known Bugs.
- Priority: High — fix-and-test together.
- **Update 2026-05-12:** The bug itself is fixed (see "Known Bugs" section above). The test gap is folded into SDK-496's acceptance criteria, which require the `ConvertTo-C3DApiResponse` normalization to be tested in isolation — exactly the layer this bug lived in.

**No tests for `Import-C3DEnvironment` quoting / multi-equals edge cases:**

- What's not tested: A `.env` line like `KEY="value with = inside"` (the regex `^([^=]+)=(.*)$` at `Import-C3DEnvironment.ps1:25` does the right thing — greedy on first `=` — but verify); behavior when value contains literal `\n`; behavior when file uses CRLF line endings.
- Files: `C3DUploadTools/Private/Core/Import-C3DEnvironment.ps1:14-56`.
- Risk: Quiet misload of API key with embedded special characters.
- Priority: Medium.

**No tests for bash `load_env_file`:**

- What's not tested: Behavior with quoted values (it does NOT strip quotes — see `upload-utils.sh:48-53` — whereas the PowerShell `Import-C3DEnvironment` DOES strip them at line 31-33). This is a real bash↔PowerShell divergence: a `.env` containing `C3D_DEVELOPER_API_KEY="abc123"` will export the **quoted** literal in bash but the **unquoted** value in PowerShell.
- Files: `upload-utils.sh:31-77`, `C3DUploadTools/Private/Core/Import-C3DEnvironment.ps1:29-33`.
- Risk: High — silent auth failures on the bash side if user copy-pastes a quoted value into `.env`.
- Priority: High.

**No tests for `list-objects.sh` divergent code paths:** ⚠️ **Largely moot after PR #14 (`832e55c`), 2026-05-12.**

- What's not tested: The script's bespoke logging and HTTP error handling.
- Files: `list-objects.sh:21-46,127-179`.
- Priority: Medium.
- **Update 2026-05-12:** PR #14 deleted the bespoke logging and HTTP-error-handling code paths entirely — `list-objects.sh` now uses the shared `upload-utils.sh` helpers. The remaining test gap is "no tests for `upload-utils.sh` shared helpers," which is the same gap covered by SDK-496 (Pester adoption). Live smoke test against scene `82dc7b38-...` on dev was run during PR #14 verification and confirmed end-to-end behavior.

**No integration test for dual-version scene update path:**

- What's not tested: `upload-scene.sh` / `Upload-C3DScene.ps1` calls `get_scene_version` before update (`upload-scene.sh:276-285`, equivalent in PowerShell), then uploads. The "scene exists with prior version" branch is only exercised manually.
- Files: `upload-utils.sh:112-147` (`get_scene_version`), `upload-scene.sh:274-285`.
- Priority: Medium.

## TODO/FIXME/HACK/XXX Markers Found

None. A repo-wide grep across all `.sh`, `.ps1`, `.psm1`, `.psd1` files turned up zero matches for `TODO`, `FIXME`, `HACK`, or `XXX`. The only loose end discovered through reading is the explicit "placeholder" comment in `C3DUploadTools/Public/Test-C3DUploads.ps1:7,20` (`Placeholder implementation for module structure testing` / `throw "Not implemented yet - placeholder..."`) — captured above under Tech Debt.

## Concerns Discovered After Initial Snapshot

These items surfaced during PR review of #10-#14 (post-snapshot) and are tracked for follow-up. Listed here so the audit trail covers everything in flight at the 2026-05-12 mark.

🎫 **Bash `log_*` helpers write to stdout, mixing with script output** — [SDK-499](https://linear.app/cognitive3d/issue/SDK-499/). All four `log_*` helpers in `upload-utils.sh` use `echo` without `>&2`, so `./list-objects.sh ... | jq` fails on the diagnostic lines and `./script ... > out.json` writes ANSI-coded logs into the file. PR #14 partially mitigated by demoting `validate_api_key`'s success-case to `log_debug`. SDK-499 (2 points) covers the full structural fix.

🎫 **psm1/psd1 export contract relies on intersection** — [SDK-497](https://linear.app/cognitive3d/issue/SDK-497/). `C3DUploadTools.psm1` exports every `.ps1` in `Public/` then relies on `FunctionsToExport` in the manifest to filter back out unwanted ones (e.g., `Test-C3DUploads`). Brittle: importing the psm1 directly bypasses the filter. Future placeholder files in `Public/` auto-export unless excluded explicitly in psd1. SDK-497 (2 points) covers either moving placeholders to `Private/` or making the psm1 export list explicit.

🎫 **`test-module-structure.ps1` Test 6 false-failure** — [SDK-498](https://linear.app/cognitive3d/issue/SDK-498/). Test 6 uses `$_.Exception.Message -contains "Not implemented yet"` to detect expected failures, but `-contains` is collection-membership, not substring-match — so every test run prints a red ❌ even though the script reports "All Tests Completed Successfully!" at the end. SDK-498 (1 point) fixes the operator + updates the assertion to anticipate the actual current behavior.

---

*Concerns audit: 2026-05-12*
*Inline resolution annotations added: 2026-05-12 (post-merge of PR #10-#14).*
