#!/bin/bash
set -euo pipefail

# Usage:
#   ./run_voxpipe.sh
#
# Runs the prebuilt bin/voxpipe from the repo root, so AppConfig.json is found.
# Build it first with: make build

INFO() { echo -e "\e[34m[SH] [INFO] $*\e[0m"; }
DEBUG() { echo -e "\e[90m[SH] [DEBUG] $*\e[0m"; }
WARN() { echo -e "\e[33m[SH] [WARN] $*\e[0m"; }
ERROR() { echo -e "\e[31m[SH] [ERROR] $*\e[0m"; }

# Directory of the currently executing script
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

DEBUG "Using directory: $ROOT_DIR"

BINARY="$ROOT_DIR/bin/voxpipe"
if [[ ! -x "$BINARY" ]]; then
  ERROR "$BINARY not found or not executable. Build it first with: make build"
  exit 1
fi

# Check ydotoold daemon is running
if ! pgrep -x ydotoold &>/dev/null; then
  WARN "ydotoold daemon is not running, start it with: ./start_ydotoold.sh"
fi

# Guard against concurrent instances
if pgrep -x voxpipe &>/dev/null; then
  ERROR "Another instance of VoxPipe is already running"
  exit 1
fi

# Configuration is read from AppConfig.json only.
set +e
"$BINARY"
EXIT_CODE=$?
set -e

DEBUG "VoxPipe exited with code: $EXIT_CODE"
INFO "Done!"
exit "$EXIT_CODE"
