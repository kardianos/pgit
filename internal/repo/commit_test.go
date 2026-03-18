package repo

import (
	"context"
	"testing"

	"github.com/imgajeed76/pgit/v4/internal/util"
)

// T67: Commit creates commit and clears index
func TestCommitCreatesCommitAndClearsIndex(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "commit staged files clears index and updates HEAD"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := initTestRepo(t)
			ctx := context.Background()

			writeFile(t, r, "file1.txt", "hello")
			writeFile(t, r, "file2.txt", "world")

			if err := r.StageFile(ctx, "file1.txt"); err != nil {
				t.Fatal(err)
			}
			if err := r.StageFile(ctx, "file2.txt"); err != nil {
				t.Fatal(err)
			}

			commit, err := r.Commit(ctx, CommitOptions{Message: "initial commit"})
			if err != nil {
				t.Fatalf("Commit: %v", err)
			}

			if commit.ID == "" {
				t.Error("commit.ID is empty")
			}
			if commit.Message != "initial commit" {
				t.Errorf("commit.Message = %q, want %q", commit.Message, "initial commit")
			}

			// Verify HEAD is updated
			headID, err := r.DB().GetHead(ctx)
			if err != nil {
				t.Fatalf("GetHead: %v", err)
			}
			if headID != commit.ID {
				t.Errorf("HEAD = %q, want %q", headID, commit.ID)
			}

			// Verify index is empty
			idx, err := r.LoadIndex()
			if err != nil {
				t.Fatal(err)
			}
			if !idx.IsEmpty() {
				t.Error("index should be empty after commit")
			}
		})
	}
}

// T68: Commit with no staged changes
func TestCommitWithNoStagedChanges(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "commit with empty index returns error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := initTestRepo(t)
			ctx := context.Background()

			_, err := r.Commit(ctx, CommitOptions{Message: "empty"})
			if err == nil {
				t.Fatal("expected error for empty commit, got nil")
			}
			if err != util.ErrNothingStaged {
				t.Errorf("got error %v, want ErrNothingStaged", err)
			}
		})
	}
}

// T69: Sequential commits
func TestSequentialCommits(t *testing.T) {
	tests := []struct {
		name       string
		numCommits int
	}{
		{name: "three sequential commits", numCommits: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := initTestRepo(t)
			ctx := context.Background()

			var commitIDs []string
			for i := 0; i < tt.numCommits; i++ {
				fname := "file.txt"
				writeFile(t, r, fname, "content v"+string(rune('0'+i)))
				if err := r.StageFile(ctx, fname); err != nil {
					t.Fatalf("stage %d: %v", i, err)
				}
				c, err := r.Commit(ctx, CommitOptions{
					Message: "commit " + string(rune('0'+i)),
				})
				if err != nil {
					t.Fatalf("commit %d: %v", i, err)
				}
				commitIDs = append(commitIDs, c.ID)
			}

			// Verify log shows all commits
			log, err := r.DB().GetCommitLog(ctx, 10)
			if err != nil {
				t.Fatalf("GetCommitLog: %v", err)
			}
			if len(log) != tt.numCommits {
				t.Errorf("log has %d commits, want %d", len(log), tt.numCommits)
			}

			// Most recent first
			if log[0].ID != commitIDs[tt.numCommits-1] {
				t.Errorf("most recent commit = %q, want %q", log[0].ID, commitIDs[tt.numCommits-1])
			}
		})
	}
}
