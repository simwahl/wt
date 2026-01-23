#!/bin/bash

# wt Integration Test Script - Snapshot Testing
# Tests scenarios by comparing actual output to expected output

set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counter
TESTS_RUN=0
TESTS_PASSED=0

# Ensure WT_ROOT is set (Makefile should set this)
if [ -z "$WT_ROOT" ]; then
    echo "Error: WT_ROOT not set"
    exit 1
fi

# Skip prompts during testing (timer is in silent mode by default)
export WT_SKIP_PROMPTS=1

# Helper function to set mock time for testing
mock_time() {
    export WT_MOCK_TIME="$1"
}

# Helper function to run wt commands silently
run_wt() {
    $WT_CMD "$@" > /dev/null 2>&1
}

print_test() {
    echo -e "${YELLOW}TEST $1: $2${NC}"
}

print_pass() {
    echo -e "${GREEN}✓ PASS: $1${NC}"
    TESTS_PASSED=$((TESTS_PASSED + 1))
}

print_fail() {
    echo -e "${RED}✗ FAIL: $1${NC}"
    echo -e "${RED}Expected:${NC}"
    echo "$2"
    echo -e "${RED}Got:${NC}"
    echo "$3"
    exit 1
}

# Helper to compare output
check_output() {
    local test_name="$1"
    local expected="$2"
    local actual="$3"
    
    TESTS_RUN=$((TESTS_RUN + 1))
    
    if [ "$expected" = "$actual" ]; then
        print_pass "$test_name"
    else
        print_fail "$test_name" "$expected" "$actual"
    fi
}

# Set up test environment
setup_test() {
    rm -rf "$WT_ROOT/.out"
    mkdir -p "$WT_ROOT/.out"
    unset WT_MOCK_TIME
}

echo "=========================================="
echo "WT Integration Test Suite (Snapshot Testing)"
echo "=========================================="
echo ""
echo "Test directory: $WT_ROOT"
echo ""

###############################################################################
# Test 1: Full work day simulation (08:00-16:30)
###############################################################################
print_test "1" "Full work day simulation"
setup_test

# === Morning: Started working at 08:00 but forgot to start timer ===
mock_time "2026-01-20 08:45"
run_wt new
run_wt start 45  # Backdate 45 min to 08:00

# First work block until coffee break
mock_time "2026-01-20 09:30"
run_wt stop  # Work: 90 min (08:00-09:30)

# === Short break, back to work ===
mock_time "2026-01-20 09:45"
run_wt start  # Break: 15 min

# Interrupted by a call, need to pause
mock_time "2026-01-20 10:15"
run_wt pause

mock_time "2026-01-20 10:25"
run_wt start  # Resume after 10 min pause

mock_time "2026-01-20 11:00"
run_wt stop  # Work: 65 min (75 total - 10 paused)

# === Quick break, but actually started 5 min earlier ===
mock_time "2026-01-20 11:20"
run_wt start 5  # Break reduced from 20 to 15 min, backdates start to 11:15

mock_time "2026-01-20 12:00"
run_wt stop  # Work: 45 min (11:15-12:00)

# === Lunch break ===
mock_time "2026-01-20 13:00"
run_wt start  # Break: 60 min (lunch)

# Realize I had a 10 min interruption I forgot to pause for
mock_time "2026-01-20 13:30"
run_wt mod 7 pause add 10  # Add 10 min paused to current cycle

# === Afternoon: Use next for quick cycle change ===
mock_time "2026-01-20 14:00"
run_wt next  # Stop (work: 50 min = 60 total - 10 paused) + start immediately

mock_time "2026-01-20 14:45"
run_wt stop  # Work: 45 min

# === Realize first morning cycle was actually 10 min longer ===
run_wt mod 1 add 10  # Cycle 1: 90 -> 100 min

# === Afternoon continued ===
mock_time "2026-01-20 15:00"
run_wt start  # Break: 15 min

mock_time "2026-01-20 15:30"
run_wt pause  # Meeting

mock_time "2026-01-20 15:45"
run_wt start  # Resume after 15 min pause

