package viffui

import (
	"testing"
	"time"
)

// TestAgoClock lays out the timestamps the log list shows in both clock
// formats. The 15:04 layout is the default and must be unchanged; "3:04 PM" is
// what the Time format setting selects.
func TestAgoClock(t *testing.T) {
	loc := time.FixedZone("test", 8*60*60)
	now := time.Date(2026, time.November, 7, 15, 0, 0, 0, loc)
	cases := []struct {
		name string
		when time.Time
		hour string
		want string
	}{
		{"afternoon 24-hour", time.Date(2026, time.November, 7, 12, 51, 0, 0, loc), "15:04", "Today 12:51"},
		{"afternoon 12-hour", time.Date(2026, time.November, 7, 12, 51, 0, 0, loc), "3:04 PM", "Today 12:51 PM"},
		{"morning 24-hour", time.Date(2026, time.November, 7, 4, 51, 0, 0, loc), "15:04", "Today 04:51"},
		{"morning 12-hour", time.Date(2026, time.November, 7, 4, 51, 0, 0, loc), "3:04 PM", "Today 4:51 AM"},
		{"yesterday 12-hour", time.Date(2026, time.November, 6, 16, 21, 0, 0, loc), "3:04 PM", "Yesterday 4:21 PM"},
		{"earlier this year 12-hour", time.Date(2026, time.November, 4, 20, 43, 0, 0, loc), "3:04 PM", "Nov 4, 8:43 PM"},
		{"earlier this year 24-hour", time.Date(2026, time.November, 4, 20, 43, 0, 0, loc), "15:04", "Nov 4, 20:43"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ago(tc.when, now, tc.hour); got != tc.want {
				t.Errorf("ago = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestAgoComparesInOneZone is the regression for the SVN time bug: a commit
// timestamp that still carries UTC must be bucketed by the day the user lives
// in, not by its own zone. Before the fix this rendered as an absolute date in
// another zone ("Nov 6, 20:51") instead of "Today".
func TestAgoComparesInOneZone(t *testing.T) {
	loc := time.FixedZone("test", 8*60*60)
	now := time.Date(2026, time.November, 7, 15, 0, 0, 0, loc)
	// 04:51 on Nov 7 in UTC+8, which is 20:51 on Nov 6 in UTC.
	morning := time.Date(2026, time.November, 7, 4, 51, 0, 0, loc).UTC()
	if got, want := ago(morning, now, "15:04"), "Today 04:51"; got != want {
		t.Errorf("ago(UTC timestamp) = %q, want %q", got, want)
	}
}
