package settings

import (
	"context"
	"fmt"
	"os"
	"time"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/extensions"
	"agent-screensaver/internal/install"
)

// WarmUpTimeout bounds the first run of a converter. macOS checks a new
// program the first time it starts, which can take far longer than a
// converter's own timeout.
const WarmUpTimeout = 10 * time.Second

// WarmUpConverters runs each converter program of the installed agent id
// once, with empty input and the environment the hook gives it, so macOS
// has checked the program before the first real event. The output is
// ignored. It returns one warning per program that failed; a failure here
// does not undo the install. Other extensions have no programs.
func (s Service) WarmUpConverters(id string) []string {
	ext, err := s.find(id)
	if err != nil || ext.Manifest.Type != extensions.TypeAgent {
		return nil
	}
	agent, err := agents.FromManifest(ext.Manifest, ext.Dir)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", id, err)}
	}
	env := agents.ConverterEnv(s.Paths.Home, os.Getenv("PATH"), agent.Source, "")
	var warnings []string
	for _, program := range install.Programs(agent) {
		converter := agents.Converter{Run: program, Timeout: WarmUpTimeout}
		if _, err := agents.RunProgram(context.Background(), converter, nil, env); err != nil {
			warnings = append(warnings, fmt.Sprintf("the first run of %s failed (%v); its first event may be lost", program, err))
		}
	}
	return warnings
}