mock_time "2026-01-20 16:30"
run_wt stop  # Work: 65 min (16:30 - 15:05 timeline start - 15 paused)

# === End of day adjustment: actually started 5 min earlier ===
run_wt mod start sub 5  # Day start: 08:00 -> 07:55

# === Validate full day log ===
expected_log="01. [07:55 => 09:35] Work: 1h:40m (1h:40m)
02. [09:35 => 09:50] Break: 0h:15m
03. [09:50 => 11:05] Work: 1h:05m |10m| (2h:45m)
04. [11:05 => 11:20] Break: 0h:15m
05. [11:20 => 12:05] Work: 0h:45m (3h:30m)
06. [12:05 => 13:05] Break: 1h:00m
07. [13:05 => 14:05] Work: 0h:50m |10m| (4h:20m)
08. [14:05 => 14:05] Break: 0h:00m
09. [14:05 => 14:50] Work: 0h:45m (5h:05m)
10. [14:50 => 15:05] Break: 0h:15m
11. [15:05 => 16:25] Work: 1h:05m |15m| (6h:10m)"
actual_log=$($WT_CMD log)
check_output "full day log matches expected" "$expected_log" "$actual_log"

# === Validate full day report ===
expected_report="2026-01-20 | 07:55 -> 16:25 | Work: 6h:10m | Break: 1h:45m | Paused: 0h:35m | Total: 8h:30m"
actual_report=$($WT_CMD report)
check_output "full day report matches expected" "$expected_report" "$actual_report"

###############################################################################
# Test 2: Basic pause/resume workflow
###############################################################################
print_test "2" "Basic pause/resume workflow"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

run_wt start

mock_time "2026-01-20 09:25"
run_wt pause

mock_time "2026-01-20 09:35"
run_wt start

mock_time "2026-01-20 09:50"
run_wt stop

expected_log="01. [09:00 => 09:50] Work: 0h:40m |10m| (0h:40m)"
actual_log=$($WT_CMD log)
check_output "log shows work and paused time" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 09:50 | Work: 0h:40m | Break: 0h:00m | Paused: 0h:10m | Total: 0h:50m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 3: Multiple work cycles with breaks
###############################################################################
print_test "3" "Multiple work cycles with breaks"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

run_wt start
mock_time "2026-01-20 09:20"
run_wt stop

mock_time "2026-01-20 09:25"
run_wt start
mock_time "2026-01-20 09:40"
run_wt stop

mock_time "2026-01-20 09:50"
run_wt start
mock_time "2026-01-20 10:05"
run_wt stop

expected_log="01. [09:00 => 09:20] Work: 0h:20m (0h:20m)
02. [09:20 => 09:25] Break: 0h:05m
03. [09:25 => 09:40] Work: 0h:15m (0h:35m)
04. [09:40 => 09:50] Break: 0h:10m
05. [09:50 => 10:05] Work: 0h:15m (0h:50m)"
actual_log=$($WT_CMD log)
check_output "log shows all cycles and breaks" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 10:05 | Work: 0h:50m | Break: 0h:15m | Paused: 0h:00m | Total: 1h:05m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 4: Start with backdate on first cycle
###############################################################################
print_test "4" "Start with backdate on first cycle"
setup_test

mock_time "2026-01-20 10:00"
run_wt new

run_wt start 30
mock_time "2026-01-20 10:15"
run_wt stop

expected_log="01. [09:30 => 10:15] Work: 0h:45m (0h:45m)"
actual_log=$($WT_CMD log)
check_output "log shows backdated start time" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:30 -> 10:15 | Work: 0h:45m | Break: 0h:00m | Paused: 0h:00m | Total: 0h:45m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 5: Start with break reduction on subsequent cycle
###############################################################################
print_test "5" "Start with break reduction on subsequent cycle"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

run_wt start
mock_time "2026-01-20 09:20"
run_wt stop

mock_time "2026-01-20 09:35"
run_wt start 10
mock_time "2026-01-20 09:45"
run_wt stop

