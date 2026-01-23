# Copilot Instructions for WT (WorkTime)

## Project Overview
WT is a CLI work timer for tracking pomodoro-style work/break cycles. Implemented in Go (`wt.go`) using the `urfave/cli/v3` framework.

## Architecture

### Core Data Model
- **Timer struct**: Holds all state - `Status`, `Timeline`, `DayStart`, `PausedMinutes`, `PauseStartStr`, `StopDatetimeStr`
- **TimelineEntry struct**: Work entries have `Type: "work"`, `Minutes`, `PausedMinutes`. Break entries have `Type: "break"`, `Minutes`. Key methods:
  - `ElapsedMinutes()` returns `Minutes + PausedMinutes` (clock time for this entry)
  - `Duration()` returns elapsed time for timestamp calculations (handles work vs break)
- **Timestamps are computed, not stored**: Only `DayStart` is stored; all cycle timestamps are calculated via `Timer.CurrentCycleStart()` which sums timeline durations
- **PausedMinutes**: Time spent paused within current active cycle (accumulates across pause/resume)

### Critical Invariant: Contiguous Timeline
**The calculated timeline MUST always be contiguous** - where one cycle ends, the next must start at the same time. This has been a recurring source of bugs.

When displaying the log:
- Cycle 1 starts at `DayStart`
- Each subsequent cycle starts exactly where the previous one ended
- End time = Start time + `entry.Duration()` (which is `ElapsedMinutes()` for work, `Minutes` for breaks)

Example of correct output:
```
01. [09:00 => 09:30] Work: 0h:30m
02. [09:30 => 09:45] Break: 0h:15m   <-- starts where cycle 1 ended
03. [09:45 => 10:15] Work: 0h:30m    <-- starts where cycle 2 ended
```

**Never store separate timestamps** for cycle starts - always calculate from `DayStart + sum(previous durations)`. Storing timestamps leads to gaps/overlaps when modifying cycle durations.

### State Machine
```
StatusStopped <--> StatusRunning <--> StatusPaused
```
- `startCmd()` → Running (adds break to timeline if resuming from stopped, calculates pause duration on resume from paused)
- `stopCmd()` → Stopped (calculates work = total_cycle_time - paused_time; adds work entry with minutes, paused_minutes, total_minutes; merges consecutive work entries if no break between them)
- `pauseCmd()` → Paused (records pause start time in StartDatetimeStr)
- `nextCmd()` → stop + add 0-min break + start
- `mod` → Modify day start, cycle durations, or paused time (works during running/paused states). Without args, shows usage help.
- `modDropCmd()` → When dropping break while running: removes both break and previous work from timeline, merges accumulated paused time
- **Break reduction**: `start X` on subsequent cycles reduces the previous break by X minutes (cycle start is calculated from timeline)

### File Structure
All data stored under `$WT_ROOT/.out/`:
- `wt.json` - Timer state (JSON serialization of Timer struct)
- `debug-log` - Command execution log with timestamps
- `daily-reports` - Accumulated daily summaries

**Note**: The info-log is generated on-the-fly from timeline data when you run `wt log`, not stored as a file.

## Key Patterns

### Time String Format
User input time is 1-4 digit `HHMM` format parsed by `stringTimeToMinutes()`:
- `5` → 5 minutes
- `15` → 15 minutes  
- `130` → 1 hour 30 minutes
- `0215` → 2 hours 15 minutes

### Log Generation
The `historyCmd()` function generates the log display on-the-fly from the timeline. There is no persistent info-log file. This ensures the log always matches the current timeline state and prevents synchronization issues.

### Current Cycle State
Timeline only contains **completed** cycles. The current running/paused cycle state is tracked separately:
- `Status` (Running/Paused/Stopped)
- `PausedMinutes` - accumulated pause time in current cycle
- `PauseStartStr` - when current pause began (if paused)

The current cycle start time is always **calculated** via `timer.CurrentCycleStart()`, never stored. When `stopCmd()` is called, the current cycle is calculated and added to timeline. When displaying the log, `calculateCurrentMinutes()` computes work time for active cycles: `total_elapsed - paused_minutes`.

### Important Helper Methods
- `Timer.CurrentCycleStart()` - Returns start time of current/next cycle (DayStart + sum of timeline durations)
- `TimelineEntry.Duration()` - Returns elapsed time for an entry (handles work vs break distinction)
- `TimelineEntry.ElapsedMinutes()` - Returns clock time for work entries (Minutes + PausedMinutes)
- `Timer.CompletedMinutes()` - Returns total work minutes from completed cycles in timeline

### Important Helper Functions
- `calculateCurrentMinutes(timer)` - Returns work minutes for current running/paused cycle
- `printMessageIfNotSilent(timer, message)` - Use for success messages in commands (respects silent mode; errors always print)
- `stringTimeToMinutes(timeStr)` - Parses HHMM format to minutes

### Environment Requirement
`$WT_ROOT` environment variable **must** be set. All file paths are relative to this. The test script sets this to a temp directory.

### Mock Time for Testing
`$WT_MOCK_TIME` environment variable enables deterministic testing without sleep:
- Format: `"YYYY-MM-DD HH:MM"` (e.g., `"2026-01-20 09:00"`)
- When set, `getCurrentTime()` returns the mocked time instead of `time.Now()`
- Allows instant test execution with precise time control

## Development Workflow

### Building
```bash
go build -o .out/wt wt.go
```

### Running Tests
```bash
make test
```
Tests use snapshot testing (exact output matching) with `$WT_MOCK_TIME` for deterministic execution. Each test:
1. Sets up isolated environment via Makefile (`/tmp/wt-test-{pid}`)
2. Uses mocked time for instant, reproducible results
3. Compares actual output to expected snapshots
4. Cleanup is automatic (even on failure)

### Manual Testing
```bash
export WT_ROOT=/tmp/wt-dev
go build -o .out/wt wt.go && ./.out/wt new
./.out/wt start
./.out/wt check
```

## Common Modification Points

### Adding a New Command
1. Add new `cli.Command` in the `Commands` slice in `main()`
2. Implement the command function (e.g., `fooCmd(timer *Timer) error`)
3. Call `logDebug()` for command logging
4. Call `save(timer)` after state changes
5. Use `printMessageIfNotSilent()` for user feedback
6. Add test case in `wt-test.sh`

### Modifying Timeline Logic
When changing how cycles are recorded/modified:
1. Update the relevant function (`stopCmd()`, `modDurationCmd()`, `modPauseCmd()`, etc.)
2. Ensure `historyCmd()` correctly displays the updated timeline
3. Remember: no need to regenerate files, log is generated on-the-fly
4. Test with both stopped and running/paused states if applicable

### After Completing Changes
Always review whether documentation needs updating:
1. **README.md** - Update if adding/changing user-facing commands, features, or usage patterns
2. **copilot-instructions.md** - Update if changing architecture, data structures, or development patterns
3. Consider: Does this change affect how future developers should understand or work with the code?

## Go Notes

### Dependencies
- `github.com/urfave/cli/v3` - CLI framework for command parsing and help generation

### Error Handling
- Commands return `error` instead of calling `quit()`
- Main function handles errors and exits appropriately
- Use `fmt.Errorf()` for error messages
