package main

import (
	"os"
	"reflect"
	"strings"
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
	}
}

func TestGameMinimalDisplay(t *testing.T) {
	t.Setenv("WT_MOCK_TIME", "2026-08-06 12:00")
	flexPath := t.TempDir() + "/Flex.md"
	if err := os.WriteFile(flexPath, []byte("1.5h\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WT_FLEX_FILE", flexPath)

	game := newGame("2026-08-01 08:00")
	game.LongestStreak = 12
	game.NewAchievements = []string{"streak_3"}
	timer := &Timer{
		Status:   StatusRunning,
		DayStart: "2026-08-06 08:15",
	}

	got := gameMinimalDisplay(game, timer)
	for _, want := range []string{
		"=== Status ===",
		"Streak:",
		"milestone progress",
		"Best streak: 12.0 days",
		"Current Session",
		"Today",
		"Finish ETA:",
		"Break time:",
		"Flex:",
		"+1.5h",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("minimal output missing %q:\n%s", want, got)
		}
	}

	for _, unwanted := range []string{
		"XP",
		"xp",
		"⚔️",
		"Work Timer RPG",
		"LVL",
		"save this streak",
		"Chain:",
		"Net earnings",
		"Quest",
		"Total today",
		"ACHIEVEMENT",
	} {
		if strings.Contains(got, unwanted) {
			t.Errorf("minimal output unexpectedly contains %q:\n%s", unwanted, got)
		}
	}
}

