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

This starts a new cycle. If a timer is paused, it resets and starts fresh.

**Pause the timer:**

```bash
wt pause
```

**Stop the timer:**

```bash
wt stop
```

**Stop and start a new timer all in one:**

```bash
wt next
```

This completes the current cycle and adds its time to your total.

### Manual Adjustments

**Adjust current cycle time** using format `HHMM` (hours and minutes):

```bash
wt add 15     # Add 15 minutes to current cycle
wt sub 120    # Subtract 1 hour 20 minutes from current cycle
```

- When **running/paused**: Backdates/forward-dates the start time
- When **stopped**: Modifies the last work cycle duration

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

**How timestamps work:** The timer only stores your day start time and the duration of each work/break cycle. All the timestamps you see in `wt log` and `wt mod` are calculated by adding up durations from the day start. This means when you modify the day start or any cycle duration, all subsequent timestamps automatically recalculate correctly.

**Modify historical cycles:**

First, list all cycles (must be stopped):

```bash
wt mod
# Output shows:
# 01. [09:00 => 10:30] Work: 1h:30m (1h:30m)
# 02. [10:30 => 10:45] Break: 0h:15m
# 03. [10:45 => 12:00] Work: 1h:15m (2h:45m)
```

Then modify a specific cycle:

```bash
wt mod 1 add 10   # Add 10 minutes to cycle 1's duration
wt mod 3 sub 5    # Subtract 5 minutes from cycle 3
wt mod 2 drop     # Remove cycle 2 (merges adjacent work/break)
```

**Note:**

- All `wt mod` modifications require the timer to be stopped
- Can reduce duration to 0 minutes (helpful for finding mistakes)
- Dropping a break between work cycles merges them
- Dropping a work cycle between breaks merges the breaks

### Shortcuts

Start and add time in one command (useful if you forgot to start):

```bash
wt start 15
```

Reset, start, and add time all at once:

```bash
wt restart 15
```

This is equivalent to:

```bash
wt reset
wt start
wt add 15
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
