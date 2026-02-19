package swarm

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Integration branch errors
var (
	ErrBranchExists     = errors.New("branch already exists")
	ErrBranchNotFound   = errors.New("branch not found")
	ErrNotOnIntegration = errors.New("not on integration branch")
)

// SwarmGitError contains raw output from a git command for observation.
// ZFC: Callers observe the raw output and decide what to do.
type SwarmGitError struct {
	Command string
	Stdout  string
	Stderr  string
	Err     error
}

func (e *SwarmGitError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("%s: %s", e.Command, e.Stderr)
	}
	return fmt.Sprintf("%s: %v", e.Command, e.Err)
}

// CreateIntegrationBranch creates the integration branch for a swarm.
// The branch is created from the swarm's BaseCommit and pushed to origin.
func (m *Manager) CreateIntegrationBranch(swarmID string) error {
	// Acquire per-swarm lock to prevent concurrent git operations
	fl, err := m.lockSwarm(swarmID)
	if err != nil {
		return err
	}
	defer func() { _ = fl.Unlock() }()

	swarm, err := m.LoadSwarm(swarmID)
	if err != nil {
		return err
	}

	branchName := swarm.Integration

	// Check if branch already exists
	if m.branchExists(branchName) {
		return ErrBranchExists
	}

	// Create branch from BaseCommit
	if err := m.gitRun("checkout", "-b", branchName, swarm.BaseCommit); err != nil {
		return fmt.Errorf("creating branch: %w", err)
	}

	// Push to origin (non-fatal: may not have remote)
	_ = m.gitRun("push", "-u", "origin", branchName)

	return nil
}

// fetchRetries is the number of retries for transient fetch failures.
const fetchRetries = 3

// fetchRetryDelay is the base delay between fetch retries.
const fetchRetryDelay = 2 * time.Second

// MergeToIntegration merges a worker branch into the integration branch.
// Returns ErrMergeConflict if the merge has conflicts.
func (m *Manager) MergeToIntegration(swarmID, workerBranch string) error {
	// Acquire per-swarm lock to prevent concurrent merge races
	fl, err := m.lockSwarm(swarmID)
	if err != nil {
		return err
	}
	defer func() { _ = fl.Unlock() }()

	swarm, err := m.LoadSwarm(swarmID)
	if err != nil {
		return err
	}

	// Ensure we're on the integration branch
	currentBranch, err := m.getCurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}
	if currentBranch != swarm.Integration {
		if err := m.gitRun("checkout", swarm.Integration); err != nil {
			return fmt.Errorf("checking out integration: %w", err)
		}
	}

	// Fetch the worker branch with retry for transient failures
	if fetchErr := m.fetchWithRetry("origin", workerBranch); fetchErr != nil {
		// Non-fatal: may not exist on remote, try local merge
		_ = fetchErr
	}

	// Attempt merge
	err = m.gitRun("merge", "--no-ff", "-m",
		fmt.Sprintf("Merge %s into %s", workerBranch, swarm.Integration),
		workerBranch)
	if err != nil {
		// ZFC: Use git's porcelain output to detect conflicts instead of parsing stderr.
		conflicts, conflictErr := m.getConflictingFiles()
		if conflictErr == nil && len(conflicts) > 0 {
			// Return the original error with raw output for observation
			return err
		}
		return fmt.Errorf("merging: %w", err)
	}

	return nil
}

// fetchWithRetry attempts a git fetch with retries for transient failures.
// Returns nil on success, or the last error after all retries are exhausted.
func (m *Manager) fetchWithRetry(remote, branch string) error {
	var lastErr error
	for i := 0; i < fetchRetries; i++ {
		lastErr = m.gitRun("fetch", remote, branch)
		if lastErr == nil {
			return nil
		}
		// Check if the error looks transient (network/timeout) vs permanent (branch not found)
		if !isTransientFetchError(lastErr) {
			return lastErr
		}
		if i < fetchRetries-1 {
			time.Sleep(fetchRetryDelay * time.Duration(i+1))
		}
	}
	return lastErr
}

// isTransientFetchError checks if a fetch error is likely transient (network issue)
// vs permanent (branch doesn't exist, auth failure).
func isTransientFetchError(err error) bool {
	if err == nil {
		return false
	}
	var gitErr *SwarmGitError
	if !errors.As(err, &gitErr) {
		return true // Unknown error type, assume transient
	}
	stderr := strings.ToLower(gitErr.Stderr)
	// Permanent errors - don't retry
	permanentPatterns := []string{
		"couldn't find remote ref",
		"not found",
		"does not appear to be a git repository",
		"permission denied",
		"authentication failed",
	}
	for _, pattern := range permanentPatterns {
		if strings.Contains(stderr, pattern) {
			return false
		}
	}
	return true
}

