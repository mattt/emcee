package internal

import (
	"context"
	"os/exec"
	"testing"
)

func TestResolveSecretReference(t *testing.T) {
	// Save the original functions and restore them after the test
	originalCommand := CommandContext
	originalLookPath := LookPath
	t.Cleanup(func() {
		CommandContext = originalCommand
		LookPath = originalLookPath
	})

	tests := []struct {
		name               string
		input              string
		mockCommandContext func(ctx context.Context, name string, args ...string) *exec.Cmd
		mockLookPath       func(string) (string, error)
		wantValue          string
		wantSecret         bool
		wantErr            bool
	}{
		{
			name:       "non-secret value",
			input:      "regular-value",
			wantValue:  "regular-value",
			wantSecret: false,
		},
		{
			name:  "successful secret resolution",
			input: "op://vault/item/field",
			mockLookPath: func(string) (string, error) {
				return "/usr/local/bin/op", nil
			},
			mockCommandContext: func(ctx context.Context, name string, args ...string) *exec.Cmd {
				return exec.CommandContext(ctx, "echo", "secret-value")
			},
			wantValue:  "secret-value",
			wantSecret: true,
		},
		{
			name:  "op CLI not found",
			input: "op://vault/item/field",
			mockLookPath: func(string) (string, error) {
				return "", exec.ErrNotFound
			},
			wantValue:  "",
			wantSecret: true,
			wantErr:    true,
		},
		{
			name:  "op command execution failed",
			input: "op://vault/item/field",
			mockLookPath: func(string) (string, error) {
				return "/usr/local/bin/op", nil
			},
			mockCommandContext: func(ctx context.Context, name string, args ...string) *exec.Cmd {
				// Return a command that will fail
				return exec.CommandContext(ctx, "false")
			},
			wantValue:  "",
			wantSecret: true,
			wantErr:    true,
		},
		{
			name:       "empty input",
			input:      "",
			wantValue:  "",
			wantSecret: false,
		},
		{
			name:       "malformed op reference",
			input:      "op://invalid",
			wantValue:  "",
			wantSecret: true,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockCommandContext != nil {
				CommandContext = tt.mockCommandContext
			}
			if tt.mockLookPath != nil {
				LookPath = tt.mockLookPath
			}

			got, isSecret, err := ResolveSecretReference(context.Background(), tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveSecretReference() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantValue {
				t.Errorf("ResolveSecretReference() got = %v, want %v", got, tt.wantValue)
			}
			if isSecret != tt.wantSecret {
				t.Errorf("ResolveSecretReference() isSecret = %v, want %v", isSecret, tt.wantSecret)
			}
		})
	}
}
