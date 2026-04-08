package main

import (
	"os"
	"testing"
)

func TestMainWithArgs(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantExitCalled bool
		wantExitCode   int
	}{
		{
			name:           "version command",
			args:           []string{"version"},
			wantExitCalled: false,
			wantExitCode:   0,
		},
		{
			name:           "help command",
			args:           []string{"help"},
			wantExitCalled: false,
			wantExitCode:   0,
		},
		{
			name:           "start without config fails",
			args:           []string{"start"},
			wantExitCalled: true,
			wantExitCode:   1,
		},
		{
			name:           "unknown command fails",
			args:           []string{"unknown"},
			wantExitCalled: true,
			wantExitCode:   1,
		},
		{
			name:           "no args",
			args:           []string{},
			wantExitCalled: true,
			wantExitCode:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testExitCode := -1
			exitCalled := false

			// Override osExit to capture exit code
			oldOsExit := osExit
			osExit = func(code int) {
				testExitCode = code
				exitCalled = true
			}
			defer func() { osExit = oldOsExit }()

			// Run main with args
			if len(tt.args) == 0 {
				os.Args = []string{"apicerberus"}
			} else {
				os.Args = append([]string{"apicerberus"}, tt.args...)
			}

			main()

			if exitCalled != tt.wantExitCalled {
				t.Errorf("exit called = %v, want %v", exitCalled, tt.wantExitCalled)
			}

			if exitCalled && testExitCode != tt.wantExitCode {
				t.Errorf("exit code = %v, want %v", testExitCode, tt.wantExitCode)
			}
		})
	}
}
