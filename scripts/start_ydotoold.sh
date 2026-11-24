#!/bin/bash
set -euo pipefail

# Usage:
#   ./scripts/start_ydotoold.sh
#
# Starts the ydotoold daemon in the foreground (requires root). Leave it
# running in its own terminal; run_voxpipe.sh can also start it on demand.

INFO() { echo -e "\e[34m[SH] [INFO] $*\e[0m"; }
DEBUG() { echo -e "\e[90m[SH] [DEBUG] $*\e[0m"; }
WARN() { echo -e "\e[33m[SH] [WARN] $*\e[0m"; }
ERROR() { echo -e "\e[31m[SH] [ERROR] $*\e[0m"; }

YDOTOOL_SOCKET=/tmp/.ydotool_socket
export YDOTOOL_SOCKET

if ! command -v ydotoold &>/dev/null; then
  ERROR "ydotoold not found on PATH. See the ydotool section in README.md"
  exit 1
fi

if pgrep -x ydotoold &>/dev/null; then
  WARN "ydotoold is already running"
  exit 0
fi

DEBUG "Using socket: $YDOTOOL_SOCKET"
INFO "Starting ydotoold (sudo), press Ctrl-C to stop"

set +e
sudo env YDOTOOL_SOCKET="$YDOTOOL_SOCKET" \
  ydotoold --socket-own="$(id -u):$(id -g)"
EXIT_CODE=$?
set -e

DEBUG "ydotoold exited with code: $EXIT_CODE"
INFO "Done!"
exit "$EXIT_CODE"
