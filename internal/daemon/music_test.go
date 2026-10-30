package daemon

import (
	"errors"
	"testing"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/nowplaying"
	"agent-screensaver/internal/paths"
)

// fakeMusic answers each read with the next of its answers, then repeats
// the last one.
type fakeMusic struct {
	answers []musicResult
	reads   int
}

func (f *fakeMusic) Read(nowMs int64) (nowplaying.Track, bool, error) {
	answer := f.answers[min(f.reads, len(f.answers)-1)]
	f.reads++
	answer.track.AtMs = nowMs
	return answer.track, answer.playing, answer.err
}

var song = nowplaying.Track{App: "Spotify", Title: "Sloop John B", Artist: "The Beach Boys", PositionMs: 10_000, DurationMs: 179_000}

func newMusicDaemon(t *testing.T, music *fakeMusic, nowPlaying, privacy bool) (*Daemon, paths.Paths) {
	t.Helper()
	d, p := newTestDaemon(t)
	d.Music = music
	d.MusicRunner = func(read func()) { read() }
	cfg := config.Default()
	cfg.NowPlaying, cfg.Privacy = nowPlaying, privacy
	saveDemoConfig(t, p, cfg)
	return d, p
}

func TestMusicIsReadOnlyWhileNowPlayingIsOn(t *testing.T) {
	tests := []struct {
		name       string
		nowPlaying bool
		privacy    bool
		answer     musicResult
		wantSong   bool
		wantReads  int
	}{
		{"off never asks the apps", false, false, musicResult{track: song, playing: true}, false, 0},
		{"on shows the song", true, false, musicResult{track: song, playing: true}, true, 1},
		{"paused shows nothing", true, false, musicResult{track: song, playing: false}, false, 1},
		{"a failed read shows nothing", true, false, musicResult{err: errors.New("not allowed")}, false, 1},
		{"privacy leaves the song out", true, true, musicResult{track: song, playing: true}, false, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			music := &fakeMusic{answers: []musicResult{tt.answer}}
			d, p := newMusicDaemon(t, music, tt.nowPlaying, tt.privacy)
			d.Step(1_000, false)
			got := readStateFile(t, p).NowPlaying
			if (got != nil) != tt.wantSong || music.reads != tt.wantReads {
				t.Fatalf("now_playing = %+v after %d reads, want song %v after %d", got, music.reads, tt.wantSong, tt.wantReads)
			}
			if got != nil && (got.Title != song.Title || got.AtMs != 1_000) {
				t.Errorf("now_playing = %+v, want %q read at 1000", got, song.Title)
			}
		})
	}
}

func TestMusicReadsAreSpaced(t *testing.T) {
	music := &fakeMusic{answers: []musicResult{{track: song, playing: true}}}
	d, _ := newMusicDaemon(t, music, true, false)
	for _, nowMs := range []int64{1_000, 1_500, 2_000, 3_999, 4_000, 4_500} {
		d.Step(nowMs, false)
	}
	if music.reads != 2 {
		t.Errorf("reads = %d, want 2 (at 1000 and 4000)", music.reads)
	}
}

func TestMusicStopsWhenTurnedOff(t *testing.T) {
	music := &fakeMusic{answers: []musicResult{{track: song, playing: true}}}
	d, p := newMusicDaemon(t, music, true, false)
	d.Step(1_000, false)
	saveDemoConfig(t, p, config.Default())
	d.Step(5_000, false)
	if got := readStateFile(t, p).NowPlaying; got != nil || music.reads != 1 {
		t.Errorf("after turning off: now_playing = %+v, reads = %d, want none and 1", got, music.reads)
	}
}

func TestMusicErrorIsLoggedOnce(t *testing.T) {
	music := &fakeMusic{answers: []musicResult{{err: errors.New("not allowed")}}}
	d, _ := newMusicDaemon(t, music, true, false)
	var logged int
	d.Logf = func(string, ...any) { logged++ }
	for _, nowMs := range []int64{1_000, 4_000, 7_000} {
		d.Step(nowMs, false)
	}
	if music.reads != 3 || logged != 1 {
		t.Errorf("reads = %d, logs = %d, want 3 reads and 1 log", music.reads, logged)
	}
}

func TestSameSong(t *testing.T) {
	shown := song
	shown.AtMs = 1_000
	at := func(positionMs, atMs int64) nowplaying.Track {
		next := song
		next.PositionMs, next.AtMs = positionMs, atMs
		return next
	}
	other := at(13_000, 4_000)
	other.Title = "Wouldn't It Be Nice"
	tests := []struct {
		name string
		next nowplaying.Track
		want bool
	}{
		{"played on as expected", at(13_000, 4_000), true},
		{"a little off", at(14_500, 4_000), true},
		{"a seek", at(60_000, 4_000), false},
		{"started over", at(0, 4_000), false},
		{"another song", other, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameSong(&shown, tt.next); got != tt.want {
				t.Errorf("sameSong = %v, want %v", got, tt.want)
			}
		})
	}
	if sameSong(nil, song) {
		t.Error("sameSong(nil, song) = true, want false")
	}
}

func TestDemoSongFollowsNowPlaying(t *testing.T) {
	for _, nowPlaying := range []bool{false, true} {
		d, p := newTestDaemon(t)
		cfg := config.Default()
		cfg.NowPlaying, cfg.DemoSeconds, cfg.DemoArmedMs = nowPlaying, 10, 1_000
		saveDemoConfig(t, p, cfg)
		d.Step(2_000, true)
		file := readStateFile(t, p)
		if !file.Demo || (file.NowPlaying != nil) != nowPlaying {
			t.Errorf("now playing %v: demo %v, song %+v", nowPlaying, file.Demo, file.NowPlaying)
		}
		if file.NowPlaying != nil && file.NowPlaying.Title != "Slow Lights" {
			t.Errorf("demo song = %+v, want the made-up one", file.NowPlaying)
		}
	}
}
