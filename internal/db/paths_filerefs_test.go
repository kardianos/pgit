package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/imgajeed76/pgit/v4/internal/testdb"
)

// T36: GetOrCreatePath — idempotent creation
func TestGetOrCreatePath(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		path    string
		groupID int32
	}{
		{name: "simple_file", path: "README.md", groupID: 1},
		{name: "nested_path", path: "src/main.go", groupID: 2},
		{name: "deep_path", path: "a/b/c/d.txt", groupID: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pathID1, groupID1, err := d.GetOrCreatePath(ctx, tt.path, tt.groupID)
			if err != nil {
				t.Fatalf("first GetOrCreatePath: %v", err)
			}
			if pathID1 == 0 {
				t.Error("pathID should not be 0")
			}
			if groupID1 != tt.groupID {
				t.Errorf("groupID = %d, want %d", groupID1, tt.groupID)
			}

			// Second call should return same IDs (idempotent)
			pathID2, groupID2, err := d.GetOrCreatePath(ctx, tt.path, 999)
			if err != nil {
				t.Fatalf("second GetOrCreatePath: %v", err)
			}
			if pathID2 != pathID1 {
				t.Errorf("pathID changed: %d != %d", pathID2, pathID1)
			}
			if groupID2 != groupID1 {
				t.Errorf("groupID changed: %d != %d (should ignore new groupID)", groupID2, groupID1)
			}
		})
	}
}

// T37: GetOrCreatePathsBatch
func TestGetOrCreatePathsBatch(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	tests := []struct {
		name  string
		paths []string
	}{
		{
			name:  "batch_of_three",
			paths: []string{"file_a.txt", "file_b.txt", "dir/file_c.txt"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pathToGroup := make(map[string]int32)
			for i, p := range tt.paths {
				pathToGroup[p] = int32(i + 100)
			}

			result, err := d.GetOrCreatePathsBatch(ctx, tt.paths, pathToGroup)
			if err != nil {
				t.Fatalf("GetOrCreatePathsBatch: %v", err)
			}
			if len(result) != len(tt.paths) {
				t.Fatalf("got %d results, want %d", len(result), len(tt.paths))
			}

			for _, p := range tt.paths {
				ids, ok := result[p]
				if !ok {
					t.Errorf("path %q not in result", p)
					continue
				}
				if ids.PathID == 0 {
					t.Errorf("path %q has pathID 0", p)
				}
			}

			// Call again — should be idempotent
			result2, err := d.GetOrCreatePathsBatch(ctx, tt.paths, pathToGroup)
			if err != nil {
				t.Fatalf("second GetOrCreatePathsBatch: %v", err)
			}
			for _, p := range tt.paths {
				if result2[p].PathID != result[p].PathID {
					t.Errorf("path %q: pathID changed on second call", p)
				}
			}
		})
	}
}

// T38: CreateFileRef / GetFileRefsAtCommit
func TestCreateFileRefAndGetAtCommit(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	// Create prerequisite: a path and a commit
	pathID, _, err := d.GetOrCreatePath(ctx, "test_file.go", 10)
	if err != nil {
		t.Fatalf("GetOrCreatePath: %v", err)
	}
	commitID := "01FILEREF0000000000000001AA"
	c := makeCommit(commitID, nil, "commit for filerefs", 0)
	if err := d.CreateCommit(ctx, c); err != nil {
		t.Fatalf("CreateCommit: %v", err)
	}

	tests := []struct {
		name string
		ref  *db.FileRef
	}{
		{
			name: "normal_file_ref",
			ref: &db.FileRef{
				PathID:      pathID,
				CommitID:    commitID,
				VersionID:   1,
				ContentHash: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
				Mode:        33188,
				IsSymlink:   false,
				IsBinary:    false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := d.CreateFileRef(ctx, tt.ref); err != nil {
				t.Fatalf("CreateFileRef: %v", err)
			}

			refs, err := d.GetFileRefsAtCommit(ctx, commitID)
			if err != nil {
				t.Fatalf("GetFileRefsAtCommit: %v", err)
			}
			if len(refs) != 1 {
				t.Fatalf("got %d refs, want 1", len(refs))
			}

			got := refs[0]
			if got.PathID != tt.ref.PathID {
				t.Errorf("PathID = %d, want %d", got.PathID, tt.ref.PathID)
			}
			if got.CommitID != tt.ref.CommitID {
				t.Errorf("CommitID = %q, want %q", got.CommitID, tt.ref.CommitID)
			}
			if got.VersionID != tt.ref.VersionID {
				t.Errorf("VersionID = %d, want %d", got.VersionID, tt.ref.VersionID)
			}
			if got.Mode != tt.ref.Mode {
				t.Errorf("Mode = %d, want %d", got.Mode, tt.ref.Mode)
			}
		})
	}
}

