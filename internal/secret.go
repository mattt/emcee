package internal

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var (
	// Command is a variable that allows overriding the command creation for testing
	CommandContext = exec.CommandContext
	// LookPath is a variable that allows overriding the lookup behavior for testing
	LookPath = exec.LookPath
)

// ResolveSecretReference attempts to resolve a 1Password secret reference (e.g. op://vault/item/field)
// Returns the resolved value and whether it was a secret reference
func ResolveSecretReference(ctx context.Context, value string) (string, bool, error) {
	if !strings.HasPrefix(value, "op://") {
		return value, false, nil
	}

	// Check if op CLI is available
	if _, err := LookPath("op"); err != nil {
		return "", true, fmt.Errorf("1Password CLI (op) not found in PATH: %w", err)
	}

	// Create command to read secret
	cmd := CommandContext(ctx, "op", "read", value)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", true, fmt.Errorf("failed to read secret from 1Password: %s", string(exitErr.Stderr))
		}
		return "", true, fmt.Errorf("failed to read secret from 1Password: %w", err)
	}

	// Trim any whitespace/newlines from the output
	return strings.TrimSpace(string(output)), true, nil
}
