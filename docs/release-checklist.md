# Release checklist (2.0.x)

## Before tagging

1. Move the `CHANGELOG.md` entries from Unreleased to a new `X.Y.Z` section with today's date.
2. Run the live smoke test against dev on macOS and on Windows, and once against prod on macOS, and keep the outputs:

   ```bash
   C3D_UPLOAD_LIVE=1 C3D_DEVELOPER_API_KEY="$(op read 'op://<vault>/<dev item>/credential')" scripts/smoke.sh
   C3D_UPLOAD_LIVE=prod C3D_DEVELOPER_API_KEY="$(op read 'op://<vault>/<prod item>/credential')" scripts/smoke.sh --env prod
   ```

   Take the key from a command substitution like this, or from a variable already exported in the shell, never as a literal on the command line, so it stays out of shell history. On Windows run it from Git Bash with the Windows binary: `COGNITIVE3D_CLI=./cognitive3d-cli-upload.exe`. It leaves the scene it creates in that environment for inspection; the prod key must belong to a test project.
3. Open the `develop` to `main` pull request with both smoke outputs attached. CI must be green.
4. Merge the pull request.

## Tag and publish

1. Tag `main` and push the tag:

   ```bash
   git checkout main && git pull
   git tag vX.Y.Z
   git push origin vX.Y.Z
   ```

2. Watch the Release workflow. The `release` environment asks its required reviewers to approve before the job runs.
3. Confirm the GitHub release has seven assets: six archives (macOS, Linux and Windows, each for amd64 and arm64) and the checksums file.
4. On a clean macOS machine:

   ```bash
   brew tap cognitivevr/tap && brew install --cask cognitive3d-cli-upload
   cognitive3d-cli-upload version
   ```

5. On Windows, unzip the Windows archive and run `cognitive3d-cli-upload version`.
6. Confirm the cask commit exists in the `cognitivevr/homebrew-tap` repository.
7. Announce in the team Slack channel with the install lines from the README.

## Release workflow dispatch

Running the Release workflow by hand (Actions, Release, Run workflow) takes a `dry_run` input.

- `dry_run` true (the default) builds a snapshot with `goreleaser release --snapshot --clean` and uploads `dist/` as the `snapshot-dist` artifact. Nothing is published.
- Signing secrets are used only for `v*` tags and for dispatches from `main`. A dispatch from any other branch builds an unsigned snapshot.
- A run that signs fails before building if any of `APPLE_TEAM_ID`, `APPLE_ID`, `APPLE_ID_PASSWORD`, `APPLE_DEVELOPER_ID_P12_BASE64`, `APPLE_DEVELOPER_ID_P12_PASSWORD` or `HOMEBREW_TAP_TOKEN` is empty, and names the empty ones.
- A real release (a tag, or `dry_run` false) fails unless its commit is on `main`, and sets `REQUIRE_SIGNING=1`, so `scripts/sign-macos.sh` fails instead of skipping when `APPLE_TEAM_ID` is unset.
- `sign-macos.sh` fails unless notarization reports `Accepted`. It prints the submission id and status; on failure, run the `xcrun notarytool log <id>` command it prints to see why.
- A prerelease tag such as `v2.1.0-rc.1` publishes the GitHub prerelease but skips the Homebrew cask.
- `dry_run` false on a ref that is not a tag fails at GoReleaser, because it cannot publish without a tag.

## One-time repository setup

- A `release` GitHub environment exists with required reviewers.
- A tag protection rule covers `v*`.
