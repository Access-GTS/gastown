package swarm

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/steveyegge/gastown/internal/rig"
)

// TestLoadSwarmNotFound tests that LoadSwarm returns error for missing epic.
func TestLoadSwarmNotFound(t *testing.T) {
	r := &rig.Rig{
		Name: "test-rig",
		Path: "/tmp/test-rig",
	}
	m := NewManager(r)

	// LoadSwarm for non-existent epic should fail (no beads available)
	_, err := m.LoadSwarm("nonexistent-epic")
	if err == nil {
		t.Error("LoadSwarm should fail for non-existent epic")
	}
}

// TestGetSwarmNotFound tests that GetSwarm returns error for missing swarm.
func TestGetSwarmNotFound(t *testing.T) {
	r := &rig.Rig{
		Name: "test-rig",
		Path: "/tmp/test-rig",
	}
	m := NewManager(r)

	_, err := m.GetSwarm("nonexistent")
	if err == nil {
		t.Error("GetSwarm for nonexistent should return error")
	}
}

// TestGetReadyTasksNotFound tests that GetReadyTasks returns error for missing swarm.
func TestGetReadyTasksNotFound(t *testing.T) {
	r := &rig.Rig{
		Name: "test-rig",
		Path: "/tmp/test-rig",
	}
	m := NewManager(r)

	_, err := m.GetReadyTasks("nonexistent")
	if err != ErrSwarmNotFound {
		t.Errorf("GetReadyTasks = %v, want ErrSwarmNotFound", err)
	}
}

// TestIsCompleteNotFound tests that IsComplete returns error for missing swarm.
func TestIsCompleteNotFound(t *testing.T) {
	r := &rig.Rig{
		Name: "test-rig",
		Path: "/tmp/test-rig",
	}
	m := NewManager(r)

	_, err := m.IsComplete("nonexistent")
	if err != ErrSwarmNotFound {
		t.Errorf("IsComplete = %v, want ErrSwarmNotFound", err)
	}
}

// TestLockSwarm tests that lockSwarm creates lock files and provides mutual exclusion.
func TestLockSwarm(t *testing.T) {
	tmpDir := t.TempDir()
	r := &rig.Rig{
		Name: "test-rig",
		Path: tmpDir,
	}
	m := NewManager(r)

	// Acquire lock
	fl, err := m.lockSwarm("test-swarm")
	if err != nil {
		t.Fatalf("lockSwarm failed: %v", err)
	}

	// Verify lock file exists
	lockPath := filepath.Join(tmpDir, ".swarm-locks", "test-swarm.lock")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("lock file should exist after locking")
	}

	// Release lock
	if err := fl.Unlock(); err != nil {
		t.Fatalf("Unlock failed: %v", err)
	}
}

// TestLockSwarmMutualExclusion tests that two goroutines cannot hold the same lock simultaneously.
func TestLockSwarmMutualExclusion(t *testing.T) {
	tmpDir := t.TempDir()
	r := &rig.Rig{
		Name: "test-rig",
		Path: tmpDir,
	}
	m := NewManager(r)

	// This test verifies that locking serializes concurrent access.
	// We use a shared counter protected only by the file lock.
	var mu sync.Mutex
	var maxConcurrent int
	var current int

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			fl, err := m.lockSwarm("test-swarm")
			if err != nil {
				t.Errorf("lockSwarm failed: %v", err)
				return
			}

			mu.Lock()
			current++
			if current > maxConcurrent {
				maxConcurrent = current
			}
			mu.Unlock()

			// Simulate work
			mu.Lock()
			current--
			mu.Unlock()

			_ = fl.Unlock()
		}()
	}

	wg.Wait()

	if maxConcurrent > 1 {
		t.Errorf("max concurrent holders = %d, want 1 (lock not providing mutual exclusion)", maxConcurrent)
	}
}

