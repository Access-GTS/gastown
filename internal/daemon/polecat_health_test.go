package daemon

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/tmux"
)

// writeFakeTestTmux creates a shell script in dir named "tmux" that simulates
// "session not found" for has-session calls and fails on anything else.
func writeFakeTestTmux(t *testing.T, dir string) {
	t.Helper()
	script := "#!/bin/sh\n" +
		"case \"$*\" in\n" +
		"  *has-session*) echo \"can't find session\" >&2; exit 1;;\n" +
		"  *) echo 'unexpected tmux command' >&2; exit 1;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0755); err != nil {
		t.Fatalf("writing fake tmux: %v", err)
	}
}

// writeFakeTestBD creates a shell script in dir named "bd" that outputs a
// polecat agent bead JSON with the given agentState and hookBead values.
// The description field contains "agent_state: <agentState>" which is parsed
// by ParseAgentFieldsFromDescription to populate AgentBeadInfo.State.
func writeFakeTestBD(t *testing.T, dir, agentState, hookBead string) string {
	t.Helper()
	// description parsed by ParseAgentFieldsFromDescription for State field
	desc := "agent_state: " + agentState
	// JSON matches the structure that getAgentBeadInfo expects from bd show --json
	bdJSON := `[{"id":"gt-myr-polecat-mycat","issue_type":"agent","labels":["gt:agent"],"description":"` +
		desc + `","hook_bead":"` + hookBead + `","agent_state":"` + agentState + `","updated_at":"2024-01-01T00:00:00Z"}]`
	script := "#!/bin/sh\necho '" + bdJSON + "'\n"
	path := filepath.Join(dir, "bd")
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("writing fake bd: %v", err)
	}
	return path
}

// TestCheckPolecatHealth_SkipsSpawning verifies that checkPolecatHealth does NOT
// attempt to restart a polecat in agent_state=spawning. This is the regression
// test for the double-spawn bug (issue #1752): the daemon heartbeat fires during
// the window between bead creation (hook_bead set atomically by gt sling) and the
// actual tmux session launch, causing a second Claude process to start.
func TestCheckPolecatHealth_SkipsSpawning(t *testing.T) {
	binDir := t.TempDir()
	writeFakeTestTmux(t, binDir)
	bdPath := writeFakeTestBD(t, binDir, "spawning", "gt-xyz")

	// Prepend binDir to PATH so exec.Command("tmux",...) finds our fake binary.
	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	var logBuf strings.Builder
	d := &Daemon{
		config: &Config{TownRoot: t.TempDir()},
		logger: log.New(&logBuf, "", 0),
		tmux:   tmux.NewTmux(),
		bdPath: bdPath,
	}

	d.checkPolecatHealth("myr", "mycat")

	got := logBuf.String()
	if !strings.Contains(got, "spawning") {
		t.Errorf("expected log to mention 'spawning', got: %q", got)
	}
	if strings.Contains(got, "CRASH DETECTED") {
		t.Errorf("spawning polecat must not trigger CRASH DETECTED, got: %q", got)
	}
}

// TestCheckPolecatHealth_DetectsCrashedPolecat verifies that checkPolecatHealth
// does detect a crash for a polecat in agent_state=working with a dead session.
// This ensures the spawning guard in issue #1752 does not accidentally suppress
// legitimate crash detection for polecats that were running normally.
func TestCheckPolecatHealth_DetectsCrashedPolecat(t *testing.T) {
	binDir := t.TempDir()
	writeFakeTestTmux(t, binDir)
	bdPath := writeFakeTestBD(t, binDir, "working", "gt-xyz")

	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	var logBuf strings.Builder
	d := &Daemon{
		config: &Config{TownRoot: t.TempDir()},
		logger: log.New(&logBuf, "", 0),
		tmux:   tmux.NewTmux(),
		bdPath: bdPath,
	}

	d.checkPolecatHealth("myr", "mycat")

	got := logBuf.String()
	if !strings.Contains(got, "CRASH DETECTED") {
		t.Errorf("expected CRASH DETECTED for working polecat with dead session, got: %q", got)
	}
}
