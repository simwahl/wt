#!/bin/bash

# wt Integration Test Script
# Tests various commands and validates log output

# Don't exit on error - we want to continue testing
# set -e

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counter
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Test directory
TEST_DIR="/tmp/wt-test-$$"
WT_CMD="python3 $(pwd)/wt.py"
ORIGINAL_WT_ROOT="${WT_ROOT:-}"

# Helper function to print test status
print_test() {
    echo -e "${YELLOW}TEST: $1${NC}"
}

print_pass() {
    echo -e "${GREEN}✓ PASS: $1${NC}"
    ((TESTS_PASSED++))
}

print_fail() {
    echo -e "${RED}✗ FAIL: $1${NC}"
    ((TESTS_FAILED++))
}

# Helper function to run a command and check it doesn't error
run_cmd() {
    local cmd="$1"
    local test_name="$2"
    
    ((TESTS_RUN++))
    
    # Run the command and capture output for debugging
    local output
    local exit_code
    output=$(eval "$cmd" 2>&1) || exit_code=$?
    exit_code=${exit_code:-0}
    
    if [ $exit_code -eq 0 ]; then
        print_pass "$test_name"
        return 0
    else
        print_fail "$test_name - Command failed with exit code $exit_code: $cmd"
        echo "Output: $output"
        return 1
    fi
}

# Helper function to check if log contains expected text
check_log_contains() {
    local expected="$1"
    local test_name="$2"
    local log_type="${3:-info}"
    
    ((TESTS_RUN++))
    local log_output=$($WT_CMD log "$log_type" 2>&1)
    if echo "$log_output" | grep -q "$expected"; then
        print_pass "$test_name"
        return 0
    else
        print_fail "$test_name - Expected: '$expected' in log"
        echo "Log output:"
        echo "$log_output"
        return 1
    fi
}

# Helper function to check log matches pattern (count lines, etc)
check_log_line_count() {
    local expected_count="$1"
    local pattern="$2"
    local test_name="$3"
    local log_type="${4:-info}"
    
    ((TESTS_RUN++))
    local log_output=$($WT_CMD log "$log_type" 2>&1)
    local actual_count=$(echo "$log_output" | grep -c "$pattern" || true)
    
    if [ "$actual_count" -eq "$expected_count" ]; then
        print_pass "$test_name"
        return 0
    else
        print_fail "$test_name - Expected $expected_count occurrences, got $actual_count"
        echo "Log output:"
        echo "$log_output"
        return 1
    fi
}

# Setup test environment
setup() {
    # Clean up any previous test directories
    rm -rf /tmp/wt-test-* 2>/dev/null || true
    
    echo "Setting up test environment in $TEST_DIR"
    mkdir -p "$TEST_DIR"
    cd "$TEST_DIR"
    
    # Set WT_ROOT to test directory to avoid interfering with actual timer
    export WT_ROOT="$TEST_DIR"
    echo "WT_ROOT set to: $WT_ROOT"
}

# Cleanup test environment
cleanup() {
    echo ""
    echo "Cleaning up test environment"
    
    # Restore original WT_ROOT
    if [ -n "$ORIGINAL_WT_ROOT" ]; then
        export WT_ROOT="$ORIGINAL_WT_ROOT"
        echo "WT_ROOT restored to: $WT_ROOT"
    else
        unset WT_ROOT
        echo "WT_ROOT unset (was not set before test)"
    fi
    
    # Remove test directory
    if [ -n "$TEST_DIR" ] && [ -d "$TEST_DIR" ]; then
        cd /
        rm -rf "$TEST_DIR"
        echo "Test directory removed: $TEST_DIR"
    fi
}