// TestLockSwarmDifferentSwarms tests that locks for different swarms are independent.
func TestLockSwarmDifferentSwarms(t *testing.T) {
	tmpDir := t.TempDir()
	r := &rig.Rig{
		Name: "test-rig",
		Path: tmpDir,
	}
	m := NewManager(r)

	// Lock swarm A
	flA, err := m.lockSwarm("swarm-a")
	if err != nil {
		t.Fatalf("lockSwarm(swarm-a) failed: %v", err)
	}

	// Lock swarm B should succeed (different swarm)
	flB, err := m.lockSwarm("swarm-b")
	if err != nil {
		t.Fatalf("lockSwarm(swarm-b) failed: %v", err)
	}

	_ = flB.Unlock()
	_ = flA.Unlock()
}

// TestSwarmE2ELifecycle documents the end-to-end swarm integration test protocol.
// This test documents the manual testing steps that were validated for gt-kc7yj.4.
//
// The test scenario creates a DAG of work:
//
//	     A
//	    / \
//	   B   C
//	    \ /
//	     D
//
// Test Results (verified 2025-12-29):
//
// 1. CREATE EPIC WITH DEPENDENCIES
//
//	bd create --type=epic --title="Test Epic"         → gt-xxxxx
//	bd create --type=task --title="Task A" --parent=gt-xxxxx  → gt-xxxxx.1
//	bd create --type=task --title="Task B" --parent=gt-xxxxx  → gt-xxxxx.2
//	bd create --type=task --title="Task C" --parent=gt-xxxxx  → gt-xxxxx.3
//	bd create --type=task --title="Task D" --parent=gt-xxxxx  → gt-xxxxx.4
//	bd dep add gt-xxxxx.2 gt-xxxxx.1  # B depends on A
//	bd dep add gt-xxxxx.3 gt-xxxxx.1  # C depends on A
//	bd dep add gt-xxxxx.4 gt-xxxxx.2  # D depends on B
//	bd dep add gt-xxxxx.4 gt-xxxxx.3  # D depends on C
//
// 2. VALIDATE SWARM STRUCTURE ✅
//
//	bd swarm validate gt-xxxxx
//	Expected output:
//	  Wave 1: 1 issue (Task A)
//	  Wave 2: 2 issues (Tasks B, C - parallel)
//	  Wave 3: 1 issue (Task D)
//	  Max parallelism: 2
//	  Swarmable: YES
//
// 3. CREATE SWARM MOLECULE ✅
//
//	bd swarm create gt-xxxxx
//	Expected: Creates molecule with mol_type=swarm linked to epic
//
// 4. VERIFY READY FRONT ✅
//
//	bd swarm status gt-xxxxx
//	Expected:
//	  Ready: Task A
//	  Blocked: Tasks B, C, D (with dependency info)
//
// 5. ISSUE COMPLETION ADVANCES FRONT ✅
//
//	bd close gt-xxxxx.1 --reason "Complete"
//	bd swarm status gt-xxxxx
//	Expected:
//	  Completed: Task A
//	  Ready: Tasks B, C (now unblocked)
//	  Blocked: Task D
//
// 6. PARALLEL WORK ✅
//
//	bd close gt-xxxxx.2 gt-xxxxx.3 --reason "Complete"
//	bd swarm status gt-xxxxx
//	Expected:
//	  Completed: Tasks A, B, C
//	  Ready: Task D (now unblocked)
//
// 7. FINAL COMPLETION ✅
//
//	bd close gt-xxxxx.4 --reason "Complete"
//	bd swarm status gt-xxxxx
//	Expected: Progress 4/4 complete (100%)
//
// 8. SWARM AUTO-CLOSE ⚠️
//
//	The swarm and epic remain open after all tasks complete.
//	This is by design - the Witness coordinator is responsible for
//	detecting completion and closing the swarm molecule.
//	Manual close: bd close gt-xxxxx gt-yyyyy --reason "Swarm complete"
//
// KNOWN ISSUES:
//   - gt swarm status/land fail to find issues (filed as gt-594a4)
//   - bd swarm commands work correctly as the underlying implementation
//   - Auto-close requires Witness patrol (not automatic in beads)
func TestSwarmE2ELifecycle(t *testing.T) {
	// This test documents the manual E2E testing protocol.
	// The actual test requires beads infrastructure and is run manually.
	// See the docstring above for the complete test procedure.
	t.Skip("E2E test requires beads infrastructure - see docstring for manual test protocol")
}