expected_log="01. [09:00 => 09:20] Work: 0h:20m (0h:20m)
02. [09:20 => 09:25] Break: 0h:05m
03. [09:25 => 09:45] Work: 0h:20m (0h:40m)"
actual_log=$($WT_CMD log)
check_output "log shows reduced break time" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 09:45 | Work: 0h:40m | Break: 0h:05m | Paused: 0h:00m | Total: 0h:45m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 6: Mod command to adjust cycle durations
###############################################################################
print_test "6" "Mod command to adjust cycle durations"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

run_wt start
mock_time "2026-01-20 09:05"
run_wt stop

run_wt mod 1 add 15

expected_log="01. [09:00 => 09:20] Work: 0h:20m (0h:20m)"
actual_log=$($WT_CMD log)
check_output "log shows modified duration" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 09:20 | Work: 0h:20m | Break: 0h:00m | Paused: 0h:00m | Total: 0h:20m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 7: Mod start to change day start time
###############################################################################
print_test "7" "Mod start to change day start time"
setup_test

mock_time "2026-01-20 10:00"
run_wt new

run_wt start
mock_time "2026-01-20 10:30"
run_wt stop

run_wt mod start sub 60

expected_log="01. [09:00 => 09:30] Work: 0h:30m (0h:30m)"
actual_log=$($WT_CMD log)
check_output "log shows adjusted start time" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 09:30 | Work: 0h:30m | Break: 0h:00m | Paused: 0h:00m | Total: 0h:30m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 8: Pause immediately after start
###############################################################################
print_test "8" "Pause immediately after start"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

run_wt start
run_wt pause

mock_time "2026-01-20 09:30"
run_wt start
mock_time "2026-01-20 09:45"
run_wt stop

expected_log="01. [09:00 => 09:45] Work: 0h:15m |30m| (0h:15m)"
actual_log=$($WT_CMD log)
check_output "log shows mostly paused cycle" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 09:45 | Work: 0h:15m | Break: 0h:00m | Paused: 0h:30m | Total: 0h:45m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 9: Multiple pause/resume in same cycle
###############################################################################
print_test "9" "Multiple pause/resume in same cycle"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

run_wt start
mock_time "2026-01-20 09:10"
run_wt pause

mock_time "2026-01-20 09:20"
run_wt start
mock_time "2026-01-20 09:25"
run_wt pause

mock_time "2026-01-20 09:35"
run_wt start
mock_time "2026-01-20 09:45"
run_wt stop

expected_log="01. [09:00 => 09:45] Work: 0h:25m |20m| (0h:25m)"
actual_log=$($WT_CMD log)
check_output "log shows accumulated paused time" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 09:45 | Work: 0h:25m | Break: 0h:00m | Paused: 0h:20m | Total: 0h:45m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 10: Next command (skip break)
###############################################################################
print_test "10" "Next command skips break"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

run_wt start
mock_time "2026-01-20 09:20"
run_wt next
mock_time "2026-01-20 09:40"
run_wt stop

expected_log="01. [09:00 => 09:20] Work: 0h:20m (0h:20m)
02. [09:20 => 09:20] Break: 0h:00m
03. [09:20 => 09:40] Work: 0h:20m (0h:40m)"
actual_log=$($WT_CMD log)
check_output "log shows zero-minute break" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 09:40 | Work: 0h:40m | Break: 0h:00m | Paused: 0h:00m | Total: 0h:40m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 11: Drop a cycle
###############################################################################
print_test "11" "Drop a cycle"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

run_wt start
mock_time "2026-01-20 09:20"
run_wt stop

mock_time "2026-01-20 09:25"
run_wt start
mock_time "2026-01-20 09:40"
run_wt stop

run_wt mod 1 drop

expected_log="01. [09:00 => 09:05] Break: 0h:05m
02. [09:05 => 09:20] Work: 0h:15m (0h:15m)"
actual_log=$($WT_CMD log)
check_output "log shows remaining cycle with adjusted times" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 09:20 | Work: 0h:15m | Break: 0h:05m | Paused: 0h:00m | Total: 0h:20m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 12: Drop break merges work cycles correctly
###############################################################################
print_test "12" "Drop break merges work cycles correctly"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

