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
	return ask("Proceed?", yes, stdin, stdout)
}

// ask asks a yes or no question, unless yes is set. Only "y" or "yes"
// agrees. A stdin that ends before an answer (not a terminal) says no.
func ask(question string, yes bool, stdin io.Reader, stdout io.Writer) bool {
	if yes {
		return true
	}
	fmt.Fprintf(stdout, "%s [y/N] ", question)
	answer, err := bufio.NewReader(stdin).ReadString('\n')
	answer = strings.TrimSpace(answer)
	if strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes") {
		return true
	}
	if err != nil && answer == "" {
		fmt.Fprintln(stdout, "\nNo answer: stdin is not a terminal.")
	}
	fmt.Fprintln(stdout, "Cancelled. Nothing was changed.")
	return false
}