// AbortMerge aborts an in-progress merge.
func (m *Manager) AbortMerge() error {
	return m.gitRun("merge", "--abort")
}

// LandToMain merges the integration branch to the target branch (usually main).
func (m *Manager) LandToMain(swarmID string) error {
	// Acquire per-swarm lock to prevent concurrent landing races
	fl, err := m.lockSwarm(swarmID)
	if err != nil {
		return err
	}
	defer func() { _ = fl.Unlock() }()

	swarm, err := m.LoadSwarm(swarmID)
	if err != nil {
		return err
	}

	// Checkout target branch
	if err := m.gitRun("checkout", swarm.TargetBranch); err != nil {
		return fmt.Errorf("checking out %s: %w", swarm.TargetBranch, err)
	}

	// Pull latest with retry for transient failures
	if pullErr := m.fetchWithRetry("origin", swarm.TargetBranch); pullErr != nil {
		_ = pullErr // Non-fatal: may fail if remote unreachable
	}

	// Merge integration branch
	err = m.gitRun("merge", "--no-ff", "-m",
		fmt.Sprintf("Land swarm %s", swarmID),
		swarm.Integration)
	if err != nil {
		// ZFC: Use git's porcelain output to detect conflicts instead of parsing stderr.
		conflicts, conflictErr := m.getConflictingFiles()
		if conflictErr == nil && len(conflicts) > 0 {
			// Return the original error with raw output for observation
			return err
		}
		return fmt.Errorf("merging to %s: %w", swarm.TargetBranch, err)
	}

	// Push
	if err := m.gitRun("push", "origin", swarm.TargetBranch); err != nil {
		return fmt.Errorf("pushing: %w", err)
	}

	return nil
}

// CleanupBranches removes all branches associated with a swarm.
func (m *Manager) CleanupBranches(swarmID string) error {
	// Acquire per-swarm lock to prevent concurrent cleanup races
	fl, err := m.lockSwarm(swarmID)
	if err != nil {
		return err
	}
	defer func() { _ = fl.Unlock() }()

	swarm, err := m.LoadSwarm(swarmID)
	if err != nil {
		return err
	}

	var lastErr error

	// Delete integration branch locally
	if err := m.gitRun("branch", "-D", swarm.Integration); err != nil {
		lastErr = err
	}

	// Delete integration branch remotely (best-effort cleanup)
	_ = m.gitRun("push", "origin", "--delete", swarm.Integration)

	// Delete worker branches (best-effort cleanup)
	for _, task := range swarm.Tasks {
		if task.Branch != "" {
			// Local delete
			_ = m.gitRun("branch", "-D", task.Branch)
			// Remote delete
			_ = m.gitRun("push", "origin", "--delete", task.Branch)
		}
	}

	return lastErr
}

// GetIntegrationBranch returns the integration branch name for a swarm.
func (m *Manager) GetIntegrationBranch(swarmID string) (string, error) {
	swarm, err := m.LoadSwarm(swarmID)
	if err != nil {
		return "", err
	}
	return swarm.Integration, nil
}

// GetWorkerBranch generates the branch name for a worker on a task.
func (m *Manager) GetWorkerBranch(swarmID, worker, taskID string) string {
	return fmt.Sprintf("%s/%s/%s", swarmID, worker, taskID)
}

// branchExists checks if a branch exists locally.
func (m *Manager) branchExists(branch string) bool {
	err := m.gitRun("show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// getCurrentBranch returns the current branch name.
func (m *Manager) getCurrentBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = m.gitDir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}

// getConflictingFiles returns the list of files with merge conflicts.
// ZFC: Uses git's porcelain output (diff --diff-filter=U) instead of parsing stderr.
func (m *Manager) getConflictingFiles() ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = m.gitDir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return nil, nil
	}

	files := strings.Split(out, "\n")
	var result []string
	for _, f := range files {
		if f != "" {
			result = append(result, f)
		}
	}
	return result, nil
}

// gitRun executes a git command.
// ZFC: Returns SwarmGitError with raw output for agent observation.
func (m *Manager) gitRun(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = m.gitDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Determine command name
		command := ""
		for _, arg := range args {
			if !strings.HasPrefix(arg, "-") {
				command = arg
				break
			}
		}
		if command == "" && len(args) > 0 {
			command = args[0]
		}

		return &SwarmGitError{
			Command: command,
			Stdout:  strings.TrimSpace(stdout.String()),
			Stderr:  strings.TrimSpace(stderr.String()),
			Err:     err,
		}
	}

	return nil
}
