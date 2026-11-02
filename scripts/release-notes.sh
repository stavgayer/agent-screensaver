#!/bin/bash
# Prints the message for a release tag: the version line, a blank line, then
# that version's notes from CHANGELOG.md. It fails when CHANGELOG.md has no
# section for the version, or the section is empty.
# Usage: scripts/release-notes.sh vX.Y.Z
#   git tag -a vX.Y.Z --cleanup=verbatim -F <(scripts/release-notes.sh vX.Y.Z)
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: scripts/release-notes.sh vX.Y.Z" >&2
  exit 2
fi
version="$1"
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "error: '$version' is not a version like v1.2.3" >&2
  exit 2
fi

root="$(cd "$(dirname "$0")/.." && pwd)"
changelog="$root/CHANGELOG.md"
if [ ! -f "$changelog" ]; then
  echo "error: $changelog does not exist" >&2
  exit 1
fi

# A version section starts at a line like "## v1.2.3 (2026-01-31)" and runs to
# the next such line. Other "##" lines belong to the notes.
heading='^## v[0-9]+\.[0-9]+\.[0-9]+ \('
found=0
inside=0
body=""
while IFS= read -r line || [ -n "$line" ]; do
  if [[ "$line" =~ $heading ]]; then
    inside=0
    if [[ "$line" == "## $version ("* ]]; then
      found=$((found + 1))
      if [ "$found" -eq 1 ]; then inside=1; fi
    fi
  elif [ "$inside" -eq 1 ]; then
    body+="$line"$'\n'
  fi
done < "$changelog"

if [ "$found" -eq 0 ]; then
  echo "error: CHANGELOG.md has no section '## $version (YYYY-MM-DD)'" >&2
  exit 1
fi
if [ "$found" -gt 1 ]; then
  echo "error: CHANGELOG.md has $found sections for $version, it must have one" >&2
  exit 1
fi

# Trim blank lines at both ends of the notes.
while [[ "$body" == $'\n'* ]]; do body="${body#$'\n'}"; done
while [[ "$body" == *$'\n' ]]; do body="${body%$'\n'}"; done
if [[ ! "$body" =~ [^[:space:]] ]]; then
  echo "error: the $version section of CHANGELOG.md has no notes" >&2
  exit 1
fi

printf '%s\n\n%s\n' "$version" "$body"
