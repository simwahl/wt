package main

import (
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"
)

// mustTime parses "2006-01-02 15:04" in local time, panicking on error.
func mustTime(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

// newGame returns a minimal GameState with the given streak reset datetime.
func newGame(resetDatetime string) *GameState {
	return &GameState{
		StreakResets:    []string{resetDatetime},
		WorkLog:         []GameWorkLogEntry{},
		Achievements:    []string{},
		NewAchievements: []string{},
		Consumables:     []GameConsumableEntry{},
	}
}

// ----------------------------------------------------------------------------
// streakDays
// ----------------------------------------------------------------------------

func TestStreakDays(t *testing.T) {
	cases := []struct {
		name      string
		reset     string
		reference string
		want      int
	}{
		{"same day, later time", "2026-01-20 09:00", "2026-01-20 17:00", 0},
		{"same day, same time", "2026-01-20 09:00", "2026-01-20 09:00", 0},
		{"23h elapsed — still day 0", "2026-01-20 09:00", "2026-01-21 08:00", 0},
		{"exactly 24h elapsed — day 1", "2026-01-20 09:00", "2026-01-21 09:00", 1},
		{"next day, after reset time", "2026-01-20 09:00", "2026-01-21 10:00", 1},
		{"5 days later (same time)", "2026-01-15 09:00", "2026-01-20 09:00", 5},
		{"reference before reset clamps to 0", "2026-01-20 09:00", "2026-01-19 09:00", 0},
		{"reset at midnight, reference next midnight", "2026-01-20 00:00", "2026-01-21 00:00", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			game := newGame(c.reset)
			got := streakDays(game, mustTime(c.reference))
			if got != c.want {
				t.Errorf("streakDays(%q, %q) = %d, want %d", c.reset, c.reference, got, c.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// streakHoursElapsed
// ----------------------------------------------------------------------------

func TestStreakHoursElapsed(t *testing.T) {
	cases := []struct {
		name      string
		reset     string
		reference string
		want      int
	}{
		{"same moment", "2026-01-20 09:00", "2026-01-20 09:00", 0},
		{"6 hours later same day", "2026-01-20 09:00", "2026-01-20 15:00", 6},
		// 15 hours later wraps within 24 → 15 % 24 = 15
		{"15 hours later", "2026-01-20 09:00", "2026-01-21 00:00", 15},
		// 24 hours later wraps to 0
		{"24 hours later wraps to 0", "2026-01-20 09:00", "2026-01-21 09:00", 0},
		{"reference before reset clamps to 0", "2026-01-20 09:00", "2026-01-19 09:00", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			game := newGame(c.reset)
			got := streakHoursElapsed(game, mustTime(c.reference))
			if got != c.want {
				t.Errorf("streakHoursElapsed(%q, %q) = %d, want %d", c.reset, c.reference, got, c.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// streakMultiplier
// ----------------------------------------------------------------------------

func TestStreakMultiplier(t *testing.T) {
	cases := []struct {
		days int
		want float64
	}{
		{0, 1.00},
		{1, 1.01},
		{5, 1.05},
		{50, 1.50},
		{100, 2.00},
		{101, 2.00}, // capped at day 100
		{200, 2.00},
	}
	for _, c := range cases {
		got := streakMultiplier(c.days)
		if got != c.want {
			t.Errorf("streakMultiplier(%d) = %.2f, want %.2f", c.days, got, c.want)
		}
	}
}

// ----------------------------------------------------------------------------
// streakDisplayStr
// ----------------------------------------------------------------------------

func TestStreakDisplayStr(t *testing.T) {
	cases := []struct {
		days, hours int
		want        string
	}{
		{0, 0, "0.0 days"},
		{0, 12, "0.5 days"},
		{1, 0, "1.0 days"},
		{2, 23, "2.9 days"}, // 23/24=0.958 must truncate, not round to 3.0
		{3, 0, "3.0 days"},
		{7, 6, "7.2 days"},
	}
	for _, c := range cases {
		got := streakDisplayStr(c.days, c.hours)
		if got != c.want {
			t.Errorf("streakDisplayStr(%d, %d) = %q, want %q", c.days, c.hours, got, c.want)
		}
	}
}

// ----------------------------------------------------------------------------
// xpRequiredForLevel
// ----------------------------------------------------------------------------

func TestXpRequiredForLevel(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 300}, // 300 + (1-1)*5
		{2, 305}, // 300 + (2-1)*5
		{3, 310},
		{10, 345}, // 300 + 9*5
		{50, 545},
	}
	for _, c := range cases {
		got := xpRequiredForLevel(c.level)
		if got != c.want {
			t.Errorf("xpRequiredForLevel(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

// ----------------------------------------------------------------------------
// computeLevel
// ----------------------------------------------------------------------------

func TestComputeLevel(t *testing.T) {
	cases := []struct {
		totalXP    float64
		wantLevel  int
		wantInLvl  float64
		wantForNxt int
	}{
		{0, 1, 0, 300},
		{150, 1, 150, 300},
		{300, 2, 0, 305},         // exactly level 2
		{300 + 305, 3, 0, 310},   // exactly level 3
		{300 + 151, 2, 151, 305}, // mid level 2
	}
	for _, c := range cases {
		level, inLvl, forNxt := computeLevel(c.totalXP)
		if level != c.wantLevel || inLvl != c.wantInLvl || forNxt != c.wantForNxt {
			t.Errorf("computeLevel(%.0f) = (%d, %.0f, %d), want (%d, %.0f, %d)",
				c.totalXP, level, inLvl, forNxt, c.wantLevel, c.wantInLvl, c.wantForNxt)
		}
	}
}

// ----------------------------------------------------------------------------
// nextStreakGoal / prevStreakGoal
// ----------------------------------------------------------------------------

func TestNextStreakGoal(t *testing.T) {
	cases := []struct{ days, want int }{
		{0, 3},
		{2, 3},
		{3, 7},
		{7, 14},
		{99, 100},
		{100, 150}, // beyond 100: next 50-multiple
		{150, 200},
	}
	for _, c := range cases {
		got := nextStreakGoal(c.days)
		if got != c.want {
			t.Errorf("nextStreakGoal(%d) = %d, want %d", c.days, got, c.want)
		}
	}
}

func TestPrevStreakGoal(t *testing.T) {
	cases := []struct{ days, want int }{
		{0, 0},
		{2, 0},
		{3, 3},
		{6, 3},
		{7, 7},
		{100, 100},
		{101, 100}, // beyond 100: previous 50-multiple
		{149, 100},
		{150, 150},
	}
	for _, c := range cases {
		got := prevStreakGoal(c.days)
		if got != c.want {
			t.Errorf("prevStreakGoal(%d) = %d, want %d", c.days, got, c.want)
		}
	}
}

// ----------------------------------------------------------------------------
// streakDayForDate
// ----------------------------------------------------------------------------

func TestStreakDayForDate(t *testing.T) {
	cases := []struct {
		name   string
		resets []string
		date   string
		want   int
	}{
		{"same day as reset", []string{"2026-01-20 09:00"}, "2026-01-20", 0},
		{"5 days after reset", []string{"2026-01-15 09:00"}, "2026-01-20", 5},
		{"date before all resets", []string{"2026-01-20 09:00"}, "2026-01-15", 0},
		{"uses most recent reset before date", []string{"2026-01-01 09:00", "2026-01-15 09:00"}, "2026-01-20", 5},
		{"earlier reset ignored when later reset applies", []string{"2026-01-01 09:00", "2026-01-18 09:00"}, "2026-01-20", 2},
		{"no resets", []string{}, "2026-01-20", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			game := &GameState{StreakResets: c.resets}
			got := streakDayForDate(game, c.date)
			if got != c.want {
				t.Errorf("streakDayForDate(%v, %q) = %d, want %d", c.resets, c.date, got, c.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// checkAndUnlockAchievements
// ----------------------------------------------------------------------------

func TestCheckAndUnlockAchievements(t *testing.T) {
	t.Run("no achievements unlocked below all thresholds", func(t *testing.T) {
		game := newGame("2026-01-20 09:00")
		got := checkAndUnlockAchievements(game, 2.5, 100)
		if len(got) != 0 {
			t.Errorf("expected no unlocks, got %v", got)
		}
	})

	t.Run("streak_3 unlocked at longestStreak=3", func(t *testing.T) {
		game := newGame("2026-01-20 09:00")
		got := checkAndUnlockAchievements(game, 3.0, 0)
		if !reflect.DeepEqual(got, []string{"streak_3"}) {
			t.Errorf("got %v, want [streak_3]", got)
		}
	})

	t.Run("streak_3 and streak_7 unlocked at longestStreak=7", func(t *testing.T) {
		game := newGame("2026-01-20 09:00")
		got := checkAndUnlockAchievements(game, 7.0, 0)
		if !contains(got, "streak_3") || !contains(got, "streak_7") {
			t.Errorf("got %v, want streak_3 and streak_7", got)
		}
	})

	t.Run("hours_50 unlocked at 3000 minutes (50h)", func(t *testing.T) {
		game := newGame("2026-01-20 09:00")
		got := checkAndUnlockAchievements(game, 0, 3000)
		if !contains(got, "hours_50") {
			t.Errorf("got %v, want hours_50", got)
		}
	})

	t.Run("already-unlocked achievement not returned again", func(t *testing.T) {
		game := newGame("2026-01-20 09:00")
		game.Achievements = []string{"streak_3"}
		got := checkAndUnlockAchievements(game, 5.0, 0)
		if contains(got, "streak_3") {
			t.Errorf("streak_3 should not be re-unlocked, got %v", got)
		}
	})
}

// ----------------------------------------------------------------------------
// availableConsumablesCount
// ----------------------------------------------------------------------------

func TestAvailableConsumablesCount(t *testing.T) {
	t.Run("no consumables", func(t *testing.T) {
		game := newGame("2026-01-15 09:00")
		if got := availableConsumablesCount(game, "hobby_10min"); got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})

	t.Run("one awarded, not consumed", func(t *testing.T) {
		game := newGame("2026-01-15 09:00")
		game.Consumables = []GameConsumableEntry{
			{ID: "hobby_10min", AwardedDate: "2026-01-20", ConsumedAt: ""},
		}
		if got := availableConsumablesCount(game, "hobby_10min"); got != 1 {
			t.Errorf("got %d, want 1", got)
		}
	})

	t.Run("two awarded, one consumed", func(t *testing.T) {
		game := newGame("2026-01-10 09:00")
		game.Consumables = []GameConsumableEntry{
			{ID: "hobby_10min", AwardedDate: "2026-01-15", ConsumedAt: "2026-01-16 10:00"},
			{ID: "hobby_10min", AwardedDate: "2026-01-20", ConsumedAt: ""},
		}
		if got := availableConsumablesCount(game, "hobby_10min"); got != 1 {
			t.Errorf("got %d, want 1", got)
		}
	})

	t.Run("different ID not counted", func(t *testing.T) {
		game := newGame("2026-01-15 09:00")
		game.Consumables = []GameConsumableEntry{
			{ID: "other_type", AwardedDate: "2026-01-20", ConsumedAt: ""},
		}
		if got := availableConsumablesCount(game, "hobby_10min"); got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})
}

// ----------------------------------------------------------------------------
// totalXPFromGame
// ----------------------------------------------------------------------------

func TestTotalXPFromGame(t *testing.T) {
	t.Run("no work log, no timer", func(t *testing.T) {
		game := newGame("2026-01-20 09:00")
		got := totalXPFromGame(game, nil)
		if got != 0 {
			t.Errorf("got %.2f, want 0", got)
		}
	})

	t.Run("single entry at streak day 0 (1.0x)", func(t *testing.T) {
		game := newGame("2026-01-20 09:00")
		game.WorkLog = []GameWorkLogEntry{
			{Date: "2026-01-20", Minutes: 60}, // reset day → day 0
		}
		got := totalXPFromGame(game, nil)
		want := 60.0 * 1.0
		if got != want {
			t.Errorf("got %.2f, want %.2f", got, want)
		}
	})

	t.Run("single entry at streak day 5 (1.05x)", func(t *testing.T) {
		game := newGame("2026-01-15 09:00") // reset Jan 15 → Jan 20 is day 5
		game.WorkLog = []GameWorkLogEntry{
			{Date: "2026-01-20", Minutes: 60},
		}
		got := totalXPFromGame(game, nil)
		want := 60.0 * 1.05
		if got != want {
			t.Errorf("got %.2f, want %.2f", got, want)
		}
	})

	t.Run("multiple entries sum correctly", func(t *testing.T) {
		game := newGame("2026-01-15 09:00") // Jan 20 = day 5, Jan 21 = day 6
		game.WorkLog = []GameWorkLogEntry{
			{Date: "2026-01-20", Minutes: 60}, // 60 * 1.05 = 63
			{Date: "2026-01-21", Minutes: 30}, // 30 * 1.06 = 31.8
		}
		got := totalXPFromGame(game, nil)
		want := 60.0*1.05 + 30.0*1.06
		if got != want {
			t.Errorf("got %.4f, want %.4f", got, want)
		}
	})
}

// ----------------------------------------------------------------------------
// applySessionToGame
// ----------------------------------------------------------------------------

func TestApplySessionToGame(t *testing.T) {
	t.Run("creates new work log entry", func(t *testing.T) {
		game := newGame("2026-01-15 09:00")
		dayStart := mustTime("2026-01-20 09:00") // streak day 5
		applySessionToGame(game, 60, dayStart)

		if len(game.WorkLog) != 1 {
			t.Fatalf("expected 1 work log entry, got %d", len(game.WorkLog))
		}
		entry := game.WorkLog[0]
		if entry.Date != "2026-01-20" {
			t.Errorf("date = %q, want 2026-01-20", entry.Date)
		}
		if entry.Minutes != 60 {
			t.Errorf("minutes = %d, want 60", entry.Minutes)
		}
		if entry.StreakDay != 0 {
			t.Errorf("streak_day should not be set, got %d", entry.StreakDay)
		}
	})

	t.Run("upserts existing entry for same date", func(t *testing.T) {
		game := newGame("2026-01-15 09:00")
		game.WorkLog = []GameWorkLogEntry{
			{Date: "2026-01-20", Minutes: 40},
		}
		dayStart := mustTime("2026-01-20 14:00")
		applySessionToGame(game, 30, dayStart)

		if len(game.WorkLog) != 1 {
			t.Fatalf("expected 1 work log entry after upsert, got %d", len(game.WorkLog))
		}
		if game.WorkLog[0].Minutes != 70 {
			t.Errorf("minutes after upsert = %d, want 70", game.WorkLog[0].Minutes)
		}
	})

	t.Run("updates longest streak", func(t *testing.T) {
		game := newGame("2026-01-15 09:00")
		dayStart := mustTime("2026-01-20 09:00") // day 5
		applySessionToGame(game, 30, dayStart)

		// 5 full days + some fraction of hours
		if game.LongestStreak < 5.0 {
			t.Errorf("LongestStreak = %.2f, want >= 5.0", game.LongestStreak)
		}
	})

	t.Run("does not decrease longest streak on repeated call", func(t *testing.T) {
		game := newGame("2026-01-01 09:00")
		// First session on day 10
		applySessionToGame(game, 30, mustTime("2026-01-11 09:00"))
		first := game.LongestStreak

		// Second session on day 2 (lower streak)
		applySessionToGame(game, 30, mustTime("2026-01-03 09:00"))
		if game.LongestStreak != first {
			t.Errorf("LongestStreak decreased from %.2f to %.2f", first, game.LongestStreak)
		}
	})

	t.Run("unlocks achievement and populates NewAchievements", func(t *testing.T) {
		game := newGame("2026-01-17 09:00")
		dayStart := mustTime("2026-01-20 09:00") // streak day 3 → streak_3
		newAchs := applySessionToGame(game, 30, dayStart)

		if !contains(newAchs, "streak_3") {
			t.Errorf("returned achievements %v, want streak_3", newAchs)
		}
		if !contains(game.Achievements, "streak_3") {
			t.Errorf("game.Achievements %v, want streak_3", game.Achievements)
		}
		if !contains(game.NewAchievements, "streak_3") {
			t.Errorf("game.NewAchievements %v, want streak_3", game.NewAchievements)
		}
	})

	t.Run("does not re-unlock already-held achievement", func(t *testing.T) {
		game := newGame("2026-01-17 09:00")
		game.Achievements = []string{"streak_3"}
		newAchs := applySessionToGame(game, 30, mustTime("2026-01-20 09:00"))

		if contains(newAchs, "streak_3") {
			t.Errorf("streak_3 should not be re-unlocked, got %v", newAchs)
		}
	})

	t.Run("consumable awarded on first session of a milestone streak day", func(t *testing.T) {
		game := newGame("2026-01-17 09:00") // Jan 20 = day 3 (StreakEvery=3)
		applySessionToGame(game, 60, mustTime("2026-01-20 09:00"))

		if got := availableConsumablesCount(game, "hobby_10min"); got != 1 {
			t.Errorf("available consumables = %d, want 1", got)
		}
		if game.Consumables[0].AwardedDate != "2026-01-20" {
			t.Errorf("AwardedDate = %q, want 2026-01-20", game.Consumables[0].AwardedDate)
		}
	})

	t.Run("consumable not awarded on non-milestone streak day", func(t *testing.T) {
		game := newGame("2026-01-18 09:00") // Jan 20 = day 2 (not a multiple of 3)
		applySessionToGame(game, 60, mustTime("2026-01-20 09:00"))

		if got := availableConsumablesCount(game, "hobby_10min"); got != 0 {
			t.Errorf("available consumables = %d, want 0", got)
		}
	})

	t.Run("consumable awarded only once per day (second session same day)", func(t *testing.T) {
		game := newGame("2026-01-17 09:00") // Jan 20 = day 3 (StreakEvery=3)
		applySessionToGame(game, 30, mustTime("2026-01-20 09:00"))
		applySessionToGame(game, 30, mustTime("2026-01-20 14:00")) // second session same day

		if got := availableConsumablesCount(game, "hobby_10min"); got != 1 {
			t.Errorf("available consumables = %d, want 1 (no double award)", got)
		}
	})

	t.Run("hours achievement unlocked when total work crosses threshold", func(t *testing.T) {
		game := newGame("2026-01-01 09:00")
		// 49h already logged
		game.WorkLog = []GameWorkLogEntry{
			{Date: "2026-01-10", Minutes: 49 * 60},
		}
		// Add 61 more min → crosses 50h
		newAchs := applySessionToGame(game, 61, mustTime("2026-01-11 09:00"))
		if !contains(newAchs, "hours_50") {
			t.Errorf("got %v, want hours_50", newAchs)
		}
	})
}

// ----------------------------------------------------------------------------
// minutesToDayHourMinuteStr
// ----------------------------------------------------------------------------

func TestMinutesToDayHourMinuteStr(t *testing.T) {
	cases := []struct {
		mins int
		want string
	}{
		{0, "0h 0m"},
		{30, "0h 30m"},
		{60, "1h 0m"},
		{90, "1h 30m"},
		{24 * 60, "1d 0h 0m"},
		{25*60 + 30, "1d 1h 30m"},
	}
	for _, c := range cases {
		got := minutesToDayHourMinuteStr(c.mins)
		if got != c.want {
			t.Errorf("minutesToDayHourMinuteStr(%d) = %q, want %q", c.mins, got, c.want)
		}
	}
}

// ----------------------------------------------------------------------------
// generateDailyQuest
// ----------------------------------------------------------------------------

func TestGenerateDailyQuest(t *testing.T) {
	t.Run("deterministic for same date", func(t *testing.T) {
		q1 := generateDailyQuest("2026-05-05")
		q2 := generateDailyQuest("2026-05-05")
		if q1 != q2 {
			t.Errorf("same date produced different quests: %+v vs %+v", q1, q2)
		}
	})

	t.Run("different dates produce different quests", func(t *testing.T) {
		// Over 10 days at least one should differ (probabilistic but near-certain)
		same := 0
		base := generateDailyQuest("2026-05-01")
		for d := 2; d <= 10; d++ {
			q := generateDailyQuest(fmt.Sprintf("2026-05-%02d", d))
			if q == base {
				same++
			}
		}
		if same == 9 {
			t.Error("all 10 days produced identical quests — seed not working")
		}
	})

	t.Run("values match defined quests", func(t *testing.T) {
		// Build a set of valid quest configs
		validQuests := make(map[string]bool)
		for _, def := range allQuestDefs {
			key := fmt.Sprintf("%s_%d_%d_%.2f", def.Type, def.TargetMins, def.DeadlineMins, def.RewardPct)
			validQuests[key] = true
		}
		for d := 1; d <= 30; d++ {
			q := generateDailyQuest(fmt.Sprintf("2026-06-%02d", d))
			key := fmt.Sprintf("%s_%d_%d_%.2f", q.Type, q.TargetMins, q.DeadlineMins, q.RewardPct)
			if !validQuests[key] {
				t.Errorf("day %d generated quest not in allQuestDefs: %+v", d, q)
			}
		}
	})
}

// ----------------------------------------------------------------------------
// questProgress
// ----------------------------------------------------------------------------

func TestQuestProgress(t *testing.T) {
	t.Run("long_cycle no timer", func(t *testing.T) {
		q := DailyQuest{Type: QuestTypeLongCycle, TargetMins: 90, RewardPct: 0.10}
		game := newGame("2026-05-01 09:00")
		current, target, completed := questProgress(q, nil, game)
		if current != 0 || target != 90 || completed {
			t.Errorf("got %d/%d completed=%v, want 0/90 false", current, target, completed)
		}
	})

	t.Run("long_cycle with completed cycles", func(t *testing.T) {
		q := DailyQuest{Type: QuestTypeLongCycle, TargetMins: 90, RewardPct: 0.10}
		game := newGame("2026-05-05 09:00")
		timer := &Timer{
			Status:   StatusStopped,
			DayStart: "2026-05-05 09:00",
			Timeline: []TimelineEntry{
				{Type: "work", Minutes: 45, PausedMinutes: 0},
				{Type: "break", Minutes: 10},
				{Type: "work", Minutes: 95, PausedMinutes: 5},
			},
		}
		current, target, completed := questProgress(q, timer, game)
		if current != 95 || target != 90 || !completed {
			t.Errorf("got %d/%d completed=%v, want 95/90 true", current, target, completed)
		}
	})

	t.Run("accumulate with timer and log", func(t *testing.T) {
		q := DailyQuest{Type: QuestTypeAccumulate, TargetMins: 300, RewardPct: 0.10}
		game := newGame("2026-05-01 09:00")
		game.WorkLog = []GameWorkLogEntry{{Date: "2026-05-05", Minutes: 120}}
		os.Setenv("WT_MOCK_TIME", "2026-05-05 14:00")
		defer os.Unsetenv("WT_MOCK_TIME")

		timer := &Timer{
			Status:   StatusStopped,
			DayStart: "2026-05-05 09:00",
			Timeline: []TimelineEntry{
				{Type: "work", Minutes: 180},
			},
		}
		current, target, completed := questProgress(q, timer, game)
		// 180 from timer + 120 from log = 300
		if current != 300 || target != 300 || !completed {
			t.Errorf("got %d/%d completed=%v, want 300/300 true", current, target, completed)
		}
	})

	t.Run("time_gated before deadline", func(t *testing.T) {
		q := DailyQuest{Type: QuestTypeTimeGated, TargetMins: 120, DeadlineMins: 690, RewardPct: 0.10}
		game := newGame("2026-05-01 09:00")
		os.Setenv("WT_MOCK_TIME", "2026-05-05 10:30")
		defer os.Unsetenv("WT_MOCK_TIME")

		timer := &Timer{
			Status:   StatusRunning,
			DayStart: "2026-05-05 08:00",
			Timeline: []TimelineEntry{
				{Type: "work", Minutes: 60},
				{Type: "break", Minutes: 10},
			},
			PausedMinutes: 0,
		}
		current, _, completed := questProgress(q, timer, game)
		// 60 from timeline + current running cycle (started at 08:00+60+10=09:10, now 10:30 = 80 min)
		// total = 60 + 80 = 140 >= 120 → completed
		if current < 120 || !completed {
			t.Errorf("got current %d completed=%v, expected >=120 and true", current, completed)
		}
	})
}

// ----------------------------------------------------------------------------
// isQuestCompletedToday
// ----------------------------------------------------------------------------

func TestIsQuestCompletedToday(t *testing.T) {
	game := newGame("2026-05-01 09:00")
	game.CompletedQuests = []CompletedQuestEntry{
		{Date: "2026-05-03", QuestType: QuestTypeLongCycle, XPAwarded: 30},
	}

	if !isQuestCompletedToday(game, "2026-05-03") {
		t.Error("expected true for completed date")
	}
	if isQuestCompletedToday(game, "2026-05-04") {
		t.Error("expected false for different date")
	}
}

// ----------------------------------------------------------------------------
// totalXP includes quest bonus
// ----------------------------------------------------------------------------

func TestTotalXPIncludesQuestBonus(t *testing.T) {
	game := newGame("2026-01-01 09:00")
	game.WorkLog = []GameWorkLogEntry{{Date: "2026-01-01", Minutes: 100}}
	game.CompletedQuests = []CompletedQuestEntry{
		{Date: "2026-01-01", QuestType: QuestTypeAccumulate, XPAwarded: 40},
	}
	got := totalXPFromGame(game, nil)
	// 100 min × 1.00 multiplier (day 0) + 40 quest XP = 140
	want := 140.0
	if got != want {
		t.Errorf("totalXPFromGame = %.1f, want %.1f", got, want)
	}
}

// ----------------------------------------------------------------------------
// questDescription
// ----------------------------------------------------------------------------

func TestQuestDescription(t *testing.T) {
	cases := []struct {
		quest DailyQuest
		want  string
	}{
		{DailyQuest{Type: QuestTypeLongCycle, TargetMins: 90}, "Complete one work cycle of 1h:30m+"},
		{DailyQuest{Type: QuestTypeAccumulate, TargetMins: 360}, "Accumulate 6h:00m worked today"},
		{DailyQuest{Type: QuestTypeTimeGated, TargetMins: 150, DeadlineMins: 690}, "Complete 2h:30m work before 11:30"},
	}
	for _, c := range cases {
		got := questDescription(c.quest)
		if got != c.want {
			t.Errorf("questDescription(%+v) = %q, want %q", c.quest, got, c.want)
		}
	}
}

// ----------------------------------------------------------------------------
// questRewardXP
// ----------------------------------------------------------------------------

func TestQuestRewardXP(t *testing.T) {
	t.Run("level 1 with 10% reward", func(t *testing.T) {
		q := DailyQuest{Type: QuestTypeLongCycle, TargetMins: 60, RewardPct: 0.10}
		// Level 1 needs 300 XP → 10% = 30 XP
		got := questRewardXP(q, 0)
		if got != 30 {
			t.Errorf("got %d, want 30", got)
		}
	})

	t.Run("level 1 with 8% reward", func(t *testing.T) {
		q := DailyQuest{Type: QuestTypeLongCycle, TargetMins: 60, RewardPct: 0.08}
		// Level 1 needs 300 XP → 8% = 24 → rounded to 20
		got := questRewardXP(q, 0)
		if got != 20 {
			t.Errorf("got %d, want 20", got)
		}
	})

	t.Run("higher level scales reward", func(t *testing.T) {
		q := DailyQuest{Type: QuestTypeAccumulate, TargetMins: 300, RewardPct: 0.10}
		// At 600 XP total: level 2 (300 needed for lvl1), level 2 needs 302 XP → 10% = 30.2 → 30
		got := questRewardXP(q, 600)
		if got != 30 {
			t.Errorf("got %d, want 30", got)
		}
	})

	t.Run("minimum 10 xp", func(t *testing.T) {
		q := DailyQuest{Type: QuestTypeLongCycle, TargetMins: 60, RewardPct: 0.01}
		// Level 1 needs 300 XP → 1% = 3 → clamped to 10
		got := questRewardXP(q, 0)
		if got != 10 {
			t.Errorf("got %d, want 10", got)
		}
	})
}

// ----------------------------------------------------------------------------
// calculateChain
// ----------------------------------------------------------------------------

func TestCalculateChain(t *testing.T) {
	t.Run("no timeline", func(t *testing.T) {
		timer := &Timer{Status: StatusStopped, DayStart: "2026-05-05 09:00"}
		got := calculateChain(timer)
		if got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})

	t.Run("single 30min cycle", func(t *testing.T) {
		timer := &Timer{
			Status:   StatusStopped,
			DayStart: "2026-05-05 09:00",
			Timeline: []TimelineEntry{
				{Type: "work", Minutes: 30},
			},
		}
		got := calculateChain(timer)
		if got != 1 {
			t.Errorf("got %d, want 1", got)
		}
	})

	t.Run("60min cycle gives 2", func(t *testing.T) {
		timer := &Timer{
			Status:   StatusStopped,
			DayStart: "2026-05-05 09:00",
			Timeline: []TimelineEntry{
				{Type: "work", Minutes: 62},
			},
		}
		got := calculateChain(timer)
		if got != 2 {
			t.Errorf("got %d, want 2", got)
		}
	})

	t.Run("two 30min cycles with break", func(t *testing.T) {
		timer := &Timer{
			Status:   StatusStopped,
			DayStart: "2026-05-05 09:00",
			Timeline: []TimelineEntry{
				{Type: "work", Minutes: 30},
				{Type: "break", Minutes: 10},
				{Type: "work", Minutes: 35},
			},
		}
		got := calculateChain(timer)
		if got != 2 {
			t.Errorf("got %d, want 2", got)
		}
	})

	t.Run("short cycle breaks chain", func(t *testing.T) {
		timer := &Timer{
			Status:   StatusStopped,
			DayStart: "2026-05-05 09:00",
			Timeline: []TimelineEntry{
				{Type: "work", Minutes: 45},
				{Type: "break", Minutes: 10},
				{Type: "work", Minutes: 20}, // breaks chain
				{Type: "break", Minutes: 5},
				{Type: "work", Minutes: 30},
			},
		}
		got := calculateChain(timer)
		if got != 1 {
			t.Errorf("got %d, want 1 (chain reset by 20min cycle)", got)
		}
	})

	t.Run("short cycle at start then recovers", func(t *testing.T) {
		timer := &Timer{
			Status:   StatusStopped,
			DayStart: "2026-05-05 09:00",
			Timeline: []TimelineEntry{
				{Type: "work", Minutes: 15}, // breaks chain (resets to 0)
				{Type: "break", Minutes: 5},
				{Type: "work", Minutes: 60},
				{Type: "break", Minutes: 10},
				{Type: "work", Minutes: 30},
			},
		}
		got := calculateChain(timer)
		if got != 3 {
			t.Errorf("got %d, want 3 (60/30=2 + 30/30=1)", got)
		}
	})

	t.Run("running cycle adds to chain", func(t *testing.T) {
		os.Setenv("WT_MOCK_TIME", "2026-05-05 10:05")
		defer os.Unsetenv("WT_MOCK_TIME")

		timer := &Timer{
			Status:   StatusRunning,
			DayStart: "2026-05-05 09:00",
			Timeline: []TimelineEntry{
				{Type: "work", Minutes: 30},
				{Type: "break", Minutes: 5},
			},
			PausedMinutes: 0,
		}
		// Current cycle started at 09:00+30+5=09:35, now 10:05 = 30 min
		got := calculateChain(timer)
		if got != 2 {
			t.Errorf("got %d, want 2 (1 from timeline + 1 from current 30min)", got)
		}
	})

	t.Run("25min cycle gives 0", func(t *testing.T) {
		timer := &Timer{
			Status:   StatusStopped,
			DayStart: "2026-05-05 09:00",
			Timeline: []TimelineEntry{
				{Type: "work", Minutes: 25},
			},
		}
		got := calculateChain(timer)
		if got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
