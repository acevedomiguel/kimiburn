package main

import "testing"

func TestTerminalProgressOnlyForKnownTerminals(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"ghostty", map[string]string{"TERM_PROGRAM": "ghostty"}, true},
		{"wezterm", map[string]string{"TERM_PROGRAM": "WezTerm"}, true},
		{"windows terminal", map[string]string{"WT_SESSION": "synthetic-session"}, true},
		{"conemu", map[string]string{"ConEmuANSI": "ON"}, true},
		{"iterm reads osc 9 as a notification", map[string]string{"TERM_PROGRAM": "iTerm.app"}, false},
		{"apple terminal", map[string]string{"TERM_PROGRAM": "Apple_Terminal"}, false},
		{"tmux inside windows terminal", map[string]string{"TERM_PROGRAM": "tmux", "WT_SESSION": "synthetic-session"}, false},
		{"unknown", map[string]string{}, false},
	} {
		if got := terminalProgressSupported(func(key string) string { return tc.env[key] }); got != tc.want {
			t.Errorf("%s: got %t, want %t", tc.name, got, tc.want)
		}
	}
}
