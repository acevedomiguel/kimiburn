package main

import "strings"

// terminalProgressSupported reports whether the terminal is known to draw the
// OSC 9;4 progress indicator. The list is deliberately conservative: some
// terminals read any OSC 9 as a desktop notification, so an unknown terminal
// never receives the sequence. Inside tmux or screen TERM_PROGRAM names the
// multiplexer, which keeps the sequence away from the outer terminal too.
func terminalProgressSupported(getenv func(string) string) bool {
	switch strings.ToLower(getenv("TERM_PROGRAM")) {
	case "ghostty", "wezterm":
		return true
	case "tmux", "screen":
		return false
	}
	return getenv("WT_SESSION") != "" || strings.EqualFold(getenv("ConEmuANSI"), "ON")
}
