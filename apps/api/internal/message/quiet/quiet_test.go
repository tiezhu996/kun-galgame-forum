package quiet

import (
	"testing"
	"time"
)

func loc(t *testing.T) *time.Location {
	t.Helper()
	l, err := time.LoadLocation(ScheduleTZ)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestLastEndedWindow(t *testing.T) {
	sh := loc(t)
	cases := []struct {
		name            string
		at              time.Time
		endDay, endHour int
	}{
		{"at eight", time.Date(2026, 10, 28, 8, 0, 0, 0, sh), 28, 8},
		{"five past eight", time.Date(2026, 10, 28, 8, 5, 0, 0, sh), 28, 8},
		{"before eight", time.Date(2026, 10, 28, 7, 59, 0, 0, sh), 27, 8},
		{"noon", time.Date(2026, 10, 28, 12, 0, 0, 0, sh), 28, 8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end := LastEndedWindow(c.at)
			wantEnd := time.Date(2026, 10, c.endDay, c.endHour, 0, 0, 0, sh)
			if !end.Equal(wantEnd) || end.Sub(start) != WindowHours*time.Hour {
				t.Fatalf("window %v..%v, want end %v", start, end, wantEnd)
			}
		})
	}
}

func TestWindowAt(t *testing.T) {
	sh := loc(t)
	cases := []struct {
		name   string
		at     time.Time
		active bool
		startH int // hour-of-day of the start, date encoded by at
	}{
		{"before midnight", time.Date(2026, 10, 28, 1, 30, 0, 0, sh), true, 23},
		{"seven fifty nine", time.Date(2026, 10, 28, 7, 59, 59, 0, sh), true, 23},
		{"eight exact", time.Date(2026, 10, 28, 8, 0, 0, 0, sh), false, 23},
		{"noon", time.Date(2026, 10, 28, 12, 0, 0, 0, sh), false, 23},
		{"twenty two fifty nine", time.Date(2026, 10, 28, 22, 59, 0, 0, sh), false, 23},
		{"twenty three exact", time.Date(2026, 10, 28, 23, 0, 0, 0, sh), true, 23},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end, active := WindowAt(c.at)
			if active != c.active {
				t.Fatalf("active = %v, want %v", active, c.active)
			}
			if start.Hour() != c.startH || end.Sub(start) != WindowHours*time.Hour {
				t.Fatalf("window %v..%v", start, end)
			}
			if active && !c.at.Before(end) {
				t.Fatalf("at %v not before end %v", c.at, end)
			}
		})
	}
}
