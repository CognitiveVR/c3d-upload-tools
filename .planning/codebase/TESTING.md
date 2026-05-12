# Testing Patterns

**Analysis Date:** 2026-05-12

## Test Framework

This repository does **not** use a formal test framework. Tests are hand-rolled script suites that exercise the CLI end-to-end (or by sourcing private functions and asserting on outputs):

**Runner:**

- Bash test scripts: invoked directly (`./test-scripts/<name>.sh ...`); plain bash with `set -e` / `set -u`.
- PowerShell test scripts: invoked via `pwsh -File C3DUploadTools/Tests/<name>.ps1`; plain `.ps1` with `Set-StrictMode -Version Latest` and `$ErrorActionPreference = 'Stop'`.

**Assertion Library:** None. Tests assert by:

- `grep`'ing the captured stdout/stderr of a CLI invocation for an expected substring (bash, see `check_success`, `check_scene_id_extracted`, `check_version_check`, `check_version_parameter`, `check_html_error_detection` in `test-scripts/test-utils.sh`).
- Comparing returned values inline with `if ($result -ne $true) { throw "..." }` and counting pass/fail (PowerShell, see `Test-Function` helper used throughout `C3DUploadTools/Tests/*.ps1`).

**Status:** No Pester test suite exists yet. CLAUDE.md notes "📋 Remaining: Pester test suite for automated testing." There is no `Pester.psd1`, no `*.Tests.ps1` file, and no `Invoke-Pester` invocation.

**No CI configuration is committed.** There is no `.github/`, `.circleci/`, `.gitlab-ci.yml`, `azure-pipelines.yml`, or equivalent. All tests are run manually.

**Run Commands:**

```bash
# Comprehensive end-to-end harness (bash, exercises ALL upload scripts)
# Requires a scene_id obtained from a prior `./upload-scene.sh` run.
./test-all.sh <scene_id> <env>
# Equivalent: ./test-scripts/test-all.sh <scene_id> <env>

# Bash improvement test suites (require interactive confirmation)
./test-scripts/test-scene-upload-improvements.sh
./test-scripts/test-object-upload-improvements.sh

# Bash env-workflow integration test
./test-scripts/test-env-workflow.sh --env <prod|dev> [--verbose] [--dry_run]

# PowerShell module-level tests (run individually)
pwsh -File C3DUploadTools/Tests/test-module-structure.ps1
pwsh -File C3DUploadTools/Tests/test-utilities-internal.ps1
pwsh -File C3DUploadTools/Tests/test-core-utilities.ps1
pwsh -File C3DUploadTools/Tests/test-scene-upload.ps1
pwsh -File C3DUploadTools/Tests/test-object-upload.ps1
pwsh -File C3DUploadTools/Tests/test-http-headers.ps1
pwsh -File C3DUploadTools/Tests/Test-EnvWorkflow.ps1 -Environment <prod|dev>
```

There is no `test`, `lint`, or `coverage` aggregator command, no `npm test` equivalent, and no `Makefile`.

## Test File Organization

**Location:**

- Bash CLI tests live under `test-scripts/` (one directory level, no nesting).
- PowerShell module tests live under `C3DUploadTools/Tests/` (co-located with the module they exercise).
- The single top-level convenience wrapper `./test-all.sh` is symlinked / duplicated to `./test-scripts/test-all.sh`.

**Naming:**

- Bash: `test-<feature>.sh` (kebab-case). Shared helper file `test-utils.sh` is not prefixed with `test-` to distinguish it from runnable tests.
- PowerShell: mostly kebab-case `test-<feature>.ps1` for module-internal tests; `Verb-Noun.ps1` (PascalCase) for runnable workflow scripts like `Test-EnvWorkflow.ps1`.

**Structure:**

```text
c3d-upload-tools/
├── test-scripts/
│   ├── test-all.sh                              # End-to-end harness, calls every upload script
│   ├── test-utils.sh                            # Shared helpers (colors, counters, scene-ID extraction)
│   ├── test-env-workflow.sh                     # Full workflow test against a fresh .env
│   ├── test-scene-upload-improvements.sh        # Unity-SDK-alignment regression suite for upload-scene.sh
│   └── test-object-upload-improvements.sh       # Unity-SDK-alignment regression suite for upload-object.sh
├── C3DUploadTools/
│   └── Tests/
│       ├── test-module-structure.ps1            # Verifies module manifest, exported function list
│       ├── test-utilities-internal.ps1          # Dot-sources private functions and exercises them
│       ├── test-core-utilities.ps1              # Tests core utilities through the public module surface
│       ├── test-scene-upload.ps1                # Parameter validation + dry-run tests for Upload-C3DScene
│       ├── test-object-upload.ps1               # Parameter validation + dry-run tests for Upload-C3DObject
│       ├── test-http-headers.ps1                # Real-object test for Set-C3DRequestHeaders (no mocks)
│       └── Test-EnvWorkflow.ps1                 # End-to-end workflow using PowerShell module + .env
└── (test asset directories — see below)
```

