package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"agent-screensaver/internal/install"
)

// Confirm prints the plan and asks to proceed, unless yes is set. Warning
// lines (an outside agent's own programs and plugin files) stand out. It is
// the one prompt of install, uninstall and `agent hooks`.
func Confirm(plan []install.PlannedStep, yes bool, stdin io.Reader, stdout io.Writer) bool {
	fmt.Fprintln(stdout, "This will:")
	for _, step := range plan {
		if step.Warning {
			fmt.Fprintf(stdout, "  ! WARNING: %s\n", step.Description)
			continue
		}
		fmt.Fprintf(stdout, "  - %s\n", step.Description)
	}
	if yes {
		return true
	}
	fmt.Fprint(stdout, "Proceed? [y/N] ")
	answer, _ := bufio.NewReader(stdin).ReadString('\n')
	if strings.EqualFold(strings.TrimSpace(answer), "y") || strings.EqualFold(strings.TrimSpace(answer), "yes") {
		return true
	}
	fmt.Fprintln(stdout, "Cancelled. Nothing was changed.")
	return false
}