run_wt start
mock_time "2026-01-20 09:20"
run_wt stop

mock_time "2026-01-20 09:30"
run_wt start
mock_time "2026-01-20 09:45"
run_wt stop

run_wt mod 2 drop

# When dropping a break, it means "I was actually working during that time"
# Work 1 (20m) + Break (10m, now work) + Work 2 (15m) = 45m work, 09:00-09:45
expected_log="01. [09:00 => 09:45] Work: 0h:45m (0h:45m)"
actual_log=$($WT_CMD log)
check_output "merged cycle spans from first start to second end" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 09:45 | Work: 0h:45m | Break: 0h:00m | Paused: 0h:00m | Total: 0h:45m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 12b: Drop work cycle merges breaks correctly
###############################################################################
print_test "12b" "Drop work cycle merges breaks correctly"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

run_wt start
mock_time "2026-01-20 09:20"
run_wt stop  # Work: 20m

mock_time "2026-01-20 09:30"
run_wt start  # Break: 10m
mock_time "2026-01-20 09:45"
run_wt stop  # Work: 15m

mock_time "2026-01-20 10:00"
run_wt start  # Break: 15m
mock_time "2026-01-20 10:30"
run_wt stop  # Work: 30m

run_wt mod 3 drop

# When dropping a work cycle, it means "I wasn't actually working, still on break"
# Work 1 (20m) + Break (10m) + Work 2 dropped (15m, now break) + Break (15m) + Work 3 (30m)
# = Work 1 (20m) + merged Break (10+15+15=40m) + Work 3 (30m)
expected_log="01. [09:00 => 09:20] Work: 0h:20m (0h:20m)
02. [09:20 => 10:00] Break: 0h:40m
03. [10:00 => 10:30] Work: 0h:30m (0h:50m)"
actual_log=$($WT_CMD log)
check_output "dropped work becomes break time" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 10:30 | Work: 0h:50m | Break: 0h:40m | Paused: 0h:00m | Total: 1h:30m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 13: Restart with backdate
###############################################################################
print_test "13" "Restart with backdate"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

run_wt start
mock_time "2026-01-20 09:30"
run_wt stop

mock_time "2026-01-20 10:00"
run_wt restart 15

mock_time "2026-01-20 10:20"
run_wt stop

expected_log="01. [09:45 => 10:20] Work: 0h:35m (0h:35m)"
actual_log=$($WT_CMD log)
check_output "restart with backdate creates fresh timer" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:45 -> 10:20 | Work: 0h:35m | Break: 0h:00m | Paused: 0h:00m | Total: 0h:35m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 14: Mod while timer is running
###############################################################################
print_test "14" "Mod while timer is running"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

run_wt start
mock_time "2026-01-20 09:30"
run_wt stop

mock_time "2026-01-20 09:45"
run_wt start

# While running, drop the previous break at 09:55
# At this point: first work = 30 min, break = 15 min, current running = 10 min (09:45-09:55)
# After drop: merged = 30 + 15 + 10 = 55 min, continues accumulating
mock_time "2026-01-20 09:55"
run_wt mod 2 drop

# Continue working for another 5 minutes (09:55-10:00)
mock_time "2026-01-20 10:00"
run_wt stop

# Timeline should show single merged entry: 55 + 5 = 60 min total
expected_log="01. [09:00 => 10:00] Work: 1h:00m (1h:00m)"
actual_log=$($WT_CMD log)
check_output "log shows single merged cycle after dropping break while running" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 10:00 | Work: 1h:00m | Break: 0h:00m | Paused: 0h:00m | Total: 1h:00m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 15: Mod while timer is paused
###############################################################################
print_test "15" "Mod while timer is paused"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

run_wt start
mock_time "2026-01-20 09:30"
run_wt stop

mock_time "2026-01-20 09:40"
run_wt start

mock_time "2026-01-20 09:50"
run_wt pause

# While paused, modify the first cycle duration
run_wt mod 1 add 10

mock_time "2026-01-20 10:00"
run_wt start

mock_time "2026-01-20 10:15"
run_wt stop

