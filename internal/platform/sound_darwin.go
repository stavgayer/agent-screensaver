package platform

import (
	"fmt"
	"os/exec"
)

const afplayPath = "/usr/bin/afplay"

type afplayPlayer struct{}

// NewSoundPlayer returns a player that runs afplay.
func NewSoundPlayer() SoundPlayer { return afplayPlayer{} }

// Play starts afplay and returns at once. The goroutine only reaps the
// finished process, so no zombie is left behind.
func (afplayPlayer) Play(file string) error {
	cmd := exec.Command(afplayPath, file)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", afplayPath, err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
