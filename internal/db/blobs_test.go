package db_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/imgajeed76/pgit/v4/internal/testdb"
	"github.com/imgajeed76/pgit/v4/internal/util"
)

// T46: CreateBlob / GetBlob round-trip
func TestCreateGetBlobRoundTrip(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	commitID := "01BLOB0000000000000000000001"
	c := makeCommit(commitID, nil, "blob test commit", 0)
	if err := d.CreateCommit(ctx, c); err != nil {
		t.Fatalf("CreateCommit: %v", err)
	}

	tests := []struct {
		name string
		blob *db.Blob
	}{
		{
			name: "text_file",
			blob: &db.Blob{
				Path:        "hello.txt",
				CommitID:    commitID,
				Content:     []byte("hello world\n"),
				ContentHash: util.HashBytesBlake3([]byte("hello world\n")),
				Mode:        33188,
				IsBinary:    false,
			},
		},
		{
			name: "binary_file",
			blob: &db.Blob{
				Path:        "image.png",
				CommitID:    commitID,
				Content:     []byte{0x89, 0x50, 0x4E, 0x47},
				ContentHash: util.HashBytesBlake3([]byte{0x89, 0x50, 0x4E, 0x47}),
				Mode:        33188,
				IsBinary:    true,
			},
		},
		{
			name: "symlink",
			blob: &db.Blob{
				Path:          "link.txt",
				CommitID:      commitID,
				Content:       []byte("hello.txt"),
				ContentHash:   util.HashBytesBlake3([]byte("hello.txt")),
				Mode:          41471,
				IsSymlink:     true,
				SymlinkTarget: strPtr("hello.txt"),
				IsBinary:      false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := d.CreateBlob(ctx, tt.blob); err != nil {
				t.Fatalf("CreateBlob: %v", err)
			}

			got, err := d.GetBlob(ctx, tt.blob.Path, tt.blob.CommitID)
			if err != nil {
				t.Fatalf("GetBlob: %v", err)
			}
			if got == nil {
				t.Fatal("GetBlob returned nil")
			}
			if got.Path != tt.blob.Path {
				t.Errorf("Path = %q, want %q", got.Path, tt.blob.Path)
			}
			if !bytes.Equal(got.Content, tt.blob.Content) {
				t.Errorf("Content mismatch")
			}
			if !bytes.Equal(got.ContentHash, tt.blob.ContentHash) {
				t.Errorf("ContentHash mismatch")
			}
			if got.Mode != tt.blob.Mode {
				t.Errorf("Mode = %d, want %d", got.Mode, tt.blob.Mode)
			}
			if got.IsSymlink != tt.blob.IsSymlink {
				t.Errorf("IsSymlink = %v, want %v", got.IsSymlink, tt.blob.IsSymlink)
			}
			if got.IsBinary != tt.blob.IsBinary {
				t.Errorf("IsBinary = %v, want %v", got.IsBinary, tt.blob.IsBinary)
			}
			// Verify SymlinkTarget round-trips correctly
			if tt.blob.SymlinkTarget != nil {
				if got.SymlinkTarget == nil {
					t.Errorf("SymlinkTarget = nil, want %q", *tt.blob.SymlinkTarget)
				} else if *got.SymlinkTarget != *tt.blob.SymlinkTarget {
					t.Errorf("SymlinkTarget = %q, want %q", *got.SymlinkTarget, *tt.blob.SymlinkTarget)
				}
			} else if got.SymlinkTarget != nil {
				t.Errorf("SymlinkTarget = %q, want nil", *got.SymlinkTarget)
			}
		})
	}
}