# First cycle now 40 min (30 + 10), break unchanged at 10 min, so cycle 3 starts at 09:50
expected_log="01. [09:00 => 09:40] Work: 0h:40m (0h:40m)
02. [09:40 => 09:50] Break: 0h:10m
03. [09:50 => 10:15] Work: 0h:15m |10m| (0h:55m)"
actual_log=$($WT_CMD log)
check_output "log shows modified cycle duration" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 10:15 | Work: 0h:55m | Break: 0h:10m | Paused: 0h:10m | Total: 1h:15m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 16: Report includes current cycle paused time
###############################################################################
print_test "16" "Report includes current cycle paused time"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

# Start work
run_wt start
mock_time "2026-01-20 09:20"
run_wt pause

# Check report while paused - should show 20min paused
mock_time "2026-01-20 09:40"
expected_report="2026-01-20 | 09:00 -> 09:20 | Work: 0h:20m | Break: 0h:00m | Paused: 0h:20m | Total: 0h:40m"
actual_report=$($WT_CMD report)
check_output "report shows paused time while paused" "$expected_report" "$actual_report"

# Resume and work more
run_wt start
mock_time "2026-01-20 09:50"

# Check report while running - should still show 20min paused
expected_report="2026-01-20 | 09:00 -> 09:30 | Work: 0h:30m | Break: 0h:00m | Paused: 0h:20m | Total: 0h:50m"
actual_report=$($WT_CMD report)
check_output "report shows paused time while running" "$expected_report" "$actual_report"

# Stop and verify final report
run_wt stop
expected_report="2026-01-20 | 09:00 -> 09:50 | Work: 0h:30m | Break: 0h:00m | Paused: 0h:20m | Total: 0h:50m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals after stop" "$expected_report" "$actual_report"

###############################################################################
# Test 17: Mod pause command for stopped and active cycles
###############################################################################
print_test "17" "Mod pause command for stopped and active cycles"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

# Create a stopped cycle with 5min paused
run_wt start
mock_time "2026-01-20 09:20"
run_wt pause
mock_time "2026-01-20 09:25"
run_wt start
mock_time "2026-01-20 09:40"
run_wt stop

# Add 10min to stopped cycle's paused time
run_wt mod 1 pause add 10
expected_log="01. [09:00 => 09:50] Work: 0h:35m |15m| (0h:35m)"
actual_log=$($WT_CMD log)
check_output "stopped cycle paused time increased" "$expected_log" "$actual_log"

# Subtract 5min from stopped cycle's paused time
run_wt mod 1 pause sub 5
expected_log="01. [09:00 => 09:45] Work: 0h:35m |10m| (0h:35m)"
actual_log=$($WT_CMD log)
check_output "stopped cycle paused time decreased" "$expected_log" "$actual_log"

# Start a new cycle with paused time
run_wt start
mock_time "2026-01-20 10:00"
run_wt pause
mock_time "2026-01-20 10:05"
run_wt start
mock_time "2026-01-20 10:10"

# Add 10min to current running cycle's paused time (simulating forgot to pause)
# Work is calculated from timeline start (09:45), not actual start
run_wt mod 3 pause add 10
expected_log="01. [09:00 => 09:45] Work: 0h:35m |10m| (0h:35m)
02. [09:45 => 09:45] Break: 0h:00m
03. [09:45 => .....] Work: 0h:10m |15m| (0h:45m)"
actual_log=$($WT_CMD log)
check_output "current cycle paused time modified" "$expected_log" "$actual_log"

# Verify modifying pause time while paused is blocked
run_wt pause
mock_time "2026-01-20 10:15"
expected_error="Cannot modify pause time while paused.
Resume first with 'wt start', then modify pause time."
actual_error=$($WT_CMD mod 3 pause sub 5 2>&1)
check_output "cannot modify pause time while paused" "$expected_error" "$actual_error"

###############################################################################
# Test 18: Cannot modify duration of current running/paused cycle
###############################################################################
print_test "18" "Cannot modify duration of current running/paused cycle"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

run_wt start
mock_time "2026-01-20 09:30"
run_wt stop

