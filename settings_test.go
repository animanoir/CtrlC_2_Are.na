package main

import "testing"

func TestNumberSettingParse(t *testing.T) {
	tests := []struct {
		name, in string
		setting  numberSetting
		want     int
	}{
		{"typical", "30", autoStopMinutes, 30},
		{"smallest", "1", autoStopMinutes, 1},
		{"largest", "720", autoStopMinutes, 720},
		{"surrounding spaces", " 45 ", autoStopMinutes, 45},
		{"leading zeros", "0030", autoStopMinutes, 30},

		// Out of range is clamped
		{"zero", "0", autoStopMinutes, 1},
		{"all zeros", "000", autoStopMinutes, 1},
		{"just over", "721", autoStopMinutes, 720},
		{"four digits", "1000", autoStopMinutes, 720},
		{"absurdly long", "99999999999999999999999", autoStopMinutes, 720},

		// Not a whole number means the default
		{"empty", "", autoStopMinutes, 30},
		{"spaces", "   ", autoStopMinutes, 30},
		{"letters", "abc", autoStopMinutes, 30},
		{"negative", "-5", autoStopMinutes, 30},
		{"decimal", "12.5", autoStopMinutes, 30},
		{"number with a unit", "45 min", autoStopMinutes, 30},

		// The hold has its own default and range
		{"hold typical", "5", holdSeconds, 5},
		{"hold largest", "30", holdSeconds, 30},
		{"hold just over", "31", holdSeconds, 30},
		{"hold three digits", "100", holdSeconds, 30},
		{"hold zero", "0", holdSeconds, 1},
		{"hold leading zeros", "007", holdSeconds, 7},
		{"hold empty", "", holdSeconds, 5},
		{"hold letters", "abc", holdSeconds, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.setting.parse(tt.in); got != tt.want {
				t.Errorf("parse(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}
