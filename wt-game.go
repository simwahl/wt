package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ANSI color codes for terminal output
const (
	colorReset   = "\033[0m"
	colorBold    = "\033[1m"
	colorDim     = "\033[2m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
)

// GameWorkLogEntry records work done on a specific day
type GameWorkLogEntry struct {
	Date      string `json:"date"`       // "2026-05-04"
	Minutes   int    `json:"minutes"`    // total work minutes for the session
	StreakDay int    `json:"streak_day"` // streak day count when logged (for XP multiplier)
}

// GameState holds all RPG game state
type GameState struct {
	StreakResetDate     string             `json:"streak_reset_date,omitempty"` // legacy: date only "2006-01-02"
	StreakResetDatetime string             `json:"streak_reset_datetime"`       // "2006-01-02 15:04"
	WorkLog             []GameWorkLogEntry `json:"work_log"`
	Achievements        []string           `json:"achievements"`     // unlocked achievement IDs
	NewAchievements     []string           `json:"new_achievements"` // shown once, then cleared
	ConsumedCount       int                `json:"consumed_count"`   // hobby time units consumed
	LongestStreak       float64            `json:"longest_streak"`   // best ever streak in decimal days
}

// AchievementDef defines a single unlockable achievement.
type AchievementDef struct {
	ID          string
	Label       string
	StreakDays  int // >0: unlock when longestStreak >= StreakDays
	HoursNeeded int // >0: unlock when total all-time hours >= HoursNeeded
}

var allAchievements = []AchievementDef{
	{ID: "streak_3", Label: "3-Day Streak", StreakDays: 3},
	{ID: "streak_7", Label: "7-Day Streak", StreakDays: 7},
	{ID: "streak_14", Label: "14-Day Streak", StreakDays: 14},
	{ID: "streak_20", Label: "20-Day Streak", StreakDays: 20},
	{ID: "streak_30", Label: "30-Day Streak", StreakDays: 30},
	{ID: "streak_40", Label: "40-Day Streak", StreakDays: 40},
	{ID: "streak_50", Label: "50-Day Streak", StreakDays: 50},
	{ID: "streak_60", Label: "60-Day Streak", StreakDays: 60},
	{ID: "streak_75", Label: "75-Day Streak", StreakDays: 75},
	{ID: "streak_100", Label: "100-Day Streak", StreakDays: 100},
	{ID: "hours_50", Label: "50h Worked", HoursNeeded: 50},
	{ID: "hours_100", Label: "100h Worked", HoursNeeded: 100},
	{ID: "hours_250", Label: "250h Worked", HoursNeeded: 250},
	{ID: "hours_500", Label: "500h Worked", HoursNeeded: 500},
	{ID: "hours_1000", Label: "1000h Worked", HoursNeeded: 1000},
}

// ConsumableDef defines a type of consumable reward earned at a streak interval.
type ConsumableDef struct {
	Label       string // display name shown when consuming / in overview
	StreakEvery int    // earn one every N streak days (e.g. 5 = day 5, 10, 15...)
}

var allConsumables = []ConsumableDef{
	{Label: "10min Hobby Time", StreakEvery: 5},
}

// streakMilestones are the named streak goal checkpoints.
var streakMilestones = []int{3, 7, 14, 20, 30, 40, 50, 60, 75, 100}

// gamePath returns the fixed path to the game state file.
func gamePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Documents", "wtg.json"), nil
}

