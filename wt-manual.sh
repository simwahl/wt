#!/bin/bash
# Isolated manual wrapper for wt.
# Runs wt with state in /tmp so real logs are never touched.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
STATE_ROOT="${WT_MANUAL_ROOT:-/tmp/wt-manual-${USER:-user}}"

mkdir -p "$STATE_ROOT"

export WT_ROOT="$STATE_ROOT"
export WT_GAME_PATH="$STATE_ROOT/wtg.json"
export WT_SKIP_PROMPTS=1

BINARY="$SCRIPT_DIR/.out/wt"
if [ ! -x "$BINARY" ] || [ "$SCRIPT_DIR/wt.go" -nt "$BINARY" ] || [ "$SCRIPT_DIR/wt-game.go" -nt "$BINARY" ]; then
	go build -o "$BINARY" "$SCRIPT_DIR/wt.go" "$SCRIPT_DIR/wt-game.go"
fi

if [ "$#" -eq 0 ]; then
	echo "wt-manual: isolated wt wrapper"
	echo "WT_ROOT=$WT_ROOT"
	echo "WT_GAME_PATH=$WT_GAME_PATH"
	echo ""
	echo "Usage: ./wt-manual.sh <wt-command> [args]"
	echo "Examples:"
	echo "  ./wt-manual.sh new"
	echo "  ./wt-manual.sh start"
	echo "  ./wt-manual.sh check"
	exit 0
fi

"$BINARY" "$@"
