#!/usr/bin/env bash
# Fails if a tracked Go file does not start with an SPDX licence line.
# qrcode/qrcode.go is third-party BSD-3-Clause code; everything else is AGPL-3.0-only.
set -euo pipefail
status=0
while IFS= read -r f; do
  want="AGPL-3.0-only"
  [ "$f" = "qrcode/qrcode.go" ] && want="BSD-3-Clause"
  if [ "$(head -n1 "$f")" != "// SPDX-License-Identifier: $want" ]; then
    echo "missing or wrong SPDX header: $f (want $want)"
    status=1
  fi
done < <(git ls-files '*.go')
exit $status
