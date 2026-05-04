package main

import (
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
		{"next calendar day", "2026-01-20 09:00", "2026-01-21 08:00", 1},
		{"5 days later", "2026-01-15 09:00", "2026-01-20 09:00", 5},
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
// xpRequiredForLevel
// ----------------------------------------------------------------------------

func TestXpRequiredForLevel(t *testing.T) {
	cases := []struct {
		level int
		want  int
	}{
		{1, 300}, // 300 + (1-1)*2
		{2, 302}, // 300 + (2-1)*2
		{3, 304},
		{10, 318}, // 300 + 9*2
		{50, 398},
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
		{300, 2, 0, 302},         // exactly level 2
		{300 + 302, 3, 0, 304},   // exactly level 3
		{300 + 151, 2, 151, 302}, // mid level 2
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
