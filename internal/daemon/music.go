package daemon

import "agent-screensaver/internal/nowplaying"

// musicEveryMs is the time between two reads of the playing song. Between
// reads the screen saver moves the position on by itself.
const musicEveryMs = 3_000

// musicDriftMs is how far a read position may be from the one expected
// since the last write before the state is written again: a seek, or a song
// that started over.
const musicDriftMs = 2_000

// MusicReader reads the song the music apps play: false when none plays.
type MusicReader interface {
	Read(nowMs int64) (nowplaying.Track, bool, error)
}

type musicResult struct {
	track   nowplaying.Track
	playing bool
	err     error
}

// readMusic merges a finished read and starts the next one when due, one at
// a time, only while the user has now playing on. It reports whether the
// song the file shows changed.
func (d *Daemon) readMusic(nowMs int64) bool {
	changed := d.mergeMusic()
	if !d.cfg.NowPlaying || d.Music == nil {
		changed = d.track != nil || changed
		d.track = nil
		return changed
	}
	if !d.musicInFlight && (!d.musicReadOnce || nowMs-d.musicReadMs >= musicEveryMs) {
		d.musicInFlight, d.musicReadOnce, d.musicReadMs = true, true, nowMs
		reader := d.Music
		d.MusicRunner(func() {
			track, playing, err := reader.Read(nowMs)
			d.musicResults <- musicResult{track, playing, err}
		})
		changed = d.mergeMusic() || changed // a runner that reads at once
	}
	return changed
}

// mergeMusic takes the finished read, if any. It never waits. A failed read
// shows nothing, and its error is logged only when it changes, so a user
// who did not allow the app to be controlled is told once.
func (d *Daemon) mergeMusic() bool {
	var result musicResult
	select {
	case result = <-d.musicResults:
	default:
		return false
	}
	d.musicInFlight = false
	if message := errText(result.err); message != d.musicErr {
		d.musicErr = message
		if message != "" {
			d.Logf("now playing: %s", message)
		}
	}
	if result.err != nil || !result.playing || !d.cfg.NowPlaying {
		changed := d.track != nil
		d.track = nil
		return changed
	}
	changed := !sameSong(d.track, result.track)
	next := result.track
	d.track = &next
	return changed
}

// sameSong reports whether next is the song shown, at about the position
// the screen saver shows by now.
func sameSong(shown *nowplaying.Track, next nowplaying.Track) bool {
	if shown == nil || shown.App != next.App || shown.Title != next.Title || shown.Artist != next.Artist || shown.DurationMs != next.DurationMs {
		return false
	}
	expected := shown.PositionMs + next.AtMs - shown.AtMs
	return abs(next.PositionMs-expected) <= musicDriftMs
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
