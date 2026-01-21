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

WT_CMD="python3 $(pwd)/wt.py"

# Ensure WT_ROOT is set (Makefile should set this)
if [ -z "$WT_ROOT" ]; then
    echo "Error: WT_ROOT not set"
    exit 1
fi

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
# Test 1: Basic pause/resume workflow
###############################################################################
print_test "1" "Basic pause/resume workflow"
setup_test

export WT_MOCK_TIME="2026-01-20 09:00"
echo 'y' | $WT_CMD new > /dev/null 2>&1

$WT_CMD start > /dev/null 2>&1

export WT_MOCK_TIME="2026-01-20 09:25"
$WT_CMD pause > /dev/null 2>&1

export WT_MOCK_TIME="2026-01-20 09:35"
$WT_CMD start > /dev/null 2>&1

export WT_MOCK_TIME="2026-01-20 09:50"
$WT_CMD stop > /dev/null 2>&1

expected_log="01. [09:00 => 09:50] Work: 0h:40m, Paused: 0h:10m (0h:40m)"
actual_log=$($WT_CMD log)
check_output "log shows work and paused time" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 09:50 | Work: 0h:40m | Break: 0h:00m | Paused: 0h:10m | Total: 0h:50m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 2: Multiple work cycles with breaks
###############################################################################
print_test "2" "Multiple work cycles with breaks"
setup_test

export WT_MOCK_TIME="2026-01-20 09:00"
echo 'y' | $WT_CMD new > /dev/null 2>&1

$WT_CMD start > /dev/null 2>&1
export WT_MOCK_TIME="2026-01-20 09:20"
$WT_CMD stop > /dev/null 2>&1

export WT_MOCK_TIME="2026-01-20 09:25"
$WT_CMD start > /dev/null 2>&1
export WT_MOCK_TIME="2026-01-20 09:40"
$WT_CMD stop > /dev/null 2>&1

export WT_MOCK_TIME="2026-01-20 09:50"
$WT_CMD start > /dev/null 2>&1
export WT_MOCK_TIME="2026-01-20 10:05"
$WT_CMD stop > /dev/null 2>&1

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
# Test 3: Start with backdate on first cycle
###############################################################################
print_test "3" "Start with backdate on first cycle"
setup_test

export WT_MOCK_TIME="2026-01-20 10:00"
echo 'y' | $WT_CMD new > /dev/null 2>&1

$WT_CMD start 30 > /dev/null 2>&1
export WT_MOCK_TIME="2026-01-20 10:15"
$WT_CMD stop > /dev/null 2>&1

expected_log="01. [09:30 => 10:15] Work: 0h:45m (0h:45m)"
actual_log=$($WT_CMD log)
check_output "log shows backdated start time" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:30 -> 10:15 | Work: 0h:45m | Break: 0h:00m | Paused: 0h:00m | Total: 0h:45m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 4: Start with break reduction on subsequent cycle
###############################################################################
print_test "4" "Start with break reduction on subsequent cycle"
setup_test

export WT_MOCK_TIME="2026-01-20 09:00"
echo 'y' | $WT_CMD new > /dev/null 2>&1

$WT_CMD start > /dev/null 2>&1
export WT_MOCK_TIME="2026-01-20 09:20"
$WT_CMD stop > /dev/null 2>&1

export WT_MOCK_TIME="2026-01-20 09:35"
$WT_CMD start 10 > /dev/null 2>&1
export WT_MOCK_TIME="2026-01-20 09:45"
$WT_CMD stop > /dev/null 2>&1

expected_log="01. [09:00 => 09:20] Work: 0h:20m (0h:20m)
02. [09:20 => 09:25] Break: 0h:05m
03. [09:25 => 09:45] Work: 0h:20m (0h:40m)"
actual_log=$($WT_CMD log)
check_output "log shows reduced break time" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 09:45 | Work: 0h:40m | Break: 0h:05m | Paused: 0h:00m | Total: 0h:45m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 5: Mod command to adjust cycle durations
###############################################################################
print_test "5" "Mod command to adjust cycle durations"
setup_test

export WT_MOCK_TIME="2026-01-20 09:00"
echo 'y' | $WT_CMD new > /dev/null 2>&1

$WT_CMD start > /dev/null 2>&1
export WT_MOCK_TIME="2026-01-20 09:05"
$WT_CMD stop > /dev/null 2>&1

$WT_CMD mod 1 add 15 > /dev/null 2>&1

expected_log="01. [09:00 => 09:20] Work: 0h:20m (0h:20m)"
actual_log=$($WT_CMD log)
check_output "log shows modified duration" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 09:20 | Work: 0h:20m | Break: 0h:00m | Paused: 0h:00m | Total: 0h:20m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 6: Mod start to change day start time
###############################################################################
print_test "6" "Mod start to change day start time"
setup_test

export WT_MOCK_TIME="2026-01-20 10:00"
echo 'y' | $WT_CMD new > /dev/null 2>&1

$WT_CMD start > /dev/null 2>&1
export WT_MOCK_TIME="2026-01-20 10:30"
$WT_CMD stop > /dev/null 2>&1

$WT_CMD mod start sub 60 > /dev/null 2>&1

expected_log="01. [09:00 => 09:30] Work: 0h:30m (0h:30m)"
actual_log=$($WT_CMD log)
check_output "log shows adjusted start time" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 09:30 | Work: 0h:30m | Break: 0h:00m | Paused: 0h:00m | Total: 0h:30m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 7: Pause immediately after start
###############################################################################
print_test "7" "Pause immediately after start"
setup_test

