// Package nowplaying asks the Spotify and Music apps what they play, through
// their public AppleScript interface. macOS no longer lets an ordinary
// program read the system-wide Now Playing info, and the apps' web APIs
// would need the network and a login, so this is the one clean route. It
// never starts an app: an app that is not running is skipped.
package nowplaying

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ReadTimeout bounds one read. A busy app can hold an Apple event for
// seconds; the next read simply tries again.
const ReadTimeout = 2 * time.Second

// Track is the song an app plays. DurationMs is 0 when the app does not
// say, as Music does for songs it streams. AtMs is when it was read, so the
// screen saver can move the position on between reads.
type Track struct {
	App        string `json:"app"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	PositionMs int64  `json:"position_ms"`
	DurationMs int64  `json:"duration_ms"`
	AtMs       int64  `json:"at_ms"`
}

// fieldSeparator splits the script's answer. It is the ASCII unit
// separator, which no song or artist name carries, unlike a tab.
const fieldSeparator = "\x1f"

// script answers app, title, artist, position and length (both in whole
// milliseconds, so the decimal mark of the user's locale never matters),
// or an empty text when neither app plays. Each app's part is compiled
// only when the app runs (run script), so a missing app never makes
// AppleScript ask where it is. Spotify gives the length in milliseconds,
// Music in seconds or not at all.
const script = `
on probe(appName, source)
	if application appName is running then
		try
			return run script source
		end try
	end if
	return ""
end probe
set sep to character id 31
set answer to probe("Spotify", "tell application \"Spotify\"
	if player state is not playing then return \"\"
	set sep to character id 31
	return \"Spotify\" & sep & (name of current track) & sep & (artist of current track) & sep & ((round (player position * 1000)) as text) & sep & ((duration of current track) as text)
end tell")
if answer is "" then set answer to probe("Music", "tell application \"Music\"
	if player state is not playing then return \"\"
	set sep to character id 31
	set songLength to duration of current track
	if songLength is missing value then set songLength to 0
	return \"Music\" & sep & (name of current track) & sep & (artist of current track) & sep & ((round (player position * 1000)) as text) & sep & ((round (songLength * 1000)) as text)
end tell")
return answer
`

// Reader reads the playing song with osascript.
type Reader struct{}

// Read returns the song that plays now, and false when nothing plays. An
// error means the read itself failed: osascript is missing, it timed out,
// or the user did not allow the app to be controlled.
func (Reader) Read(nowMs int64) (Track, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ReadTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "osascript", "-e", script).Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(exit.Stderr) > 0 {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exit.Stderr)))
		}
		return Track{}, false, fmt.Errorf("osascript: %w", err)
	}
	return Parse(string(out), nowMs)
}

// Parse reads the script's answer. Empty means nothing plays. A song
// without a title is treated as nothing: there is nothing to show.
func Parse(answer string, nowMs int64) (Track, bool, error) {
	answer = strings.TrimRight(answer, "\r\n")
	if answer == "" {
		return Track{}, false, nil
	}
	fields := strings.Split(answer, fieldSeparator)
	if len(fields) != 5 {
		return Track{}, false, fmt.Errorf("now playing: want 5 fields, got %d", len(fields))
	}
	position, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return Track{}, false, fmt.Errorf("now playing position: %w", err)
	}
	duration, err := strconv.ParseInt(fields[4], 10, 64)
	if err != nil {
		return Track{}, false, fmt.Errorf("now playing length: %w", err)
	}
	track := Track{App: fields[0], Title: fields[1], Artist: fields[2], PositionMs: max(position, 0), DurationMs: max(duration, 0), AtMs: nowMs}
	if track.Title == "" {
		return Track{}, false, nil
	}
	return track, true, nil
}
