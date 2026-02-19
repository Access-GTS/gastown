package swarm

import (
	"fmt"
	"testing"

	"github.com/steveyegge/gastown/internal/rig"
)

func TestGetWorkerBranch(t *testing.T) {
	r := &rig.Rig{
		Name: "test-rig",
		Path: "/tmp/test-rig",
	}
	m := NewManager(r)

	branch := m.GetWorkerBranch("sw-1", "Toast", "task-123")
	expected := "sw-1/Toast/task-123"
	if branch != expected {
		t.Errorf("branch = %q, want %q", branch, expected)
	}
}

// TestIsTransientFetchError tests classification of fetch errors.
func TestIsTransientFetchError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		transient bool
	}{
		{
			name:      "nil error",
			err:       nil,
			transient: false,
		},
		{
			name:      "branch not found (permanent)",
			err:       &SwarmGitError{Command: "fetch", Stderr: "fatal: couldn't find remote ref some-branch"},
			transient: false,
		},
		{
			name:      "permission denied (permanent)",
			err:       &SwarmGitError{Command: "fetch", Stderr: "Permission denied (publickey)"},
			transient: false,
		},
		{
			name:      "auth failed (permanent)",
			err:       &SwarmGitError{Command: "fetch", Stderr: "Authentication failed for 'https://...'"},
			transient: false,
		},
		{
			name:      "not a git repo (permanent)",
			err:       &SwarmGitError{Command: "fetch", Stderr: "does not appear to be a git repository"},
			transient: false,
		},
		{
			name:      "network timeout (transient)",
			err:       &SwarmGitError{Command: "fetch", Stderr: "fatal: unable to access: Could not resolve host"},
			transient: true,
		},
		{
			name:      "connection reset (transient)",
			err:       &SwarmGitError{Command: "fetch", Stderr: "fatal: the remote end hung up unexpectedly"},
			transient: true,
		},
		{
			name:      "generic error (assume transient)",
			err:       fmt.Errorf("some other error"),
			transient: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTransientFetchError(tt.err)
			if got != tt.transient {
				t.Errorf("isTransientFetchError() = %v, want %v", got, tt.transient)
			}
		})
	}
}

// Note: Integration tests that require git operations and beads
// are covered by the E2E test (gt-kc7yj.4).