// T39: GetChangedFileRefs
func TestGetChangedFileRefs(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	pathID1, _, err := d.GetOrCreatePath(ctx, "changed_a.txt", 20)
	if err != nil {
		t.Fatalf("GetOrCreatePath(changed_a.txt): %v", err)
	}
	pathID2, _, err := d.GetOrCreatePath(ctx, "changed_b.txt", 21)
	if err != nil {
		t.Fatalf("GetOrCreatePath(changed_b.txt): %v", err)
	}

	commit1 := "01CHANGED000000000000000001"
	commit2 := "01CHANGED000000000000000002"
	for _, c := range []*db.Commit{
		makeCommit(commit1, nil, "c1", 0),
		makeCommit(commit2, strPtr(commit1), "c2", 1),
	} {
		if err := d.CreateCommit(ctx, c); err != nil {
			t.Fatalf("CreateCommit: %v", err)
		}
	}

	// File A changed in commit1, File B changed in commit2
	if err := d.CreateFileRef(ctx, &db.FileRef{PathID: pathID1, CommitID: commit1, VersionID: 1, ContentHash: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, Mode: 33188}); err != nil {
		t.Fatalf("CreateFileRef(A, c1): %v", err)
	}
	if err := d.CreateFileRef(ctx, &db.FileRef{PathID: pathID2, CommitID: commit2, VersionID: 1, ContentHash: []byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17}, Mode: 33188}); err != nil {
		t.Fatalf("CreateFileRef(B, c2): %v", err)
	}

	tests := []struct {
		name      string
		from      string
		to        string
		wantCount int
	}{
		{name: "changes_between_c1_and_c2", from: commit1, to: commit2, wantCount: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs, err := d.GetChangedFileRefs(ctx, tt.from, tt.to)
			if err != nil {
				t.Fatalf("GetChangedFileRefs: %v", err)
			}
			if len(refs) != tt.wantCount {
				t.Errorf("got %d changed refs, want %d", len(refs), tt.wantCount)
			}
		})
	}
}

// T40: GetFileRefHistory
func TestGetFileRefHistory(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	pathID, _, _ := d.GetOrCreatePath(ctx, "history_file.txt", 30)

	commitIDs := []string{
		"01HISTORY000000000000000001",
		"01HISTORY000000000000000002",
		"01HISTORY000000000000000003",
	}
	var parent *string
	for i, id := range commitIDs {
		c := makeCommit(id, parent, "history "+id, i)
		if err := d.CreateCommit(ctx, c); err != nil {
			t.Fatalf("CreateCommit: %v", err)
		}
		parent = strPtr(id)

		hash := make([]byte, 16)
		hash[0] = byte(i + 1)
		ref := &db.FileRef{PathID: pathID, CommitID: id, VersionID: int32(i + 1), ContentHash: hash, Mode: 33188}
		if err := d.CreateFileRef(ctx, ref); err != nil {
			t.Fatalf("CreateFileRef: %v", err)
		}
	}

	tests := []struct {
		name      string
		wantCount int
	}{
		{name: "three_versions", wantCount: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history, err := d.GetFileRefHistory(ctx, pathID)
			if err != nil {
				t.Fatalf("GetFileRefHistory: %v", err)
			}
			if len(history) != tt.wantCount {
				t.Errorf("got %d history entries, want %d", len(history), tt.wantCount)
			}
			// Should be in DESC commit_id order
			if len(history) >= 2 && history[0].CommitID < history[1].CommitID {
				t.Error("history should be in descending commit_id order")
			}
		})
	}
}

// T41: GetTreeRefsAtCommit
func TestGetTreeRefsAtCommit(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	pathA, _, err := d.GetOrCreatePath(ctx, "tree_a.txt", 40)
	if err != nil {
		t.Fatalf("GetOrCreatePath(tree_a.txt): %v", err)
	}
	pathB, _, err := d.GetOrCreatePath(ctx, "tree_b.txt", 41)
	if err != nil {
		t.Fatalf("GetOrCreatePath(tree_b.txt): %v", err)
	}

	commit1 := "01TREE00000000000000000001"
	commit2 := "01TREE00000000000000000002"

	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := d.CreateCommit(ctx, &db.Commit{
		ID: commit1, TreeHash: "t1", Message: "c1",
		AuthorName: "A", AuthorEmail: "a@x.com", AuthoredAt: ts,
		CommitterName: "A", CommitterEmail: "a@x.com", CommittedAt: ts,
	}); err != nil {
		t.Fatalf("CreateCommit(c1): %v", err)
	}
	if err := d.CreateCommit(ctx, &db.Commit{
		ID: commit2, ParentID: strPtr(commit1), TreeHash: "t2", Message: "c2",
		AuthorName: "A", AuthorEmail: "a@x.com", AuthoredAt: ts.Add(time.Minute),
		CommitterName: "A", CommitterEmail: "a@x.com", CommittedAt: ts.Add(time.Minute),
	}); err != nil {
		t.Fatalf("CreateCommit(c2): %v", err)
	}

	// File A created in commit1
	if err := d.CreateFileRef(ctx, &db.FileRef{PathID: pathA, CommitID: commit1, VersionID: 1, ContentHash: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, Mode: 33188}); err != nil {
		t.Fatalf("CreateFileRef(A, c1): %v", err)
	}
	// File B created in commit2
	if err := d.CreateFileRef(ctx, &db.FileRef{PathID: pathB, CommitID: commit2, VersionID: 1, ContentHash: []byte{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17}, Mode: 33188}); err != nil {
		t.Fatalf("CreateFileRef(B, c2): %v", err)
	}

	tests := []struct {
		name      string
		commitID  string
		wantCount int
	}{
		{name: "tree_at_commit1_has_one_file", commitID: commit1, wantCount: 1},
		{name: "tree_at_commit2_has_two_files", commitID: commit2, wantCount: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs, err := d.GetTreeRefsAtCommit(ctx, tt.commitID)
			if err != nil {
				t.Fatalf("GetTreeRefsAtCommit: %v", err)
			}
			if len(refs) != tt.wantCount {
				t.Errorf("got %d refs, want %d", len(refs), tt.wantCount)
			}
		})
	}
}
