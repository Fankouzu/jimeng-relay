#!/usr/bin/env bash
set -euo pipefail

: "${VOLC_ACCESSKEY:?set VOLC_ACCESSKEY}"
: "${VOLC_SECRETKEY:?set VOLC_SECRETKEY}"

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SIGNED_JSON="$(${ROOT_DIR}/client/scripts/gen_signed_video_submit.sh)"

python3 - <<'PY' "$SIGNED_JSON"
import json
import shlex
import subprocess
import sys

doc = json.loads(sys.argv[1])
url = doc["url"]
body = doc["body"]
headers = doc["headers"]

cmd = [
    "curl", "-i", "--http1.1", url,
    "-X", "POST",
]
for k in sorted(headers.keys()):
    v = headers[k]
    if v:
        cmd += ["-H", f"{k}: {v}"]
cmd += ["--data-binary", body]

print("=== SDK Signed Curl ===")
print(" ".join(shlex.quote(x) for x in cmd))
print()
sys.stdout.flush()

proc = subprocess.run(cmd, check=False)
sys.exit(proc.returncode)
PY
