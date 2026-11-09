# Changelog

All notable changes to this project are documented in this file. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [2.0.0] - unreleased

### Added

- `cognitive3d-cli-upload`, a single Go binary for Windows, macOS and Linux (amd64 and arm64).
- `scene`, `object` and `ego` commands to upload scenes, dynamic objects and egocentric models.
- JSON output for scripting and documented exit codes. JSON error records carry a `hint` with the
  steps to fix the error or the command to check with.
- Read requests retry on 429, 5xx (except 501 and 505) and network errors, and the wait for a new
  scene version keeps polling through them. The version wait and `ego upload --wait` bound every
  poll, retry and `Retry-After` by their own time limit.
- `go install github.com/cognitivevr/c3d-upload-tools/v2@latest` as an alternative to the release
  binaries.
- `scene upload` checks that the scene directory is writable before it sends anything (exit 4);
  on Windows a read-only `settings.json` also blocks it.
- After a new scene version appears, one confirming poll catches a concurrent upload.
- `scene upload` also finds `screenshot.png` in the `screenshot` subdirectory the Unity SDK exports to.
- The missing-key and rejected-key messages name `--api-key` as well as `C3D_DEVELOPER_API_KEY`.
- `object upload` uploads the object's own manifest entry after its files when the version does not
  list the object with this mesh yet, so a new object appears in the dashboard without a second
  command and a renamed mesh updates its row; an id with no local entry is reused from the version's
  object for that mesh before a new one is generated, and a local entry created for a listed object
  starts from the version's scale and position. `--upload-manifest=false` skips both.

### Changed

- The Go binary replaces the bash scripts and the PowerShell module. See the
  [migration guide](docs/migration-2.0.md).
- A server error (5xx) answered after an upload's body was sent names the command to check whether it
  landed, instead of saying to try again later.
- A `settings.json` write that fails after the gateway accepted the scene is a warning (exit 0).
- `object manifest upload` exits 0 when only the follow-up object listing fails, with
  `objectsWithoutFiles` set to `null`.
- `object upload` re-reads the manifest before saving, so concurrent entries survive.
- A failed `ego upload --wait` poll exits 1 with a message that the model was uploaded; a declined
  `ego delete` exits 0 and prints `deleted: false` in JSON mode.
- Scene `.gltf` and `.bin` files must be named exactly `scene.gltf` and `scene.bin`.
- `desktop.ini` is skipped like `Thumbs.db`; a `.env` directory is ignored.
- The model check rejects a `.gltf` with a byte order mark, warns when a `.bin` is longer than its
  `byteLength`, and accepts a `.glb` JSON chunk of up to 64 MB.
- Console detection works on Windows, and bidirectional control characters from the gateway are
  escaped.
- A file swapped for a link between planning and upload is refused; `settings.json` must be a
  regular file.
- Releases fail on a missing signing secret, a notarization that is not accepted, or a tag that is
  not on `main`; prereleases skip the Homebrew cask.