# Run tests
run_tests() {
    # Test 1: Basic workflow - start, add time, stop
    print_test "Test 1: Basic workflow - new, start, add, stop"
    run_cmd "echo 'y' | $WT_CMD new" "create new timer"
    run_cmd "$WT_CMD mode normal" "set mode to normal"
    run_cmd "$WT_CMD start" "start timer"
    sleep 2
    run_cmd "$WT_CMD add 5" "add 5 minutes"
    run_cmd "$WT_CMD stop" "stop timer"
    
    # Validate info log has work entry with added time
    check_log_contains "Work: 0h:05m" "log shows 5 min work (added time)"
    check_log_contains "(0h:05m)" "log shows total of 5 min"
    
    # Test 2: Multiple cycles
    print_test "Test 2: Multiple work cycles"
    run_cmd "$WT_CMD start" "start second cycle"
    sleep 2
    run_cmd "$WT_CMD add 3" "add 3 minutes"
    run_cmd "$WT_CMD stop" "stop second cycle"
    run_cmd "$WT_CMD start" "start third cycle"
    sleep 2
    run_cmd "$WT_CMD add 7" "add 7 minutes"
    run_cmd "$WT_CMD stop" "stop third cycle"
    
    check_log_line_count 3 "Work:" "log has 3 work entries"
    # Check cumulative totals
    check_log_contains "(0h:05m)" "first cycle total is 5 min"
    check_log_contains "(0h:08m)" "second cycle cumulative total is 8 min"
    check_log_contains "(0h:15m)" "third cycle cumulative total is 15 min"
    
    # Test 3: Pause and resume
    print_test "Test 3: Pause and resume"
    run_cmd "$WT_CMD start" "start cycle"
    sleep 2
    run_cmd "$WT_CMD add 4" "add 4 minutes"
    run_cmd "$WT_CMD pause" "pause timer"
    run_cmd "$WT_CMD start" "resume from pause"
    sleep 1
    run_cmd "$WT_CMD stop" "stop after resume"
    
    check_log_line_count 4 "Work:" "log has 4 work entries"
    check_log_contains "(0h:19m)" "fourth cycle cumulative total is 19 min"
    
    # Test 4: Add and subtract time from stopped cycle
    print_test "Test 4: Add and subtract time from last cycle"
    run_cmd "$WT_CMD add 10" "add 10 minutes to last cycle"
    run_cmd "$WT_CMD sub 5" "subtract 5 minutes"
    
    # Check that the last work entry was updated (regenerated log)
    check_log_contains "(0h:24m)" "last cycle total is now 24 min after add/sub"
    
    # Test 5: Modify cycle duration
    print_test "Test 5: Modify first cycle duration"
    run_cmd "$WT_CMD mod 1 add 10" "add 10 min to cycle 1"
    
    # First cycle should now show 15 min (was 5, added 10)
    local log=$($WT_CMD log 2>&1)
    ((TESTS_RUN++))
    if echo "$log" | grep "Work: 0h:15m (0h:15m)"; then
        print_pass "first cycle now shows 15 min"
    else
        print_fail "first cycle doesn't show correct duration"
        echo "$log" | head -5
    fi
    
    # Test 6: Modify start time
    print_test "Test 6: Modify day start time"
    run_cmd "$WT_CMD mod start sub 30" "start 30 min earlier"
    
    # Validate timestamps shifted (check format)
    check_log_contains "=>" "log has timestamp arrows"
    
    # Test 7: Report command
    print_test "Test 7: Report shows summary"
    ((TESTS_RUN++))
    local report=$($WT_CMD report 2>&1)
    if echo "$report" | grep -q "Work:" && echo "$report" | grep -q "Break:" && echo "$report" | grep -q "Total:"; then
        print_pass "report shows work, break, and total times"
    else
        print_fail "report missing expected fields"
        echo "Report: $report"
    fi
    
    # Test 8: Next command (creates 0-min break)
    print_test "Test 8: Next command"
    run_cmd "$WT_CMD next" "next cycle"
    sleep 1
    run_cmd "$WT_CMD add 8" "add 8 minutes"
    run_cmd "$WT_CMD stop" "stop after next"
    
    check_log_line_count 5 "Work:" "log has 5 work entries after next"
    
    # Test 9: Start with backdate parameter
    print_test "Test 9: Start with time parameter"
    run_cmd "echo 'y' | $WT_CMD reset" "reset timer"
    run_cmd "$WT_CMD mode normal" "set mode to normal again"
    run_cmd "$WT_CMD start 20" "start with 20 min backdate"
    sleep 1
    run_cmd "$WT_CMD stop" "stop"
    
    # Should show at least 20 minutes of work
    ((TESTS_RUN++))
    local log=$($WT_CMD log 2>&1)
    if echo "$log" | grep -q "0h:2[0-9]m"; then
        print_pass "log shows ~20+ minutes of work"
    else
        print_fail "log doesn't show expected backdated time"
        echo "$log"
    fi
    
    # Test 9.5: Verify backdating actually changes start timestamp
    print_test "Test 9.5: Backdating on first cycle changes start timestamp"
    run_cmd "echo 'y' | $WT_CMD new" "create fresh timer"
    run_cmd "$WT_CMD mode normal" "set mode to normal"
    
    # Record current time (HH:MM format)
    local current_time=$(date +"%H:%M")
    local current_hour=$(date +"%H")
    local current_min=$(date +"%M")
    
    run_cmd "$WT_CMD start" "start timer"
    run_cmd "$WT_CMD add 30" "add 30 minutes (backdate first cycle)"
    run_cmd "$WT_CMD stop" "stop timer"
    
    # Extract start time from log
    ((TESTS_RUN++))
    local log_line=$($WT_CMD log 2>&1 | grep "Work:")
    local start_timestamp=$(echo "$log_line" | grep -oE '[0-9]{2}:[0-9]{2}' | head -1)
    
    # Calculate expected time (30 min before current)
    local expected_min=$((10#$current_min - 30))
    local expected_hour=$current_hour
    if [ $expected_min -lt 0 ]; then
        expected_min=$((expected_min + 60))
        expected_hour=$((10#$current_hour - 1))
    fi
    expected_hour=$(printf "%02d" $expected_hour)
    expected_min=$(printf "%02d" $expected_min)
    
    # Start timestamp should be earlier than current time (approximately 30 min before)
    if [[ "$start_timestamp" < "$current_time" ]]; then
        print_pass "start timestamp ($start_timestamp) is earlier than current time ($current_time)"
    else
        print_fail "start timestamp ($start_timestamp) should be earlier than current time ($current_time)"
        echo "$log_line"
    fi
    
    # Test 10: Restart command
    print_test "Test 10: Restart command"
    run_cmd "echo 'y' | $WT_CMD restart 15" "restart with 15 min"
    sleep 1
    run_cmd "$WT_CMD stop" "stop after restart"
    
    check_log_contains "0h:1[5-9]m" "log shows ~15+ minutes"
    
    # Test 11: Mod drop cycle
    print_test "Test 11: Drop a cycle"
    run_cmd "$WT_CMD start" "create another cycle"
    sleep 1
    run_cmd "$WT_CMD add 12" "add 12 minutes"
    run_cmd "$WT_CMD stop" "stop"
    
    # Should have 2 cycles now
    check_log_line_count 2 "Work:" "log has 2 cycles before drop"
    
    run_cmd "$WT_CMD mod 1 drop" "drop first cycle"
    check_log_line_count 1 "Work:" "log has 1 cycle after drop"
    
    # Test 11.5: Add on subsequent cycle preserves break and wt log matches wt mod
    print_test "Test 11.5: Add on subsequent cycle doesn't modify break"
    run_cmd "echo 'y' | $WT_CMD restart 15" "restart with 15 min"
    sleep 1
    run_cmd "$WT_CMD stop" "stop"
    sleep 2
    run_cmd "$WT_CMD start" "start again (creates break)"
    run_cmd "$WT_CMD add 5" "add 5 minutes to second cycle"
    run_cmd "$WT_CMD stop" "stop second cycle"
    
    # Verify wt log and wt mod show same output
    ((TESTS_RUN++))
    local log_output=$($WT_CMD log 2>&1)
    local mod_output=$($WT_CMD mod 2>&1 | grep -E "^[0-9]{2}\.")
    local log_cycles=$(echo "$log_output" | grep -E "^\[")
    local mod_cycles=$(echo "$mod_output" | sed 's/^[0-9]*\. //')
    
    if [ "$log_cycles" = "$mod_cycles" ]; then
        print_pass "wt log and wt mod show identical cycles"
    else
        print_fail "wt log and wt mod differ"
        echo "=== wt log ==="
        echo "$log_cycles"
        echo "=== wt mod ==="
        echo "$mod_cycles"
    fi
    
    # Verify break exists and is preserved
    ((TESTS_RUN++))
    if echo "$log_output" | grep -q "Break:"; then
        print_pass "break preserved between cycles"
    else
        print_fail "break missing from log"
        echo "$log_output"
    fi
    
    # Test 12: Status and mode commands
    print_test "Test 12: Status and mode commands"
    ((TESTS_RUN++))
    local status=$($WT_CMD status)
    if [ "$status" = "stopped" ]; then
        print_pass "status returns stopped"
    else
        print_fail "status should be stopped, got: $status"
    fi
    
    ((TESTS_RUN++))
    local mode=$($WT_CMD mode)
    if [ "$mode" = "normal" ]; then
        print_pass "mode returns normal"
    else
        print_fail "mode should be normal, got: $mode"
    fi
    
    # Test 13: Check command output format
    print_test "Test 13: Check command shows status"
    ((TESTS_RUN++))
    local check=$($WT_CMD check)
    if echo "$check" | grep -q "STOPPED" && echo "$check" | grep -q "total"; then
        print_pass "check shows status and total"
    else
        print_fail "check output incorrect: $check"
    fi
    
    # Test 14: Remove timer  
    print_test "Test 14: Remove timer"
    run_cmd "echo 'y' | $WT_CMD remove" "remove timer"
    
    ((TESTS_RUN++))
    local check_after=$($WT_CMD check 2>&1)
    if echo "$check_after" | grep -q "No timer exists"; then
        print_pass "timer successfully removed"
    else
        print_fail "unexpected output after removal: $check_after"
    fi
}

# Main execution
echo "=========================================="
echo "WT Integration Test Suite"
echo "=========================================="
echo ""

# Trap cleanup on exit
trap cleanup EXIT

setup
run_tests

echo ""
echo "=========================================="
echo "Test Results"
echo "=========================================="
echo "Tests run: $TESTS_RUN"
echo -e "${GREEN}Tests passed: $TESTS_PASSED${NC}"
if [ $TESTS_FAILED -gt 0 ]; then
    echo -e "${RED}Tests failed: $TESTS_FAILED${NC}"
    exit 1
else
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
fi