export WT_MOCK_TIME="2026-01-20 09:00"
echo 'y' | $WT_CMD new > /dev/null 2>&1

$WT_CMD start > /dev/null 2>&1
$WT_CMD pause > /dev/null 2>&1

export WT_MOCK_TIME="2026-01-20 09:30"
$WT_CMD start > /dev/null 2>&1
export WT_MOCK_TIME="2026-01-20 09:45"
$WT_CMD stop > /dev/null 2>&1

expected_log="01. [09:00 => 09:45] Work: 0h:15m, Paused: 0h:30m (0h:15m)"
actual_log=$($WT_CMD log)
check_output "log shows mostly paused cycle" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 09:45 | Work: 0h:15m | Break: 0h:00m | Paused: 0h:30m | Total: 0h:45m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 8: Multiple pause/resume in same cycle
###############################################################################
print_test "8" "Multiple pause/resume in same cycle"
setup_test

export WT_MOCK_TIME="2026-01-20 09:00"
echo 'y' | $WT_CMD new > /dev/null 2>&1

$WT_CMD start > /dev/null 2>&1
export WT_MOCK_TIME="2026-01-20 09:10"
$WT_CMD pause > /dev/null 2>&1

export WT_MOCK_TIME="2026-01-20 09:20"
$WT_CMD start > /dev/null 2>&1
export WT_MOCK_TIME="2026-01-20 09:25"
$WT_CMD pause > /dev/null 2>&1

export WT_MOCK_TIME="2026-01-20 09:35"
$WT_CMD start > /dev/null 2>&1
export WT_MOCK_TIME="2026-01-20 09:45"
$WT_CMD stop > /dev/null 2>&1

expected_log="01. [09:00 => 09:45] Work: 0h:25m, Paused: 0h:20m (0h:25m)"
actual_log=$($WT_CMD log)
check_output "log shows accumulated paused time" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 09:45 | Work: 0h:25m | Break: 0h:00m | Paused: 0h:20m | Total: 0h:45m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 9: Next command (skip break)
###############################################################################
print_test "9" "Next command skips break"
setup_test

export WT_MOCK_TIME="2026-01-20 09:00"
echo 'y' | $WT_CMD new > /dev/null 2>&1

$WT_CMD start > /dev/null 2>&1
export WT_MOCK_TIME="2026-01-20 09:20"
$WT_CMD next > /dev/null 2>&1
export WT_MOCK_TIME="2026-01-20 09:40"
$WT_CMD stop > /dev/null 2>&1

expected_log="01. [09:00 => 09:20] Work: 0h:20m (0h:20m)
02. [09:20 => 09:20] Break: 0h:00m
03. [09:20 => 09:40] Work: 0h:20m (0h:40m)"
actual_log=$($WT_CMD log)
check_output "log shows zero-minute break" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 09:40 | Work: 0h:40m | Break: 0h:00m | Paused: 0h:00m | Total: 0h:40m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 10: Drop a cycle
###############################################################################
print_test "10" "Drop a cycle"
setup_test

export WT_MOCK_TIME="2026-01-20 09:00"
echo 'y' | $WT_CMD new > /dev/null 2>&1

$WT_CMD start > /dev/null 2>&1
export WT_MOCK_TIME="2026-01-20 09:20"
$WT_CMD stop > /dev/null 2>&1

export WT_MOCK_TIME="2026-01-20 09:25"
$WT_CMD start > /dev/null 2>&1
export WT_MOCK_TIME="2026-01-20 09:40"
$WT_CMD stop > /dev/null 2>&1

$WT_CMD mod 1 drop > /dev/null 2>&1

expected_log="01. [09:00 => 09:05] Break: 0h:05m
02. [09:05 => 09:20] Work: 0h:15m (0h:15m)"
actual_log=$($WT_CMD log)
check_output "log shows remaining cycle with adjusted times" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 09:20 | Work: 0h:15m | Break: 0h:05m | Paused: 0h:00m | Total: 0h:20m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

###############################################################################
# Test 11: Drop break merges work cycles correctly
###############################################################################
print_test "11" "Drop break merges work cycles correctly"
setup_test

export WT_MOCK_TIME="2026-01-20 09:00"
echo 'y' | $WT_CMD new > /dev/null 2>&1

$WT_CMD start > /dev/null 2>&1
export WT_MOCK_TIME="2026-01-20 09:20"
$WT_CMD stop > /dev/null 2>&1

export WT_MOCK_TIME="2026-01-20 09:30"
$WT_CMD start > /dev/null 2>&1
export WT_MOCK_TIME="2026-01-20 09:45"
$WT_CMD stop > /dev/null 2>&1

$WT_CMD mod 2 drop > /dev/null 2>&1

expected_log="01. [09:00 => 09:45] Work: 0h:35m (0h:35m)"
actual_log=$($WT_CMD log)
check_output "merged cycle spans from first start to second end" "$expected_log" "$actual_log"

expected_report="2026-01-20 | 09:00 -> 09:45 | Work: 0h:35m | Break: 0h:00m | Paused: 0h:00m | Total: 0h:35m"
actual_report=$($WT_CMD report)
check_output "report shows correct totals" "$expected_report" "$actual_report"

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
