package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

// Constants
const (
	OutputFolder     = ".out"
	OutputFileName   = "wt.json"
	DebugLogName     = "debug-log"
	DailyReportName  = "daily-reports"
	DT_FORMAT        = "2006-01-02 15:04"
	TIME_ONLY_FORMAT = "15:04"
)

// Status enum
const (
	StatusStopped = "stopped"
	StatusPaused  = "paused"
	StatusRunning = "running"
)

// Mode enum
const (
	ModeSilent  = "silent"
	ModeNormal  = "normal"
	ModeVerbose = "verbose"
)

// TimelineEntry represents a work or break cycle
type TimelineEntry struct {
	Type          string `json:"type"`                     // "work" or "break"
	Minutes       int    `json:"minutes"`                  // Duration of actual work (excludes paused time) or break
	PausedMinutes int    `json:"paused_minutes,omitempty"` // Time spent paused during this work cycle (only for work entries)
}

// ElapsedMinutes returns the elapsed clock time for this entry (work + paused for work entries)
func (e *TimelineEntry) ElapsedMinutes() int {
	return e.Minutes + e.PausedMinutes
}

// Duration returns the elapsed time for this entry (used for timestamp calculations)
func (e *TimelineEntry) Duration() int {
	if e.Type == "work" {
		return e.ElapsedMinutes()
	}
	return e.Minutes
}

// Timer represents the timer state
type Timer struct {
	Status          string          `json:"status"`            // Current state: "stopped", "running", or "paused"
	PauseStartStr   string          `json:"pause_start_str"`   // When the current pause began (if paused)
	StopDatetimeStr string          `json:"stop_datetime_str"` // Last stop time (used to calculate break duration)
	PausedMinutes   int             `json:"paused_minutes"`    // Accumulated pause time in current active cycle
	Mode            string          `json:"mode"`              // Output verbosity: "silent", "normal", or "verbose"
	Timeline        []TimelineEntry `json:"timeline"`          // Completed work and break cycles
	DayStart        string          `json:"day_start"`         // When the work day started (all timestamps computed from this)
}

