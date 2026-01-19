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

Manually adjust time using format `HHMM` (hours and minutes):

```bash
wt add 15     # Add 15 minutes
wt sub 120    # Subtract 1 hour 20 minutes
```

**Note:** Adjustments modify the current cycle if it's running or paused, otherwise they modify the total time.

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
wt log
```
