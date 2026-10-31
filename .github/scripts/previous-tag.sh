#!/usr/bin/env bash
# Prints the tag release notes should start from, or nothing if there is none.
# Final release (v1.5.0): nearest earlier tag that is not a release candidate (v1.4.0).
# Release candidate (v1.5.0-rc1): nearest earlier tag of any kind.
# Walks ancestry of <tag>^, so tags that sort higher but are not behind it are never picked.
# Usage: previous-tag.sh <tag>
set -uo pipefail
[ $# -eq 1 ] || { echo "Usage: $0 <tag>" >&2; exit 1; }
tag="$1"
case "$tag" in
  *-*) git describe --tags --abbrev=0 "${tag}^" 2>/dev/null || true ;;
  *)   git describe --tags --abbrev=0 --exclude '*-*' "${tag}^" 2>/dev/null || true ;;
esac
