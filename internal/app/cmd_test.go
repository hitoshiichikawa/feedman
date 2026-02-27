package app

import (
	"testing"
)

func TestParseCommand_DefaultsToServe(t *testing.T) {
	cmd := ParseCommand([]string{})
	if cmd != CommandServe {
		t.Errorf("ParseCommand([]) = %q, want %q", cmd, CommandServe)
	}
}

func TestParseCommand_Serve(t *testing.T) {
	cmd := ParseCommand([]string{"serve"})
	if cmd != CommandServe {
		t.Errorf("ParseCommand([serve]) = %q, want %q", cmd, CommandServe)
	}
}

func TestParseCommand_Worker(t *testing.T) {
	cmd := ParseCommand([]string{"worker"})
	if cmd != CommandWorker {
		t.Errorf("ParseCommand([worker]) = %q, want %q", cmd, CommandWorker)
	}
}

func TestParseCommand_Migrate(t *testing.T) {
	cmd := ParseCommand([]string{"migrate"})
	if cmd != CommandMigrate {
		t.Errorf("ParseCommand([migrate]) = %q, want %q", cmd, CommandMigrate)
	}
}

func TestParseCommand_UnknownDefaultsToServe(t *testing.T) {
	cmd := ParseCommand([]string{"unknown"})
	if cmd != CommandServe {
		t.Errorf("ParseCommand([unknown]) = %q, want %q", cmd, CommandServe)
	}
}

func TestParseCommand_IgnoresExtraArgs(t *testing.T) {
	cmd := ParseCommand([]string{"worker", "--flag", "value"})
	if cmd != CommandWorker {
		t.Errorf("ParseCommand([worker --flag value]) = %q, want %q", cmd, CommandWorker)
	}
}

func TestCommandString(t *testing.T) {
	tests := []struct {
		cmd  Command
		want string
	}{
		{CommandServe, "serve"},
		{CommandWorker, "worker"},
		{CommandMigrate, "migrate"},
	}

	for _, tt := range tests {
		if got := string(tt.cmd); got != tt.want {
			t.Errorf("Command(%q) string = %q, want %q", tt.cmd, got, tt.want)
		}
	}
}