// isGameEnabled returns true if the game state file exists.
func isGameEnabled() bool {
	path, err := gamePath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// loadGame reads game state from disk.
func loadGame() (*GameState, error) {
	path, err := gamePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var game GameState
	if err := json.Unmarshal(data, &game); err != nil {
		return nil, err
	}
	// Migrate from legacy date-only field
	if game.StreakResetDatetime == "" && game.StreakResetDate != "" {
		game.StreakResetDatetime = game.StreakResetDate + " 00:00"
	}
	return &game, nil
}

// saveGame writes game state to disk.
func saveGame(game *GameState) error {
	path, err := gamePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(game, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// streakResetTime parses the streak reset datetime from game state.
func streakResetTime(game *GameState) time.Time {
	if game.StreakResetDatetime != "" {
		t, err := time.ParseInLocation(DT_FORMAT, game.StreakResetDatetime, time.Local)
		if err == nil {
			return t
		}
	}
	if game.StreakResetDate != "" {
		t, err := time.ParseInLocation("2006-01-02", game.StreakResetDate, time.Local)
		if err == nil {
			return t
		}
	}
	return time.Now()
}

// streakDays returns calendar days since streak reset (minimum 0). Same calendar day = 0.
func streakDays(game *GameState, reference time.Time) int {
	resetTime := streakResetTime(game)
	resetMidnight := time.Date(resetTime.Year(), resetTime.Month(), resetTime.Day(), 0, 0, 0, 0, time.Local)
	refMidnight := time.Date(reference.Year(), reference.Month(), reference.Day(), 0, 0, 0, 0, time.Local)
	days := int(refMidnight.Sub(resetMidnight).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// streakHoursElapsed returns hours elapsed since streak reset within the current day slot (0-23).
func streakHoursElapsed(game *GameState, reference time.Time) int {
	resetTime := streakResetTime(game)
	elapsed := reference.Sub(resetTime)
	if elapsed < 0 {
		return 0
	}
	return int(elapsed.Hours()) % 24
}

// minutesToDayHourMinuteStr formats minutes as "Xd Yh Zm" (omits days if 0, omits minutes if 0).
func minutesToDayHourMinuteStr(mins int) string {
	d := mins / (60 * 24)
	h := (mins % (60 * 24)) / 60
	m := mins % 60
	if d > 0 {
		return fmt.Sprintf("%dd %dh %dm", d, h, m)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

// streakDisplayStr returns a human-readable streak string like "0.5 days" or "2.9 days".
func streakDisplayStr(days, hours int) string {
	value := float64(days) + float64(hours)/24.0
	return fmt.Sprintf("%.1f days", value)
}

// streakMultiplier returns the XP multiplier for a given streak day count.
// Scales linearly from 1.01× at day 1 to 2.00× at day 100, capped there.
func streakMultiplier(days int) float64 {
	return 1.0 + float64(min(days, 100))*0.01
}

// xpRequiredForLevel returns XP needed to go from level N to N+1.
// Level 1→2 = 300 XP, 2→3 = 302 XP, N→N+1 = 300+(N-1)*2 XP.
func xpRequiredForLevel(level int) int {
	return 300 + (level-1)*2
}

// computeLevel returns level, XP within the current level, and XP needed for next level.
func computeLevel(totalXP float64) (level int, xpInLevel float64, xpForNext int) {
	level = 1
	accumulated := 0.0
	for {
		needed := float64(xpRequiredForLevel(level))
		if accumulated+needed > totalXP {
			xpInLevel = totalXP - accumulated
			xpForNext = int(needed)
			return
		}
		accumulated += needed
		level++
	}
}

// totalWorkMinutesFromTimer returns total work minutes from the timer, including any live session.
func totalWorkMinutesFromTimer(timer *Timer) int {
	total := 0
	for _, entry := range timer.Timeline {
		if entry.Type == "work" {
			total += entry.Minutes
		}
	}
	if timer.Status == StatusRunning || timer.Status == StatusPaused {
		total += calculateCurrentMinutes(timer)
	}
	return total
}

// totalXPFromGame computes total XP from all work log entries plus the current live timer session.
func totalXPFromGame(game *GameState, timer *Timer) float64 {
	total := 0.0
	for _, entry := range game.WorkLog {
		total += float64(entry.Minutes) * streakMultiplier(entry.StreakDay)
	}
	if timer != nil && timer.DayStart != "" {
		dayStart, err := parseTime(timer.DayStart)
		if err == nil {
			currentMins := totalWorkMinutesFromTimer(timer)
			currentStreakDay := streakDays(game, dayStart)
			total += float64(currentMins) * streakMultiplier(currentStreakDay)
		}
	}
	return total
}

// streakResetDateStr returns the date portion ("2006-01-02") of the streak reset,
// preferring StreakResetDatetime over the legacy StreakResetDate field.
func streakResetDateStr(game *GameState) string {
	if game.StreakResetDatetime != "" {
		return game.StreakResetDatetime[:10] // first 10 chars are the date
	}
	return game.StreakResetDate
}

// computeAwardedConsumables counts consumables earned since the streak reset,
// based on allConsumables[0].StreakEvery milestone days in the work log.
func computeAwardedConsumables(game *GameState) int {
	if len(allConsumables) == 0 {
		return 0
	}
	every := allConsumables[0].StreakEvery
	if every <= 0 {
		return 0
	}
	resetDate := streakResetDateStr(game)
	awarded := 0
	for _, entry := range game.WorkLog {
		if entry.Date >= resetDate && entry.StreakDay > 0 && entry.StreakDay%every == 0 {
			awarded++
		}
	}
	return awarded
}

// hasAchievement returns true if the achievement ID is already unlocked.
func hasAchievement(game *GameState, id string) bool {
	for _, a := range game.Achievements {
		if a == id {
			return true
		}
	}
	return false
}

// achievementLabel returns the display label for an achievement ID.
func achievementLabel(id string) string {
	for _, ach := range allAchievements {
		if ach.ID == id {
			return ach.Label
		}
	}
	return id
}

// checkAndUnlockAchievements checks all achievements and returns newly unlocked IDs.
func checkAndUnlockAchievements(game *GameState, longestStreak float64, totalMinutes int) []string {
	var newlyUnlocked []string
	totalHours := totalMinutes / 60
	for _, ach := range allAchievements {
		if hasAchievement(game, ach.ID) {
			continue
		}
		if ach.StreakDays > 0 && int(longestStreak) >= ach.StreakDays {
			newlyUnlocked = append(newlyUnlocked, ach.ID)
		} else if ach.HoursNeeded > 0 && totalHours >= ach.HoursNeeded {
			newlyUnlocked = append(newlyUnlocked, ach.ID)
		}
	}
	return newlyUnlocked
}

// updateGameOnReset is called when the timer is reset/new/restarted.
// It records the session's work in the game log and checks for new achievements.
func updateGameOnReset(timer *Timer) error {
	if !isGameEnabled() {
		return nil
	}
	if timer.DayStart == "" {
		return nil
	}

	game, err := loadGame()
	if err != nil {
		return err
	}

	sessionMins := totalWorkMinutesFromTimer(timer)
	if sessionMins == 0 {
		return nil
	}

	dayStart, err := parseTime(timer.DayStart)
	if err != nil {
		return err
	}
	dateStr := dayStart.Format("2006-01-02")
	streakDay := streakDays(game, dayStart)
	streakHours := streakHoursElapsed(game, dayStart)
	streakDecimal := float64(streakDay) + float64(streakHours)/24.0

	// Upsert work log entry for this date
	found := false
	for i, entry := range game.WorkLog {
		if entry.Date == dateStr {
			game.WorkLog[i].Minutes += sessionMins
			game.WorkLog[i].StreakDay = streakDay
			found = true
			break
		}
	}
	if !found {
		game.WorkLog = append(game.WorkLog, GameWorkLogEntry{
			Date:      dateStr,
			Minutes:   sessionMins,
			StreakDay: streakDay,
		})
	}

	// Update longest streak
	if streakDecimal > game.LongestStreak {
		game.LongestStreak = streakDecimal
	}

	// Total all-time minutes (post-upsert) for achievement checking
	totalAllTimeMins := 0
	for _, entry := range game.WorkLog {
		totalAllTimeMins += entry.Minutes
	}

	// Check and unlock achievements
	newAchs := checkAndUnlockAchievements(game, game.LongestStreak, totalAllTimeMins)
	if len(newAchs) > 0 {
		game.Achievements = append(game.Achievements, newAchs...)
		game.NewAchievements = append(game.NewAchievements, newAchs...)
	}

	return saveGame(game)
}

// nextStreakGoal returns the next streak milestone above the current day count.
func nextStreakGoal(days int) int {
	for _, goal := range streakMilestones {
		if days < goal {
			return goal
		}
	}
	// Beyond 100: use 50-day milestones
	return ((days / 50) + 1) * 50
}

// prevStreakGoal returns the most recent streak milestone at or below the current day count.
func prevStreakGoal(days int) int {
	if days > 100 {
		return (days / 50) * 50
	}
	prev := 0
	for _, g := range streakMilestones {
		if g <= days {
			prev = g
		}
	}
	return prev
}

// renderBar renders a fixed-width progress bar with colored fill and dim empty blocks.
func renderBar(filled, total, width int) string {
	if total <= 0 {
		total = 1
	}
	filledCount := filled * width / total
	if filledCount > width {
		filledCount = width
	}
	if filledCount < 0 {
		filledCount = 0
	}
	emptyCount := width - filledCount
	bar := colorCyan + strings.Repeat("█", filledCount) + colorReset +
		colorDim + strings.Repeat("░", emptyCount) + colorReset
	return "[" + bar + "]"
}

// gameOverviewDisplay builds and returns the full RPG overview string.
func gameOverviewDisplay(game *GameState, timer *Timer) string {
	var sb strings.Builder
	const barWidth = 22

	// Compute values
	totalXP := totalXPFromGame(game, timer)
	level, xpInLevel, xpForNext := computeLevel(totalXP)

	today := getCurrentTime()
	days := streakDays(game, today)
	hours := streakHoursElapsed(game, today)
	multiplier := streakMultiplier(days)
	streakStr := streakDisplayStr(days, hours)

	nextGoal := nextStreakGoal(days)
	prevGoal := prevStreakGoal(days)
	streakBarFilled := days - prevGoal
	streakBarTotal := nextGoal - prevGoal

	// Current session stats
	sessionMins := 0
	if timer != nil && timer.DayStart != "" {
		sessionMins = totalWorkMinutesFromTimer(timer)
	}
	sessionXP := float64(sessionMins) * multiplier

	// Total all-time minutes (log + current session)
	totalAllTimeMins := sessionMins
	for _, entry := range game.WorkLog {
		totalAllTimeMins += entry.Minutes
	}

	// Consumables
	awarded := computeAwardedConsumables(game)
	available := awarded - game.ConsumedCount
	if available < 0 {
		available = 0
	}

	// Header
	sb.WriteString(colorBold + "=== Work Timer RPG ===" + colorReset + "\n")

	// Level & XP
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %sLVL %d%s\n", colorBold+colorYellow, level, colorReset))
	sb.WriteString("  XP\n")
	xpBar := renderBar(int(xpInLevel), xpForNext, barWidth)
	sb.WriteString(fmt.Sprintf("  %s  %s%.0f%s / %d xp\n", xpBar, colorCyan, xpInLevel, colorReset, xpForNext))

	// Streak
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  Streak: %s%s%s   %s×%.2f XP%s\n",
		colorBold+colorMagenta, streakStr, colorReset,
		colorBold+colorGreen, multiplier, colorReset))
	streakBar := renderBar(streakBarFilled, streakBarTotal, barWidth)
	sb.WriteString(fmt.Sprintf("  %s  %snext milestone: %d days%s\n",
		streakBar, colorDim, nextGoal, colorReset))

	// Stats
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %sBest streak: %.1f days%s\n", colorDim, game.LongestStreak, colorReset))
	sb.WriteString(fmt.Sprintf("  %sTotal worked: %s%s\n", colorDim, minutesToDayHourMinuteStr(totalAllTimeMins), colorReset))

	// Today's session
	if sessionMins > 0 {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("  Today   %s+%.0f xp%s  (%s × %.2fx)\n",
			colorBold+colorGreen, sessionXP, colorReset,
			minutesToHourMinuteStr(sessionMins), multiplier))
	}

	// Consumables
	if available > 0 {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("  %s%s  ×%d available%s   [wt game consume]\n",
			colorBold+colorYellow, allConsumables[0].Label, available, colorReset))
	}

	// New achievement unlocks (shown once, then cleared)
	if len(game.NewAchievements) > 0 {
		sb.WriteString("\n")
		for _, id := range game.NewAchievements {
			sb.WriteString(fmt.Sprintf("  %s★ ACHIEVEMENT UNLOCKED: %s!%s\n",
				colorBold+colorYellow, achievementLabel(id), colorReset))
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

// gameCmd shows the RPG overview.
func gameCmd() error {
	if !isGameEnabled() {
		fmt.Println("Game not enabled. Run 'wt game enable' to get started.")
		return nil
	}
	game, err := loadGame()
	if err != nil {
		return err
	}
	timer, _ := load()
	fmt.Print(gameOverviewDisplay(game, timer))
	// Clear new achievements after displaying once
	if len(game.NewAchievements) > 0 {
		game.NewAchievements = nil
		return saveGame(game)
	}
	return nil
}

// gameEnableCmd creates the game state file and enables the game.
func gameEnableCmd() error {
	if isGameEnabled() {
		fmt.Println("Game already enabled.")
		return nil
	}
	now := getCurrentTime().Format(DT_FORMAT)
	game := &GameState{
		StreakResetDatetime: now,
		WorkLog:             []GameWorkLogEntry{},
		Achievements:        []string{},
		NewAchievements:     []string{},
		ConsumedCount:       0,
		LongestStreak:       0,
	}
	if err := saveGame(game); err != nil {
		return err
	}
	fmt.Println(colorBold + "Game enabled! Welcome to Work Timer RPG." + colorReset)
	timer, _ := load()
	fmt.Println()
	fmt.Print(gameOverviewDisplay(game, timer))
	return nil
}

// gameStreakResetCmd resets the streak start date to today.
func gameStreakResetCmd() error {
	if !isGameEnabled() {
		fmt.Println("Game not enabled. Run 'wt game enable' to get started.")
		return nil
	}
	game, err := loadGame()
	if err != nil {
		return err
	}
	now := getCurrentTime().Format(DT_FORMAT)
	game.StreakResetDatetime = now
	if err := saveGame(game); err != nil {
		return err
	}
	fmt.Printf("Streak reset. %s0 days%s starting now (%s).\n", colorBold+colorMagenta, colorReset, now)
	return nil
}

// gameConsumeCmd lists available consumables, or consumes one by 1-based number.
func gameConsumeCmd(arg string) error {
	if !isGameEnabled() {
		fmt.Println("Game not enabled. Run 'wt game enable' to get started.")
		return nil
	}
	game, err := loadGame()
	if err != nil {
		return err
	}

	// Build per-consumable available counts (only one type today, keyed by index)
	type consumableStatus struct {
		def       ConsumableDef
		awarded   int
		available int
	}
	resetDate := streakResetDateStr(game)
	statuses := make([]consumableStatus, len(allConsumables))
	for i, c := range allConsumables {
		// Re-use computeAwardedConsumables logic inline for each type
		awarded := 0
		for _, entry := range game.WorkLog {
			if c.StreakEvery > 0 && entry.Date >= resetDate && entry.StreakDay > 0 && entry.StreakDay%c.StreakEvery == 0 {
				awarded++
			}
		}
		// consumed count: use ConsumedCount for index 0 (backward compat)
		consumed := 0
		if i == 0 {
			consumed = game.ConsumedCount
		}
		available := awarded - consumed
		if available < 0 {
			available = 0
		}
		statuses[i] = consumableStatus{def: c, awarded: awarded, available: available}
	}

	// If an index arg was given, consume that item
	if arg != "" {
		idx := 0
		if _, err := fmt.Sscanf(arg, "%d", &idx); err != nil || idx < 1 || idx > len(allConsumables) {
			return fmt.Errorf("invalid consumable number %q — use 'wt game consume' to see the list", arg)
		}
		idx-- // convert to 0-based
		if statuses[idx].available <= 0 {
			fmt.Printf("No %s available.\n", statuses[idx].def.Label)
			return nil
		}
		if idx == 0 {
			game.ConsumedCount++
		}
		if err := saveGame(game); err != nil {
			return err
		}
		remaining := statuses[idx].available - 1
		fmt.Printf("%s🎮 Enjoy your %s!%s  (%d remaining)\n",
			colorBold+colorYellow, statuses[idx].def.Label, colorReset, remaining)
		return nil
	}

	// No arg: list available consumables
	anyAvailable := false
	for _, s := range statuses {
		if s.available > 0 {
			anyAvailable = true
			break
		}
	}
	if !anyAvailable {
		fmt.Println("No available consumables.")
	} else {
		fmt.Println(colorBold + "Available:" + colorReset)
		for i, s := range statuses {
			if s.available > 0 {
				fmt.Printf("  %d. %s%s%s  ×%d\n", i+1, colorBold+colorYellow, s.def.Label, colorReset, s.available)
			}
		}
		fmt.Println()
		fmt.Printf("%sUse 'wt game consume <number>' to consume.%s\n", colorDim, colorReset)
	}
	fmt.Println()
	fmt.Println(colorBold + "Earnable:" + colorReset)
	for _, s := range statuses {
		fmt.Printf("  %s  — every %d streak days\n", s.def.Label, s.def.StreakEvery)
	}
	return nil
}

// gameAchievementsCmd shows all achievements with locked/unlocked status and progress.
func gameAchievementsCmd() error {
	if !isGameEnabled() {
		fmt.Println("Game not enabled. Run 'wt game enable' to get started.")
		return nil
	}
	game, err := loadGame()
	if err != nil {
		return err
	}
	timer, _ := load()

	totalAllTimeMins := 0
	for _, entry := range game.WorkLog {
		totalAllTimeMins += entry.Minutes
	}
	if timer != nil && timer.DayStart != "" {
		totalAllTimeMins += totalWorkMinutesFromTimer(timer)
	}

	fmt.Println(colorBold + "=== Achievements ===" + colorReset)
	fmt.Println()
	fmt.Println("  " + colorBold + "Streak:" + colorReset)
	for _, ach := range allAchievements {
		if ach.StreakDays == 0 {
			continue
		}
		if hasAchievement(game, ach.ID) {
			fmt.Printf("  %s✓ %s%s\n", colorGreen+colorBold, ach.Label, colorReset)
		} else {
			fmt.Printf("  %s✗ %s%s\n", colorDim, ach.Label, colorReset)
		}
	}
	fmt.Println()
	fmt.Println("  " + colorBold + "Cumulative Work:" + colorReset)
	for _, ach := range allAchievements {
		if ach.HoursNeeded == 0 {
			continue
		}
		if hasAchievement(game, ach.ID) {
			fmt.Printf("  %s✓ %s%s\n", colorGreen+colorBold, ach.Label, colorReset)
		} else {
			fmt.Printf("  %s✗ %s%s\n", colorDim, ach.Label, colorReset)
		}
	}
	return nil
}
