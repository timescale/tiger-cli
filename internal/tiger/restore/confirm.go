package restore

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// confirmDestructive prompts the user to confirm destructive operations
func (r *Restorer) confirmDestructive() error {
	fmt.Fprintf(r.options.Errors, "\n⚠️  WARNING: This will drop existing database objects before restore.\n")
	fmt.Fprintf(r.options.Errors, "Type 'yes' to confirm: ")

	// Read user input from stdin
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return fmt.Errorf("failed to read confirmation")
	}

	confirmation := strings.TrimSpace(scanner.Text())
	if strings.ToLower(confirmation) != "yes" {
		return fmt.Errorf("restore cancelled")
	}

	return nil
}
