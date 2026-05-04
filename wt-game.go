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
	Date      string `json:"date"`                 // "2026-05-04"
	Minutes   int    `json:"minutes"`              // total work minutes for the session
	StreakDay int    `json:"streak_day,omitempty"` // legacy field, present in old files only; used during migration
}

// GameConsumableEntry is one unit of a consumable reward.
type GameConsumableEntry struct {
	ID          string `json:"id"`           // matches ConsumableDef.ID
	AwardedDate string `json:"awarded_date"` // "2026-01-20" — date this was earned
	ConsumedAt  string `json:"consumed_at"`  // "2006-01-02 15:04" or "" if still available
}

// GameState holds all RPG game state
type GameState struct {
	StreakResets    []string              `json:"streak_resets"` // append-only reset datetime history
	WorkLog         []GameWorkLogEntry    `json:"work_log"`
	Achievements    []string              `json:"achievements"`
	NewAchievements []string              `json:"new_achievements"` // shown once, then cleared
	Consumables     []GameConsumableEntry `json:"consumables"`
	Saves           []string              `json:"saves"`          // append-only datetimes of willpower saves
	LongestStreak   float64               `json:"longest_streak"` // best ever streak in decimal days
	// Legacy fields — read for migration only, not written (omitempty)
	StreakResetDate     string `json:"streak_reset_date,omitempty"`
	StreakResetDatetime string `json:"streak_reset_datetime,omitempty"`
	ConsumedCount       int    `json:"consumed_count,omitempty"`
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
	ID          string // unique identifier used in GameConsumableEntry
	Label       string // display name shown when consuming / in overview
	StreakEvery int    // earn one every N streak days (e.g. 5 = day 5, 10, 15...)
}

var allConsumables = []ConsumableDef{
	{ID: "hobby_10min", Label: "10min Hobby Time", StreakEvery: 3},
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
	migrateGameState(&game)
	return &game, nil
}

