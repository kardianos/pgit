package repo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/imgajeed76/pgit/v4/internal/config"
	"github.com/imgajeed76/pgit/v4/internal/testdb"
	"github.com/imgajeed76/pgit/v4/internal/util"
)

// initTestRepo creates a fresh repo in a temp dir with a DB connection.
func initTestRepo(t *testing.T) *Repository {
	t.Helper()

	dir := t.TempDir()

	// Create .pgit directory and config manually so we don't need a container for Init
	pgitDir := filepath.Join(dir, ".pgit")
	if err := os.MkdirAll(pgitDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultConfig(dir)
	cfg.User.Name = "Test User"
	cfg.User.Email = "test@example.com"
	if err := cfg.Save(dir); err != nil {
		t.Fatal(err)
	}

	idx := config.NewIndex()
	if err := idx.Save(dir); err != nil {
		t.Fatal(err)
	}

	d := testdb.Acquire(t)

	repo := &Repository{
		Root:   dir,
		Config: cfg,
		DB:     d,
	}

	return repo
}

// writeFile creates a file in the repo.
func writeFile(t *testing.T, repo *Repository, relPath, content string) {
	t.Helper()
	abs := repo.AbsPath(relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// removeFile removes a file from the repo.
func removeFile(t *testing.T, repo *Repository, relPath string) {
	t.Helper()
	abs := repo.AbsPath(relPath)
	if err := os.Remove(abs); err != nil {
		t.Fatal(err)
	}
}

// T63: StageFile / GetStagedChanges
func TestStageFileAndGetStagedChanges(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, r *Repository)
		stagePaths []string
		wantCount  int
		wantStatus []ChangeStatus
	}{
		{
			name: "stage new file",
			setup: func(t *testing.T, r *Repository) {
				writeFile(t, r, "new.txt", "hello")
			},
			stagePaths: []string{"new.txt"},
			wantCount:  1,
			wantStatus: []ChangeStatus{StatusNew},
		},
		{
			name: "stage modified file",
			setup: func(t *testing.T, r *Repository) {
				// Create a file, commit it, then modify
				writeFile(t, r, "mod.txt", "original")
				ctx := context.Background()
				if err := r.StageFile(ctx, "mod.txt"); err != nil {
					t.Fatal(err)
				}
				_, err := r.Commit(ctx, CommitOptions{Message: "initial"})
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, r, "mod.txt", "modified")
			},
			stagePaths: []string{"mod.txt"},
			wantCount:  1,
			wantStatus: []ChangeStatus{StatusModified},
		},
		{
			name: "stage deleted file",
			setup: func(t *testing.T, r *Repository) {
				writeFile(t, r, "del.txt", "gone")
				ctx := context.Background()
				if err := r.StageFile(ctx, "del.txt"); err != nil {
					t.Fatal(err)
				}
				_, err := r.Commit(ctx, CommitOptions{Message: "initial"})
				if err != nil {
					t.Fatal(err)
				}
				removeFile(t, r, "del.txt")
			},
			stagePaths: []string{"del.txt"},
			wantCount:  1,
			wantStatus: []ChangeStatus{StatusDeleted},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := initTestRepo(t)
			tt.setup(t, r)

			ctx := context.Background()
			for _, p := range tt.stagePaths {
				if err := r.StageFile(ctx, p); err != nil {
					t.Fatalf("StageFile(%q): %v", p, err)
				}
			}

			changes, err := r.GetStagedChanges(ctx)
			if err != nil {
				t.Fatalf("GetStagedChanges: %v", err)
			}

			if len(changes) != tt.wantCount {
				t.Fatalf("got %d staged changes, want %d", len(changes), tt.wantCount)
			}

			for i, want := range tt.wantStatus {
				if changes[i].Status != want {
					t.Errorf("change[%d].Status = %v, want %v", i, changes[i].Status, want)
				}
			}
		})
	}
}

