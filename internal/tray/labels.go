package tray

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// The functions in this file are the Go copy of web/src/format.ts. The menu
// and the screen saver must write the same text for the same input, so the
// cases in labels_test.go are copies of the web tests. Change both together.

const (
	msPerMinute    = 60_000
	minutesPerHour = 60
	msPerDay       = 24 * minutesPerHour * msPerMinute
)

// FormatDuration writes elapsed time, for example "3m" or "1h 12m". Under one
// minute is "<1m".
func FormatDuration(ms int64) string {
	minutes := ms / msPerMinute
	if ms < 0 || minutes < 1 {
		return "<1m"
	}
	if minutes < minutesPerHour {
		return fmt.Sprintf("%dm", minutes)
	}
	hours, rest := minutes/minutesPerHour, minutes%minutesPerHour
	if rest == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, rest)
}

// FormatResetIn writes the time until a limit resets, for example "1h40m".
// resetsAtS is in seconds. It rounds up so a limit never shows "0m" before the
// reset.
func FormatResetIn(resetsAtS, nowMs int64) string {
	remainingMs := resetsAtS*1000 - nowMs
	if remainingMs <= 0 {
		return "reset, waiting for new data"
	}
	minutes := (remainingMs + msPerMinute - 1) / msPerMinute
	if minutes < minutesPerHour {
		return fmt.Sprintf("%dm", minutes)
	}
	hours, rest := minutes/minutesPerHour, minutes%minutesPerHour
	if rest == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%dm", hours, rest)
}

// FormatResetDate writes the reset date, for example "resets 3 Nov", in the
// local time zone. The month is spelled out here, never taken from the
// locale, so it matches the web text.
func FormatResetDate(resetsAtS int64) string {
	date := time.Unix(resetsAtS, 0)
	return fmt.Sprintf("resets %d %s", date.Day(), date.Month().String()[:3])
}

// FormatResetLabel is a countdown, or the date once the reset is more than 24
// hours away.
func FormatResetLabel(resetsAtS, nowMs int64) string {
	if resetsAtS*1000-nowMs > msPerDay {
		return FormatResetDate(resetsAtS)
	}
	return FormatResetIn(resetsAtS, nowMs)
}

// DisplayName gives a source a capital first letter: "claude" is "Claude".
func DisplayName(source string) string {
	first, size := utf8.DecodeRuneInString(source)
	if size == 0 {
		return ""
	}
	return strings.ToUpper(string(first)) + source[size:]
}