// T47: GetBlobsAtCommit / GetTreeAtCommit — multi-commit history
func TestGetBlobsAtCommitAndTree(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	commit1 := "01BLOBTREE0000000000000001"
	commit2 := "01BLOBTREE0000000000000002"

	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	d.CreateCommit(ctx, &db.Commit{
		ID: commit1, TreeHash: "t1", Message: "c1",
		AuthorName: "A", AuthorEmail: "a@x.com", AuthoredAt: ts,
		CommitterName: "A", CommitterEmail: "a@x.com", CommittedAt: ts,
	})
	d.CreateCommit(ctx, &db.Commit{
		ID: commit2, ParentID: strPtr(commit1), TreeHash: "t2", Message: "c2",
		AuthorName: "A", AuthorEmail: "a@x.com", AuthoredAt: ts.Add(time.Minute),
		CommitterName: "A", CommitterEmail: "a@x.com", CommittedAt: ts.Add(time.Minute),
	})

	// commit1: add file_a.txt
	d.CreateBlob(ctx, &db.Blob{
		Path: "file_a.txt", CommitID: commit1,
		Content: []byte("aaa"), ContentHash: util.HashBytesBlake3([]byte("aaa")),
		Mode: 33188,
	})
	// commit2: add file_b.txt
	d.CreateBlob(ctx, &db.Blob{
		Path: "file_b.txt", CommitID: commit2,
		Content: []byte("bbb"), ContentHash: util.HashBytesBlake3([]byte("bbb")),
		Mode: 33188,
	})

	tests := []struct {
		name              string
		commitID          string
		wantBlobsAtCommit int
		wantTreeSize      int
	}{
		{name: "commit1_one_blob_one_tree", commitID: commit1, wantBlobsAtCommit: 1, wantTreeSize: 1},
		{name: "commit2_one_blob_two_tree", commitID: commit2, wantBlobsAtCommit: 1, wantTreeSize: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blobs, err := d.GetBlobsAtCommit(ctx, tt.commitID)
			if err != nil {
				t.Fatalf("GetBlobsAtCommit: %v", err)
			}
			if len(blobs) != tt.wantBlobsAtCommit {
				t.Errorf("GetBlobsAtCommit: got %d, want %d", len(blobs), tt.wantBlobsAtCommit)
			}

			tree, err := d.GetTreeAtCommit(ctx, tt.commitID)
			if err != nil {
				t.Fatalf("GetTreeAtCommit: %v", err)
			}
			if len(tree) != tt.wantTreeSize {
				t.Errorf("GetTreeAtCommit: got %d, want %d", len(tree), tt.wantTreeSize)
			}
		})
	}
}

// T48: GetFileHistory — file modifications across commits
func TestGetFileHistory(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	commits := []string{
		"01FILEHIST0000000000000001",
		"01FILEHIST0000000000000002",
		"01FILEHIST0000000000000003",
	}
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	var parent *string
	for i, id := range commits {
		d.CreateCommit(ctx, &db.Commit{
			ID: id, ParentID: parent, TreeHash: "t" + id, Message: "c" + id,
			AuthorName: "A", AuthorEmail: "a@x.com", AuthoredAt: ts.Add(time.Duration(i) * time.Minute),
			CommitterName: "A", CommitterEmail: "a@x.com", CommittedAt: ts.Add(time.Duration(i) * time.Minute),
		})
		parent = strPtr(id)
	}

	// Modify same file across all three commits
	for i, id := range commits {
		content := []byte("version " + id)
		d.CreateBlob(ctx, &db.Blob{
			Path: "evolving.txt", CommitID: id,
			Content: content, ContentHash: util.HashBytesBlake3(content),
			Mode: 33188,
		})
		_ = i
	}

	tests := []struct {
		name      string
		path      string
		wantCount int
	}{
		{name: "three_versions", path: "evolving.txt", wantCount: 3},
		{name: "nonexistent_file", path: "nonexistent.txt", wantCount: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history, err := d.GetFileHistory(ctx, tt.path)
			if err != nil {
				t.Fatalf("GetFileHistory: %v", err)
			}
			if len(history) != tt.wantCount {
				t.Errorf("got %d history entries, want %d", len(history), tt.wantCount)
			}
		})
	}
}

