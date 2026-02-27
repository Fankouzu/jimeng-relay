#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
exec "${ROOT_DIR}/scripts/volc_video_submit_sdk_curl.sh"