## Test Structure

### Bash Pattern (suite-level)

`test-scripts/test-scene-upload-improvements.sh` and `test-scripts/test-object-upload-improvements.sh` follow this template:

```bash
#!/bin/bash
set -e
set -u

# Change to repo root so relative asset paths work
cd "$(dirname "$0")/.."

# Source shared test helpers (colors, counters, asserts)
source test-scripts/test-utils.sh

main() {
  print_section "SCENE UPLOAD IMPROVEMENT TESTS"
  echo "Testing environment: dev (data.c3ddev.com)"
  echo "Test scenes: scene-test, test-scene-vancouver"
  read -p "Press Enter to start tests..."   # interactive guard

  # TEST N
  print_test "1" "Dry Run - No Breaking Changes"
  OUTPUT=$(./upload-scene.sh --scene_dir scene-test --env dev --dry_run --verbose 2>&1)
  if echo "$OUTPUT" | grep -q "DRY RUN completed"; then
    print_pass "Dry run completed successfully, no breaking changes"
  else
    print_fail "Dry run failed"
    echo "$OUTPUT"
  fi

  # ... more numbered tests ...

  print_summary    # exits non-zero if any test failed
}

main "$@"
```

`test-scripts/test-utils.sh` provides:

- Colors: `COLOR_GREEN`, `COLOR_RED`, `COLOR_BLUE`, `COLOR_YELLOW`, `COLOR_RESET` (lines 15-19).
- Counters: `TESTS_PASSED`, `TESTS_FAILED`, `TESTS_TOTAL` (lines 24-26).
- Reporters: `print_section`, `print_test "N" "title"`, `print_pass "msg"`, `print_fail "msg"`, `print_summary` (auto `exit 1` on any failure).
- Scene-ID extraction with comprehensive ANSI stripping: `extract_scene_id "$OUTPUT"` (lines 90-111) — strips `\x1b\[[0-9;]*[mKHfJABCDsu]` then `grep --color=never -oE '<uuid-regex>' | head -1`.
- Substring assertions: `check_success`, `check_scene_id_extracted`, `check_version_check`, `check_html_error_detection`, `check_version_parameter`.
- `update_scene_timestamp <scene_dir>` (lines 172-199) — rewrites `<scene_dir>/settings.json` `.sceneName` with an ISO-8601 UTC suffix so each test run uses a unique scene name and prior date suffixes are stripped.

### PowerShell Pattern (test-script-level)

Two slightly different patterns coexist:

**Pattern A — manual `Write-TestResult` (used by `test-scene-upload.ps1`, `test-object-upload.ps1`):**

```powershell
#!/usr/bin/env pwsh
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$testStartTime = Get-Date
$testsPassed = 0
$testsFailed = 0
$testResults = @()

function Write-TestResult {
    param([string]$TestName, [bool]$Passed, [string]$Details = "", [string]$Error = "")
    $status = if ($Passed) { "✅ PASS" } else { "❌ FAIL" }
    $color  = if ($Passed) { "Green"   } else { "Red"    }
    Write-Host "$status $TestName" -ForegroundColor $color
    if ($Details) { Write-Host "   $Details" -ForegroundColor Gray }
    if ($Error)   { Write-Host "   ERROR: $Error" -ForegroundColor Red }
    $script:testResults += [PSCustomObject]@{
        Test = $TestName; Passed = $Passed; Details = $Details; Error = $Error; Timestamp = Get-Date
    }
    if ($Passed) { $script:testsPassed++ } else { $script:testsFailed++ }
}

function Test-ParameterValidation {
    try {
        Upload-C3DScene -ErrorAction Stop
        Write-TestResult -TestName "Missing Mandatory Parameter" -Passed $false -Error "Should have failed"
    } catch {
        if ($_.Exception.Message -like "*SceneDirectory*") {
            Write-TestResult -TestName "Missing Mandatory Parameter" -Passed $true `
                -Details "Correctly rejected missing SceneDirectory"
        } else {
            Write-TestResult -TestName "Missing Mandatory Parameter" -Passed $false `
                -Error "Wrong error message: $($_.Exception.Message)"
        }
    }
}
```

**Pattern B — `Test-Function` script-block wrapper (used by `test-core-utilities.ps1`, `test-utilities-internal.ps1`, `test-http-headers.ps1`):**

```powershell
function Test-Function {
    param([string]$TestName, [scriptblock]$TestCode)
    Write-Host "`n📋 Test: $TestName" -ForegroundColor Cyan
    try {
        & $TestCode
        Write-Host "✅ PASSED: $TestName" -ForegroundColor Green
        $script:testsPassed++
    } catch {
        Write-Host "❌ FAILED: $TestName - $($_.Exception.Message)" -ForegroundColor Red
        $script:testsFailed++
    }
}