// UnmarshalJSON implements custom unmarshaling for backward compatibility
func (t *Timer) UnmarshalJSON(data []byte) error {
	type Alias Timer
	aux := &struct {
		AccumulatedMinutes *int `json:"accumulated_minutes,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(t),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Backward compatibility: use accumulated_minutes if paused_minutes not present
	if aux.AccumulatedMinutes != nil && t.PausedMinutes == 0 {
		t.PausedMinutes = *aux.AccumulatedMinutes
	}

	return nil
}

// CurrentCycleStart returns the start time of the current (or next) cycle
// by calculating DayStart + sum of all timeline entry durations.
// This is the single source of truth for cycle start times.
func (t *Timer) CurrentCycleStart() time.Time {
	start, _ := parseTime(t.DayStart)
	for _, entry := range t.Timeline {
		start = start.Add(time.Duration(entry.Duration()) * time.Minute)
	}
	return start
}

// CompletedMinutes returns total work minutes from timeline
func (t *Timer) CompletedMinutes() int {
	total := 0
	for _, entry := range t.Timeline {
		if entry.Type == "work" {
			total += entry.Minutes
		}
	}
	return total
}

func main() {
	app := &cli.Command{
		Name:  "wt",
		Usage: "Work timer for tracking pomodoro-style work/break cycles",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() > 0 {
				return fmt.Errorf("No such command: %s. Run 'wt help' to see available commands.", cmd.Args().Get(0))
			}

			// Default action when no command is provided
			timer, err := load()
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			return checkCmd(timer)
		},
		Commands: []*cli.Command{
			{
				Name:        "start",
				Usage:       "Starts a new timer or continues paused timer",
				ArgsUsage:   "[time]",
				Description: "Optionally provide time in HHMM format to backdate start (first cycle) or reduce previous break (subsequent cycles)",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					timer, err := load()
					if err != nil {
						return err
					}
					startTime := ""
					if cmd.Args().Len() > 0 {
						startTime = cmd.Args().Get(0)
					}
					return startCmd(timer, startTime)
				},
			},
			{
				Name:        "stop",
				Usage:       "Stops running or paused timer",
				ArgsUsage:   "[time]",
				Description: "Optionally provide HH:MM clock time to stop at, or HHMM duration to subtract from work time",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					timer, err := load()
					if err != nil {
						return err
					}
					stopTime := ""
					if cmd.Args().Len() > 0 {
						stopTime = cmd.Args().Get(0)
					}
					return stopCmd(timer, stopTime)
				},
			},
			{
				Name:        "pause",
				Usage:       "Pauses currently running timer",
				ArgsUsage:   "[time]",
				Description: "Optionally provide time in HHMM format to add pause time",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					timer, err := load()
					if err != nil {
						return err
					}
					pauseTime := ""
					if cmd.Args().Len() > 0 {
						pauseTime = cmd.Args().Get(0)
					}
					return pauseCmd(timer, pauseTime)
				},
			},
			{
				Name:  "check",
				Usage: "Prints current and total time along with status",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					timer, err := load()
					if err != nil {
						return err
					}
					return checkCmd(timer)
				},
			},
			{
				Name:        "log",
				Usage:       "Show log of timer activity",
				ArgsUsage:   "[type]",
				Description: "Defaults to info log. Use 'debug' to see command execution timestamps",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					timer, err := load()
					if err != nil {
						return err
					}
					logType := ""
					if cmd.Args().Len() > 0 {
						logType = cmd.Args().Get(0)
					}
					return historyCmd(timer, logType)
				},
			},
			{
				Name:      "mod",
				Usage:     "Modify timeline entries (work and break cycles)",
				ArgsUsage: "<num> [drop|pause|start|end|<add|sub>] [time]",
				Description: `Modify cycle boundaries, cycle durations, or paused time.
   Examples:
     wt mod                           - Show usage help
	     wt mod 1 start 08:30             - Set cycle 1/day start boundary
	     wt mod 3 start 14:10             - Set cycle 3 start boundary
	     wt mod 3 end 15:20               - Set cycle 3 end boundary
     wt mod 3 add 15                  - Add 15min to cycle 3
     wt mod 5 pause add 10            - Add 10min paused time to cycle 5
     wt mod 2 drop                    - Remove cycle 2`,
				Action: func(ctx context.Context, cmd *cli.Command) error {
					timer, err := load()
					if err != nil {
						return err
					}

					args := cmd.Args().Slice()
					if len(args) == 0 {
						return modListCmd()
					}

					if len(args) == 2 && args[1] == "drop" {
						return modDropCmd(timer, args[0])
					}

					if len(args) == 3 && args[1] == "start" {
						return modCycleStartCmd(timer, args[0], args[2])
					}

					if len(args) == 3 && args[1] == "end" {
						return modCycleEndCmd(timer, args[0], args[2])
					}

					if len(args) == 4 && args[1] == "pause" {
						return modPauseCmd(timer, args[0], args[2], args[3])
					}

					if len(args) == 3 {
						return modDurationCmd(timer, args[0], args[1], args[2])
					}

					return modListCmd()
				},
			},
			{
				Name:  "next",
				Usage: "Stop current timer and start next",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					timer, err := load()
					if err != nil {
						return err
					}
					return nextCmd(timer)
				},
			},
			{
				Name:  "reset",
				Usage: "Stops and sets current and total timers to zero",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return resetCmd("Timer reset.")
				},
			},
			{
				Name:        "restart",
				Usage:       "Reset and start new timer",
				ArgsUsage:   "[time]",
				Description: "Optionally provide time in HHMM format to backdate start",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					startTime := ""
					if cmd.Args().Len() > 0 {
						startTime = cmd.Args().Get(0)
					}
					return restartCmd(startTime)
				},
			},
			{
				Name:  "new",
				Usage: "Creates a new timer (alias for reset)",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return newCmd()
				},
			},
			{
				Name:  "remove",
				Usage: "Deletes the timer and related files",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return removeCmd()
				},
			},
			{
				Name:  "status",
				Usage: "Print current status (stopped/running/paused)",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return statusCmd()
				},
			},
			{
				Name:        "mode",
				Usage:       "Change output verbosity",
				ArgsUsage:   "[type]",
				Description: "Types: silent (only errors), normal (messages after actions), verbose (normal + auto check). If no type is provided, prints current mode.",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.Args().Len() == 0 {
						timer, err := load()
						if err != nil {
							return err
						}
						fmt.Println(timer.Mode)
						return nil
					}
					return modeCmd(cmd.Args().Get(0))
				},
			},
			{
				Name:        "report",
				Usage:       "Print a one-line summary of the day's work",
				Description: "Shows date, start time, end time, total work time, total break time, and total time",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "work-time",
						Aliases: []string{"w"},
						Usage:   "Show only date and work time",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					timer, err := load()
					if err != nil {
						return err
					}
					return reportCmd(timer, cmd.Bool("work-time"))
				},
			},
			{
				Name:  "game",
				Usage: "RPG gamification — track XP, levels, and streaks",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return gameCmd()
				},
				Commands: []*cli.Command{
					{
						Name:  "enable",
						Usage: "Enable the game and create game state file",
						Action: func(ctx context.Context, cmd *cli.Command) error {
							return gameEnableCmd()
						},
					},
					{
						Name:  "streak",
						Usage: "Manage your streak",
						Commands: []*cli.Command{
							{
								Name:  "reset",
								Usage: "Reset streak to day 1 starting today",
								Action: func(ctx context.Context, cmd *cli.Command) error {
									return gameStreakResetCmd()
								},
							},
						},
					},
					{
						Name:  "consume",
						Usage: "Consume a hobby time reward (10 minutes)",
						Action: func(ctx context.Context, cmd *cli.Command) error {
							return gameConsumeCmd()
						},
					},
					{
						Name:  "achievements",
						Usage: "Show all achievements with locked/unlocked status",
						Action: func(ctx context.Context, cmd *cli.Command) error {
							return gameAchievementsCmd()
						},
					},
				},
			},
			{
				Name:  "debug",
				Usage: "Prints debug info",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return debugCmd()
				},
			},
			{
				Name:  "help",
				Usage: "Show help",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return cli.ShowAppHelp(cmd)
				},
			},
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Helper functions

func getCurrentTime() time.Time {
	mockTime := os.Getenv("WT_MOCK_TIME")
	if mockTime != "" {
		t, err := time.ParseInLocation(DT_FORMAT, mockTime, time.Local)
		if err == nil {
			return t
		}
	}
	return time.Now()
}

// parseTime parses a datetime string in local timezone
func parseTime(s string) (time.Time, error) {
	return time.ParseInLocation(DT_FORMAT, s, time.Local)
}

func projectRootPath() (string, error) {
	root := os.Getenv("WT_ROOT")
	if root == "" {
		return "", fmt.Errorf("Env $WT_ROOT not set.")
	}
	return root, nil
}

func outputFilePath() (string, error) {
	root, err := projectRootPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, OutputFolder, OutputFileName), nil
}

func debugLogFilePath() (string, error) {
	root, err := projectRootPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, OutputFolder, DebugLogName), nil
}

func dailyReportFilePath() (string, error) {
	// Prefer WT_REPORT_FILE if set
	if reportFile := os.Getenv("WT_REPORT_FILE"); reportFile != "" {
		return reportFile, nil
	}

	root, err := projectRootPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, OutputFolder, DailyReportName), nil
}

func outputFolderPath() (string, error) {
	root, err := projectRootPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, OutputFolder), nil
}

func deltaMinutes(start, end time.Time) int {
	return int(end.Sub(start).Minutes())
}

func hourMinuteStrFromMinutes(minutes int) string {
	h := minutes / 60
	m := minutes % 60
	return fmt.Sprintf("%dh %02dm", h, m)
}

func minutesToHourMinuteStr(mins int) string {
	h := mins / 60
	m := mins % 60
	return fmt.Sprintf("%dh:%02dm", h, m)
}

func stringTimeToMinutes(timeStr string) (int, error) {
	if !isDigits(timeStr) {
		return 0, fmt.Errorf("Invalid time format. Should be digits only.")
	}

	var hour, minute int
	switch len(timeStr) {
	case 4:
		h, _ := strconv.Atoi(timeStr[:2])
		m, _ := strconv.Atoi(timeStr[2:])
		hour, minute = h, m
	case 3:
		h, _ := strconv.Atoi(timeStr[:1])
		m, _ := strconv.Atoi(timeStr[1:])
		hour, minute = h, m
	case 2, 1:
		m, _ := strconv.Atoi(timeStr)
		minute = m
	default:
		return 0, fmt.Errorf("Incorrect time format. Should be 1-4 digit HHMM.")
	}

	return hour*60 + minute, nil
}

func validateTimeString(timeStr string) error {
	if len(timeStr) < 1 || len(timeStr) > 4 || !isDigits(timeStr) {
		return fmt.Errorf("Incorrect time format. Should be 1-4 digit HHMM.")
	}

	if len(timeStr) >= 2 {
		minutes, _ := strconv.Atoi(timeStr[len(timeStr)-2:])
		if minutes > 59 {
			return fmt.Errorf("Incorrect time format. Minutes cannot exceed 59.")
		}
	}

	return nil
}

func validateClockTimeString(timeStr string) error {
	if len(timeStr) != 5 {
		return fmt.Errorf("Incorrect time format. Should be HH:MM.")
	}

	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 || len(parts[0]) != 2 || len(parts[1]) != 2 {
		return fmt.Errorf("Incorrect time format. Should be HH:MM.")
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("Incorrect time format. Should be HH:MM.")
	}

	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("Incorrect time format. Should be HH:MM.")
	}

	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return fmt.Errorf("Incorrect time format. Should be HH:MM.")
	}

	return nil
}

func resolveClockTimeNear(reference time.Time, timeStr string) (time.Time, error) {
	if err := validateClockTimeString(timeStr); err != nil {
		return time.Time{}, err
	}

	hour, _ := strconv.Atoi(timeStr[:2])
	minute, _ := strconv.Atoi(timeStr[3:])

	base := time.Date(reference.Year(), reference.Month(), reference.Day(), hour, minute, 0, 0, reference.Location())
	before := base.Add(-24 * time.Hour)
	after := base.Add(24 * time.Hour)

	best := base
	bestDiff := absDuration(reference.Sub(base))

	beforeDiff := absDuration(reference.Sub(before))
	if beforeDiff < bestDiff {
		best = before
		bestDiff = beforeDiff
	}

	afterDiff := absDuration(reference.Sub(after))
	if afterDiff < bestDiff {
		best = after
	}

	return best, nil
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func maxCycleNumber(timer *Timer) int {
	maxCycle := len(timer.Timeline)
	if timer.Status == StatusRunning || timer.Status == StatusPaused {
		maxCycle++
	}
	return maxCycle
}

func cycleStartByNumber(timer *Timer, cycleNum int) (time.Time, error) {
	if timer.DayStart == "" {
		return time.Time{}, fmt.Errorf("No day_start to modify.")
	}

	if cycleNum < 1 || cycleNum > maxCycleNumber(timer) {
		return time.Time{}, fmt.Errorf("Cycle %d does not exist. Valid range: 1-%d", cycleNum, maxCycleNumber(timer))
	}

	start, _ := parseTime(timer.DayStart)
	for i := 1; i < cycleNum; i++ {
		start = start.Add(time.Duration(timer.Timeline[i-1].Duration()) * time.Minute)
	}

	return start, nil
}

func cycleEndByNumber(timer *Timer, cycleNum int) (time.Time, error) {
	if cycleNum < 1 || cycleNum > len(timer.Timeline) {
		return time.Time{}, fmt.Errorf("Cycle %d does not exist. Valid range: 1-%d", cycleNum, len(timer.Timeline))
	}

	start, err := cycleStartByNumber(timer, cycleNum)
	if err != nil {
		return time.Time{}, err
	}

	entry := timer.Timeline[cycleNum-1]
	return start.Add(time.Duration(entry.Duration()) * time.Minute), nil
}

func setEntryElapsedDuration(entry *TimelineEntry, newElapsed int) error {
	if newElapsed < 0 {
		return fmt.Errorf("Error: Duration would be negative.")
	}

	if entry.Type == "break" {
		entry.Minutes = newElapsed
		return nil
	}

	newWork := newElapsed - entry.PausedMinutes
	if newWork < 0 {
		return fmt.Errorf("Error: Elapsed duration (%s) cannot be less than paused time (%s).", minutesToHourMinuteStr(newElapsed), minutesToHourMinuteStr(entry.PausedMinutes))
	}

	entry.Minutes = newWork
	return nil
}

func validateTimerConsistency(timer *Timer) error {
	if timer.DayStart == "" {
		return nil
	}

	for i, entry := range timer.Timeline {
		if entry.Minutes < 0 {
			return fmt.Errorf("Cycle %d has negative duration.", i+1)
		}
		if entry.PausedMinutes < 0 {
			return fmt.Errorf("Cycle %d has negative paused time.", i+1)
		}
	}

	now := getCurrentTime()
	cycleStart := timer.CurrentCycleStart()
	if timer.Status == StatusStopped && len(timer.Timeline) > 0 && cycleStart.After(now) {
		return fmt.Errorf("Invalid state: last completed cycle cannot end in the future.")
	}
	if (timer.Status == StatusRunning || timer.Status == StatusPaused) && cycleStart.After(now) {
		return fmt.Errorf("Invalid state: current cycle start cannot be in the future.")
	}

	if timer.Status == StatusRunning {
		elapsed := deltaMinutes(cycleStart, now)
		if timer.PausedMinutes > elapsed {
			return fmt.Errorf("Invalid state: paused time cannot exceed elapsed cycle time.")
		}
	}

	if timer.Status == StatusPaused {
		pauseStart, err := parseTime(timer.PauseStartStr)
		if err != nil {
			return err
		}
		if pauseStart.After(now) {
			return fmt.Errorf("Invalid state: pause start cannot be in the future.")
		}

		elapsed := deltaMinutes(cycleStart, now)
		totalPaused := timer.PausedMinutes + deltaMinutes(pauseStart, now)
		if totalPaused > elapsed {
			return fmt.Errorf("Invalid state: paused time cannot exceed elapsed cycle time.")
		}
	}

	return nil
}

func syncStopDatetimeForStopped(timer *Timer) {
	if timer.Status != StatusStopped {
		return
	}

	if timer.DayStart == "" || len(timer.Timeline) == 0 {
		timer.StopDatetimeStr = ""
		return
	}

	timer.StopDatetimeStr = timer.CurrentCycleStart().Format(DT_FORMAT)
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func calculateCurrentMinutes(timer *Timer) int {
	if timer.Status == StatusStopped {
		return 0
	}

	cycleStart := timer.CurrentCycleStart()
	totalElapsed := deltaMinutes(cycleStart, getCurrentTime())

	var totalPaused int
	if timer.Status == StatusPaused {
		pauseStart, _ := parseTime(timer.PauseStartStr)
		currentPause := deltaMinutes(pauseStart, getCurrentTime())
		totalPaused = timer.PausedMinutes + currentPause
	} else {
		totalPaused = timer.PausedMinutes
	}

	workMinutes := totalElapsed - totalPaused
	if workMinutes < 0 {
		return 0
	}
	return workMinutes
}

func printMessageIfNotSilent(timer *Timer, message string) {
	if timer.Mode != ModeSilent {
		fmt.Println(message)
	}
}

func printCheckIfVerbose(timer *Timer) {
	if timer.Mode == ModeVerbose {
		checkCmd(timer)
	}
}

func yesOrNoPrompt(msg string) bool {
	if os.Getenv("WT_SKIP_PROMPTS") != "" {
		return true
	}

	fmt.Printf("%s y / n [n]: ", msg)
	var answer string
	fmt.Scanln(&answer)
	return strings.ToLower(answer) == "y"
}

// File I/O functions

func save(timer *Timer) error {
	folderPath, err := outputFolderPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(folderPath); os.IsNotExist(err) {
		if err := os.MkdirAll(folderPath, 0755); err != nil {
			return err
		}
	}

	filePath, err := outputFilePath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(timer, "", "    ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

func load() (*Timer, error) {
	filePath, err := outputFilePath()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("No timer exists.")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var timer Timer
	if err := json.Unmarshal(data, &timer); err != nil {
		return nil, err
	}

	return &timer, nil
}

func logDebug(msg string) error {
	filePath, err := debugLogFilePath()
	if err != nil {
		return err
	}

	timestamp := getCurrentTime().Format(DT_FORMAT)
	logLine := fmt.Sprintf("[%s] %s\n", timestamp, msg)

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(logLine)
	return err
}

func saveDailyReport(timer *Timer) error {
	if timer.DayStart == "" {
		return nil
	}

	// Calculate totals from timeline
	totalWorkMins := 0
	totalBreakMins := 0
	totalPausedMins := 0

	for _, entry := range timer.Timeline {
		if entry.Type == "work" {
			totalWorkMins += entry.Minutes
			totalPausedMins += entry.PausedMinutes
		} else {
			totalBreakMins += entry.Minutes
		}
	}

	// Add current running/paused time if applicable
	currentMins := 0
	currentPausedMins := 0
	if timer.Status == StatusRunning || timer.Status == StatusPaused {
		currentMins = calculateCurrentMinutes(timer)
		totalWorkMins += currentMins

		// Add current cycle's paused time
		currentPausedMins = timer.PausedMinutes
		if timer.Status == StatusPaused {
			pauseStart, _ := parseTime(timer.PauseStartStr)
			currentPausedMins += deltaMinutes(pauseStart, getCurrentTime())
		}
		totalPausedMins += currentPausedMins
	}

	// Calculate end time (includes work + paused time for running/paused cycles)
	startDt, _ := parseTime(timer.DayStart)
	endDt := timer.CurrentCycleStart()

	// Add current running time (work minutes + paused minutes = elapsed time)
	if timer.Status == StatusRunning || timer.Status == StatusPaused {
		endDt = endDt.Add(time.Duration(currentMins+currentPausedMins) * time.Minute)
	}

	// Format output
	dateStr := startDt.Format("2006-01-02")
	startTime := startDt.Format(TIME_ONLY_FORMAT)
	endTime := endDt.Format(TIME_ONLY_FORMAT)
	workStr := minutesToHourMinuteStr(totalWorkMins)
	breakStr := minutesToHourMinuteStr(totalBreakMins)
	pausedStr := minutesToHourMinuteStr(totalPausedMins)
	totalStr := minutesToHourMinuteStr(totalWorkMins + totalBreakMins + totalPausedMins)

	// Check if crossed midnight
	dayDiff := int(endDt.Sub(startDt).Hours() / 24)
	dayIndicator := ""
	if dayDiff > 0 {
		dayIndicator = fmt.Sprintf(" [+%d day]", dayDiff)
	}

	reportLine := fmt.Sprintf("%s | %s -> %s | Work: %s | Break: %s | Paused: %s | Total: %s%s",
		dateStr, startTime, endTime, workStr, breakStr, pausedStr, totalStr, dayIndicator)

	// Prepend to daily report file (newest at top)
	filePath, err := dailyReportFilePath()
	if err != nil {
		return err
	}

	existingContent := ""
	if data, err := os.ReadFile(filePath); err == nil {
		existingContent = strings.TrimSpace(string(data))
	}

	// Build final content: new line, then existing (if any)
	finalContent := reportLine
	if existingContent != "" {
		finalContent = reportLine + "\n" + existingContent
	}
	finalContent += "\n"

	return os.WriteFile(filePath, []byte(finalContent), 0644)
}

// Command implementations

func startCmd(timer *Timer, startTime string) error {
	if startTime != "" {
		if err := validateTimeString(startTime); err != nil {
			return err
		}
	}

	message := ""
	switch timer.Status {
	case StatusRunning:
		fmt.Println("Already running.")
		return nil
	case StatusPaused:
		message = "Resuming timer."
		// Calculate pause duration and add to paused_minutes
		pauseStart, _ := parseTime(timer.PauseStartStr)
		pauseDuration := deltaMinutes(pauseStart, getCurrentTime())
		timer.PausedMinutes += pauseDuration
	case StatusStopped:
		message = "Starting timer."
	}

	// Track if this is first cycle (before adding break)
	isFirstCycle := len(timer.Timeline) == 0

	// If start_time is provided on subsequent cycle, validate break duration first
	if startTime != "" && !isFirstCycle {
		backdateMinutes, _ := stringTimeToMinutes(startTime)
		// Calculate what the break would be
		if timer.StopDatetimeStr != "" {
			breakStart, _ := parseTime(timer.StopDatetimeStr)
			breakStop := getCurrentTime()
			breakMins := deltaMinutes(breakStart, breakStop)

			if breakMins < backdateMinutes {
				fmt.Printf("Cannot reduce break below 0. Break was %s, tried to subtract %s.\n",
					minutesToHourMinuteStr(breakMins), minutesToHourMinuteStr(backdateMinutes))
				return nil
			}
		} else {
			// No stop time means we're resuming from paused, can't backdate
			fmt.Println("Cannot backdate start time - no break to reduce.")
			return nil
		}
	}

	// Calculate break if resuming from stopped state
	if timer.StopDatetimeStr != "" {
		stopDt, _ := parseTime(timer.StopDatetimeStr)
		breakMinutes := deltaMinutes(stopDt, getCurrentTime())
		timer.Timeline = append(timer.Timeline, TimelineEntry{
			Type:    "break",
			Minutes: breakMinutes,
		})
	}

	timer.StopDatetimeStr = ""
	now := getCurrentTime()
	timer.PauseStartStr = now.Format(DT_FORMAT)

	// If this is the first cycle of the day, set day_start
	if timer.DayStart == "" {
		timer.DayStart = timer.PauseStartStr
	}

	timer.Status = StatusRunning

	startTimeLog := ""
	if startTime != "" {
		startTimeLog = " " + startTime
	}
	logDebug(fmt.Sprintf("wt start%s", startTimeLog))

	if err := save(timer); err != nil {
		return err
	}

	printMessageIfNotSilent(timer, message)
	printCheckIfVerbose(timer)

	// Handle start_time parameter
	if startTime != "" {
		backdateMinutes, _ := stringTimeToMinutes(startTime)

		if isFirstCycle {
			// Backdate the day_start and pause_start_str
			dayStart, _ := parseTime(timer.DayStart)
			timer.DayStart = dayStart.Add(-time.Duration(backdateMinutes) * time.Minute).Format(DT_FORMAT)

			pauseStartDt, _ := parseTime(timer.PauseStartStr)
			timer.PauseStartStr = pauseStartDt.Add(-time.Duration(backdateMinutes) * time.Minute).Format(DT_FORMAT)

			if err := save(timer); err != nil {
				return err
			}
		} else {
			// Reduce the last break duration to backdate cycle start
			lastIdx := len(timer.Timeline) - 1
			timer.Timeline[lastIdx].Minutes -= backdateMinutes

			// Also backdate pause_start_str
			pauseStartDt, _ := parseTime(timer.PauseStartStr)
			timer.PauseStartStr = pauseStartDt.Add(-time.Duration(backdateMinutes) * time.Minute).Format(DT_FORMAT)

			if err := save(timer); err != nil {
				return err
			}
		}
	}

	return nil
}

func stopCmd(timer *Timer, stopTime string) error {
	switch timer.Status {
	case StatusStopped:
		fmt.Println("Timer already stopped.")
		return nil
	case StatusRunning, StatusPaused:
		// Calculate effective stop time
		now := getCurrentTime()
		effectiveStopTime := now

		if stopTime != "" {
			if strings.Contains(stopTime, ":") {
				// Absolute clock time (HH:MM) — resolve on same day as cycle start
				if err := validateClockTimeString(stopTime); err != nil {
					return err
				}
				hour, _ := strconv.Atoi(stopTime[:2])
				minute, _ := strconv.Atoi(stopTime[3:])

				cycleStart := timer.CurrentCycleStart()
				y, m, d := cycleStart.Date()
				effectiveStopTime = time.Date(y, m, d, hour, minute, 0, 0, cycleStart.Location())

				if effectiveStopTime.Before(cycleStart) {
					return fmt.Errorf("Stop time %s is before cycle start %s.", stopTime, cycleStart.Format(TIME_ONLY_FORMAT))
				}
				if effectiveStopTime.After(now) {
					return fmt.Errorf("Cannot set stop time in the future.")
				}
			} else {
				// Duration subtraction (HHMM) — existing behavior
				if err := validateTimeString(stopTime); err != nil {
					return err
				}
				subtractMinutes, err := stringTimeToMinutes(stopTime)
				if err != nil {
					return err
				}
				cycleStart := timer.CurrentCycleStart()
				elapsed := deltaMinutes(cycleStart, now)
				if subtractMinutes > elapsed {
					return fmt.Errorf("Cannot subtract more time than currently elapsed in this cycle.")
				}
				effectiveStopTime = now.Add(-time.Duration(subtractMinutes) * time.Minute)
			}
		}

		stopTimeStr := effectiveStopTime.Format(DT_FORMAT)

		// Calculate work duration: total_cycle_time - paused_time
		totalPaused := timer.PausedMinutes
		if timer.Status == StatusPaused {
			pauseStart, _ := parseTime(timer.PauseStartStr)
			currentPause := deltaMinutes(pauseStart, effectiveStopTime)
			totalPaused += currentPause
		}

		cycleStart := timer.CurrentCycleStart()
		totalCycleTime := deltaMinutes(cycleStart, effectiveStopTime)

		// Work time = total cycle time - paused time
		cycleMinutes := totalCycleTime - totalPaused

		// Ensure we don't go below 0
		if cycleMinutes < 0 {
			cycleMinutes = 0
		}

		// If last entry is work (no break between), merge into it
		mergedIntoExisting := false
		if len(timer.Timeline) > 0 && timer.Timeline[len(timer.Timeline)-1].Type == "work" {
			lastWork := &timer.Timeline[len(timer.Timeline)-1]
			lastWork.Minutes += cycleMinutes
			lastWork.PausedMinutes += totalPaused
			mergedIntoExisting = true
		}

		if !mergedIntoExisting {
			timer.Timeline = append(timer.Timeline, TimelineEntry{
				Type:          "work",
				Minutes:       cycleMinutes,
				PausedMinutes: totalPaused,
			})
		}

		timer.StopDatetimeStr = stopTimeStr
		timer.PauseStartStr = ""
		timer.PausedMinutes = 0
		timer.Status = StatusStopped

		if stopTime != "" {
			logDebug(fmt.Sprintf("wt stop %s", stopTime))
		} else {
			logDebug("wt stop")
		}
		if err := save(timer); err != nil {
			return err
		}

		printMessageIfNotSilent(timer, "Timer stopped.")
		printCheckIfVerbose(timer)
	default:
		fmt.Printf("Unhandled status: %s\n", timer.Status)
	}

	return nil
}

func pauseCmd(timer *Timer, pauseTime string) error {
	switch timer.Status {
	case StatusPaused:
		fmt.Println("Timer already paused.")
		return nil
	case StatusStopped:
		fmt.Println("Cannot pause stopped timer.")
		return nil
	case StatusRunning:
		// Validate and handle optional pause time parameter
		additionalPause := 0
		if pauseTime != "" {
			// Check if user is trying to use add/sub syntax
			if pauseTime == "add" || pauseTime == "sub" {
				return fmt.Errorf("To modify pause time, use: wt mod <cycle> pause add/sub <minutes>")
			}
			if err := validateTimeString(pauseTime); err != nil {
				return err
			}
			var err error
			additionalPause, err = stringTimeToMinutes(pauseTime)
			if err != nil {
				return err
			}

			// Calculate current cycle elapsed time
			cycleStart := timer.CurrentCycleStart()
			elapsed := deltaMinutes(cycleStart, getCurrentTime())

			// Verify total pause doesn't exceed elapsed time
			totalPause := timer.PausedMinutes + additionalPause
			if totalPause > elapsed {
				return fmt.Errorf("Cannot pause longer than currently elapsed time.")
			}
		}

		// Set pause start time (backdated if additional pause time provided)
		now := getCurrentTime()
		if additionalPause > 0 {
			timer.PauseStartStr = now.Add(-time.Duration(additionalPause) * time.Minute).Format(DT_FORMAT)
		} else {
			timer.PauseStartStr = now.Format(DT_FORMAT)
		}
		timer.Status = StatusPaused

		// Log command
		pauseTimeLog := ""
		if pauseTime != "" {
			pauseTimeLog = fmt.Sprintf(" %s", pauseTime)
		}
		logDebug(fmt.Sprintf("wt pause%s", pauseTimeLog))
		if err := save(timer); err != nil {
			return err
		}

		// Print success message
		message := "Paused timer"
		if additionalPause > 0 {
			message = fmt.Sprintf("Paused timer (added %dm pause time)", additionalPause)
		}
		printMessageIfNotSilent(timer, message)
		printCheckIfVerbose(timer)
	default:
		return fmt.Errorf("Unhandled status: %s", timer.Status)
	}

	return nil
}

func checkCmd(timer *Timer) error {
	// Warn if timer has been running since a previous day
	if timer.Status == StatusRunning || timer.Status == StatusPaused {
		cycleStart := timer.CurrentCycleStart()
		now := getCurrentTime()
		_, _, cycleDay := cycleStart.Date()
		_, _, nowDay := now.Date()
		if nowDay != cycleDay || now.Sub(cycleStart) >= 24*time.Hour {
			fmt.Printf("Warning: Timer has been running since %s.\n  Use 'wt stop HH:MM' to set the actual stop time on %s.\n",
				cycleStart.Format(DT_FORMAT), cycleStart.Format("2006-01-02"))
		}
	}

	lines, err := buildInfoLogLines(timer)
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		return nil
	}

	fmt.Println(lines[len(lines)-1])
	return nil
}

func buildInfoLogLines(timer *Timer) ([]string, error) {
	if len(timer.Timeline) == 0 && timer.Status == StatusStopped {
		return []string{"No work cycles recorded."}, nil
	}

	var currentTime time.Time
	if timer.DayStart != "" {
		currentTime, _ = parseTime(timer.DayStart)
	} else {
		currentTime = getCurrentTime()
	}

	runningTotal := 0
	lineNum := 1
	lines := make([]string, 0)

	for _, entry := range timer.Timeline {
		if entry.Type == "work" {
			workMins := entry.Minutes
			pausedMins := entry.PausedMinutes

			startTime := currentTime
			endTime := currentTime.Add(time.Duration(entry.Duration()) * time.Minute)

			runningTotal += workMins

			startTimeStr := startTime.Format(TIME_ONLY_FORMAT)
			endTimeStr := endTime.Format(TIME_ONLY_FORMAT)
			workStr := minutesToHourMinuteStr(workMins)
			totalStr := minutesToHourMinuteStr(runningTotal)

			pausedStr := ""
			if pausedMins > 0 {
				pausedStr = fmt.Sprintf(" |%02dm|", pausedMins)
			}

			// Calculate day indicator for midnight crossing
			dayDiff := int(endTime.Sub(startTime.Truncate(24*time.Hour)).Hours()/24) - int(startTime.Sub(startTime.Truncate(24*time.Hour)).Hours()/24)
			startYear, startMonth, startDay := startTime.Date()
			endYear, endMonth, endDay := endTime.Date()
			startDate := time.Date(startYear, startMonth, startDay, 0, 0, 0, 0, startTime.Location())
			endDate := time.Date(endYear, endMonth, endDay, 0, 0, 0, 0, endTime.Location())
			dayDiff = int(endDate.Sub(startDate).Hours() / 24)
			dayIndicator := ""
			if dayDiff > 0 {
				dayIndicator = fmt.Sprintf("  [+%d day]", dayDiff)
			}

			lines = append(lines, fmt.Sprintf("%02d. [%s => %s] Work: %s%s (%s)%s",
				lineNum, startTimeStr, endTimeStr, workStr, pausedStr, totalStr, dayIndicator))

			currentTime = endTime
		} else {
			breakMins := entry.Minutes
			endTime := currentTime.Add(time.Duration(breakMins) * time.Minute)

			startTimeStr := currentTime.Format(TIME_ONLY_FORMAT)
			endTimeStr := endTime.Format(TIME_ONLY_FORMAT)
			breakStr := minutesToHourMinuteStr(breakMins)

			lines = append(lines, fmt.Sprintf("%02d. [%s => %s] Break: %s",
				lineNum, startTimeStr, endTimeStr, breakStr))

			currentTime = endTime
		}
		lineNum++
	}

	if timer.Status == StatusRunning || timer.Status == StatusPaused {
		currentMinutes := calculateCurrentMinutes(timer)
		totalMinutes := currentMinutes + runningTotal

		currentStr := minutesToHourMinuteStr(currentMinutes)
		totalStr := minutesToHourMinuteStr(totalMinutes)
		startTimeOnly := currentTime.Format(TIME_ONLY_FORMAT)

		now := getCurrentTime()
		dayDiff := int(now.Sub(currentTime).Hours() / 24)
		dayIndicator := ""
		if dayDiff > 0 {
			dayIndicator = fmt.Sprintf("  [+%d day]", dayDiff)
		}

		totalPaused := timer.PausedMinutes
		if timer.Status == StatusPaused {
			pauseStart, _ := parseTime(timer.PauseStartStr)
			currentPause := deltaMinutes(pauseStart, now)
			totalPaused += currentPause
		}

		pausedStr := ""
		if totalPaused > 0 {
			pausedStr = fmt.Sprintf(" |%02dm|", totalPaused)
		}

		statusSuffix := ""
		if timer.Status == StatusPaused {
			statusSuffix = " (paused)"
		}

		lines = append(lines, fmt.Sprintf("%02d. [%s => .....] Work%s: %s%s (%s)%s",
			lineNum, startTimeOnly, statusSuffix, currentStr, pausedStr, totalStr, dayIndicator))
	}

	if timer.Status == StatusStopped && len(timer.Timeline) > 0 {
		breakMinutes := deltaMinutes(currentTime, getCurrentTime())
		if breakMinutes >= 0 {
			startTimeOnly := currentTime.Format(TIME_ONLY_FORMAT)
			breakStr := minutesToHourMinuteStr(breakMinutes)
			totalStr := minutesToHourMinuteStr(runningTotal)

			now := getCurrentTime()
			dayDiff := int(now.Sub(currentTime).Hours() / 24)
			dayIndicator := ""
			if dayDiff > 0 {
				dayIndicator = fmt.Sprintf("  [+%d day]", dayDiff)
			}

			lines = append(lines, fmt.Sprintf("%02d. [%s => .....] Break: %s (%s)%s",
				lineNum, startTimeOnly, breakStr, totalStr, dayIndicator))
		}
	}

	return lines, nil
}

func historyCmd(timer *Timer, logType string) error {
	validTypes := []string{"info", "debug"}
	if logType != "" {
		valid := false
		for _, t := range validTypes {
			if t == logType {
				valid = true
				break
			}
		}
		if !valid {
			fmt.Printf("Invalid log type: %s. Use one of: ['info', 'debug']\n", logType)
			return nil
		}
	}

	// Debug log still reads from file
	if logType == "debug" {
		filePath, err := debugLogFilePath()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		fmt.Print(string(data))
		return nil
	}

	lines, err := buildInfoLogLines(timer)
	if err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Println(line)
	}

	return nil
}

func reportCmd(timer *Timer, workTimeOnly bool) error {
	if timer.DayStart == "" {
		fmt.Println("No work recorded today.")
		return nil
	}

	// Calculate totals from timeline
	totalWorkMins := 0
	totalBreakMins := 0
	totalPausedMins := 0

	for _, entry := range timer.Timeline {
		if entry.Type == "work" {
			totalWorkMins += entry.Minutes
			totalPausedMins += entry.PausedMinutes
		} else {
			totalBreakMins += entry.Minutes
		}
	}

	// Add current running/paused time if applicable
	currentMins := 0
	if timer.Status == StatusRunning || timer.Status == StatusPaused {
		currentMins = calculateCurrentMinutes(timer)
		totalWorkMins += currentMins

		// Add current cycle's paused time
		if timer.Status == StatusPaused {
			pauseStart, _ := parseTime(timer.PauseStartStr)
			currentPause := deltaMinutes(pauseStart, getCurrentTime())
			totalPausedMins += timer.PausedMinutes + currentPause
		} else {
			totalPausedMins += timer.PausedMinutes
		}
	}

	// Calculate end time
	startDt, _ := parseTime(timer.DayStart)
	endDt := timer.CurrentCycleStart()

	// Add current running time
	if timer.Status == StatusRunning || timer.Status == StatusPaused {
		endDt = endDt.Add(time.Duration(currentMins) * time.Minute)
	}

	// Format output
	dateStr := startDt.Format("2006-01-02")
	startTime := startDt.Format(TIME_ONLY_FORMAT)
	endTime := endDt.Format(TIME_ONLY_FORMAT)
	workStr := minutesToHourMinuteStr(totalWorkMins)
	breakStr := minutesToHourMinuteStr(totalBreakMins)
	pausedStr := minutesToHourMinuteStr(totalPausedMins)
	totalStr := minutesToHourMinuteStr(totalWorkMins + totalBreakMins + totalPausedMins)

	// Check if crossed midnight
	startYear, startMonth, startDay := startDt.Date()
	endYear, endMonth, endDay := endDt.Date()
	startDate := time.Date(startYear, startMonth, startDay, 0, 0, 0, 0, startDt.Location())
	endDate := time.Date(endYear, endMonth, endDay, 0, 0, 0, 0, endDt.Location())
	dayDiff := int(endDate.Sub(startDate).Hours() / 24)
	dayIndicator := ""
	if dayDiff > 0 {
		dayIndicator = fmt.Sprintf(" [+%d day]", dayDiff)
	}

	if workTimeOnly {
		fmt.Printf("%s | %s\n", dateStr, workStr)
	} else {
		fmt.Printf("%s | %s -> %s | Work: %s | Break: %s | Paused: %s | Total: %s%s\n",
			dateStr, startTime, endTime, workStr, breakStr, pausedStr, totalStr, dayIndicator)
	}

	return nil
}

func modListCmd() error {
	fmt.Println("Usage:")
	fmt.Println("  wt mod 1 start <HH:MM>              - set day start (cycle 1 boundary)")
	fmt.Println("  wt mod <num> start <HH:MM>          - set cycle start boundary")
	fmt.Println("  wt mod <num> end <HH:MM>            - set cycle end boundary")
	fmt.Println("  wt mod <num> <add|sub> <time>       - adjust cycle duration")
	fmt.Println("  wt mod <num> pause <add|sub> <time> - adjust paused time")
	fmt.Println("  wt mod <num> drop                   - remove cycle")
	return nil
}

func modCycleStartCmd(timer *Timer, cycleNumStr, timeStr string) error {
	if !isDigits(cycleNumStr) {
		fmt.Printf("Invalid cycle number: %s\n", cycleNumStr)
		return nil
	}

	cycleNum, _ := strconv.Atoi(cycleNumStr)
	maxCycle := maxCycleNumber(timer)
	if cycleNum < 1 || cycleNum > maxCycle {
		fmt.Printf("Cycle %d does not exist. Valid range: 1-%d\n", cycleNum, maxCycle)
		return nil
	}

	oldStart, err := cycleStartByNumber(timer, cycleNum)
	if err != nil {
		return err
	}

	newStart, err := resolveClockTimeNear(oldStart, timeStr)
	if err != nil {
		return err
	}
	if newStart.After(getCurrentTime()) {
		return fmt.Errorf("Cannot set cycle start in the future.")
	}

	shiftMinutes := deltaMinutes(oldStart, newStart)

	if cycleNum == 1 {
		timer.DayStart = newStart.Format(DT_FORMAT)

		if len(timer.Timeline) == 0 && (timer.Status == StatusRunning || timer.Status == StatusPaused) && timer.PauseStartStr != "" {
			pauseStart, _ := parseTime(timer.PauseStartStr)
			timer.PauseStartStr = pauseStart.Add(time.Duration(shiftMinutes) * time.Minute).Format(DT_FORMAT)
		}
	} else {
		prevEntry := &timer.Timeline[cycleNum-2]
		newPrevElapsed := prevEntry.Duration() + shiftMinutes
		if err := setEntryElapsedDuration(prevEntry, newPrevElapsed); err != nil {
			fmt.Println(err)
			return nil
		}
	}

	if err := validateTimerConsistency(timer); err != nil {
		return err
	}
	syncStopDatetimeForStopped(timer)

	logDebug(fmt.Sprintf("wt mod %s start %s", cycleNumStr, timeStr))
	if err := save(timer); err != nil {
		return err
	}

	printMessageIfNotSilent(timer, fmt.Sprintf("Set cycle %d start to %s", cycleNum, newStart.Format(TIME_ONLY_FORMAT)))

	return nil
}

func modCycleEndCmd(timer *Timer, cycleNumStr, timeStr string) error {
	if !isDigits(cycleNumStr) {
		fmt.Printf("Invalid cycle number: %s\n", cycleNumStr)
		return nil
	}

	cycleNum, _ := strconv.Atoi(cycleNumStr)
	if cycleNum < 1 || cycleNum > len(timer.Timeline) {
		fmt.Printf("Cycle %d does not exist. Valid range: 1-%d\n", cycleNum, len(timer.Timeline))
		return nil
	}

	oldEnd, err := cycleEndByNumber(timer, cycleNum)
	if err != nil {
		return err
	}

	newEnd, err := resolveClockTimeNear(oldEnd, timeStr)
	if err != nil {
		return err
	}
	if newEnd.After(getCurrentTime()) {
		return fmt.Errorf("Cannot set cycle end in the future.")
	}

	start, err := cycleStartByNumber(timer, cycleNum)
	if err != nil {
		return err
	}

	newElapsed := deltaMinutes(start, newEnd)
	entry := &timer.Timeline[cycleNum-1]
	if err := setEntryElapsedDuration(entry, newElapsed); err != nil {
		fmt.Println(err)
		return nil
	}

	if err := validateTimerConsistency(timer); err != nil {
		return err
	}
	syncStopDatetimeForStopped(timer)

	logDebug(fmt.Sprintf("wt mod %s end %s", cycleNumStr, timeStr))
	if err := save(timer); err != nil {
		return err
	}

	printMessageIfNotSilent(timer, fmt.Sprintf("Set cycle %d end to %s", cycleNum, newEnd.Format(TIME_ONLY_FORMAT)))

	return nil
}

func modDurationCmd(timer *Timer, cycleNumStr, operation, timeStr string) error {
	if !isDigits(cycleNumStr) {
		fmt.Printf("Invalid cycle number: %s\n", cycleNumStr)
		return nil
	}

	cycleNum, _ := strconv.Atoi(cycleNumStr)

	// Check if user is trying to modify current running/paused cycle
	if (timer.Status == StatusRunning || timer.Status == StatusPaused) && cycleNum == len(timer.Timeline)+1 {
		fmt.Println("Cannot modify duration of current running cycle.")
		fmt.Printf("To set this cycle start directly: wt mod %d start <HH:MM>\n", cycleNum)
		fmt.Printf("To set this cycle end directly: wt mod %d end <HH:MM>\n", cycleNum)
		fmt.Printf("To adjust paused time: wt mod %d pause <add|sub> <time>\n", cycleNum)
		return nil
	}

	if cycleNum < 1 || cycleNum > len(timer.Timeline) {
		fmt.Printf("Cycle %d does not exist. Valid range: 1-%d\n", cycleNum, len(timer.Timeline))
		return nil
	}

	if operation != "add" && operation != "sub" {
		fmt.Printf("Invalid operation: %s. Use 'add' or 'sub'\n", operation)
		return nil
	}

	if !isDigits(timeStr) {
		fmt.Println("Invalid time format. Should be digits only.")
		return nil
	}

	minutes, err := stringTimeToMinutes(timeStr)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	entryIdx := cycleNum - 1
	entry := &timer.Timeline[entryIdx]

	if operation == "add" {
		entry.Minutes += minutes
	} else {
		newDuration := entry.Minutes - minutes
		if newDuration < 0 {
			fmt.Printf("Error: Duration would be negative. Current: %s\n", minutesToHourMinuteStr(entry.Minutes))
			return nil
		}
		entry.Minutes = newDuration
	}

	logDebug(fmt.Sprintf("wt mod %s %s %s", cycleNumStr, operation, timeStr))
	if err := validateTimerConsistency(timer); err != nil {
		return err
	}
	syncStopDatetimeForStopped(timer)
	if err := save(timer); err != nil {
		return err
	}

	sign := "+"
	if operation == "sub" {
		sign = "-"
	}
	printMessageIfNotSilent(timer, fmt.Sprintf("Modified cycle %d duration by %s%s", cycleNum, sign, minutesToHourMinuteStr(minutes)))

	return nil
}

func modPauseCmd(timer *Timer, cycleNumStr, operation, timeStr string) error {
	if !isDigits(cycleNumStr) {
		fmt.Printf("Invalid cycle number: %s\n", cycleNumStr)
		return nil
	}

	cycleNum, _ := strconv.Atoi(cycleNumStr)

	isCurrentCycle := (timer.Status == StatusRunning || timer.Status == StatusPaused) &&
		cycleNum == len(timer.Timeline)+1

	maxCycle := len(timer.Timeline)
	if timer.Status == StatusRunning || timer.Status == StatusPaused {
		maxCycle++
	}

	if !isCurrentCycle && (cycleNum < 1 || cycleNum > len(timer.Timeline)) {
		fmt.Printf("Cycle %d does not exist. Valid range: 1-%d\n", cycleNum, maxCycle)
		return nil
	}

	if operation != "add" && operation != "sub" {
		fmt.Printf("Invalid operation: %s. Use 'add' or 'sub'\n", operation)
		return nil
	}

	if !isDigits(timeStr) {
		fmt.Println("Invalid time format. Should be digits only.")
		return nil
	}

	minutes, err := stringTimeToMinutes(timeStr)
	if err != nil {
		fmt.Println(err)
		return nil
	}

	if isCurrentCycle {
		if timer.Status == StatusPaused {
			now := getCurrentTime()
			pauseStart, _ := parseTime(timer.PauseStartStr)
			currentPause := deltaMinutes(pauseStart, now)
			totalPaused := timer.PausedMinutes + currentPause

			newTotalPaused := totalPaused
			if operation == "add" {
				newTotalPaused += minutes
			} else {
				newTotalPaused -= minutes
				if newTotalPaused < 0 {
					fmt.Printf("Error: Paused time would be negative. Current: %s\n", minutesToHourMinuteStr(totalPaused))
					return nil
				}
			}

			cycleStart := timer.CurrentCycleStart()
			elapsed := deltaMinutes(cycleStart, now)
			if newTotalPaused > elapsed {
				fmt.Printf("Error: Paused time cannot exceed elapsed cycle time (%s).\n", minutesToHourMinuteStr(elapsed))
				return nil
			}

			// Keep paused state active while rebasing total paused time on pause start.
			timer.PausedMinutes = 0
			timer.PauseStartStr = now.Add(-time.Duration(newTotalPaused) * time.Minute).Format(DT_FORMAT)
		} else {
			if operation == "add" {
				timer.PausedMinutes += minutes
			} else {
				newPaused := timer.PausedMinutes - minutes
				if newPaused < 0 {
					fmt.Printf("Error: Paused time would be negative. Current: %s\n", minutesToHourMinuteStr(timer.PausedMinutes))
					return nil
				}
				timer.PausedMinutes = newPaused
			}
		}

		logDebug(fmt.Sprintf("wt mod %s pause %s %s", cycleNumStr, operation, timeStr))
		if err := validateTimerConsistency(timer); err != nil {
			return err
		}
		syncStopDatetimeForStopped(timer)
		if err := save(timer); err != nil {
			return err
		}

		sign := "+"
		if operation == "sub" {
			sign = "-"
		}
		printMessageIfNotSilent(timer, fmt.Sprintf("Modified current cycle paused time by %s%s", sign, minutesToHourMinuteStr(minutes)))
	} else {
		entryIdx := cycleNum - 1
		entry := &timer.Timeline[entryIdx]

		if entry.Type != "work" {
			fmt.Printf("Cycle %d is a break. Paused time can only be modified for work cycles.\n", cycleNum)
			return nil
		}

		currentPaused := entry.PausedMinutes

		var newPaused int
		if operation == "add" {
			newPaused = currentPaused + minutes
		} else {
			newPaused = currentPaused - minutes
			if newPaused < 0 {
				fmt.Printf("Error: Paused time would be negative. Current: %s\n", minutesToHourMinuteStr(currentPaused))
				return nil
			}
		}

		entry.PausedMinutes = newPaused

		logDebug(fmt.Sprintf("wt mod %s pause %s %s", cycleNumStr, operation, timeStr))
		if err := validateTimerConsistency(timer); err != nil {
			return err
		}
		syncStopDatetimeForStopped(timer)
		if err := save(timer); err != nil {
			return err
		}

		sign := "+"
		if operation == "sub" {
			sign = "-"
		}
		printMessageIfNotSilent(timer, fmt.Sprintf("Modified cycle %d paused time by %s%s", cycleNum, sign, minutesToHourMinuteStr(minutes)))
	}

	return nil
}

func modDropCmd(timer *Timer, cycleNumStr string) error {
	if !isDigits(cycleNumStr) {
		fmt.Printf("Invalid cycle number: %s\n", cycleNumStr)
		return nil
	}

	cycleNum, _ := strconv.Atoi(cycleNumStr)
	if cycleNum < 1 || cycleNum > len(timer.Timeline) {
		fmt.Printf("Cycle %d does not exist. Valid range: 1-%d\n", cycleNum, len(timer.Timeline))
		return nil
	}

	entryIdx := cycleNum - 1
	entry := timer.Timeline[entryIdx]
	entryType := entry.Type

	mergeMsg := ""

	if entryType == "break" {
		hasPrevWork := entryIdx > 0 && timer.Timeline[entryIdx-1].Type == "work"
		hasNextWork := entryIdx < len(timer.Timeline)-1 && timer.Timeline[entryIdx+1].Type == "work"

		isCurrentlyActive := timer.Status == StatusRunning || timer.Status == StatusPaused
		isLastBreak := entryIdx == len(timer.Timeline)-1

		if hasPrevWork && isCurrentlyActive && isLastBreak {
			prevWork := timer.Timeline[entryIdx-1]

			// Calculate when the original work session started (before the previous work entry)
			originalStart, _ := parseTime(timer.DayStart)
			for i := 0; i < entryIdx-1; i++ {
				originalStart = originalStart.Add(time.Duration(timer.Timeline[i].Duration()) * time.Minute)
			}

			combinedPaused := prevWork.PausedMinutes + timer.PausedMinutes

			// Remove the break and the previous work entry
			timer.Timeline = append(timer.Timeline[:entryIdx-1], timer.Timeline[entryIdx+1:]...)

			timer.PausedMinutes = combinedPaused

			// Calculate total work time for the message
			now := getCurrentTime()
			totalCycleTime := deltaMinutes(originalStart, now)
			totalPausedCalc := combinedPaused
			if timer.Status == StatusPaused {
				pauseStart, _ := parseTime(timer.PauseStartStr)
				currentPause := deltaMinutes(pauseStart, now)
				totalPausedCalc += currentPause
			}
			totalWork := totalCycleTime - totalPausedCalc

			mergeMsg = fmt.Sprintf(" (merged with running cycle: %s)", minutesToHourMinuteStr(totalWork))
		} else if hasPrevWork && hasNextWork {
			prevWork := &timer.Timeline[entryIdx-1]
			breakMins := timer.Timeline[entryIdx].Minutes
			nextWork := timer.Timeline[entryIdx+1]

			// Merge work cycles: break was actually work time, so add it to work minutes
			mergedWorkMins := prevWork.Minutes + breakMins + nextWork.Minutes
			mergedPausedMins := prevWork.PausedMinutes + nextWork.PausedMinutes

			prevWork.Minutes = mergedWorkMins
			prevWork.PausedMinutes = mergedPausedMins

			// Remove the break and next work
			timer.Timeline = append(timer.Timeline[:entryIdx], timer.Timeline[entryIdx+2:]...)
			mergeMsg = fmt.Sprintf(" (merged adjacent work cycles: %s)", minutesToHourMinuteStr(mergedWorkMins))
		} else {
			timer.Timeline = append(timer.Timeline[:entryIdx], timer.Timeline[entryIdx+1:]...)
		}
	} else { // work cycle
		hasPrevBreak := entryIdx > 0 && timer.Timeline[entryIdx-1].Type == "break"
		hasNextBreak := entryIdx < len(timer.Timeline)-1 && timer.Timeline[entryIdx+1].Type == "break"

		if hasPrevBreak && hasNextBreak {
			prevBreakMins := timer.Timeline[entryIdx-1].Minutes
			workMins := timer.Timeline[entryIdx].ElapsedMinutes() // Work time becomes break (wasn't actually working)
			nextBreakMins := timer.Timeline[entryIdx+1].Minutes
			mergedMins := prevBreakMins + workMins + nextBreakMins

			timer.Timeline[entryIdx-1].Minutes = mergedMins
			timer.Timeline = append(timer.Timeline[:entryIdx], timer.Timeline[entryIdx+2:]...)
			mergeMsg = fmt.Sprintf(" (merged adjacent breaks: %s)", minutesToHourMinuteStr(mergedMins))
		} else {
			timer.Timeline = append(timer.Timeline[:entryIdx], timer.Timeline[entryIdx+1:]...)
		}
	}

	logDebug(fmt.Sprintf("wt mod %s drop", cycleNumStr))
	if err := validateTimerConsistency(timer); err != nil {
		return err
	}
	syncStopDatetimeForStopped(timer)
	if err := save(timer); err != nil {
		return err
	}

	printMessageIfNotSilent(timer, fmt.Sprintf("Removed cycle %d%s", cycleNum, mergeMsg))

	return nil
}

func nextCmd(timer *Timer) error {
	if err := stopCmd(timer, ""); err != nil {
		return err
	}

	// Reload timer after stop
	var err error
	timer, err = load()
	if err != nil {
		return err
	}

	timer.Timeline = append(timer.Timeline, TimelineEntry{
		Type:    "break",
		Minutes: 0,
	})

	if err := save(timer); err != nil {
		return err
	}

	timer.StopDatetimeStr = ""
	now := getCurrentTime()
	timer.PauseStartStr = now.Format(DT_FORMAT)
	timer.PausedMinutes = 0
	timer.Status = StatusRunning

	logDebug("wt next")
	if err := save(timer); err != nil {
		return err
	}

	printMessageIfNotSilent(timer, "Next cycle started.")
	printCheckIfVerbose(timer)

	return nil
}

func resetCmd(msg string) error {
	var oldMode string
	var dailyReportContent []byte

	filePath, err := outputFilePath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(filePath); err == nil {
		oldTimer, err := load()
		if err != nil {
			return err
		}

		if !yesOrNoPrompt("Reset timer?") {
			os.Exit(0)
		}

		oldMode = oldTimer.Mode
		saveDailyReport(oldTimer)
		updateGameOnReset(oldTimer)

		dailyReportPath, _ := dailyReportFilePath()
		if data, err := os.ReadFile(dailyReportPath); err == nil {
			dailyReportContent = data
		}
	}

	outputFolder, err := outputFolderPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(outputFolder); err == nil {
		os.RemoveAll(outputFolder)
	}

	os.MkdirAll(outputFolder, 0755)

	debugPath, _ := debugLogFilePath()
	os.Create(debugPath)

	if dailyReportContent != nil {
		dailyPath, _ := dailyReportFilePath()
		os.WriteFile(dailyPath, dailyReportContent, 0644)
	}

	timer := &Timer{
		Status:          StatusStopped,
		PauseStartStr:   "",
		StopDatetimeStr: "",
		PausedMinutes:   0,
		Mode:            ModeSilent,
		Timeline:        []TimelineEntry{},
		DayStart:        "",
	}

	if oldMode != "" {
		timer.Mode = oldMode
	}

	if err := save(timer); err != nil {
		return err
	}

	printMessageIfNotSilent(timer, msg)
	printCheckIfVerbose(timer)

	return nil
}

func restartCmd(startTime string) error {
	if startTime != "" {
		if err := validateTimeString(startTime); err != nil {
			return err
		}
	}

	if err := resetCmd("Timer reset."); err != nil {
		return err
	}

	timer, err := load()
	if err != nil {
		return err
	}

	return startCmd(timer, startTime)
}

func newCmd() error {
	return resetCmd("New timer initialized.")
}

func removeCmd() error {
	timer, err := load()
	if err != nil {
		return err
	}

	if !yesOrNoPrompt("Remove timer?") {
		os.Exit(0)
	}

	// Save daily report before removing timer
	saveDailyReport(timer)

	filePath, _ := outputFilePath()
	os.Remove(filePath)

	debugPath, _ := debugLogFilePath()
	os.Remove(debugPath)

	dailyPath, _ := dailyReportFilePath()
	if _, err := os.Stat(dailyPath); err == nil {
		os.Remove(dailyPath)
	}

	printMessageIfNotSilent(timer, "Timer removed.")

	return nil
}

func statusCmd() error {
	filePath, err := outputFilePath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		fmt.Println(StatusStopped)
		return nil
	}

	timer, err := load()
	if err != nil {
		return err
	}

	fmt.Println(timer.Status)
	return nil
}

func modeCmd(mode string) error {
	if mode != ModeSilent && mode != ModeNormal && mode != ModeVerbose {
		fmt.Printf("Unhandled mode: %s\n", mode)
		return nil
	}

	timer, err := load()
	if err != nil {
		return err
	}

	timer.Mode = mode
	if err := save(timer); err != nil {
		return err
	}

	printMessageIfNotSilent(timer, fmt.Sprintf("Timer mode set to %s", timer.Mode))

	return nil
}

func debugCmd() error {
	filePath, err := outputFilePath()
	if err != nil {
		return err
	}

	fmt.Printf("output_file_path() = %s\nDT_FORMAT = %s\n", filePath, DT_FORMAT)

	if _, err := os.Stat(filePath); err == nil {
		timer, err := load()
		if err != nil {
			return err
		}

		data, _ := json.MarshalIndent(timer, "", "    ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("No file at %s\n", filePath)
	}

	return nil
}
