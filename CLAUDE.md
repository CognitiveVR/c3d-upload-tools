# CLAUDE.md

Guidance for working in this repository. The product is the Go CLI `cognitive3d-cli-upload`; the bash scripts and PowerShell module are deprecated legacy tools.

## Build, test and lint

- `make build`: build the binary for the current platform
- `make test`: `go test -race -shuffle=on` over all packages
- `make lint`: golangci-lint (pinned version in the Makefile)
- `gofmt -l .` must print nothing and `go vet ./...` must be clean before a commit
- `make install-hooks`: point git at `.githooks` for the pre-commit hook
- Go 1.25 in `go.mod`, with no `toolchain` line. Dependencies are limited to `github.com/spf13/cobra` and `gopkg.in/yaml.v3`; tests use the standard `testing` package only

Markdown is checked in CI with the exact commands below; run them after editing any `*.md`:

```bash
npx --yes markdownlint-cli@0.49.1 --config .markdownlint.json '**/*.md'
npx --yes cspell@10.4.0 --config cspell.json --no-progress '**/*.md'
```

New product or flag names go into the `words` list of `cspell.json`.

## Package map

```text
main.go               wires cli.Execute() to a signal-aware context; maps the result to the process exit code
internal/cli/         cobra commands, one file per noun (root, auth, scene, object, ego, version); runtime and preflight
internal/config/      settings resolution: flag, environment, .env, ~/.cognitive3d/upload.yaml
internal/dotenv/      bash-compatible .env parser
internal/gateway/     HTTP client, error mapping and exit codes, GET retry, multipart streaming
internal/localfile/   regular-file checks (no symlinks) and multipart parts from disk
internal/model/       model-file rule (glb vs gltf+bin), texture allow-list, glTF/GLB checks, size thresholds, SafeSegment
internal/scene/       scene directory plan, settings.json, upload, version attribution
internal/object/      object directory plan, upload, list
internal/manifest/    manifest types, validation, merge, list-to-manifest reshape
internal/ego/         ego file plan, client calls, wait for optimized
internal/ui/          stderr logger (text and JSON), progress, terminal detection, printable-text escaping
internal/version/     build info and the sdkVersion string
internal/fakegateway/ in-process httptest gateway for tests
testdata/             small fixtures: scene-glb, scene-gltf, object-gltf, ego-head
scripts/              sign-macos.sh
docs/                 migration-2.0.md
```

The release pipeline is `.goreleaser.yml` plus `.github/workflows/`. The plan and the decision table live outside this repo, in `cvr/c3d-agentic-scratch/matt/refactor-cli-upload-tools/plan.md`.

## Conventions

- **stdout is data, stderr is logs.** A command prints only its result on stdout (one JSON object or array with `--format json`). Logs, warnings and errors go to stderr as `[YYYY-MM-DD HH:MM:SS] [LEVEL] message`, or one JSON record per line in JSON mode. Never print a diagnostic to stdout.
- **Exit codes:** 0 success, 1 general, 2 authentication, 3 not found or not accessible, 4 validation or usage.
- **One sanitizer for path segments.** Every file name that enters a URL path or a multipart filename passes through `model.SafeSegment` first.
- **Local first.** Every command runs its planning and validation before its first network call, and `--dry-run` returns before it. The API key is checked lazily, only when the first request is about to be made.
- **Preflight.** Every command that names a scene calls `preflight` before any other request, so a later 401 with an empty body means "scene not accessible" (exit 3), not "bad key".
- **Signals.** `main.go` owns the one signal handler (`signal.NotifyContext`); no other code installs one.
- **Flags** are kebab-case; the root normalizer also accepts the legacy underscore spellings.
- Units in identifiers and JSON are full words (`milliseconds`, `bytes`).
- Comments state invariants the code cannot express; no ticket ids, dates or decision history.
- `.gitattributes` keeps `*.go`, `*.md`, `*.json`, `*.gltf` and `*.sh` on LF; fixtures (`*.glb`, `*.bin`, images) are binary, so copy them with `cp`.

## Testing

- Tests run against `internal/fakegateway`, an in-process `httptest` server that implements the gateway contract. Never call the live gateway and never read a real key in a test.
- Run with `-race`; CI runs Linux, macOS and Windows, so use `filepath` and `t.TempDir()` and avoid POSIX-only assumptions.
- Fixtures are small files under `testdata/` or synthesized in the test. The large sample directories at the repo root belong to the legacy tools.
- `internal/cli/readme_examples_test.go` parses every `cognitive3d-cli-upload` line in the README fenced blocks against the real command tree. Change a flag and the README fails until it is updated.
- Update `README.md`, `docs/migration-2.0.md` and `CHANGELOG.md` when behavior or flags change.

## Legacy 1.x tools

The bash scripts (`upload-scene.sh`, `upload-object.sh`, `upload-object-manifest.sh`, `list-objects.sh`, `upload-utils.sh`) and the PowerShell module (`C3DUploadTools/`) print a deprecation banner and are removed in 3.0. Fix only critical bugs in them.

- `bash test-scripts/test-model-files.sh`: offline tests of the model-file rule
- `bash test-scripts/test-all.sh <scene_id> <env>`: end-to-end script tests against a real gateway
- `pwsh -File C3DUploadTools/Tests/test-module-structure.ps1`: PowerShell module structure check
- `pwsh -File C3DUploadTools/Tests/test-model-files.ps1`: PowerShell model-file rule tests
