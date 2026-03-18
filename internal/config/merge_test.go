package config

import (
	"os"
	"path/filepath"
	"testing"
)

// helper: create temp dir with .pgit for merge state
func setupMergeRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".pgit"), 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// T21: MergeState persistence
func TestMergeStateAddRemoveConflicts(t *testing.T) {
	tests := []struct {
		name            string
		addConflicts    []string
		removeConflicts []string
		wantConflicts   []string
		wantHas         bool
	}{
		{
			name:          "no conflicts",
			addConflicts:  nil,
			wantConflicts: nil,
			wantHas:       false,
		},
		{
			name:          "add single conflict",
			addConflicts:  []string{"file.sql"},
			wantConflicts: []string{"file.sql"},
			wantHas:       true,
		},
		{
			name:          "add multiple conflicts",
			addConflicts:  []string{"a.sql", "b.sql", "c.sql"},
			wantConflicts: []string{"a.sql", "b.sql", "c.sql"},
			wantHas:       true,
		},
		{
			name:            "add and remove conflict",
			addConflicts:    []string{"a.sql", "b.sql"},
			removeConflicts: []string{"a.sql"},
			wantConflicts:   []string{"b.sql"},
			wantHas:         true,
		},
		{
			name:            "remove all conflicts",
			addConflicts:    []string{"a.sql"},
			removeConflicts: []string{"a.sql"},
			wantConflicts:   nil,
			wantHas:         false,
		},
		{
			name:         "duplicate add is idempotent",
			addConflicts: []string{"file.sql", "file.sql", "file.sql"},
			wantConflicts: []string{"file.sql"},
			wantHas:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &MergeState{InProgress: true}

			for _, path := range tt.addConflicts {
				ms.AddConflict(path)
			}
			for _, path := range tt.removeConflicts {
				ms.RemoveConflict(path)
			}

			if got := ms.HasConflicts(); got != tt.wantHas {
				t.Errorf("HasConflicts() = %v, want %v", got, tt.wantHas)
			}

			if len(ms.ConflictedFiles) != len(tt.wantConflicts) {
				t.Fatalf("ConflictedFiles = %v, want %v", ms.ConflictedFiles, tt.wantConflicts)
			}
			for i, want := range tt.wantConflicts {
				if ms.ConflictedFiles[i] != want {
					t.Errorf("ConflictedFiles[%d] = %q, want %q", i, ms.ConflictedFiles[i], want)
				}
			}
		})
	}
}

func TestMergeStateSaveLoadRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		state MergeState
	}{
		{
			name: "active merge with conflicts",
			state: MergeState{
				InProgress:      true,
				RemoteName:      "origin",
				RemoteCommitID:  "abc123",
				LocalCommitID:   "def456",
				CommonAncestor:  "aaa000",
				ConflictedFiles: []string{"schema/users.sql", "data/seed.sql"},
			},
		},
		{
			name: "active merge no conflicts",
			state: MergeState{
				InProgress:     true,
				RemoteName:     "upstream",
				RemoteCommitID: "fff999",
				LocalCommitID:  "111222",
				CommonAncestor: "000111",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupMergeRepo(t)

			if err := tt.state.Save(dir); err != nil {
				t.Fatalf("Save: %v", err)
			}

			loaded, err := LoadMergeState(dir)
			if err != nil {
				t.Fatalf("LoadMergeState: %v", err)
			}

			if loaded.InProgress != tt.state.InProgress {
				t.Errorf("InProgress = %v, want %v", loaded.InProgress, tt.state.InProgress)
			}
			if loaded.RemoteName != tt.state.RemoteName {
				t.Errorf("RemoteName = %q, want %q", loaded.RemoteName, tt.state.RemoteName)
			}
			if loaded.RemoteCommitID != tt.state.RemoteCommitID {
				t.Errorf("RemoteCommitID = %q, want %q", loaded.RemoteCommitID, tt.state.RemoteCommitID)
			}
			if loaded.LocalCommitID != tt.state.LocalCommitID {
				t.Errorf("LocalCommitID = %q, want %q", loaded.LocalCommitID, tt.state.LocalCommitID)
			}
			if loaded.CommonAncestor != tt.state.CommonAncestor {
				t.Errorf("CommonAncestor = %q, want %q", loaded.CommonAncestor, tt.state.CommonAncestor)
			}
			if len(loaded.ConflictedFiles) != len(tt.state.ConflictedFiles) {
				t.Fatalf("ConflictedFiles len = %d, want %d", len(loaded.ConflictedFiles), len(tt.state.ConflictedFiles))
			}
			for i, want := range tt.state.ConflictedFiles {
				if loaded.ConflictedFiles[i] != want {
					t.Errorf("ConflictedFiles[%d] = %q, want %q", i, loaded.ConflictedFiles[i], want)
				}
			}
		})
	}
}

func TestMergeStateClearDeletesFile(t *testing.T) {
	dir := setupMergeRepo(t)

	ms := &MergeState{
		InProgress:      true,
		RemoteName:      "origin",
		RemoteCommitID:  "abc",
		ConflictedFiles: []string{"file.sql"},
	}

	if err := ms.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file exists
	statePath := MergeStatePath(dir)
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("merge state file should exist after Save: %v", err)
	}

	// Clear should delete the file
	if err := ms.Clear(dir); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Error("merge state file should not exist after Clear")
	}

	if ms.InProgress {
		t.Error("InProgress should be false after Clear")
	}
	if ms.ConflictedFiles != nil {
		t.Error("ConflictedFiles should be nil after Clear")
	}
}

func TestMergeStateLoadNonExistent(t *testing.T) {
	dir := setupMergeRepo(t)

	// Loading from a repo with no merge state should return empty state
	ms, err := LoadMergeState(dir)
	if err != nil {
		t.Fatalf("LoadMergeState: %v", err)
	}
	if ms.InProgress {
		t.Error("InProgress should be false for nonexistent merge state")
	}
}