Test-Function "API Key Validation (Test Key)" {
    $env:C3D_DEVELOPER_API_KEY = "test_api_key_1234567890"
    $result = Test-C3DApiKey
    if ($result -ne $true) { throw "Expected true with valid test API key" }
}
```

Pass/fail counts are accumulated in module-scope variables (`$script:testsPassed`, `$script:testsFailed`); scripts typically `exit 1` at the end if any test failed.

### End-to-End Harness (`./test-all.sh`)

`test-scripts/test-all.sh` is the canonical full-workflow harness:

1. `cd "$(dirname "$0")/.."` to repo root.
2. `source ./upload-utils.sh` then `load_env_file` to pick up `.env`.
3. Resolve `SCENE_ID` from `$1`, then `$C3D_SCENE_ID`, then error.
4. Resolve `ENV` from `$2`, then `$C3D_DEFAULT_ENVIRONMENT`, default `prod`.
5. Block on `read -p "Press Enter to continue..."` (operator gate).
6. Sequentially:
   - `./upload-scene.sh --scene_id $SCENE_ID --scene_dir SciFiHelmet --env $ENV --verbose`
   - `export C3D_SCENE_ID="$SCENE_ID"` (proves env-var fallback for remaining commands)
   - `./upload-object.sh --object_filename cube --object_id cube --object_dir object-test --env $ENV --verbose`
   - `./upload-object-manifest.sh --env $ENV --verbose`
   - `./upload-object.sh --object_filename Lantern --object_id Lantern-PNG --object_dir lantern-test --env $ENV --verbose`
   - `./upload-object-manifest.sh --env $ENV --verbose`
   - `./upload-object.sh --object_filename Lantern --object_id Lantern-JPEG --object_dir lantern-test-jpg --env $ENV --verbose`
   - `./upload-object-manifest.sh --env $ENV --verbose`
   - `./list-objects.sh --env $ENV --verbose`

This is a smoke-test, not an automated assertion suite: success is determined by visual review of the logs and the dashboard.

## Mocking

**Framework:** None — and by design.

`C3DUploadTools/Tests/test-http-headers.ps1:1-14` is explicit about this:

> "Instantiates real .NET objects (no mocks) to verify that restricted headers like User-Agent are set via the correct API on each request type."

**Patterns:**

- HTTP-header tests instantiate the real `System.Net.WebClient` / `System.Net.HttpWebRequest` objects and inspect their headers after calling production helpers. Resources are disposed in `finally` blocks.
- API-key / UUID / file-system validators are tested by mutating real env vars (`$env:C3D_DEVELOPER_API_KEY = "..."`) and real temp directories (`New-Item -ItemType Directory -Path (Join-Path $env:TMPDIR "c3d-test-$(Get-Random)")` in `test-module-structure.ps1:134`).
- Actual upload tests hit the live dev environment (`data.c3ddev.com`) with real API keys loaded from `.env.sample.dev`. There is no recorded-cassette / replay layer.

**What to Mock:** Nothing. If you need an isolated test, use the `--dry_run` / `-DryRun` flag — the production code paths are designed to short-circuit before any network call.

**What NOT to Mock:**

- `System.Net.WebClient`, `System.Net.HttpWebRequest`, `curl`, `jq`, `Get-Item`, file-system primitives, regex matchers.
- `.NET` exception types (`[System.Net.WebException]`, `[System.IO.IOException]`, `[System.UnauthorizedAccessException]`) — typed `catch` clauses are exercised by triggering the real errors.

## Fixtures and Factories

**Test data:** committed under the repo root as ready-to-upload asset directories.

| Directory | Purpose | Required contents |
|-----------|---------|-------------------|
| `scene-test/` | Minimal scene for fast scene-upload tests | `scene.bin`, `scene.gltf`, `screenshot.png`, `settings.json`, plus `texture1.png`/`texture2.png`/`texture3.png` for the multi-image upload path |
| `SciFiHelmet/` | Larger realistic scene with PNG **and** WEBP textures; used by `./test-all.sh` and `test-env-workflow` | `SciFiHelmet.bin`, `SciFiHelmet.gltf`, `scene.bin`/`scene.gltf`, `screenshot.png`, `settings.json`, `cvr_object_thumbnail.png`, paired `*.png` + `*.webp` texture maps, `backup-<timestamp>/` snapshot |
| `test-scene-vancouver/` | Alternate scene used by `test-scene-upload-improvements.sh` | `scene.bin`, `scene.glb`, `scene.gltf`, `screenshot.png`, `settings.json` |
| `object-test/` | Minimal "cube" object for fast object-upload tests | `cube.bin`, `cube.gltf`, `cvr_object_thumbnail.png` |
| `lantern-test/` | Realistic object with PNG textures | `lantern.bin`, `lantern.gltf`, `cvr_object_thumbnail.png`, four `lantern_*.png` texture maps |
| `lantern-test-jpg/` | Same as `lantern-test/` but JPEG textures — exercises non-PNG texture path | `Lantern.bin`, `Lantern.gltf`, `cvr_object_thumbnail.png`, four `Lantern_*.jpeg` texture maps |
| `icosphere-test/` | Generic icosphere object asset | `icosphere.bin`, `icosphere.gltf`, `C3D-Icon-1k.png`, `cvr_object_thumbnail.png`, source `*.blend*` files (informational) |

**Generated artifacts (gitignored):**

- `<scene_id>_object_manifest.json` and `<scene_id>_object_list.json` files at repo root — accumulated by `upload-object.sh` / `list-objects.sh` per scene. Several historical examples are committed at repo root (e.g. `09f1abb4-...json`) for reference.
- `<scene_dir>/settings.json` — regenerated by every upload (matched by `**/settings.json` in `.gitignore`).

**Fixture factories:** None — test data is static. Where uniqueness is needed (e.g. unique `sceneName` per test run), `update_scene_timestamp <scene_dir>` in `test-scripts/test-utils.sh:172-199` mutates the asset's `settings.json` to suffix the scene name with a fresh ISO-8601 UTC timestamp.

**Environment fixtures:** `.env.sample.dev` and `.env.sample.prod` are committed templates. `test-scripts/test-env-workflow.sh` and `C3DUploadTools/Tests/Test-EnvWorkflow.ps1` copy the appropriate sample to `.env` at the start of the run and clean it up at the end.

## Coverage

**Requirements:** None enforced. No coverage tool is configured (no `coverlet`, no `Pester -CodeCoverage`, no `bashcov`).

**View Coverage:** Not available.

## Test Types

### Unit-style Tests (PowerShell only)

Located in `C3DUploadTools/Tests/`. Scope: a single function or a small group of related private helpers.

- `test-utilities-internal.ps1` — dot-sources every `.ps1` under `C3DUploadTools/Private/*.ps1` and invokes helpers directly (bypasses the module export surface).
- `test-core-utilities.ps1` — imports the module via `Import-Module ../ -Force` and exercises core utilities through the public surface.
- `test-http-headers.ps1` — narrowly scoped: dot-sources only `Write-C3DLog.ps1` + `Set-C3DRequestHeaders.ps1` and asserts header-assignment behavior against real `WebClient` and `HttpWebRequest` objects.
- `test-module-structure.ps1` — manifest validation (`Test-ModuleManifest`), required-file checks, `Get-Command` lookups for exported functions, parameter-validation smoke tests.

### Integration-style Tests

End-to-end against the live `dev` API or a `--dry_run` preview path.

- `test-scripts/test-scene-upload-improvements.sh` — runs `./upload-scene.sh` against `scene-test/`, `test-scene-vancouver/`, and `SciFiHelmet/` in `dev`, asserting on log substrings (HTTP 200/201 markers, scene-ID extraction, version-check messages, dry-run completion).
- `test-scripts/test-object-upload-improvements.sh` — same shape for `./upload-object.sh` and `./upload-object-manifest.sh`; uses a `get_manifest_object_count` helper that runs `jq '.objects | length' "$manifest_file"`.
- `test-scripts/test-env-workflow.sh` and `C3DUploadTools/Tests/Test-EnvWorkflow.ps1` — copy `.env.sample.<env>` → `.env`, run the full upload chain, append the resulting `C3D_SCENE_ID` to `.env`, exercise env-var fallback, then clean up.
- `C3DUploadTools/Tests/test-scene-upload.ps1` and `test-object-upload.ps1` — parameter-validation suites that mostly run in `-DryRun` mode and assert on thrown exception messages (`-like "*SceneDirectory*"`, `-like "*does not exist*"`, `-like "*Cannot validate argument*"`).

### E2E Tests

`./test-all.sh <scene_id> <env>` is the canonical end-to-end smoke test (see "End-to-End Harness" above). It is the test referenced in `CLAUDE.md` under "Testing" and the README. There is no browser-based E2E (no Playwright/Cypress/etc.) because this repo ships CLI tooling, not a UI.

## Dry-Run Mode (Test Safety Net)

Both implementations expose a first-class **safe-test mode** that should be used before any live upload:

- Bash: `--dry_run` on `upload-scene.sh`, `upload-object.sh`, `upload-object-manifest.sh`. When set, scripts print the exact `curl` command they *would* execute (with `Authorization: APIKEY:DEVELOPER [REDACTED]`), the file inventory with sizes, and a preview of any `settings.json` they *would* generate — without making any network call (`upload-scene.sh:288-334`).
- PowerShell: `-DryRun [switch]` on every upload cmdlet, with the same redaction and preview behavior (`Upload-C3DScene.ps1:335-358`).

Asserting on dry-run output is the primary way `test-scripts/test-scene-upload-improvements.sh` validates non-network behavior (e.g. the "DRY RUN completed" check at lines 49-54).

## Common Patterns

### Asserting on CLI Output (Bash)

```bash
OUTPUT=$(./upload-scene.sh --scene_dir scene-test --env dev --dry_run --verbose 2>&1)
if echo "$OUTPUT" | grep -q "DRY RUN completed"; then
  print_pass "Dry run completed successfully"
else
  print_fail "Dry run failed"
  echo "$OUTPUT"        # dump captured output for postmortem
fi
```

Always merge stderr into stdout with `2>&1` so log output (which goes to stderr in some paths) is searchable.

### Asserting on Exceptions (PowerShell)

```powershell
try {
    Upload-C3DScene -SceneDirectory "./non-existent-dir" -ErrorAction Stop
    Write-TestResult -TestName "Invalid Directory" -Passed $false -Error "Should have failed"
} catch {
    if ($_.Exception.Message -like "*does not exist*") {
        Write-TestResult -TestName "Invalid Directory" -Passed $true `
            -Details "Correctly rejected non-existent directory"
    } else {
        Write-TestResult -TestName "Invalid Directory" -Passed $false `
            -Error "Wrong error message: $($_.Exception.Message)"
    }
}
```

Always pass `-ErrorAction Stop` so non-terminating errors become catchable. Assert on substring matches against `$_.Exception.Message`, not on exception type alone, since `[ValidateScript]` failures surface as parameter-binding errors with embedded user messages.

### Async / Long-Running Operations

Not applicable — uploads are synchronous `curl --form` calls (bash) or synchronous `System.Net.WebClient` calls (PowerShell). Tests block on completion and parse the full response.

### Cross-Platform Path Handling in Tests

- Bash test scripts always `cd "$(dirname "$0")/.."` to anchor the working directory to the repo root before running uploads (see every script in `test-scripts/`).
- PowerShell test scripts use `$PSScriptRoot` (`test-http-headers.ps1:25-26` dot-sources via `"$PSScriptRoot/../Private/Core/Write-C3DLog.ps1"`).
- `test-module-structure.ps1` resolves temp dirs via `Join-Path $env:TMPDIR "c3d-test-$(Get-Random)"` and unconditionally cleans up with `Remove-Item ... -Recurse -Force -ErrorAction SilentlyContinue` in a `finally` block.

## Operational Notes

- Live-environment tests **consume real API quota**. Run them against `--env dev` (`data.c3ddev.com`) unless you specifically need to validate prod.
- Many bash tests are gated behind `read -p "Press Enter to start tests..."` — they are not unattended-CI-ready as written.
- Generated `<scene_id>_object_manifest.json` files in the repo root may have to be cleaned up between runs of the object-upload improvement suite to verify the "accumulate vs. overwrite" behavior.

---

*Testing analysis: 2026-05-12*