func TestGameOverviewUsesOnlyStreakXP(t *testing.T) {
	t.Setenv("WT_MOCK_TIME", "2026-02-24 08:00")
	t.Setenv("WT_FLEX_FILE", t.TempDir()+"/Flex.md")

	game := &GameState{StreakResets: []string{
		"2026-01-01 08:00",
		"2026-01-20 07:00",
		"2026-02-14 08:00",
	}}
	got := gameOverviewDisplay(game, nil)

	for _, want := range []string{"LVL 8", "4 / 8 xp", "4 xp remaining", "+1xp"} {
		if !strings.Contains(got, want) {
			t.Errorf("overview missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"×", "⚔️", "🔥", "Chain:", "Current Session", "Net earnings", "Quest", "Total today", "save this streak"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("overview unexpectedly contains %q:\n%s", unwanted, got)
		}
	}
}

func TestGameOverviewShowsCurrentStreakXP(t *testing.T) {
	t.Setenv("WT_MOCK_TIME", "2026-02-24 08:00")
	t.Setenv("WT_FLEX_FILE", t.TempDir()+"/Flex.md")

	game := newGame("2026-02-08 03:00")
	full := gameOverviewDisplay(game, nil)
	minimal := gameMinimalDisplay(game, nil)

	if !strings.Contains(full, "Streak: 16.2 days") || !strings.Contains(full, colorBold+colorGreen+"+7xp"+colorReset) {
		t.Errorf("overview missing current streak XP:\n%s", full)
	}
	if strings.Contains(minimal, "+7xp") {
		t.Errorf("minimal output unexpectedly contains current streak XP:\n%s", minimal)
	}
}

func TestGameOverviewDiffersFromMinimalOnlyByLevelAndStreakXP(t *testing.T) {
	t.Setenv("WT_MOCK_TIME", "2026-02-24 08:00")
	t.Setenv("WT_FLEX_FILE", t.TempDir()+"/Flex.md")

	game := &GameState{StreakResets: []string{
		"2026-01-01 08:00",
		"2026-01-20 07:00",
		"2026-02-14 08:00",
	}}
	timer := &Timer{
		Status:   StatusRunning,
		DayStart: "2026-02-24 07:30",
	}
	full := gameOverviewDisplay(game, timer)
	minimal := gameMinimalDisplay(game, timer)

	lines := strings.Split(full, "\n")
	for i, line := range lines {
		if strings.Contains(line, "LVL ") {
			lines = append(lines[:i], lines[i+2:]...)
			break
		}
	}
	for i, line := range lines {
		if strings.Contains(line, "Streak:") {
			lines[i] = strings.Replace(line, "  "+colorBold+colorGreen+"+1xp"+colorReset, "", 1)
			break
		}
	}
	if got := strings.Join(lines, "\n"); got != minimal {
		t.Errorf("overview differs from minimal outside the level row and streak XP:\n%s", got)
	}
}

func TestGameDisplaysMinuteOnlyFinishETADiff(t *testing.T) {
	t.Setenv("WT_MOCK_TIME", "2026-08-06 10:00")
	t.Setenv("WT_FLEX_FILE", t.TempDir()+"/Flex.md")

	game := newGame("2026-08-01 08:00")
	timer := &Timer{
		Status:   StatusRunning,
		DayStart: "2026-08-06 08:15",
	}

	displays := map[string]func(*GameState, *Timer) string{
		"game":         gameOverviewDisplay,
		"game minimal": gameMinimalDisplay,
	}
	for name, display := range displays {
		t.Run(name, func(t *testing.T) {
			got := display(game, timer)
			if !strings.Contains(got, "-20m") {
				t.Errorf("output missing minute-only ETA diff %q:\n%s", "-20m", got)
			}
			if strings.Contains(got, "-0h 20m") {
				t.Errorf("output contains zero-hour ETA diff:\n%s", got)
			}
		})
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
// streakXPForDays
// ----------------------------------------------------------------------------

func TestStreakXPForDays(t *testing.T) {
	cases := []struct {
		days, want int
	}{
		{9, 0},
		{10, 1},
		{19, 10},
		{20, 12},
		{29, 30},
		{30, 33},
		{43, 76},
	}
	for _, c := range cases {
		got := streakXPForDays(c.days)
		if got != c.want {
			t.Errorf("streakXPForDays(%d) = %d, want %d", c.days, got, c.want)
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
		{1, 1},
		{2, 2},
		{3, 3},
		{10, 10},
		{50, 50},
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
		totalXP, wantLevel, wantInLvl, wantForNxt int
	}{
		{0, 1, 0, 1},
		{1, 2, 0, 2},
		{3, 3, 0, 3},
		{6, 4, 0, 4},
		{69, 12, 3, 12},
	}
	for _, c := range cases {
		level, inLvl, forNxt := computeLevel(c.totalXP)
		if level != c.wantLevel || inLvl != c.wantInLvl || forNxt != c.wantForNxt {
			t.Errorf("computeLevel(%d) = (%d, %d, %d), want (%d, %d, %d)",
				c.totalXP, level, inLvl, forNxt, c.wantLevel, c.wantInLvl, c.wantForNxt)
		}
	}
}

func TestTotalStreakXP(t *testing.T) {
	game := &GameState{StreakResets: []string{
		"2026-01-01 08:00",
		"2026-01-20 07:00", // 18 complete days, not 19
		"2026-02-14 08:00", // 25 complete days
	}}

	if got := totalStreakXP(game, mustTime("2026-02-24 07:00")); got != 31 {
		t.Errorf("totalStreakXP() = %d, want 31", got)
	}
	if got := totalStreakXP(game, mustTime("2026-02-24 08:00")); got != 32 {
		t.Errorf("totalStreakXP() = %d, want 32", got)
	}
}

func TestValidateStreakResets(t *testing.T) {
	for _, resets := range [][]string{
		{"not-a-timestamp"},
		{"2026-01-02 08:00", "2026-01-01 08:00"},
	} {
		if err := validateStreakResets(&GameState{StreakResets: resets}); err == nil {
			t.Errorf("validateStreakResets(%v) returned nil error", resets)
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

func TestFormatNormDiffDuration(t *testing.T) {
	cases := []struct {
		mins int
		want string
	}{
		{0, "0m"},
		{26, "26m"},
		{59, "59m"},
		{60, "1h 0m"},
		{90, "1h 30m"},
	}
	for _, c := range cases {
		got := formatNormDiffDuration(c.mins)
		if got != c.want {
			t.Errorf("formatNormDiffDuration(%d) = %q, want %q", c.mins, got, c.want)
		}
	}
}

func TestFormatNormDiff(t *testing.T) {
	cases := []struct {
		diff int
		want string
	}{
		{0, "-0m"},
		{26, "-26m"},
		{59, "-59m"},
		{60, "-1h 0m"},
		{95, "-1h 35m"},
		{-26, "+26m"},
		{-59, "+59m"},
		{-60, "+1h 0m"},
		{-125, "+2h 5m"},
	}
	for _, c := range cases {
		got := formatNormDiff(c.diff)
		if got != c.want {
			t.Errorf("formatNormDiff(%d) = %q, want %q", c.diff, got, c.want)
		}
	}
}

func TestRefActivityAtOffset(t *testing.T) {
	tests := []struct {
		name   string
		offset int
		want   string
	}{
		{"before start", -1, activityNone},
		{"day start is work", 0, activityWork},
		{"mid first work block", 30, activityWork},
		{"end of first work block still work", 44, activityWork},
		{"start of first break", 45, activityBreak},
		{"mid first break", 55, activityBreak},
		{"end of first break still break", 64, activityBreak},
		{"second work block", 65, activityWork},
		{"just before finish is work", 494, activityWork},
		{"finish offset is work", 495, activityWork},
		{"beyond finish extends work", 600, activityWork},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := refActivityAtOffset(tt.offset)
			if got != tt.want {
				t.Errorf("refActivityAtOffset(%d) = %q, want %q", tt.offset, got, tt.want)
			}
		})
	}
}

func TestBuildActualActivity(t *testing.T) {
	t.Run("on-time start with work, pause, break", func(t *testing.T) {
		// Day starts at the anchor (08:15), so anchorToDayStart = 0.
		// Timeline: 30m work + 10m paused, then 15m break, then 20m work.
		timer := &Timer{
			Status:   StatusStopped,
			DayStart: "2026-05-05 08:15",
			Timeline: []TimelineEntry{
				{Type: "work", Minutes: 30, PausedMinutes: 10},
				{Type: "break", Minutes: 15},
				{Type: "work", Minutes: 20},
			},
		}
		activity := buildActualActivity(timer, 0, 0, normSpanMins)

		if len(activity) != normSpanMins {
			t.Fatalf("len = %d, want %d", len(activity), normSpanMins)
		}
		// 30m work + 10m pause: [0,15) work, [15,25) pause, [25,40) work,
		// [40,55) break, [55,75) work, then none.
		checks := map[int]string{
			0:   activityWork,
			14:  activityWork,
			15:  activityPause,
			24:  activityPause,
			25:  activityWork,
			39:  activityWork,
			40:  activityBreak,
			54:  activityBreak,
			55:  activityWork,
			74:  activityWork,
			75:  activityNone,
			200: activityNone,
		}
		for idx, want := range checks {
			if activity[idx] != want {
				t.Errorf("activity[%d] = %q, want %q", idx, activity[idx], want)
			}
		}
	})

	t.Run("late start leaves pre-start as none", func(t *testing.T) {
		// Day starts at 08:45 -> anchorToDayStart = 30.
		timer := &Timer{
			Status:   StatusStopped,
			DayStart: "2026-05-05 08:45",
			Timeline: []TimelineEntry{
				{Type: "work", Minutes: 20},
			},
		}
		activity := buildActualActivity(timer, 30, 0, normSpanMins)
		// Minutes 0..29 (08:15-08:45) are before day start -> none.
		for i := 0; i < 30; i++ {
			if activity[i] != activityNone {
				t.Errorf("activity[%d] = %q, want none (pre-start)", i, activity[i])
			}
		}
		// Minutes 30..49 are the 20m work block.
		if activity[30] != activityWork || activity[49] != activityWork {
			t.Errorf("expected work at anchor offsets 30 and 49")
		}
		if activity[50] != activityNone {
			t.Errorf("activity[50] = %q, want none", activity[50])
		}
	})

	t.Run("running cycle fills work then pause up to now", func(t *testing.T) {
		os.Setenv("WT_MOCK_TIME", "2026-05-05 09:15")
		defer os.Unsetenv("WT_MOCK_TIME")

		// Day start 08:15. One completed 30m work cycle, then a running cycle
		// that started at offset 30. Now is 60 min in. 10 min paused so far,
		// so 20m work split around 10m pause in the middle.
		timer := &Timer{
			Status:   StatusRunning,
			DayStart: "2026-05-05 08:15",
			Timeline: []TimelineEntry{
				{Type: "work", Minutes: 30},
			},
			PausedMinutes: 10,
		}
		activity := buildActualActivity(timer, 0, 0, normSpanMins)
		// [0,30) work (completed), [30,40) work, [40,50) pause, [50,60) work.
		checks := map[int]string{
			0:  activityWork,
			29: activityWork,
			30: activityWork,
			39: activityWork,
			40: activityPause,
			49: activityPause,
			50: activityWork,
			59: activityWork,
			60: activityNone,
		}
		for idx, want := range checks {
			if activity[idx] != want {
				t.Errorf("activity[%d] = %q, want %q", idx, activity[idx], want)
			}
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
