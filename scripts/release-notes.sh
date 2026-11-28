#!/usr/bin/env bash
# Prints the release notes for one version: its section of the changelog,
# reflowed for a GitHub release page, where a wrapped line renders as a break.
# Usage: scripts/release-notes.sh <version>|latest|Unreleased [changelog]
# "latest" is the newest versioned section. A prerelease version uses the first
# of these that has entries: its own section, the section of the version it
# leads to, the Unreleased section. The section used is named on stderr.
set -euo pipefail

usage="usage: scripts/release-notes.sh <version>|latest|Unreleased [changelog]"
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
# the text line above it is joined to that line. Fenced code, nested list
# items, block quotes and table rows keep their own lines.
section() {
  awk -v heading="## [$1]" '
    function emit(line) {
      if (line == "") { blanks++; return }
      if (printed) while (blanks-- > 0) print ""
      blanks = 0
      print line
      printed = 1
    }
    function flush() { if (held) emit(previous); held = 0 }
    function hold(line, text) { flush(); previous = line; held = 1; joinable = text }
    { sub(/\r$/, "") }
    /^[ \t]*(```|~~~)/ {
      fenced = !fenced
      if (wanted) hold($0, 0)
      next
    }
    !fenced && /^## \[/ {
      flush()
      wanted = (index($0, heading) == 1)
      next
    }
    !wanted { next }
    fenced { hold($0, 0); next }
    held && joinable && /^[ \t]+[^ \t]/ && $0 !~ /^[ \t]+([-*+][ \t]|[>|]|[0-9]+[.)][ \t]|#+[ \t])/ {
      sub(/^[ \t]+/, " ")
      previous = previous $0
      next
    }
    { hold($0, $0 != "" && $0 !~ /^#/) }
    END { if (wanted) flush() }
  ' "$changelog"
}

# A section holding only blank lines and headings has no entries.
has_entries() { grep -qvE '^(#.*|[[:space:]]*)$' <<<"$1"; }

candidates=("$version")
case "$version" in
  *-*) candidates+=("${version%%-*}" Unreleased) ;;
esac

notes=
tried=
for name in "${candidates[@]}"; do
  tried="${tried:+$tried, }## [$name]"
  found=$(section "$name")
  if has_entries "$found"; then
    notes=$found
    break
  fi
done
if [ -z "$notes" ]; then
  echo "release-notes: $changelog has no entries under $tried" >&2
  exit 1
fi
echo "release-notes: using the ## [$name] section of $changelog" >&2

# Relative targets break on a release page, so they point at the tagged tree.
printf '%s\n' "$notes" |
  sed -E \
    -e "s|(!\[[^]]*\])\(([^)#:][^):]*)\)|\1(${repository_url}/raw/v${version}/\2)|g" \
    -e "s|\]\(([^)#:][^):]*)\)|](${repository_url}/blob/v${version}/\1)|g"
