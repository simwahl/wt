#!/bin/bash
# Isolated manual wrapper for wt.
# Runs wt with state in /tmp so real logs are never touched.

set -euo pipefail

# Resolve the real script location even when invoked via a symlink on PATH
# (e.g. /usr/local/bin/wt-sandbox -> .../wt/wt-sandbox.sh).
SOURCE="${BASH_SOURCE[0]}"
while [ -h "$SOURCE" ]; do
	DIR="$(cd "$(dirname "$SOURCE")" && pwd)"
	SOURCE="$(readlink "$SOURCE")"
	[[ "$SOURCE" != /* ]] && SOURCE="$DIR/$SOURCE"
done
SCRIPT_DIR="$(cd "$(dirname "$SOURCE")" && pwd)"
STATE_ROOT="${WT_SANDBOX_ROOT:-/tmp/wt-sandbox-${USER:-user}}"

mkdir -p "$STATE_ROOT"

export WT_ROOT="$STATE_ROOT"
export WT_GAME_PATH="$STATE_ROOT/wtg.json"
export WT_REPORT_FILE="$STATE_ROOT/daily-reports"
export WT_FLEX_FILE="$STATE_ROOT/Flex.md"
export WT_SKIP_PROMPTS=1

BINARY="$SCRIPT_DIR/.out/wt"
if [ ! -x "$BINARY" ] || [ "$SCRIPT_DIR/wt.go" -nt "$BINARY" ] || [ "$SCRIPT_DIR/wt-game.go" -nt "$BINARY" ]; then
	go build -o "$BINARY" "$SCRIPT_DIR/wt.go" "$SCRIPT_DIR/wt-game.go"
fi

# Optional leading --at/-t flag sets WT_MOCK_TIME without an inline env prefix,
# so the whole command starts with `wt-sandbox` and is easy to auto-approve.
if [ "$#" -ge 2 ] && { [ "$1" = "--at" ] || [ "$1" = "-t" ]; }; then
	export WT_MOCK_TIME="$2"
	shift 2
fi

if [ "$#" -eq 0 ]; then
	echo "wt-sandbox: isolated wt wrapper"
	echo "WT_ROOT=$WT_ROOT"
	echo "WT_GAME_PATH=$WT_GAME_PATH"
	echo "WT_REPORT_FILE=$WT_REPORT_FILE"
	echo "WT_FLEX_FILE=$WT_FLEX_FILE"
	echo ""
	echo "Usage: wt-sandbox [--at \"YYYY-MM-DD HH:MM\"] <wt-command> [args]"
	echo "Examples:"
	echo "  wt-sandbox new"
	echo "  wt-sandbox start"
	echo "  wt-sandbox check"
	echo "  wt-sandbox --at \"2026-01-20 09:00\" start"
	exit 0
fi

"$BINARY" "$@"
