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
  scene version keeps polling through them.

### Changed

- The Go binary replaces the bash scripts and the PowerShell module. See the
  [migration guide](docs/migration-2.0.md).
