# WT - WorkTime

Work timer used to time cycles of work. Useful for pomodoro or similar work/break cycles. Total time is the sum of currently running/paused cycle and previously completed cycles. Cycles can also be thought of as laps in a traditional timer.

## Basic Use Case

### Setup

Create a new timer:

```bash
wt new
```

Or reset an existing timer:

```bash
wt reset
```

### Timer Controls

**Start a work session:**

```bash
wt start
```

This starts a new cycle. If resuming from paused state, it continues the current cycle (accumulated pause time is tracked separately).

**Pause the timer:**

```bash
wt pause
```

Pauses the current work cycle. You can resume with `wt start`. Paused time is tracked separately from work time.

**Stop the timer:**

```bash
wt stop
```

Stops the current cycle and records it to the timeline. For cycles with pauses, the entry will show both work time and paused time.

**Stop and start a new timer all in one:**

```bash
wt next
```

This completes the current cycle and adds its time to your total.

### Manual Adjustments

**Adjust day start time** (when you actually started working):

```bash
wt mod start sub 30  # Started 30 min earlier than first start command
wt mod start add 15  # Started 15 min later than first start command
```

This is useful when you forgot to start the timer on time. The timer tracks your work day start time and calculates all cycle timestamps from there. For example:

- You run `wt start` at 09:30
- You realize you actually started working at 09:00
- Run `wt mod start sub 30` to adjust the day start to 09:00
- All timestamps in your log will shift to reflect the correct start time

**How timestamps work:** The timer only stores your day start time and the duration of each work/break cycle. All the timestamps you see in `wt log` are calculated by adding up durations from the day start. This means when you modify the day start or any cycle duration, all subsequent timestamps automatically recalculate correctly.

**Modify historical cycles:**

View your numbered timeline:

```bash
wt log
# Output shows:
# 01. [09:00 => 10:30] Work: 1h:30m (1h:30m)
# 02. [10:30 => 10:45] Break: 0h:15m
# 03. [10:45 => 12:00] Work: 1h:15m (2h:45m)
```

Then modify a specific cycle using its number:

```bash
wt mod 1 add 10        # Add 10 minutes to cycle 1's duration
wt mod 3 sub 5         # Subtract 5 minutes from cycle 3
wt mod 2 drop          # Remove cycle 2 (merges adjacent work/break)
wt mod 1 pause add 10  # Add 10 minutes to cycle 1's paused time (work cycles only)
```

**Modify current running/paused cycle:**

You can also modify the currently active cycle (useful when you forgot to pause):

```bash
wt mod 3 pause add 5   # Add 5 min pause to current cycle (e.g., bathroom break)
wt mod 2 drop          # Remove previous break while timer is running
```

To see usage help for the mod command:

```bash
wt mod            # Shows available mod commands
```

**Note:**

- Most mod commands work while timer is running or paused
- Can reduce duration to 0 minutes (helpful for finding mistakes)
- Dropping a break between work cycles merges them (break time becomes work time, since you were actually working)
- Dropping a work cycle between breaks merges them (work time becomes break time, since you weren't actually working)
- `mod pause` only works for work cycles (not breaks)

### Shortcuts

**Backdate the start of your first cycle** (useful if you forgot to start):

```bash
wt start 15  # On first cycle: backdates start by 15 minutes
```

**Reduce break time on subsequent cycles:**

```bash
wt start 10  # After stopping: reduces previous break by 10 minutes
```

For example:

- Stop at 14:00, take a break
- Start again at 14:20 (20 min break)
- Run `wt start 10` to reduce break to 10 min (cycle starts at 14:10 instead)

Reset and start with backdated time:

```bash
wt restart 15
```

This is equivalent to:

```bash
wt reset
wt start 15  # Backdate start by 15 minutes
```

### Status & History

Check current cycle and total time:

```bash
wt check
# or simply
wt
```

View your timer action history:

```bash
wt log        # Show activity log with actual work times
wt log info   # Same as above (default)
wt log debug  # Show command execution log with timestamps
```

Get a one-line summary of the day's work:

```bash
wt report
```

This displays:

- Date
- Start and end times (clock time, includes pauses)
- Total work time (excludes pauses)
- Total break time
- Total paused time (time paused during work cycles)
- Total elapsed time (work + break + pause)
- Day crossing indicator (if you worked past midnight)

Example: `2026-01-20 | 09:00 -> 17:30 | Work: 7h:30m | Break: 0h:45m | Paused: 0h:15m | Total: 8h:30m`
