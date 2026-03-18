package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/imgajeed76/pgit/v4/internal/testdb"
	"github.com/imgajeed76/pgit/v4/internal/util"
)

// T57: GetRepoStatsFast — verify non-zero stats after inserting data
func TestGetRepoStatsFast(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	// Insert known data: 1 commit, 1 blob
	commitID := "01STATS0000000000000000001A"
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := d.CreateCommit(ctx, &db.Commit{
		ID: commitID, TreeHash: "t1", Message: "stats test",
		AuthorName: "A", AuthorEmail: "a@x.com", AuthoredAt: ts,
		CommitterName: "A", CommitterEmail: "a@x.com", CommittedAt: ts,
	}); err != nil {
		t.Fatalf("CreateCommit: %v", err)
	}
	if err := d.CreateBlob(ctx, &db.Blob{
		Path: "stats_file.txt", CommitID: commitID,
		Content: []byte("stats content here"),
		ContentHash: util.HashBytesBlake3([]byte("stats content here")),
		Mode: 33188,
	}); err != nil {
		t.Fatalf("CreateBlob: %v", err)
	}

	tests := []struct {
		name             string
		wantCommits      int64
		wantUniqueFiles  int64
		wantTotalBlobs   int64
	}{
		{name: "exact_counts_after_insert", wantCommits: 1, wantUniqueFiles: 1, wantTotalBlobs: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats, err := d.GetRepoStatsFast(ctx)
			if err != nil {
				t.Fatalf("GetRepoStatsFast: %v", err)
			}
			if stats == nil {
				t.Fatal("stats is nil")
			}
			if stats.TotalCommits != tt.wantCommits {
				t.Errorf("TotalCommits = %d, want %d", stats.TotalCommits, tt.wantCommits)
			}
			if stats.UniqueFiles != tt.wantUniqueFiles {
				t.Errorf("UniqueFiles = %d, want %d", stats.UniqueFiles, tt.wantUniqueFiles)
			}
			if stats.TotalBlobs != tt.wantTotalBlobs {
				t.Errorf("TotalBlobs = %d, want %d", stats.TotalBlobs, tt.wantTotalBlobs)
			}
		})
	}
}

// T58: Metadata CRUD — Set/Get/Delete cycle
func TestMetadataCRUD(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "simple_key_value", key: "test_key", value: "test_value"},
		{name: "repo_path", key: "repo_path", value: "/tmp/test-repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set
			if err := d.SetMetadata(ctx, tt.key, tt.value); err != nil {
				t.Fatalf("SetMetadata: %v", err)
			}

			// Get
			got, err := d.GetMetadata(ctx, tt.key)
			if err != nil {
				t.Fatalf("GetMetadata: %v", err)
			}
			if got != tt.value {
				t.Errorf("GetMetadata = %q, want %q", got, tt.value)
			}

			// Update
			newValue := tt.value + "_updated"
			if err := d.SetMetadata(ctx, tt.key, newValue); err != nil {
				t.Fatalf("SetMetadata update: %v", err)
			}
			got, _ = d.GetMetadata(ctx, tt.key)
			if got != newValue {
				t.Errorf("after update: %q, want %q", got, newValue)
			}

			// Delete
			if err := d.DeleteMetadata(ctx, tt.key); err != nil {
				t.Fatalf("DeleteMetadata: %v", err)
			}
			_, err = d.GetMetadata(ctx, tt.key)
			if err == nil {
				t.Error("expected error after delete, got nil")
			}
		})
	}
}

// T59: SyncState CRUD — Set/Get/Delete/GetAll
func TestSyncStateCRUD(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		remoteName string
		commitID   string
	}{
		{name: "origin_remote", remoteName: "origin", commitID: "01SYNC0000000000000000001AA"},
		{name: "upstream_remote", remoteName: "upstream", commitID: "01SYNC0000000000000000002BB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set
			if err := d.SetSyncState(ctx, tt.remoteName, &tt.commitID); err != nil {
				t.Fatalf("SetSyncState: %v", err)
			}

			// Get
			state, err := d.GetSyncState(ctx, tt.remoteName)
			if err != nil {
				t.Fatalf("GetSyncState: %v", err)
			}
			if state == nil {
				t.Fatal("GetSyncState returned nil")
			}
			if state.RemoteName != tt.remoteName {
				t.Errorf("RemoteName = %q, want %q", state.RemoteName, tt.remoteName)
			}
			if state.LastCommitID == nil || *state.LastCommitID != tt.commitID {
				t.Errorf("LastCommitID mismatch")
			}
			if state.SyncedAt.IsZero() {
				t.Error("SyncedAt should not be zero")
			}

			// Delete
			if err := d.DeleteSyncState(ctx, tt.remoteName); err != nil {
				t.Fatalf("DeleteSyncState: %v", err)
			}
			state, err = d.GetSyncState(ctx, tt.remoteName)
			if err != nil {
				t.Fatalf("GetSyncState after delete: %v", err)
			}
			if state != nil {
				t.Errorf("expected nil after delete, got %+v", state)
			}
		})
	}

	// Test GetAllSyncStates
	t.Run("get_all", func(t *testing.T) {
		cid1 := "01SYNCALL00000000000000001"
		cid2 := "01SYNCALL00000000000000002"
		d.SetSyncState(ctx, "remote_a", &cid1)
		d.SetSyncState(ctx, "remote_b", &cid2)

		all, err := d.GetAllSyncStates(ctx)
		if err != nil {
			t.Fatalf("GetAllSyncStates: %v", err)
		}
		if len(all) < 2 {
			t.Errorf("got %d sync states, want >= 2", len(all))
		}
	})
}
