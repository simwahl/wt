# Copilot Instructions for WT (WorkTime)

## Project Overview
WT is a CLI work timer for tracking pomodoro-style work/break cycles. Single-file Python CLI (`wt.py`) with no external dependencies beyond Python stdlib.

## Architecture

### Core Data Model
- **Timer class** (line ~40): Holds all state - `status`, `timeline`, `day_start`, `cycle_start_datetime_str`, `paused_minutes`, `start_datetime_str`, `stop_datetime_str`
- **Timeline**: Work entries have `{"type": "work", "minutes": work_time, "paused_minutes": pause_time, "total_minutes": work+pause}`. Break entries have `{"type": "break", "minutes": duration}`
- **Timestamps are computed, not stored**: Only `day_start` is stored; all cycle timestamps are calculated by summing timeline `total_minutes` (for work) or `minutes` (for breaks)
- **paused_minutes**: Time spent paused within current active cycle (accumulates across pause/resume)
- **cycle_start_datetime_str**: Timestamp when the current work cycle began (set on start from stopped, used to calculate total cycle time)

### State Machine
```
Status.Stopped <--> Status.Running <--> Status.Paused
```
- `start()` → Running (adds break to timeline if resuming from stopped; sets cycle_start on new cycle, calculates pause duration on resume from paused)
- `stop()` → Stopped (calculates work = total_cycle_time - paused_time; adds work entry with minutes, paused_minutes, total_minutes)
- `pause()` → Paused (records pause start time in start_datetime_str)
- `next()` → stop + add 0-min break + start
- `mod` → Modify day start or cycle durations
- **Break reduction**: `start X` on subsequent cycles reduces the previous break by X minutes and backdates cycle start

### File Structure
All data stored under `$WT_ROOT/.out/`:
- `wt.json` - Timer state (JSON serialization of Timer class)
- `info-log` - Human-readable work/break log (regenerated on modifications)
- `debug-log` - Command execution log with timestamps
- `daily-reports` - Accumulated daily summaries

## Key Patterns

### Time String Format
User input time is 1-4 digit `HHMM` format parsed by `string_time_to_minutes()`:
- `5` → 5 minutes
- `15` → 15 minutes  
- `130` → 1 hour 30 minutes
- `0215` → 2 hours 15 minutes

### Log Regeneration
When modifying historical data (`add`, `sub`, `mod`), call `regenerate_info_log(timer)` to rebuild the info-log from the timeline. This ensures timestamps recalculate correctly.

### Environment Requirement
`$WT_ROOT` environment variable **must** be set. All file paths are relative to this. The test script sets this to a temp directory.

### Mock Time for Testing
`$WT_MOCK_TIME` environment variable enables deterministic testing without sleep:
- Format: `"YYYY-MM-DD HH:MM"` (e.g., `"2026-01-20 09:00"`)
- When set, `get_current_time()` returns the mocked time instead of `dt.now()`
- Allows instant test execution with precise time control

## Development Workflow

### Running Tests
```bash
make test  # or ./wt-test.sh
```
Tests use snapshot testing (exact output matching) with `$WT_MOCK_TIME` for deterministic execution. Each test:
1. Sets up isolated environment via Makefile (`/tmp/wt-test-{pid}`)
2. Uses mocked time for instant, reproducible results
3. Compares actual output to expected snapshots
4. Cleanup is automatic (even on failure)

### Manual Testing
```bash
export WT_ROOT=/tmp/wt-dev
python3 wt.py new
python3 wt.py start
python3 wt.py check
```

## Common Modification Points

### Adding a New Command
1. Add case in `main()` match statement (~line 77)
2. Implement function following existing patterns
3. Call `log_debug()` for command logging
4. Call `save(timer)` after state changes
5. Use `print_message_if_not_silent()` for user feedback
6. Add test case in `wt-test.sh`

### Modifying Timeline Logic
When changing how cycles are recorded/modified:
1. Update the relevant function (`stop()`, `add()`, `sub()`, etc.)
2. Call `regenerate_info_log(timer)` if timestamps could change
3. Verify with `wt log` and `wt mod` showing identical data
