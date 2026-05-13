package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
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
	Saves           []string              `json:"saves"`            // append-only datetimes of willpower saves
	LongestStreak   float64               `json:"longest_streak"`   // best ever streak in decimal days
	CompletedQuests []CompletedQuestEntry `json:"completed_quests"` // daily quest completions
	// Legacy fields — read for migration only, not written (omitempty)
	StreakResetDate     string `json:"streak_reset_date,omitempty"`
	StreakResetDatetime string `json:"streak_reset_datetime,omitempty"`
	ConsumedCount       int    `json:"consumed_count,omitempty"`
}

// CompletedQuestEntry records a completed daily quest.
type CompletedQuestEntry struct {
	Date      string `json:"date"`       // "2026-05-04"
	QuestType string `json:"quest_type"` // "long_cycle", "accumulate", "time_gated"
	XPAwarded int    `json:"xp_awarded"`
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
	// Consumables system disabled for now
	// {ID: "hobby_10min", Label: "10min Hobby Time", StreakEvery: 3},
}

// --- Daily Quest System ---

// QuestType identifies the kind of daily quest.
const (
	QuestTypeLongCycle  = "long_cycle"
	QuestTypeAccumulate = "accumulate"
	QuestTypeTimeGated  = "time_gated"
)

// QuestDef defines one possible daily quest with static values.
type QuestDef struct {
	Type         string  // QuestTypeLongCycle, QuestTypeAccumulate, QuestTypeTimeGated
	TargetMins   int     // target work minutes
	DeadlineMins int     // time_gated only: deadline as minutes from midnight
	RewardPct    float64 // XP reward as fraction of current level's XP requirement (0.0-1.0)
}

// allQuestDefs defines the pool of possible daily quests.
var allQuestDefs = []QuestDef{
	{Type: QuestTypeLongCycle, TargetMins: 60, RewardPct: 0.08},
	{Type: QuestTypeLongCycle, TargetMins: 75, RewardPct: 0.10},
	{Type: QuestTypeLongCycle, TargetMins: 90, RewardPct: 0.12},
	{Type: QuestTypeAccumulate, TargetMins: 330, RewardPct: 0.08},
	{Type: QuestTypeAccumulate, TargetMins: 360, RewardPct: 0.10},
	{Type: QuestTypeAccumulate, TargetMins: 390, RewardPct: 0.12},
	{Type: QuestTypeTimeGated, TargetMins: 90, DeadlineMins: 690, RewardPct: 0.10},
	{Type: QuestTypeTimeGated, TargetMins: 120, DeadlineMins: 690, RewardPct: 0.12},
	{Type: QuestTypeTimeGated, TargetMins: 150, DeadlineMins: 720, RewardPct: 0.15},
}

// DailyQuest is a fully instantiated quest for a specific day.
type DailyQuest struct {
	Type         string  // quest type
	TargetMins   int     // target work minutes
	DeadlineMins int     // time_gated only: deadline as minutes from midnight
	RewardPct    float64 // XP reward as fraction of current level requirement
}

// generateDailyQuest produces a deterministic quest for a given date string ("2006-01-02").
func generateDailyQuest(date string) DailyQuest {
	h := sha256.Sum256([]byte("wt-quest-" + date))
	seed := int64(binary.LittleEndian.Uint64(h[:8]))
	rng := rand.New(rand.NewSource(seed))

	def := allQuestDefs[rng.Intn(len(allQuestDefs))]

	return DailyQuest{
		Type:         def.Type,
		TargetMins:   def.TargetMins,
		DeadlineMins: def.DeadlineMins,
		RewardPct:    def.RewardPct,
	}
}

// questRewardXP computes the actual XP reward for a quest based on the current level.
func questRewardXP(q DailyQuest, totalXP float64) int {
	level, _, _ := computeLevel(totalXP)
	needed := xpRequiredForLevel(level)
	reward := int(float64(needed)*q.RewardPct+5) / 10 * 10 // round to nearest 10
	if reward < 10 {
		reward = 10
	}
	return reward
}

