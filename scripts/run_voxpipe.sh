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

BINARY="$ROOT_DIR/bin/voxpipe"
if [[ ! -x "$BINARY" ]]; then
  ERROR "$BINARY not found or not executable. Build it first with: make build"
  exit 1
fi

# Guard against concurrent instances, before starting anything else
if pgrep -x voxpipe &>/dev/null; then
  ERROR "Another instance of VoxPipe is already running"
  exit 1
fi

# First run: create ./config.toml from the checked-in example, unless a user config exists
USER_CONFIG="${XDG_CONFIG_HOME:-$HOME/.config}/voxpipe/config.toml"
if [[ ! -f "$ROOT_DIR/config.toml" && ! -f "$USER_CONFIG" ]]; then
  WARN "config.toml not found, copying config.example.toml. Review it (modelDir, deviceName, outputs)."
  cp "$ROOT_DIR/config.example.toml" "$ROOT_DIR/config.toml"
fi

# Check the ydotoold daemon is running; offer to start it (needs sudo). It is left running
# after voxpipe exits, same as when started by hand with scripts/start_ydotoold.sh.
YDOTOOLD_PID="$(pgrep -x ydotoold | head -1 || true)"
if [[ -n "$YDOTOOLD_PID" ]]; then
  DEBUG "ydotoold already running (pid $YDOTOOLD_PID)"
else
  ANSWER="n"
  if [[ -t 0 ]]; then
    WARN "ydotoold daemon is not running."
    read -r -n 1 -p "$(echo -e "\e[33m[SH] [WARN] Start it now with sudo? [y/N] \e[0m")" ANSWER
    echo
  fi

  if [[ "$ANSWER" =~ ^[Yy]$ ]]; then
    # Authenticate in the foreground so the backgrounded helper never prompts on the tty.
    sudo -v || { ERROR "sudo authentication failed"; exit 1; }
    "$ROOT_DIR/scripts/start_ydotoold.sh" &

    for _ in {1..50}; do
      [[ -S /tmp/.ydotool_socket ]] && break
      sleep 0.1
    done
    if [[ ! -S /tmp/.ydotool_socket ]]; then
      ERROR "ydotoold socket /tmp/.ydotool_socket never appeared"
      exit 1
    fi
    DEBUG "ydotoold ready; it keeps running after VoxPipe exits"
  else
    WARN "Continuing without ydotoold; start it with: ./scripts/start_ydotoold.sh"
  fi
fi

# Terminal state, captured before voxpipe puts stdin in cbreak mode, so a crash or kill
# does not leave the shell in raw mode.
if [[ -t 0 ]] && TTY_STATE="$(stty -g 2>/dev/null)"; then
  trap 'stty "$TTY_STATE" 2>/dev/null || stty sane 2>/dev/null || true' EXIT
fi

# Configuration is read from ./config.toml, then ~/.config/voxpipe/config.toml.
EXIT_CODE=0
"$BINARY" || EXIT_CODE=$?

DEBUG "VoxPipe exited with code: $EXIT_CODE"
exit "$EXIT_CODE"
