#!/usr/bin/env bash
# Prints the release notes for one version: its section of the changelog,
# reflowed for a GitHub release page, where a wrapped line renders as a break.
# Usage: scripts/release-notes.sh <version>|latest [changelog]
# "latest" is the newest versioned section. A prerelease version without a
# section of its own uses the Unreleased section.
set -euo pipefail

usage="usage: scripts/release-notes.sh <version>|latest [changelog]"
version=${1:?$usage}
changelog=${2:-CHANGELOG.md}
version=${version#v}
repository_url=https://github.com/CognitiveVR/c3d-upload-tools

[ -f "$changelog" ] || { echo "release-notes: $changelog not found" >&2; exit 1; }

if [ "$version" = latest ]; then
  version=$(awk '/^## \[[0-9]/ { sub(/^## \[/, ""); sub(/\].*/, ""); print; exit }' "$changelog")
  [ -n "$version" ] || { echo "release-notes: $changelog has no versioned section" >&2; exit 1; }
fi

# Prints the body of one "## [name]" section. An indented line that continues
# the line above is joined to it; nested list items and fenced code are kept.
section() {
  awk -v heading="## [$1]" '
    function emit(line) {
      if (line == "") { blanks++; return }
      if (printed) while (blanks-- > 0) print ""
      blanks = 0
      print line
      printed = 1
    }
    /^## \[/ {
      if (wanted && held) emit(previous)
      held = 0
      wanted = (index($0, heading) == 1)
      next
    }
    !wanted { next }
    /^```/ { fenced = !fenced }
    !fenced && held && previous != "" && /^ +[^ ]/ && $0 !~ /^ +([-*+]|[0-9]+\.) / {
      sub(/^ +/, " ")
      previous = previous $0
      next
    }
    { if (held) emit(previous); previous = $0; held = 1 }
    END { if (wanted && held) emit(previous) }
  ' "$changelog"
}

name=$version
notes=$(section "$name")
case "$version" in
  *-*)
    if [ -z "$notes" ]; then
      name=Unreleased
      notes=$(section "$name")
    fi
    ;;
esac
if [ -z "$notes" ]; then
  echo "release-notes: $changelog has no entries under ## [$name]" >&2
  exit 1
fi

# Relative links break on a release page, so they point at the tagged tree.
printf '%s\n' "$notes" |
  sed -E "s|\]\(([^)#:][^):]*)\)|](${repository_url}/blob/v${version}/\1)|g"