// questDescription returns a human-readable description of a daily quest.
func questDescription(q DailyQuest) string {
	switch q.Type {
	case QuestTypeLongCycle:
		return fmt.Sprintf("Complete one work cycle of %s+", minutesToHourMinuteStr(q.TargetMins))
	case QuestTypeAccumulate:
		return fmt.Sprintf("Accumulate %s worked today", minutesToHourMinuteStr(q.TargetMins))
	case QuestTypeTimeGated:
		h := q.DeadlineMins / 60
		m := q.DeadlineMins % 60
		return fmt.Sprintf("Complete %s work before %02d:%02d", minutesToHourMinuteStr(q.TargetMins), h, m)
	}
	return "Unknown quest"
}

// questProgress returns current progress and whether the quest is completed.
func questProgress(q DailyQuest, timer *Timer, game *GameState) (current int, target int, completed bool) {
	target = q.TargetMins
	today := getCurrentTime().Format("2006-01-02")

	switch q.Type {
	case QuestTypeLongCycle:
		// Find the longest single work cycle today
		maxCycle := 0
		if timer != nil {
			for _, entry := range timer.Timeline {
				if entry.Type == "work" && entry.Minutes > maxCycle {
					maxCycle = entry.Minutes
				}
			}
			// Check current running/paused cycle
			if timer.Status == StatusRunning || timer.Status == StatusPaused {
				currentMins := calculateCurrentMinutes(timer)
				if currentMins > maxCycle {
					maxCycle = currentMins
				}
			}
		}
		current = maxCycle

	case QuestTypeAccumulate:
		// Total work minutes today (committed log + current session)
		current = 0
		if timer != nil {
			current = totalWorkMinutesFromTimer(timer)
		}
		// Add any previously committed minutes for today from the game log
		for _, entry := range game.WorkLog {
			if entry.Date == today {
				current += entry.Minutes
				break
			}
		}

	case QuestTypeTimeGated:
		// Work completed before the deadline time today
		if timer != nil && timer.DayStart != "" {
			dayStart, err := parseTime(timer.DayStart)
			if err == nil {
				deadlineTime := time.Date(dayStart.Year(), dayStart.Month(), dayStart.Day(),
					q.DeadlineMins/60, q.DeadlineMins%60, 0, 0, time.Local)
				now := getCurrentTime()

				for _, entry := range timer.Timeline {
					if entry.Type == "work" {
						current += entry.Minutes
					}
				}
				// Add current cycle work if still before deadline
				if (timer.Status == StatusRunning || timer.Status == StatusPaused) && now.Before(deadlineTime) {
					current += calculateCurrentMinutes(timer)
				}
				// Cap at what was achievable before deadline
				if now.After(deadlineTime) {
					// All timeline entries happened — use total timeline work
					// (current cycle was already excluded if after deadline)
				}
			}
		}
	}

	completed = current >= target
	return
}

// isQuestCompletedToday checks if a quest was already completed and recorded for the given date.
func isQuestCompletedToday(game *GameState, date string) bool {
	for _, q := range game.CompletedQuests {
		if q.Date == date {
			return true
		}
	}
	return false
}

// streakMilestones are the named streak goal checkpoints.
var streakMilestones = []int{3, 7, 14, 20, 30, 40, 50, 60, 75, 100}

