#!/bin/bash
set -euo pipefail

# Usage:
#   ./run_pvw.sh
#
# Builds (if needed) and runs bin/gowhisper from the repo root, so AppConfig.json is found.

INFO() { echo -e "\e[34m[SH] [INFO] $*\e[0m"; }
DEBUG() { echo -e "\e[90m[SH] [DEBUG] $*\e[0m"; }
WARN() { echo -e "\e[33m[SH] [WARN] $*\e[0m"; }
ERROR() { echo -e "\e[31m[SH] [ERROR] $*\e[0m"; }

# Directory of the currently executing script
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

DEBUG "Using directory: $ROOT_DIR"

for tool in go make cmake; do
  if ! command -v "$tool" &>/dev/null; then
    ERROR "$tool is not installed or not in PATH"
    exit 1
  fi
done

# Check ydotoold daemon is running
if ! pgrep -x ydotoold &>/dev/null; then
  WARN "ydotoold daemon is not running, start it with: ./start_ydotoold.sh"
fi

# Guard against concurrent instances
if pgrep -x gowhisper &>/dev/null; then
  ERROR "Another instance of GoWhisper is already running"
  exit 1
fi

INFO "Building..."
make build

# Configuration is read from AppConfig.json only.
set +e
./bin/gowhisper
EXIT_CODE=$?
set -e

DEBUG "GoWhisper exited with code: $EXIT_CODE"
INFO "Done!"
exit "$EXIT_CODE"