mock_time "2026-01-20 09:40"
run_wt start
mock_time "2026-01-20 09:50"

# Try to modify current running cycle (cycle 3) - should error
expected_error="Cannot modify duration of current running cycle.
To adjust when this cycle started, modify the previous cycle or break duration.
To adjust paused time: wt mod 3 pause <add|sub> <time>"
actual_error=$($WT_CMD mod 3 add 10 2>&1)
check_output "error when modifying running cycle duration" "$expected_error" "$actual_error"

# Pause and try again - should also error
run_wt pause
mock_time "2026-01-20 09:55"
expected_error="Cannot modify duration of current running cycle.
To adjust when this cycle started, modify the previous cycle or break duration.
To adjust paused time: wt mod 3 pause <add|sub> <time>"
actual_error=$($WT_CMD mod 3 sub 5 2>&1)
check_output "error when modifying paused cycle duration" "$expected_error" "$actual_error"

# Verify we CAN modify previous cycles
# This extends cycle 1 by 10 min, shifting timeline. Cycle 3 work is calculated
# from timeline start, so with the shift, current work shows 0 min.
run_wt mod 1 add 10
expected_log="01. [09:00 => 09:40] Work: 0h:40m (0h:40m)
02. [09:40 => 09:50] Break: 0h:10m
03. [09:50 => .....] Work (paused): 0h:00m |05m| (0h:40m)"
actual_log=$($WT_CMD log)
check_output "can still modify previous cycle while current is paused" "$expected_log" "$actual_log"

###############################################################################
# Test 19: Status command edge cases
###############################################################################
print_test "19" "Status command edge cases"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

# Status when stopped
expected_status="stopped"
actual_status=$($WT_CMD status)
check_output "status shows stopped" "$expected_status" "$actual_status"

# Status when running
run_wt start
expected_status="running"
actual_status=$($WT_CMD status)
check_output "status shows running" "$expected_status" "$actual_status"

# Status when paused
run_wt pause
expected_status="paused"
actual_status=$($WT_CMD status)
check_output "status shows paused" "$expected_status" "$actual_status"

###############################################################################
# Test 20: Duplicate state transitions (already running/stopped/paused)
###############################################################################
print_test "20" "Duplicate state transitions"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

# Start when stopped - then start again
run_wt start
expected_msg="Already running."
actual_msg=$($WT_CMD start 2>&1)
check_output "start when already running" "$expected_msg" "$actual_msg"

# Pause then pause again
run_wt pause
expected_msg="Timer already paused."
actual_msg=$($WT_CMD pause 2>&1)
check_output "pause when already paused" "$expected_msg" "$actual_msg"

# Stop then stop again
run_wt stop
expected_msg="Timer already stopped."
actual_msg=$($WT_CMD stop 2>&1)
check_output "stop when already stopped" "$expected_msg" "$actual_msg"

# Pause when stopped
expected_msg="Cannot pause stopped timer."
actual_msg=$($WT_CMD pause 2>&1)
check_output "pause when stopped" "$expected_msg" "$actual_msg"

###############################################################################
# Test 21: Check command output format
###############################################################################
print_test "21" "Check command output format"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

# Check when stopped
expected_check="--:-- STOPPED (0h 00m)"
actual_check=$($WT_CMD check)
check_output "check when stopped" "$expected_check" "$actual_check"

# Check when running
run_wt start
mock_time "2026-01-20 09:15"
expected_check="0h 15m RUNNING (0h 15m)"
actual_check=$($WT_CMD check)
check_output "check when running" "$expected_check" "$actual_check"

# Check when paused
run_wt pause
mock_time "2026-01-20 09:20"
expected_check="0h 15m PAUSED |05m| (0h 15m)"
actual_check=$($WT_CMD check)
check_output "check when paused shows pause time" "$expected_check" "$actual_check"

###############################################################################
# Test 22: Mode command
###############################################################################
print_test "22" "Mode command"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

# Default mode is silent
expected_mode="silent"
actual_mode=$($WT_CMD mode)
check_output "default mode is silent" "$expected_mode" "$actual_mode"