// gamePath returns the game state file path.
// If WT_GAME_PATH is set, it is used (for isolated testing/dev wrappers).
func gamePath() (string, error) {
	override := os.Getenv("WT_GAME_PATH")
	if override != "" {
		return override, nil
	}

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

// streakDays returns complete 24-hour periods since streak reset (minimum 0).
func streakDays(game *GameState, reference time.Time) int {
	resetTime := streakResetTime(game)
	elapsed := reference.Sub(resetTime)
	if elapsed < 0 {
		return 0
	}
	return int(elapsed.Hours() / 24)
}

// streakHoursElapsed returns hours elapsed within the current 24-hour period (0-23).
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
// Truncates (floors) to 1 decimal place so streak never appears reached before it actually is.
func streakDisplayStr(days, hours int) string {
	value := float64(days) + float64(hours)/24.0
	value = float64(int(value*10)) / 10
	return fmt.Sprintf("%.1f days", value)
}

// streakMultiplier returns the XP multiplier for a given streak day count.
// Scales linearly from 1.01× at day 1 to 2.00× at day 100, capped there.
func streakMultiplier(days int) float64 {
	return 1.0 + float64(min(days, 100))*0.01
}

// xpRequiredForLevel returns XP needed to go from level N to N+1.
// Level 1→2 = 300 XP, 2→3 = 305 XP, N→N+1 = 300+(N-1)*5 XP.
func xpRequiredForLevel(level int) int {
	return 300 + (level-1)*5
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

// breakTimeCmd calculates available break time to finish by a target clock time.
func breakTimeCmd(targetStr string, targetWorkMins int) error {
	timer, err := load()
	if err != nil {
		return err
	}

	now := getCurrentTime()

	// Parse target time as HH:MM on today
	targetTime, err := time.Parse("15:04", targetStr)
	if err != nil {
		return fmt.Errorf("invalid time format %q — use HH:MM (e.g. 16:30)", targetStr)
	}
	target := time.Date(now.Year(), now.Month(), now.Day(), targetTime.Hour(), targetTime.Minute(), 0, 0, now.Location())

	if target.Before(now) {
		return fmt.Errorf("target time %s is in the past", targetStr)
	}

	// Get today's total work (timer + game log)
	todayMins := 0
	if timer != nil && timer.DayStart != "" {
		todayMins = totalWorkMinutesFromTimer(timer)
	}
	game, _ := loadGame()
	if game != nil {
		todayDate := now.Format("2006-01-02")
		for _, entry := range game.WorkLog {
			if entry.Date == todayDate {
				todayMins += entry.Minutes
			}
		}
	}

	remainingWork := targetWorkMins - todayMins
	if remainingWork <= 0 {
		fmt.Println("Work target already complete!")
		return nil
	}

	clockRemaining := int(target.Sub(now).Minutes())
	breakAvailable := clockRemaining - remainingWork

	fmt.Printf("Work remaining:       %s\n", minutesToDayHourMinuteStr(remainingWork))
	fmt.Printf("Clock remaining:      %s (to %s)\n", minutesToDayHourMinuteStr(clockRemaining), targetStr)
	if breakAvailable < 0 {
		fmt.Printf("Break available:      not enough time (need %dm more)\n", -breakAvailable)
	} else {
		fmt.Printf("Break available:      %s\n", minutesToDayHourMinuteStr(breakAvailable))
	}
	return nil
}

// calculateChain returns the current chain count: consecutive 30-min segments.
// A completed work cycle <30min resets the chain to 0.
func calculateChain(timer *Timer) int {
	const chainUnit = 30
	chainCount := 0
	for _, entry := range timer.Timeline {
		if entry.Type == "work" {
			if entry.Minutes >= chainUnit {
				chainCount += entry.Minutes / chainUnit
			} else {
				chainCount = 0
			}
		}
	}
	if timer.Status == StatusRunning || timer.Status == StatusPaused {
		currentMins := calculateCurrentMinutes(timer)
		chainCount += currentMins / chainUnit
	}
	return chainCount
}

// totalXPFromGame computes total XP from all work log entries, quest bonuses, plus the current live timer session.
func totalXPFromGame(game *GameState, timer *Timer) float64 {
	total := 0.0
	for _, entry := range game.WorkLog {
		day := streakDayForDate(game, entry.Date)
		total += float64(entry.Minutes) * streakMultiplier(day)
	}
	for _, q := range game.CompletedQuests {
		total += float64(q.XPAwarded)
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
	// Use date-based streak day (midnight-to-midnight) so that the streak day
	// is stable regardless of the exact dayStart time within that date.
	dateStreakDay := streakDayForDate(game, dateStr)
	if dateStreakDay > 0 {
		for _, c := range allConsumables {
			if c.StreakEvery > 0 && dateStreakDay%c.StreakEvery == 0 {
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

// refEntry represents a boundary point in the reference work day.
// offsetMins is minutes since day start; cumulativeWork is work minutes accumulated to that point.
type refEntry struct {
	offsetMins     int
	cumulativeWork int
}

// refDayStart defines the fixed clock time the reference day begins.
// Used by normCmd to anchor the "Normal" column to an absolute schedule.
const (
	refDayStartHour = 8
	refDayStartMin  = 15
)

// refAnchorTime returns today's reference start time (08:15) for the given time.
func refAnchorTime(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(),
		refDayStartHour, refDayStartMin, 0, 0, now.Location())
}

// referenceDay is a static model of a standard productive work day.
// Offsets are minutes since reference day start (08:15).
//
// 01. [+000 => +045] Work: 0h:45m  (0h:45m)
// 02. [+045 => +065] Break: 0h:20m
// 03. [+065 => +140] Work: 1h:15m  (2h:00m)
// 04. [+140 => +150] Break: 0h:10m
// 05. [+150 => +180] Work: 0h:30m  (2h:30m)
// 06. [+180 => +240] Break: 1h:00m
// 07. [+240 => +290] Work: 0h:50m  (3h:20m)
// 08. [+290 => +305] Break: 0h:15m
// 09. [+305 => +355] Work: 0h:50m  (4h:10m)
// 10. [+355 => +370] Break: 0h:15m
// 11. [+370 => +420] Work: 0h:50m  (5h:00m)
// 12. [+420 => +435] Break: 0h:15m
// 13. [+435 => +465] Work: 0h:30m  (5h:30m)  ← reference finishes at offset 465
var referenceDay = []refEntry{
	{0, 0},
	{45, 45},
	{65, 45},
	{140, 120},
	{150, 120},
	{180, 150},
	{240, 150},
	{290, 200},
	{305, 200},
	{355, 250},
	{370, 250},
	{420, 300},
	{435, 300},
	{465, 330},
}

// refWorkAtOffset returns the cumulative work minutes the reference day had accumulated
// at the given offset (minutes since day start). Linearly interpolates within work blocks;
// flat during break blocks. Returns 0 before offset 0, 330 after offset 465.
func refWorkAtOffset(offsetMins int) int {
	if offsetMins <= 0 {
		return 0
	}
	last := referenceDay[len(referenceDay)-1]
	if offsetMins >= last.offsetMins {
		return last.cumulativeWork
	}
	for i := 1; i < len(referenceDay); i++ {
		prev := referenceDay[i-1]
		curr := referenceDay[i]
		if offsetMins <= curr.offsetMins {
			if curr.offsetMins == prev.offsetMins {
				return curr.cumulativeWork
			}
			fraction := float64(offsetMins-prev.offsetMins) / float64(curr.offsetMins-prev.offsetMins)
			return int(float64(prev.cumulativeWork) + fraction*float64(curr.cumulativeWork-prev.cumulativeWork))
		}
	}
	return last.cumulativeWork
}

// refOffsetForWork returns the offset (minutes since day start) at which the reference
// day completes targetWork minutes of work. Within the reference pattern (≤330 min)
// it linearly interpolates; beyond 330 min it extends at 45 min work per 60 min clock.
func refOffsetForWork(targetWork int) int {
	const extendedRate = 50.0 / 60.0 // 50m work per hour beyond reference
	last := referenceDay[len(referenceDay)-1]
	if targetWork >= last.cumulativeWork {
		extra := float64(targetWork-last.cumulativeWork) / extendedRate
		return last.offsetMins + int(extra)
	}
	for i := 1; i < len(referenceDay); i++ {
		prev := referenceDay[i-1]
		curr := referenceDay[i]
		if targetWork <= curr.cumulativeWork {
			workDelta := curr.cumulativeWork - prev.cumulativeWork
			if workDelta == 0 {
				return curr.offsetMins
			}
			fraction := float64(targetWork-prev.cumulativeWork) / float64(workDelta)
			return prev.offsetMins + int(fraction*float64(curr.offsetMins-prev.offsetMins))
		}
	}
	return last.offsetMins
}

// actualWorkAtOffset returns cumulative work minutes at a given offset (minutes since day start)
// by walking the timeline entries and optionally including the current running/paused cycle.
func actualWorkAtOffset(timer *Timer, offsetMins int) int {
	if offsetMins <= 0 {
		return 0
	}
	cumulativeWork := 0
	entryStart := 0 // offset of current entry start
	for _, entry := range timer.Timeline {
		entryEnd := entryStart + entry.Duration()
		if entry.Type == "work" {
			if offsetMins >= entryEnd {
				cumulativeWork += entry.Minutes
			} else if offsetMins > entryStart {
				// partially through this work entry — interpolate
				fraction := float64(offsetMins-entryStart) / float64(entry.Duration())
				cumulativeWork += int(fraction * float64(entry.Minutes))
			}
		}
		if offsetMins <= entryEnd {
			return cumulativeWork
		}
		entryStart = entryEnd
	}
	// Beyond timeline — include current running/paused cycle if active
	if timer.Status == StatusRunning || timer.Status == StatusPaused {
		currentWork := calculateCurrentMinutes(timer)
		cycleStartOffset := entryStart
		now := getCurrentTime()
		dayStart, _ := parseTime(timer.DayStart)
		nowOffset := int(now.Sub(dayStart).Minutes())
		cycleDuration := nowOffset - cycleStartOffset
		if cycleDuration > 0 && offsetMins > cycleStartOffset {
			if offsetMins >= nowOffset {
				cumulativeWork += currentWork
			} else {
				fraction := float64(offsetMins-cycleStartOffset) / float64(cycleDuration)
				cumulativeWork += int(fraction * float64(currentWork))
			}
		}
	}
	return cumulativeWork
}

// normCmd shows hour-by-hour comparison of actual work vs reference day.
// The "Normal" column is anchored to a fixed reference start time (08:15)
// so that starting late correctly shows you behind the reference schedule.
func normCmd() error {
	timer, err := load()
	if err != nil {
		return err
	}
	if timer == nil || timer.DayStart == "" {
		return fmt.Errorf("no active timer — run 'wt new' first")
	}

	now := getCurrentTime()
	dayStart, err := parseTime(timer.DayStart)
	if err != nil {
		return fmt.Errorf("could not parse day start: %w", err)
	}

	anchor := refAnchorTime(now)
	nowRefOffset := int(now.Sub(anchor).Minutes())       // offset from reference anchor (for Normal column)
	nowActualOffset := int(now.Sub(dayStart).Minutes())   // offset from dayStart (for Actual column)
	anchorToDayStart := int(dayStart.Sub(anchor).Minutes()) // how far dayStart is from anchor

	// Build hour boundaries starting from the earlier of anchor and dayStart, aligned to clock hours
	earliest := anchor
	if dayStart.Before(anchor) {
		earliest = dayStart
	}
	startHour := time.Date(earliest.Year(), earliest.Month(), earliest.Day(),
		earliest.Hour(), 0, 0, 0, earliest.Location())
	if startHour.Before(earliest) {
		startHour = startHour.Add(time.Hour)
	}

	fmt.Printf("%-9s %8s %8s %8s\n", "Hour", "Normal", "Actual", "Diff")
	fmt.Println("--------- -------- -------- --------")

	printedNow := false
	for h := startHour; ; h = h.Add(time.Hour) {
		refOffset := int(h.Sub(anchor).Minutes())
		actualOffset := refOffset - anchorToDayStart

		// If we've passed "now", print the now row first
		if !printedNow && refOffset > nowRefOffset {
			printNormRow("now", nowRefOffset, nowActualOffset, timer, true)
			printedNow = true
		}

		if refOffset > nowRefOffset {
			break
		}

		printNormRow(h.Format("15:04"), refOffset, actualOffset, timer, false)
	}

	if !printedNow {
		printNormRow("now", nowRefOffset, nowActualOffset, timer, true)
	}

	return nil
}

func printNormRow(label string, refOffset int, actualOffset int, timer *Timer, isNow bool) {
	normal := refWorkAtOffset(refOffset)
	actual := actualWorkAtOffset(timer, actualOffset)
	diff := actual - normal

	normalStr := minutesToDayHourMinuteStr(normal)
	actualStr := minutesToDayHourMinuteStr(actual)

	var diffStr string
	if diff == 0 {
		diffStr = "  -"
	} else if diff > 0 {
		// Behind: worked more than normal means... actually ahead
		// Positive diff = actual > normal = ahead of schedule
		diffStr = fmt.Sprintf("%s-%s%s", colorGreen, minutesToDayHourMinuteStr(diff), colorReset)
	} else {
		diffStr = fmt.Sprintf("%s+%s%s", colorRed, minutesToDayHourMinuteStr(-diff), colorReset)
	}

	marker := ""
	if isNow {
		marker = "  ←"
	}

	fmt.Printf("%-9s %8s %8s %8s%s\n", label, normalStr, actualStr, diffStr, marker)
}

// etaCmd prints the ETA for completing a given work target (in decimal hours).
func etaCmd(targetHours float64, showBreakTime bool) error {
	timer, err := load()
	if err != nil {
		return err
	}
	if timer == nil || timer.DayStart == "" {
		return fmt.Errorf("no active timer — run 'wt new' first")
	}

	now := getCurrentTime()
	dayStart, err := parseTime(timer.DayStart)
	if err != nil {
		return fmt.Errorf("could not parse day start: %w", err)
	}

	targetWorkMins := int(targetHours * 60)

	// Get today's total work (timer + game log)
	todayMins := totalWorkMinutesFromTimer(timer)
	game, _ := loadGame()
	if game != nil {
		todayDate := now.Format("2006-01-02")
		for _, entry := range game.WorkLog {
			if entry.Date == todayDate {
				todayMins += entry.Minutes
				break
			}
		}
	}

	if todayMins >= targetWorkMins {
		fmt.Printf("Target of %.4gh already complete (%s done)\n",
			targetHours, minutesToDayHourMinuteStr(todayMins))
		return nil
	}

	todayOffsetMins := int(now.Sub(dayStart).Minutes())
	delta := todayMins - refWorkAtOffset(todayOffsetMins)
	etaOffset := refOffsetForWork(targetWorkMins) - delta
	eta := dayStart.Add(time.Duration(etaOffset) * time.Minute)

	etaStr := eta.Format("15:04")
	fmt.Printf("ETA for %.4gh:  %s\n", targetHours, etaStr)
	if showBreakTime {
		fmt.Println()
		return breakTimeCmd(etaStr, targetWorkMins)
	}
	return nil
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
	// Use hours for bar granularity so partial days show progress
	streakDecimal := float64(days) + float64(hours)/24.0
	streakBarFilled := int((streakDecimal - float64(prevGoal)) * 24)
	streakBarTotal := (nextGoal - prevGoal) * 24

	// Current session stats
	sessionMins := 0
	if timer != nil && timer.DayStart != "" {
		sessionMins = totalWorkMinutesFromTimer(timer)
	}

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

	// Consumables display disabled for now
	// available := 0
	// if len(allConsumables) > 0 {
	// 	available = availableConsumablesCount(game, allConsumables[0].ID)
	// }

	// Header
	sb.WriteString(colorBold + "=== Work Timer RPG ===" + colorReset + "\n")

	// Level (one line, no bar)
	sb.WriteString("\n")
	xpRemaining := float64(xpForNext) - xpInLevel
	sb.WriteString(fmt.Sprintf("  %sLVL %d%s   %.0f / %d xp   %s%.0f xp remaining%s\n",
		colorBold+colorYellow, level, colorReset,
		xpInLevel, xpForNext,
		colorDim, xpRemaining, colorReset))

	// Streak
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %sStreak: %s%s   %s×%.2f XP%s\n",
		colorBold+colorMagenta, streakStr, colorReset,
		colorBold+colorGreen, multiplier, colorReset))
	streakBar := renderBar(streakBarFilled, streakBarTotal, barWidth)
	sb.WriteString(fmt.Sprintf("  %s  %snext milestone: %d days%s\n",
		streakBar, colorDim, nextGoal, colorReset))
	sb.WriteString("\n")
	bestStreak := game.LongestStreak
	if streakDecimal > bestStreak {
		bestStreak = streakDecimal
	}
	bestStreakTrunc := float64(int(bestStreak*10)) / 10
	sb.WriteString(fmt.Sprintf("  %sBest streak: %.1f days%s\n", colorDim, bestStreakTrunc, colorReset))

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

	// Current session (running/paused cycle)
	if timer != nil && (timer.Status == StatusRunning || timer.Status == StatusPaused) {
		currentCycleMins := calculateCurrentMinutes(timer)
		const sessionTarget = 30
		var cycleStr string
		if currentCycleMins >= 60 {
			cycleStr = fmt.Sprintf("%dh %02dm", currentCycleMins/60, currentCycleMins%60)
		} else {
			cycleStr = fmt.Sprintf("%d min", currentCycleMins)
		}
		if currentCycleMins >= sessionTarget {
			cycleStr = colorBold + colorGreen + cycleStr + colorReset
		} else if todayMins < fullDayMins {
			cycleStr = colorRed + cycleStr + colorReset
		}
		currentCycleXP := float64(currentCycleMins) * multiplier
		sb.WriteString("\n")
		sb.WriteString("  Current Session\n")
		sb.WriteString(fmt.Sprintf("  ⚔️  %s / %d min   %s+%.0f xp%s\n", cycleStr, sessionTarget,
			colorBold+colorGreen, currentCycleXP, colorReset))
	}

	// Chain: count of consecutive 30-min segments; a work cycle <30min breaks the chain
	if timer != nil {
		chainCount := calculateChain(timer)
		if chainCount > 0 {
			sb.WriteString(fmt.Sprintf("  🔥 Chain: %d\n", chainCount))
		}
	}

	// Today's work towards full day
	sb.WriteString("\n")
	sb.WriteString("  Today\n")
	todayBar := renderBar(todayMins, fullDayMins, barWidth)
	todayTimeStr := minutesToDayHourMinuteStr(todayMins)
	if todayMins >= fullDayMins {
		todayTimeStr = colorBold + colorGreen + todayTimeStr + colorReset
	}
	todayXP := float64(todayMins) * multiplier
	todayRemaining := ""
	if todayMins < fullDayMins {
		remainingStr := minutesToDayHourMinuteStr(fullDayMins - todayMins)
		todayRemaining = fmt.Sprintf("   %s%s remaining%s", colorDim, remainingStr, colorReset)
	}
	pct := int(float64(todayMins) / float64(fullDayMins) * 100)
	if pct > 100 {
		pct = 100
	}
	pctStr := fmt.Sprintf("  %s%d%%%s", colorDim, pct, colorReset)
	sb.WriteString(fmt.Sprintf("  %s  %s / 5h 30m%s   %s+%.0f xp%s%s\n", todayBar, todayTimeStr,
		pctStr, colorBold+colorGreen, todayXP, colorReset, todayRemaining))

	// Full day ETA (only show when not yet complete)
	if todayMins < fullDayMins {
		const refFinishOffset = 465 // offset minutes when reference day completes 5h30m

		var eta time.Time
		if timer != nil && timer.DayStart != "" {
			if dayStart, err := parseTime(timer.DayStart); err == nil {
				todayOffsetMins := int(today.Sub(dayStart).Minutes())
				refWork := refWorkAtOffset(todayOffsetMins)
				delta := todayMins - refWork // positive = ahead, negative = behind
				etaOffset := refFinishOffset - delta
				eta = dayStart.Add(time.Duration(etaOffset) * time.Minute)
			}
		}
		if !eta.IsZero() {
			sb.WriteString(fmt.Sprintf("\n  Finish ETA:  %s\n", eta.Format("15:04")))
			breakInETA := int(eta.Sub(today).Minutes()) - (fullDayMins - todayMins)
			if breakInETA > 0 {
				sb.WriteString(fmt.Sprintf("  %sBreak time:  %s%s\n", colorDim, minutesToDayHourMinuteStr(breakInETA), colorReset))
			}
		}
	}

	// Daily Quest
	quest := generateDailyQuest(todayDate)
	questCompleted := isQuestCompletedToday(game, todayDate)
	sb.WriteString("\n")
	sb.WriteString("  Quest\n")
	if questCompleted {
		// Find the XP awarded
		questXP := 0
		for _, q := range game.CompletedQuests {
			if q.Date == todayDate {
				questXP = q.XPAwarded
				break
			}
		}
		sb.WriteString(fmt.Sprintf("  %s  %s✓ +%d xp%s\n",
			questDescription(quest), colorBold+colorGreen, questXP, colorReset))
	} else {
		_, _, completed := questProgress(quest, timer, game)
		rewardXP := questRewardXP(quest, totalXP)
		if completed {
			// Just completed — record it
			game.CompletedQuests = append(game.CompletedQuests, CompletedQuestEntry{
				Date:      todayDate,
				QuestType: quest.Type,
				XPAwarded: rewardXP,
			})
			sb.WriteString(fmt.Sprintf("  %s  %s✓ +%d xp%s\n",
				questDescription(quest), colorBold+colorGreen, rewardXP, colorReset))
		} else {
			sb.WriteString(fmt.Sprintf("  %s   %s+%d xp%s\n",
				questDescription(quest), colorDim, rewardXP, colorReset))
		}
	}

	// Total Today XP
	todayTotalXP := float64(todayMins) * multiplier
	// Add quest XP if completed today
	for _, q := range game.CompletedQuests {
		if q.Date == todayDate {
			todayTotalXP += float64(q.XPAwarded)
			break
		}
	}
	if todayMins > 0 || todayTotalXP > 0 {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("  Total today:  %s+%.0f xp%s\n",
			colorBold+colorGreen, todayTotalXP, colorReset))
	}

	// Consumables display disabled for now
	// if available > 0 {
	// 	sb.WriteString("\n")
	// 	sb.WriteString("  Consumables\n")
	// 	sb.WriteString(fmt.Sprintf("  %s  ×%d available   %s[wt game consume]%s\n",
	// 		allConsumables[0].Label, available, colorDim, colorReset))
	// }

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
	questCountBefore := len(game.CompletedQuests)
	fmt.Print(gameOverviewDisplay(game, timer))
	// Save if new achievements shown or quest was just completed
	needsSave := len(game.NewAchievements) > 0 || len(game.CompletedQuests) > questCountBefore
	if needsSave {
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
