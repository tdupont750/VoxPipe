#!/bin/bash
set -euo pipefail

# Usage:
#   ./scripts/toggle_voxpipe.sh [port] [command]
#
# POSTs a command to the running VoxPipe HTTP source (httpPort in config.toml).
# Defaults to port 5042 and the 'v' toggle command. Commands: v, c, x, z, q.

INFO() { echo -e "\e[34m[SH] [INFO] $*\e[0m"; }
DEBUG() { echo -e "\e[90m[SH] [DEBUG] $*\e[0m"; }
WARN() { echo -e "\e[33m[SH] [WARN] $*\e[0m"; }
ERROR() { echo -e "\e[31m[SH] [ERROR] $*\e[0m"; }

PORT="${1:-5042}"
COMMAND="${2:-v}"
URL="http://localhost:${PORT}/command/${COMMAND}"

DEBUG "Posting to: $URL"

set +e
RESPONSE=$(curl -s -S --fail --max-time 5 -X POST "$URL" 2>&1)
EXIT_CODE=$?
set -e

if (( EXIT_CODE != 0 )); then
  ERROR "POST $URL failed (curl exit $EXIT_CODE): $RESPONSE"
  WARN "Is VoxPipe running with httpPort = $PORT?"
  exit "$EXIT_CODE"
fi

[[ -n "$RESPONSE" ]] && DEBUG "Response: $RESPONSE"
INFO "Sent command '$COMMAND'"
