# Release checklist (2.0.x)

## Before tagging

1. Move the `CHANGELOG.md` entries from Unreleased to a new `X.Y.Z` section with today's date.
2. Run the live smoke test against dev on macOS and on Windows, and keep the outputs:

   ```bash
   C3D_UPLOAD_LIVE=1 C3D_DEVELOPER_API_KEY="$(op read 'op://<vault>/<item>/credential')" scripts/smoke-dev.sh
   ```

   Take the key from a command substitution like this, or from a variable already exported in the shell, never as a literal on the command line, so it stays out of shell history. On Windows run it from Git Bash with the Windows binary: `COGNITIVE3D_CLI=./cognitive3d-cli-upload.exe`. It leaves the scene it creates on dev for inspection.
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
- `dry_run` false on a ref that is not a tag fails at GoReleaser, because it cannot publish without a tag.

## One-time repository setup

- A `release` GitHub environment exists with required reviewers.
- A tag protection rule covers `v*`.