// migrateGameState upgrades legacy fields to the current data model in-memory.
// It is idempotent: safe to call on already-migrated state.
func migrateGameState(game *GameState) {
	// Migrate single reset datetime → StreakResets array
	if len(game.StreakResets) == 0 {
		if game.StreakResetDatetime != "" {
			game.StreakResets = []string{game.StreakResetDatetime}
		} else if game.StreakResetDate != "" {
			game.StreakResets = []string{game.StreakResetDate + " 00:00"}
		}
		game.StreakResetDatetime = ""
		game.StreakResetDate = ""
	}
	// Migrate ConsumedCount + legacy work log StreakDay entries → Consumables array
	if len(game.Consumables) == 0 && len(allConsumables) > 0 {
		c := allConsumables[0]
		consumed := 0
		for _, entry := range game.WorkLog {
			if c.StreakEvery > 0 && entry.StreakDay > 0 && entry.StreakDay%c.StreakEvery == 0 {
				consumedAt := ""
				if consumed < game.ConsumedCount {
					consumedAt = entry.Date + " 00:00"
					consumed++
				}
				game.Consumables = append(game.Consumables, GameConsumableEntry{
					ID:          c.ID,
					AwardedDate: entry.Date,
					ConsumedAt:  consumedAt,
				})
			}
		}
		game.ConsumedCount = 0
	}
	if game.Consumables == nil {
		game.Consumables = []GameConsumableEntry{}
	}
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

// streakResetTime returns the most recent streak reset time.
func streakResetTime(game *GameState) time.Time {
	if len(game.StreakResets) > 0 {
		last := game.StreakResets[len(game.StreakResets)-1]
		t, err := time.ParseInLocation(DT_FORMAT, last, time.Local)
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

// streakDayForDate returns the streak day count for a past date string ("2006-01-02"),
// finding the applicable reset from the StreakResets history.
func streakDayForDate(game *GameState, dateStr string) int {
	// Find the latest reset whose date portion is <= dateStr
	resetStr := ""
	for _, r := range game.StreakResets {
		if len(r) >= 10 && r[:10] <= dateStr {
			resetStr = r
		}
	}
	if resetStr == "" {
		return 0
	}
	t, err := time.ParseInLocation(DT_FORMAT, resetStr, time.Local)
	if err != nil {
		return 0
	}
	ref, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	if err != nil {
		return 0
	}
	resetMidnight := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	refMidnight := time.Date(ref.Year(), ref.Month(), ref.Day(), 0, 0, 0, 0, time.Local)
	days := int(refMidnight.Sub(resetMidnight).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
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
		day := streakDayForDate(game, entry.Date)
		total += float64(entry.Minutes) * streakMultiplier(day)
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

// availableConsumablesCount returns the number of unconsumed consumables with the given ID.
func availableConsumablesCount(game *GameState, id string) int {
	count := 0
	for _, c := range game.Consumables {
		if c.ID == id && c.ConsumedAt == "" {
			count++
		}
	}
	return count
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

// applySessionToGame records sessionMins worked on dayStart into game,
// awards consumables if this is the first session on a milestone streak day,
// updates the longest streak, checks achievements, and returns any newly
// unlocked achievement IDs. It does not perform any I/O.
func applySessionToGame(game *GameState, sessionMins int, dayStart time.Time) []string {
	dateStr := dayStart.Format("2006-01-02")
	streakDay := streakDays(game, dayStart)
	streakHoursVal := streakHoursElapsed(game, dayStart)
	streakDecimal := float64(streakDay) + float64(streakHoursVal)/24.0

	// Upsert work log entry for this date (streak day is derived from reset history, not stored)
	found := false
	for i, entry := range game.WorkLog {
		if entry.Date == dateStr {
			game.WorkLog[i].Minutes += sessionMins
			game.WorkLog[i].StreakDay = 0 // clear legacy field on upsert
			found = true
			break
		}
	}
	if !found {
		game.WorkLog = append(game.WorkLog, GameWorkLogEntry{
			Date:    dateStr,
			Minutes: sessionMins,
		})
	}

	// Award consumables on the first session of a milestone streak day
	if streakDay > 0 {
		for _, c := range allConsumables {
			if c.StreakEvery > 0 && streakDay%c.StreakEvery == 0 {
				alreadyAwarded := false
				for _, existing := range game.Consumables {
					if existing.ID == c.ID && existing.AwardedDate == dateStr {
						alreadyAwarded = true
						break
					}
				}
				if !alreadyAwarded {
					game.Consumables = append(game.Consumables, GameConsumableEntry{
						ID:          c.ID,
						AwardedDate: dateStr,
						ConsumedAt:  "",
					})
				}
			}
		}
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
	return newAchs
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

	applySessionToGame(game, sessionMins, dayStart)
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

	// Today's total work (current session + any prior sessions already committed to log)
	const fullDayMins = 330 // 5h 30m
	todayDate := today.Format("2006-01-02")
	todayMins := sessionMins
	for _, entry := range game.WorkLog {
		if entry.Date == todayDate {
			todayMins += entry.Minutes
			break
		}
	}

	// Total all-time minutes (log + current session)
	totalAllTimeMins := sessionMins
	for _, entry := range game.WorkLog {
		totalAllTimeMins += entry.Minutes
	}

	// Consumables
	available := 0
	if len(allConsumables) > 0 {
		available = availableConsumablesCount(game, allConsumables[0].ID)
	}

	// Header
	sb.WriteString(colorBold + "=== Work Timer RPG ===" + colorReset + "\n")

	// Level (one line, no bar)
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %sLVL %d%s   %.0f / %d xp\n",
		colorBold+colorYellow, level, colorReset,
		xpInLevel, xpForNext))

	// Streak
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %sStreak: %s%s   %s×%.2f XP%s\n",
		colorBold+colorMagenta, streakStr, colorReset,
		colorBold+colorGreen, multiplier, colorReset))
	streakBar := renderBar(streakBarFilled, streakBarTotal, barWidth)
	sb.WriteString(fmt.Sprintf("  %s  %snext milestone: %d days%s\n",
		streakBar, colorDim, nextGoal, colorReset))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %sBest streak: %.1f days%s\n", colorDim, game.LongestStreak, colorReset))

	// Count saves since last streak reset
	lastReset := ""
	if len(game.StreakResets) > 0 {
		lastReset = game.StreakResets[len(game.StreakResets)-1]
	}
	savesThisStreak := 0
	for _, s := range game.Saves {
		if s >= lastReset {
			savesThisStreak++
		}
	}
	if savesThisStreak > 0 {
		sb.WriteString(fmt.Sprintf("  %s🛡️ %d save%s this streak%s\n",
			colorBold+colorGreen, savesThisStreak, pluralS(savesThisStreak), colorReset))
	}

	// Today's work towards full day
	sb.WriteString("\n")
	sb.WriteString("  Today\n")
	todayBar := renderBar(todayMins, fullDayMins, barWidth)
	sb.WriteString(fmt.Sprintf("  %s  %s / 5h 30m\n", todayBar, minutesToDayHourMinuteStr(todayMins)))
	if sessionMins > 0 {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("   %s+%.0f xp%s  (%s × %.2fx)\n",
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
		StreakResets:    []string{now},
		WorkLog:         []GameWorkLogEntry{},
		Achievements:    []string{},
		NewAchievements: []string{},
		Consumables:     []GameConsumableEntry{},
		LongestStreak:   0,
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
	game.StreakResets = append(game.StreakResets, now)
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

	type consumableStatus struct {
		def       ConsumableDef
		available int
	}
	statuses := make([]consumableStatus, len(allConsumables))
	for i, c := range allConsumables {
		statuses[i] = consumableStatus{def: c, available: availableConsumablesCount(game, c.ID)}
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
		now := getCurrentTime().Format(DT_FORMAT)
		for j, cons := range game.Consumables {
			if cons.ID == allConsumables[idx].ID && cons.ConsumedAt == "" {
				game.Consumables[j].ConsumedAt = now
				break
			}
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

// pluralS returns "s" if n != 1, otherwise "".
func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// gameSavedCmd records a willpower save and reports today's count.
func gameSavedCmd() error {
	if !isGameEnabled() {
		fmt.Println("Game not enabled. Run 'wt game enable' to get started.")
		return nil
	}
	game, err := loadGame()
	if err != nil {
		return err
	}
	now := getCurrentTime()
	game.Saves = append(game.Saves, now.Format(DT_FORMAT))
	if err := saveGame(game); err != nil {
		return err
	}
	lastReset := ""
	if len(game.StreakResets) > 0 {
		lastReset = game.StreakResets[len(game.StreakResets)-1]
	}
	count := 0
	for _, s := range game.Saves {
		if s >= lastReset {
			count++
		}
	}
	fmt.Printf("%s🛡️️ Save recorded!%s  %d save%s this streak — willpower preserved.\n",
		colorBold+colorGreen, colorReset, count, pluralS(count))
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
