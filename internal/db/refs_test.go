package db_test

import (
	"context"
	"testing"

	"github.com/imgajeed76/pgit/v4/internal/testdb"
)

// T51: SetRef / GetRef / DeleteRef — CRUD round-trip
func TestRefCRUD(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		refName  string
		commitID string
	}{
		{name: "main_branch", refName: "refs/heads/main", commitID: "01REF00000000000000000001AA"},
		{name: "tag", refName: "refs/tags/v1.0", commitID: "01REF00000000000000000002BB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set
			if err := d.SetRef(ctx, tt.refName, tt.commitID); err != nil {
				t.Fatalf("SetRef: %v", err)
			}

			// Get
			ref, err := d.GetRef(ctx, tt.refName)
			if err != nil {
				t.Fatalf("GetRef: %v", err)
			}
			if ref == nil {
				t.Fatal("GetRef returned nil")
			}
			if ref.Name != tt.refName {
				t.Errorf("Name = %q, want %q", ref.Name, tt.refName)
			}
			if ref.CommitID != tt.commitID {
				t.Errorf("CommitID = %q, want %q", ref.CommitID, tt.commitID)
			}

			// Update
			newCommitID := tt.commitID + "X"
			if err := d.SetRef(ctx, tt.refName, newCommitID); err != nil {
				t.Fatalf("SetRef update: %v", err)
			}
			ref, _ = d.GetRef(ctx, tt.refName)
			if ref.CommitID != newCommitID {
				t.Errorf("after update: CommitID = %q, want %q", ref.CommitID, newCommitID)
			}

			// Delete
			if err := d.DeleteRef(ctx, tt.refName); err != nil {
				t.Fatalf("DeleteRef: %v", err)
			}
			ref, err = d.GetRef(ctx, tt.refName)
			if err != nil {
				t.Fatalf("GetRef after delete: %v", err)
			}
			if ref != nil {
				t.Errorf("ref should be nil after delete, got %+v", ref)
			}
		})
	}
}

// T52: GetAllRefs
func TestGetAllRefs(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	tests := []struct {
		name      string
		refs      map[string]string
		wantCount int
	}{
		{
			name: "multiple_refs",
			refs: map[string]string{
				"refs/heads/main":    "01ALLREF000000000000000001",
				"refs/heads/develop": "01ALLREF000000000000000002",
				"refs/tags/v1.0":     "01ALLREF000000000000000003",
			},
			wantCount: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for name, commitID := range tt.refs {
				if err := d.SetRef(ctx, name, commitID); err != nil {
					t.Fatalf("SetRef %s: %v", name, err)
				}
			}

			all, err := d.GetAllRefs(ctx)
			if err != nil {
				t.Fatalf("GetAllRefs: %v", err)
			}
			if len(all) != tt.wantCount {
				t.Errorf("got %d refs, want %d", len(all), tt.wantCount)
			}

			// Verify each ref is present
			found := make(map[string]bool)
			for _, r := range all {
				found[r.Name] = true
			}
			for name := range tt.refs {
				if !found[name] {
					t.Errorf("ref %q not found in GetAllRefs", name)
				}
			}
		})
	}
}

// T54: User ref CRUD
func TestUserRefCRUD(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		email    string
		refName  string
		commitID string
	}{
		{name: "alice_main", email: "alice@example.com", refName: "main", commitID: "01UREF0000000000000000001AA"},
		{name: "alice_feature", email: "alice@example.com", refName: "feature/x", commitID: "01UREF0000000000000000002BB"},
		{name: "bob_main", email: "bob@example.com", refName: "main", commitID: "01UREF0000000000000000003CC"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set
			if err := d.SetUserRef(ctx, tt.email, tt.refName, tt.commitID); err != nil {
				t.Fatalf("SetUserRef: %v", err)
			}

			// Get
			ref, err := d.GetUserRef(ctx, tt.email, tt.refName)
			if err != nil {
				t.Fatalf("GetUserRef: %v", err)
			}
			if ref == nil {
				t.Fatal("GetUserRef returned nil")
			}
			if ref.CommitID != tt.commitID {
				t.Errorf("CommitID = %q, want %q", ref.CommitID, tt.commitID)
			}

			// Update
			newCommitID := tt.commitID + "X"
			if err := d.SetUserRef(ctx, tt.email, tt.refName, newCommitID); err != nil {
				t.Fatalf("SetUserRef update: %v", err)
			}
			ref, _ = d.GetUserRef(ctx, tt.email, tt.refName)
			if ref.CommitID != newCommitID {
				t.Errorf("after update: CommitID = %q, want %q", ref.CommitID, newCommitID)
			}

			// Delete
			if err := d.DeleteUserRef(ctx, tt.email, tt.refName); err != nil {
				t.Fatalf("DeleteUserRef: %v", err)
			}
			ref, err = d.GetUserRef(ctx, tt.email, tt.refName)
			if err != nil {
				t.Fatalf("GetUserRef after delete: %v", err)
			}
			if ref != nil {
				t.Errorf("ref should be nil after delete, got %+v", ref)
			}
		})
	}
}

// T55: GetUserRefs — list only own refs
func TestGetUserRefs(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	// Set up refs for two users
	_ = d.SetUserRef(ctx, "alice@example.com", "branch-a", "01URLIST000000000000000001")
	_ = d.SetUserRef(ctx, "alice@example.com", "branch-b", "01URLIST000000000000000002")
	_ = d.SetUserRef(ctx, "bob@example.com", "branch-a", "01URLIST000000000000000003")

	tests := []struct {
		name      string
		email     string
		wantCount int
	}{
		{"alice_has_two", "alice@example.com", 2},
		{"bob_has_one", "bob@example.com", 1},
		{"charlie_has_none", "charlie@example.com", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs, err := d.GetUserRefs(ctx, tt.email)
			if err != nil {
				t.Fatalf("GetUserRefs: %v", err)
			}
			if len(refs) != tt.wantCount {
				t.Errorf("got %d refs, want %d", len(refs), tt.wantCount)
			}
		})
	}
}

// T56: User ref namespacing — two users can have same ref name
func TestUserRefNamespacing(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		email    string
		refName  string
		commitID string
	}{
		{"alice_main", "alice@ns.com", "main", "01URNS00000000000000000001"},
		{"bob_main", "bob@ns.com", "main", "01URNS00000000000000000002"},
	}

	for _, tt := range tests {
		if err := d.SetUserRef(ctx, tt.email, tt.refName, tt.commitID); err != nil {
			t.Fatalf("SetUserRef %s: %v", tt.name, err)
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := d.GetUserRef(ctx, tt.email, tt.refName)
			if err != nil {
				t.Fatalf("GetUserRef: %v", err)
			}
			if ref == nil {
				t.Fatal("expected ref, got nil")
			}
			if ref.CommitID != tt.commitID {
				t.Errorf("CommitID = %q, want %q (namespacing failed)", ref.CommitID, tt.commitID)
			}
		})
	}
}

// T53: SetHead / GetHead
func TestSetGetHead(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		commitID string
	}{
		{name: "set_head", commitID: "01HEAD0000000000000000001AA"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := d.SetHead(ctx, tt.commitID); err != nil {
				t.Fatalf("SetHead: %v", err)
			}

			got, err := d.GetHead(ctx)
			if err != nil {
				t.Fatalf("GetHead: %v", err)
			}
			if got != tt.commitID {
				t.Errorf("GetHead = %q, want %q", got, tt.commitID)
			}
		})
	}
}
