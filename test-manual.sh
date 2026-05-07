#!/bin/bash
# Manual testing script for WT
# Usage: ./test-manual.sh [scenario]
# Runs an isolated test environment that won't affect your real work log.

set -e

TEST_ROOT="/tmp/wt-manual-$$"
mkdir -p "$TEST_ROOT"

# Build the binary
echo "Building..."
go build -o .out/wt wt.go wt-game.go

export WT_ROOT="$TEST_ROOT"
export WT_GAME_PATH="$TEST_ROOT/wtg.json"
export WT_MOCK_TIME="${WT_MOCK_TIME:-}"
export WT_SKIP_PROMPTS=1

# Helper function to run wt commands
wt() {
    ./.out/wt "$@"
}

# Scenario: Basic start/stop/check
scenario_basic() {
    echo "=== Scenario: Basic timer ==="
    wt new
    wt start
    wt check
    sleep 2
    wt stop
    wt report
}

# Scenario: Game display
scenario_game() {
    echo "=== Scenario: Game display ==="
    wt new
    WT_MOCK_TIME="2026-05-07 08:15" wt start
    WT_MOCK_TIME="2026-05-07 10:00" wt stop
    WT_MOCK_TIME="2026-05-07 10:05" wt start
    WT_MOCK_TIME="2026-05-07 12:00" wt stop
    WT_MOCK_TIME="2026-05-07 13:00" wt start
    WT_MOCK_TIME="2026-05-07 15:00" wt game
}

# Scenario: Break time command
scenario_break_time() {
    echo "=== Scenario: Break time (wt bt) ==="
    wt new
    WT_MOCK_TIME="2026-05-07 08:15" wt start
    WT_MOCK_TIME="2026-05-07 10:00" wt stop
    WT_MOCK_TIME="2026-05-07 10:05" wt start
    WT_MOCK_TIME="2026-05-07 12:00" wt stop
    WT_MOCK_TIME="2026-05-07 13:00" wt start
    WT_MOCK_TIME="2026-05-07 14:30" wt stop
    echo ""
    WT_MOCK_TIME="2026-05-07 14:30" wt bt 16:30
    echo ""
    WT_MOCK_TIME="2026-05-07 14:30" wt bt 16:00
    echo ""
    WT_MOCK_TIME="2026-05-07 14:30" wt bt 15:00
}

# Scenario: Full realistic day
scenario_full_day() {
    echo "=== Scenario: Full realistic day ==="
    wt new
    # Morning: 08:15 - 11:15 (work with breaks)
    WT_MOCK_TIME="2026-05-07 08:15" wt start
    WT_MOCK_TIME="2026-05-07 08:45" wt stop
    WT_MOCK_TIME="2026-05-07 08:51" wt start
    WT_MOCK_TIME="2026-05-07 09:40" wt stop
    WT_MOCK_TIME="2026-05-07 09:48" wt start
    WT_MOCK_TIME="2026-05-07 10:30" wt stop
    WT_MOCK_TIME="2026-05-07 10:40" wt start
    WT_MOCK_TIME="2026-05-07 11:15" wt stop
    # Afternoon: 12:15 - 16:30 (more work)
    WT_MOCK_TIME="2026-05-07 12:15" wt start
    WT_MOCK_TIME="2026-05-07 12:55" wt stop
    WT_MOCK_TIME="2026-05-07 13:10" wt start
    WT_MOCK_TIME="2026-05-07 13:50" wt stop
    WT_MOCK_TIME="2026-05-07 14:05" wt start
    WT_MOCK_TIME="2026-05-07 16:30" wt stop
    echo ""
    WT_MOCK_TIME="2026-05-07 16:30" wt game
}

# Show available scenarios
if [ -z "$1" ]; then
    echo "Manual testing for WT"
    echo "Test environment: $TEST_ROOT"
    echo ""
    echo "Usage: $0 [scenario]"
    echo ""
    echo "Available scenarios:"
    echo "  basic       - Start, stop, check, report"
    echo "  game        - Game display with time transitions"
    echo "  break-time  - Test 'wt bt' command with various targets"
    echo "  full-day    - Realistic full workday simulation"
    echo "  interactive - Open a shell in test environment"
    echo ""
    echo "Examples:"
    echo "  $0 basic"
    echo "  $0 game"
    echo "  $0 break-time"
    exit 0
fi

case "$1" in
    basic)
        scenario_basic
        ;;
    game)
        scenario_game
        ;;
    break-time)
        scenario_break_time
        ;;
    full-day)
        scenario_full_day
        ;;
    interactive)
        echo "Opening interactive shell in $TEST_ROOT"
        echo "Run: wt [command]"
        bash
        ;;
    *)
        echo "Unknown scenario: $1"
        exit 1
        ;;
esac

echo ""
echo "Test data is in: $TEST_ROOT"
echo "To clean up: rm -rf $TEST_ROOT"