// T64: UnstageFile / UnstageAll
func TestUnstageFileAndUnstageAll(t *testing.T) {
	tests := []struct {
		name          string
		unstageFile   string // if non-empty, unstage this single file
		unstageAll    bool   // if true, unstage all
		wantRemaining int
	}{
		{
			name:          "unstage one of three",
			unstageFile:   "b.txt",
			wantRemaining: 2,
		},
		{
			name:          "unstage all",
			unstageAll:    true,
			wantRemaining: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := initTestRepo(t)
			ctx := context.Background()

			// Create and stage 3 files
			for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
				writeFile(t, r, name, "content of "+name)
				if err := r.StageFile(ctx, name); err != nil {
					t.Fatalf("StageFile(%q): %v", name, err)
				}
			}

			if tt.unstageAll {
				if err := r.UnstageAll(); err != nil {
					t.Fatalf("UnstageAll: %v", err)
				}
			} else {
				if err := r.UnstageFile(tt.unstageFile); err != nil {
					t.Fatalf("UnstageFile(%q): %v", tt.unstageFile, err)
				}
			}

			changes, err := r.GetStagedChanges(ctx)
			if err != nil {
				t.Fatalf("GetStagedChanges: %v", err)
			}

			if len(changes) != tt.wantRemaining {
				t.Errorf("got %d staged changes, want %d", len(changes), tt.wantRemaining)
			}

			// Verify which files remain (for the unstage-one case)
			if tt.unstageFile != "" && tt.wantRemaining > 0 {
				paths := make(map[string]bool)
				for _, c := range changes {
					paths[c.Path] = true
				}
				if paths[tt.unstageFile] {
					t.Errorf("unstaged file %q still in staged changes", tt.unstageFile)
				}
				// Verify the other files ARE present
				for _, name := range []string{"a.txt", "c.txt"} {
					if !paths[name] {
						t.Errorf("expected file %q still staged, but missing", name)
					}
				}
			}
		})
	}
}

// T65: StageAll
func TestStageAll(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, r *Repository)
		wantCount int
	}{
		{
			name: "stage all new files",
			setup: func(t *testing.T, r *Repository) {
				writeFile(t, r, "a.txt", "aaa")
				writeFile(t, r, "b.txt", "bbb")
			},
			wantCount: 2,
		},
		{
			name: "stage mixed create/modify/delete",
			setup: func(t *testing.T, r *Repository) {
				// Create and commit a file first
				writeFile(t, r, "existing.txt", "original")
				writeFile(t, r, "to_delete.txt", "will be deleted")
				ctx := context.Background()
				if err := r.StageFile(ctx, "existing.txt"); err != nil {
					t.Fatal(err)
				}
				if err := r.StageFile(ctx, "to_delete.txt"); err != nil {
					t.Fatal(err)
				}
				if _, err := r.Commit(ctx, CommitOptions{Message: "initial"}); err != nil {
					t.Fatal(err)
				}

				// Now create, modify, delete
				writeFile(t, r, "new.txt", "new file")
				writeFile(t, r, "existing.txt", "modified content")
				removeFile(t, r, "to_delete.txt")
			},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := initTestRepo(t)
			tt.setup(t, r)

			ctx := context.Background()
			count, err := r.StageAll(ctx)
			if err != nil {
				t.Fatalf("StageAll: %v", err)
			}
			if count != tt.wantCount {
				t.Errorf("StageAll returned count %d, want %d", count, tt.wantCount)
			}

			changes, err := r.GetStagedChanges(ctx)
			if err != nil {
				t.Fatalf("GetStagedChanges: %v", err)
			}
			if len(changes) != tt.wantCount {
				t.Errorf("got %d staged changes, want %d", len(changes), tt.wantCount)
			}
		})
	}
}

// T66: StageDelete
func TestStageDelete(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "stage deletion for tracked file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := initTestRepo(t)
			ctx := context.Background()

			// Create, stage, commit a file
			writeFile(t, r, "tracked.txt", "some content")
			if err := r.StageFile(ctx, "tracked.txt"); err != nil {
				t.Fatal(err)
			}
			if _, err := r.Commit(ctx, CommitOptions{Message: "add tracked.txt"}); err != nil {
				t.Fatal(err)
			}

			// Stage its deletion
			if err := r.StageDelete("tracked.txt"); err != nil {
				t.Fatalf("StageDelete: %v", err)
			}

			// Verify it appears as deleted in the index
			idx, err := r.LoadIndex()
			if err != nil {
				t.Fatal(err)
			}
			entry, ok := idx.Get("tracked.txt")
			if !ok {
				t.Fatal("tracked.txt not found in index after StageDelete")
			}
			if entry.Status != config.StatusDeleted {
				t.Errorf("entry.Status = %q, want %q", entry.Status, config.StatusDeleted)
			}

			// Suppress unused variable warnings
			_ = util.ErrNothingStaged
		})
	}
}
