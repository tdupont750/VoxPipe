#!/bin/bash
set -euo pipefail

# Usage:
#   ./scripts/run_voxpipe.sh
#
# Runs the prebuilt bin/voxpipe from the repo root, so config.toml is found.
# Build it first with: make build

INFO() { echo -e "\e[34m[SH] [INFO] $*\e[0m"; }
DEBUG() { echo -e "\e[90m[SH] [DEBUG] $*\e[0m"; }
WARN() { echo -e "\e[33m[SH] [WARN] $*\e[0m"; }
ERROR() { echo -e "\e[31m[SH] [ERROR] $*\e[0m"; }

# Repo root (this script lives in scripts/)
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

DEBUG "Using directory: $ROOT_DIR"

BINARY="$ROOT_DIR/bin/voxpipe"
if [[ ! -x "$BINARY" ]]; then
  ERROR "$BINARY not found or not executable. Build it first with: make build"
  exit 1
fi

# First run: create ./config.toml from the checked-in example, unless a user config exists
USER_CONFIG="${XDG_CONFIG_HOME:-$HOME/.config}/voxpipe/config.toml"
if [[ ! -f "$ROOT_DIR/config.toml" && ! -f "$USER_CONFIG" ]]; then
  WARN "config.toml not found, copying config.example.toml. Review it (modelDir, deviceName, outputs)."
  cp "$ROOT_DIR/config.example.toml" "$ROOT_DIR/config.toml"
fi

# Check ydotoold daemon is running; offer to start it (needs sudo)
YDOTOOLD_PID=""
cleanup() {
  if [[ -n "$YDOTOOLD_PID" ]]; then
    DEBUG "Stopping ydotoold (pid $YDOTOOLD_PID)"
    sudo kill "$YDOTOOLD_PID" 2>/dev/null || true
  fi
}

if ! pgrep -x ydotoold &>/dev/null; then
  ANSWER="n"
  if [[ -t 0 ]]; then
    WARN "ydotoold daemon is not running."
    read -r -n 1 -p "$(echo -e "\e[33m[SH] [WARN] Start it now with sudo? [y/N] \e[0m")" ANSWER
    echo
  fi

  if [[ "$ANSWER" =~ ^[Yy]$ ]]; then
    sudo -v || { ERROR "sudo authentication failed"; exit 1; }
    sudo env YDOTOOL_SOCKET=/tmp/.ydotool_socket \
      ydotoold --socket-own="$(id -u):$(id -g)" &
    YDOTOOLD_PID=$!
    trap cleanup EXIT INT TERM

    for _ in {1..50}; do
      [[ -S /tmp/.ydotool_socket ]] && break
      sleep 0.1
    done
    if [[ -S /tmp/.ydotool_socket ]]; then
      DEBUG "ydotoold ready (pid $YDOTOOLD_PID)"
    else
      ERROR "ydotoold socket /tmp/.ydotool_socket never appeared"
      exit 1
    fi
  else
    WARN "Continuing without ydotoold; start it with: ./scripts/start_ydotoold.sh"
  fi
fi

# Guard against concurrent instances
if pgrep -x voxpipe &>/dev/null; then
  ERROR "Another instance of VoxPipe is already running"
  exit 1
fi

# Configuration is read from ./config.toml, then ~/.config/voxpipe/config.toml.
set +e
"$BINARY"
EXIT_CODE=$?
set -e

DEBUG "VoxPipe exited with code: $EXIT_CODE"
INFO "Done!"
exit "$EXIT_CODE"