// T49: GetChangedFiles — between two commits
func TestGetChangedFiles(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	commit1 := "01CHANGED20000000000000001"
	commit2 := "01CHANGED20000000000000002"

	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	d.CreateCommit(ctx, &db.Commit{
		ID: commit1, TreeHash: "t1", Message: "c1",
		AuthorName: "A", AuthorEmail: "a@x.com", AuthoredAt: ts,
		CommitterName: "A", CommitterEmail: "a@x.com", CommittedAt: ts,
	})
	d.CreateCommit(ctx, &db.Commit{
		ID: commit2, ParentID: strPtr(commit1), TreeHash: "t2", Message: "c2",
		AuthorName: "A", AuthorEmail: "a@x.com", AuthoredAt: ts.Add(time.Minute),
		CommitterName: "A", CommitterEmail: "a@x.com", CommittedAt: ts.Add(time.Minute),
	})

	// commit1: file1.txt
	d.CreateBlob(ctx, &db.Blob{
		Path: "diff_file1.txt", CommitID: commit1,
		Content: []byte("orig"), ContentHash: util.HashBytesBlake3([]byte("orig")),
		Mode: 33188,
	})
	// commit2: file2.txt (new) and file1.txt (modified)
	d.CreateBlob(ctx, &db.Blob{
		Path: "diff_file2.txt", CommitID: commit2,
		Content: []byte("new"), ContentHash: util.HashBytesBlake3([]byte("new")),
		Mode: 33188,
	})
	d.CreateBlob(ctx, &db.Blob{
		Path: "diff_file1.txt", CommitID: commit2,
		Content: []byte("modified"), ContentHash: util.HashBytesBlake3([]byte("modified")),
		Mode: 33188,
	})

	tests := []struct {
		name      string
		from      string
		to        string
		wantCount int
	}{
		{name: "two_changed_files", from: commit1, to: commit2, wantCount: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed, err := d.GetChangedFiles(ctx, tt.from, tt.to)
			if err != nil {
				t.Fatalf("GetChangedFiles: %v", err)
			}
			if len(changed) != tt.wantCount {
				t.Errorf("got %d changed files, want %d", len(changed), tt.wantCount)
			}
		})
	}
}

// T50: SearchContent — insert known content, search with patterns
func TestSearchContent(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	commitID := "01SEARCH0000000000000000001"
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	d.CreateCommit(ctx, &db.Commit{
		ID: commitID, TreeHash: "t1", Message: "search test",
		AuthorName: "A", AuthorEmail: "a@x.com", AuthoredAt: ts,
		CommitterName: "A", CommitterEmail: "a@x.com", CommittedAt: ts,
	})
	d.SetHead(ctx, commitID)

	d.CreateBlob(ctx, &db.Blob{
		Path: "search_target.txt", CommitID: commitID,
		Content: []byte("This file contains NEEDLE_PATTERN_XYZ here."),
		ContentHash: util.HashBytesBlake3([]byte("This file contains NEEDLE_PATTERN_XYZ here.")),
		Mode: 33188,
	})
	d.CreateBlob(ctx, &db.Blob{
		Path: "other_file.txt", CommitID: commitID,
		Content: []byte("No match in this file."),
		ContentHash: util.HashBytesBlake3([]byte("No match in this file.")),
		Mode: 33188,
	})

	tests := []struct {
		name      string
		pattern   string
		wantCount int
	}{
		{name: "exact_pattern_match", pattern: "NEEDLE_PATTERN_XYZ", wantCount: 1},
		{name: "no_match", pattern: "ZZZZZ_NOT_FOUND_ZZZZZ", wantCount: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := d.SearchContent(ctx, db.SearchContentOptions{
				Pattern: tt.pattern,
			})
			if err != nil {
				t.Fatalf("SearchContent: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
			}
		})
	}
}