# Set mode to normal
run_wt mode normal
expected_mode="normal"
actual_mode=$($WT_CMD mode)
check_output "mode set to normal" "$expected_mode" "$actual_mode"

# Set mode to verbose
run_wt mode verbose
expected_mode="verbose"
actual_mode=$($WT_CMD mode)
check_output "mode set to verbose" "$expected_mode" "$actual_mode"

###############################################################################
# Test 23: Mod command validation errors
###############################################################################
print_test "23" "Mod command validation errors"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

run_wt start
mock_time "2026-01-20 09:30"
run_wt stop

# Invalid cycle number (too high)
expected_error="Cycle 5 does not exist. Valid range: 1-1"
actual_error=$($WT_CMD mod 5 add 10 2>&1)
check_output "error for invalid cycle number" "$expected_error" "$actual_error"

# Subtract more than available
expected_error="Error: Duration would be negative. Current: 0h:30m"
actual_error=$($WT_CMD mod 1 sub 45 2>&1)
check_output "error when subtracting too much duration" "$expected_error" "$actual_error"

# Try to modify pause on a break entry
mock_time "2026-01-20 09:40"
run_wt start
mock_time "2026-01-20 09:50"
run_wt stop
# Now: cycle 1 = work, cycle 2 = break, cycle 3 = work
expected_error="Cycle 2 is a break. Paused time can only be modified for work cycles."
actual_error=$($WT_CMD mod 2 pause add 5 2>&1)
check_output "error when modifying pause on break" "$expected_error" "$actual_error"

###############################################################################
# Test 24: Start with break reduction validation
###############################################################################
print_test "24" "Start with break reduction validation"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

run_wt start
mock_time "2026-01-20 09:20"
run_wt stop

# Try to reduce break by more than its duration (break is 5 min, try to subtract 10)
mock_time "2026-01-20 09:25"
expected_error="Cannot reduce break below 0. Break was 0h:05m, tried to subtract 0h:10m."
actual_error=$($WT_CMD start 10 2>&1)
check_output "error when reducing break below zero" "$expected_error" "$actual_error"

###############################################################################
# Test 25: Log with invalid type
###############################################################################
print_test "25" "Log with invalid type"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

expected_error="Invalid log type: invalid. Use one of: ['info', 'debug']"
actual_error=$($WT_CMD log invalid 2>&1)
check_output "error for invalid log type" "$expected_error" "$actual_error"

###############################################################################
# Test 26: Work crossing midnight
###############################################################################
print_test "26" "Work crossing midnight"
setup_test

mock_time "2026-01-20 23:00"
run_wt new

run_wt start
mock_time "2026-01-21 01:30"
run_wt stop

expected_log="01. [23:00 => 01:30] Work: 2h:30m (2h:30m)  [+1 day]"
actual_log=$($WT_CMD log)
check_output "log shows day indicator for midnight crossing" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 23:00 -> 01:30 | Work: 2h:30m | Break: 0h:00m | Paused: 0h:00m | Total: 2h:30m [+1 day]"
actual_report=$($WT_CMD report)
check_output "report shows day indicator for midnight crossing" "$expected_report" "$actual_report"

###############################################################################
# Test 27: Mod start add direction while running first cycle
###############################################################################
print_test "27" "Mod start add direction while running first cycle"
setup_test

mock_time "2026-01-20 09:00"
run_wt new

run_wt start
mock_time "2026-01-20 09:30"

# Mod start add 15 should make it appear we started later (09:15)
run_wt mod start add 15
mock_time "2026-01-20 09:45"
run_wt stop

expected_log="01. [09:15 => 09:45] Work: 0h:30m (0h:30m)"
actual_log=$($WT_CMD log)
check_output "mod start add adjusts first cycle later" "$expected_log" "$actual_log"

echo ""
echo "=========================================="
echo "Test Results"
echo "=========================================="
echo "Tests run: $TESTS_RUN"
echo "Tests passed: $TESTS_PASSED"

if [ $TESTS_RUN -eq $TESTS_PASSED ]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed${NC}"
    exit 1
fi
