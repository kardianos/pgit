package repo

import (
	"context"
	"testing"
)

// T70: GetWorkingTreeChanges
func TestGetWorkingTreeChanges(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, r *Repository)
		wantNew    int
		wantMod    int
		wantDel    int
	}{
		{
			name: "new file detected",
			setup: func(t *testing.T, r *Repository) {
				writeFile(t, r, "new.txt", "new content")
			},
			wantNew: 1,
		},
		{
			name: "modified file detected",
			setup: func(t *testing.T, r *Repository) {
				writeFile(t, r, "mod.txt", "original")
				ctx := context.Background()
				if err := r.StageFile(ctx, "mod.txt"); err != nil {
					t.Fatal(err)
				}
				if _, err := r.Commit(ctx, CommitOptions{Message: "init"}); err != nil {
					t.Fatal(err)
				}
				writeFile(t, r, "mod.txt", "changed")
			},
			wantMod: 1,
		},
		{
			name: "deleted file detected",
			setup: func(t *testing.T, r *Repository) {
				writeFile(t, r, "del.txt", "bye")
				ctx := context.Background()
				if err := r.StageFile(ctx, "del.txt"); err != nil {
					t.Fatal(err)
				}
				if _, err := r.Commit(ctx, CommitOptions{Message: "init"}); err != nil {
					t.Fatal(err)
				}
				removeFile(t, r, "del.txt")
			},
			wantDel: 1,
		},
		{
			name: "mixed changes",
			setup: func(t *testing.T, r *Repository) {
				writeFile(t, r, "keep.txt", "keep")
				writeFile(t, r, "remove.txt", "remove")
				ctx := context.Background()
				if err := r.StageFile(ctx, "keep.txt"); err != nil {
					t.Fatal(err)
				}
				if err := r.StageFile(ctx, "remove.txt"); err != nil {
					t.Fatal(err)
				}
				if _, err := r.Commit(ctx, CommitOptions{Message: "init"}); err != nil {
					t.Fatal(err)
				}
				writeFile(t, r, "keep.txt", "modified keep")
				removeFile(t, r, "remove.txt")
				writeFile(t, r, "brand_new.txt", "brand new")
			},
			wantNew: 1,
			wantMod: 1,
			wantDel: 1,
		},
		{
			name: "no changes on clean tree",
			setup: func(t *testing.T, r *Repository) {
				writeFile(t, r, "clean.txt", "clean")
				ctx := context.Background()
				if err := r.StageFile(ctx, "clean.txt"); err != nil {
					t.Fatal(err)
				}
				if _, err := r.Commit(ctx, CommitOptions{Message: "init"}); err != nil {
					t.Fatal(err)
				}
			},
			wantNew: 0,
			wantMod: 0,
			wantDel: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := initTestRepo(t)
			tt.setup(t, r)

			ctx := context.Background()
			changes, err := r.GetWorkingTreeChanges(ctx)
			if err != nil {
				t.Fatalf("GetWorkingTreeChanges: %v", err)
			}

			var gotNew, gotMod, gotDel int
			for _, c := range changes {
				switch c.Status {
				case StatusNew:
					gotNew++
				case StatusModified:
					gotMod++
				case StatusDeleted:
					gotDel++
				}
			}

			if gotNew != tt.wantNew {
				t.Errorf("new files: got %d, want %d", gotNew, tt.wantNew)
			}
			if gotMod != tt.wantMod {
				t.Errorf("modified files: got %d, want %d", gotMod, tt.wantMod)
			}
			if gotDel != tt.wantDel {
				t.Errorf("deleted files: got %d, want %d", gotDel, tt.wantDel)
			}
		})
	}
}

// T71: GetUnstagedChanges
func TestGetUnstagedChanges(t *testing.T) {
	// Modify files after staging, only non-staged mods returned
	t.Run("only non-staged modifications returned", func(t *testing.T) {
		r := initTestRepo(t)
		ctx := context.Background()

		// Create and commit two files
		writeFile(t, r, "staged.txt", "original")
		writeFile(t, r, "not_staged.txt", "original")
		if err := r.StageFile(ctx, "staged.txt"); err != nil {
			t.Fatal(err)
		}
		if err := r.StageFile(ctx, "not_staged.txt"); err != nil {
			t.Fatal(err)
		}
		if _, err := r.Commit(ctx, CommitOptions{Message: "init"}); err != nil {
			t.Fatal(err)
		}

		// Modify both files
		writeFile(t, r, "staged.txt", "modified")
		writeFile(t, r, "not_staged.txt", "modified")

		// Stage only one
		if err := r.StageFile(ctx, "staged.txt"); err != nil {
			t.Fatal(err)
		}

		unstaged, err := r.GetUnstagedChanges(ctx)
		if err != nil {
			t.Fatalf("GetUnstagedChanges: %v", err)
		}

		if len(unstaged) != 1 {
			t.Fatalf("got %d unstaged changes, want 1", len(unstaged))
		}
		if unstaged[0].Path != "not_staged.txt" {
			t.Errorf("unstaged path = %q, want %q", unstaged[0].Path, "not_staged.txt")
		}
	})
}
