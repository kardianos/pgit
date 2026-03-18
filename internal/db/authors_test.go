package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/imgajeed76/pgit/v4/internal/testdb"
	"github.com/imgajeed76/pgit/v4/internal/util"
)

func emailPtr(s string) *string { return &s }

func makeAuthor(name string, parentID *string, kind db.AuthorKind, perms db.Permission) *db.Author {
	a := &db.Author{
		ID:          util.NewULID(),
		ParentID:    parentID,
		Name:        name,
		Kind:        kind,
		Permissions: perms,
		CreatedAt:   time.Now().UTC().Truncate(time.Microsecond),
	}
	if parentID == nil {
		email := name + "@example.com"
		a.Email = &email
	}
	return a
}

func TestCreateGetAuthorRoundTrip(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	tests := []struct {
		name   string
		author *db.Author
	}{
		{
			name:   "root_human",
			author: makeAuthor("alice", nil, db.AuthorKindHuman, db.PermAll),
		},
		{
			name:   "root_service",
			author: makeAuthor("ci-bot", nil, db.AuthorKindService, db.PermRead|db.PermCITrigger),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := d.CreateAuthor(ctx, tt.author); err != nil {
				t.Fatalf("CreateAuthor: %v", err)
			}

			got, err := d.GetAuthor(ctx, tt.author.ID)
			if err != nil {
				t.Fatalf("GetAuthor: %v", err)
			}

			if got.ID != tt.author.ID {
				t.Errorf("ID = %q, want %q", got.ID, tt.author.ID)
			}
			if got.Name != tt.author.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.author.Name)
			}
			if got.Kind != tt.author.Kind {
				t.Errorf("Kind = %q, want %q", got.Kind, tt.author.Kind)
			}
			if got.Permissions != tt.author.Permissions {
				t.Errorf("Permissions = %d, want %d", got.Permissions, tt.author.Permissions)
			}
		})
	}
}

func TestCreateSubAuthor(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	root := makeAuthor("root-user", nil, db.AuthorKindHuman, db.PermAll)
	if err := d.CreateAuthor(ctx, root); err != nil {
		t.Fatalf("CreateAuthor root: %v", err)
	}

	tests := []struct {
		name   string
		kind   db.AuthorKind
		perms  db.Permission
	}{
		{"llm_sub", db.AuthorKindLLM, db.PermRead | db.PermComment},
		{"service_sub", db.AuthorKindService, db.PermRead},
		{"human_delegate", db.AuthorKindHuman, db.PermAll},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := makeAuthor(tt.name, &root.ID, tt.kind, tt.perms)
			if err := d.CreateAuthor(ctx, sub); err != nil {
				t.Fatalf("CreateAuthor sub: %v", err)
			}

			got, err := d.GetAuthor(ctx, sub.ID)
			if err != nil {
				t.Fatalf("GetAuthor sub: %v", err)
			}
			if got.ParentID == nil || *got.ParentID != root.ID {
				t.Errorf("ParentID = %v, want %v", got.ParentID, root.ID)
			}
			if got.Kind != tt.kind {
				t.Errorf("Kind = %q, want %q", got.Kind, tt.kind)
			}
		})
	}
}

func TestGetAuthorByName(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	a := makeAuthor("lookup-test", nil, db.AuthorKindHuman, db.PermAll)
	if err := d.CreateAuthor(ctx, a); err != nil {
		t.Fatalf("CreateAuthor: %v", err)
	}

	tests := []struct {
		name    string
		lookup  string
		wantErr bool
	}{
		{"found", "lookup-test", false},
		{"not_found", "nonexistent", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.GetAuthorByName(ctx, tt.lookup)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("GetAuthorByName: %v", err)
			}
			if got.ID != a.ID {
				t.Errorf("ID = %q, want %q", got.ID, a.ID)
			}
		})
	}
}

func TestGetAuthorChildren(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	root := makeAuthor("parent-for-children", nil, db.AuthorKindHuman, db.PermAll)
	if err := d.CreateAuthor(ctx, root); err != nil {
		t.Fatalf("CreateAuthor root: %v", err)
	}

	// Create 3 children
	childNames := []string{"child-a", "child-b", "child-c"}
	for _, name := range childNames {
		c := makeAuthor(name, &root.ID, db.AuthorKindHuman, db.PermRead)
		if err := d.CreateAuthor(ctx, c); err != nil {
			t.Fatalf("CreateAuthor %s: %v", name, err)
		}
	}

	children, err := d.GetAuthorChildren(ctx, root.ID)
	if err != nil {
		t.Fatalf("GetAuthorChildren: %v", err)
	}
	if len(children) != 3 {
		t.Fatalf("got %d children, want 3", len(children))
	}

	for i, c := range children {
		if c.Name != childNames[i] {
			t.Errorf("child[%d].Name = %q, want %q", i, c.Name, childNames[i])
		}
	}
}

func TestGetAuthorChain(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	// Create 3-level chain: grandparent -> parent -> child
	gp := makeAuthor("grandparent", nil, db.AuthorKindHuman, db.PermAll)
	if err := d.CreateAuthor(ctx, gp); err != nil {
		t.Fatalf("CreateAuthor gp: %v", err)
	}

	p := makeAuthor("parent-chain", &gp.ID, db.AuthorKindHuman, db.PermAll)
	if err := d.CreateAuthor(ctx, p); err != nil {
		t.Fatalf("CreateAuthor p: %v", err)
	}

	c := makeAuthor("child-chain", &p.ID, db.AuthorKindLLM, db.PermRead|db.PermComment)
	if err := d.CreateAuthor(ctx, c); err != nil {
		t.Fatalf("CreateAuthor c: %v", err)
	}

	tests := []struct {
		name     string
		startID  string
		wantLen  int
		wantLast string
	}{
		{"from_child", c.ID, 3, gp.ID},
		{"from_parent", p.ID, 2, gp.ID},
		{"from_root", gp.ID, 1, gp.ID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain, err := d.GetAuthorChain(ctx, tt.startID)
			if err != nil {
				t.Fatalf("GetAuthorChain: %v", err)
			}
			if len(chain) != tt.wantLen {
				t.Fatalf("chain len = %d, want %d", len(chain), tt.wantLen)
			}
			if chain[len(chain)-1].ID != tt.wantLast {
				t.Errorf("last in chain = %q, want %q", chain[len(chain)-1].ID, tt.wantLast)
			}
		})
	}
}

func TestComputeEffectivePermissions(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	// Root has all perms, child requests read+comment+vote,
	// grandchild requests read+comment.
	root := makeAuthor("eff-root", nil, db.AuthorKindHuman, db.PermAll)
	if err := d.CreateAuthor(ctx, root); err != nil {
		t.Fatalf("CreateAuthor root: %v", err)
	}

	child := makeAuthor("eff-child", &root.ID, db.AuthorKindLLM, db.PermRead|db.PermComment|db.PermVote)
	if err := d.CreateAuthor(ctx, child); err != nil {
		t.Fatalf("CreateAuthor child: %v", err)
	}

	gchild := makeAuthor("eff-grandchild", &child.ID, db.AuthorKindLLM, db.PermRead|db.PermComment)
	if err := d.CreateAuthor(ctx, gchild); err != nil {
		t.Fatalf("CreateAuthor gchild: %v", err)
	}

	tests := []struct {
		name string
		id   string
		want db.Permission
	}{
		{"root_full", root.ID, db.PermAll},
		{"child_subset", child.ID, db.PermRead | db.PermComment | db.PermVote},
		{"grandchild_intersect", gchild.ID, db.PermRead | db.PermComment},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.ComputeEffectivePermissions(ctx, tt.id)
			if err != nil {
				t.Fatalf("ComputeEffectivePermissions: %v", err)
			}
			if got != tt.want {
				t.Errorf("effective = %d (%s), want %d (%s)", got, got, tt.want, tt.want)
			}
		})
	}

	// Revocation test: revoke COMMENT from root, grandchild should lose it.
	t.Run("revocation_propagates", func(t *testing.T) {
		newPerms := db.PermAll &^ db.PermComment // all except comment
		if err := d.UpdateAuthorPermissions(ctx, root.ID, newPerms); err != nil {
			t.Fatalf("UpdateAuthorPermissions: %v", err)
		}

		got, err := d.ComputeEffectivePermissions(ctx, gchild.ID)
		if err != nil {
			t.Fatalf("ComputeEffectivePermissions after revoke: %v", err)
		}
		if got.Has(db.PermComment) {
			t.Error("grandchild should have lost COMMENT after root revocation")
		}
		if !got.Has(db.PermRead) {
			t.Error("grandchild should still have READ")
		}
	})
}

func TestSoftDeleteAuthor(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	a := makeAuthor("to-delete", nil, db.AuthorKindHuman, db.PermAll)
	if err := d.CreateAuthor(ctx, a); err != nil {
		t.Fatalf("CreateAuthor: %v", err)
	}

	// Soft delete
	if err := d.SoftDeleteAuthor(ctx, a.ID); err != nil {
		t.Fatalf("SoftDeleteAuthor: %v", err)
	}

	// GetAuthor now filters deleted — should return error
	_, err := d.GetAuthor(ctx, a.ID)
	if err == nil {
		t.Error("GetAuthor should fail for soft-deleted author")
	}

	// Double-delete should fail (already deleted)
	if err := d.SoftDeleteAuthor(ctx, a.ID); err == nil {
		t.Error("SoftDeleteAuthor should fail for already-deleted author")
	}
}

func TestAuthorTokenCreateValidate(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	a := makeAuthor("token-user", nil, db.AuthorKindHuman, db.PermAll)
	if err := d.CreateAuthor(ctx, a); err != nil {
		t.Fatalf("CreateAuthor: %v", err)
	}

	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		// The valid token is set dynamically below.
		{"invalid_token", "0000000000000000000000000000000000000000000000000000000000000000", true},
	}

	// Create a real token.
	plaintext, err := d.CreateAuthorToken(ctx, a.ID)
	if err != nil {
		t.Fatalf("CreateAuthorToken: %v", err)
	}
	if len(plaintext) != 64 { // 32 bytes hex = 64 chars
		t.Fatalf("token length = %d, want 64", len(plaintext))
	}

	// Add the valid token test case.
	tests = append([]struct {
		name    string
		token   string
		wantErr bool
	}{
		{"valid_token", plaintext, false},
	}, tests...)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.ValidateAuthorToken(ctx, tt.token)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateAuthorToken: %v", err)
			}
			if got.ID != a.ID {
				t.Errorf("validated author ID = %q, want %q", got.ID, a.ID)
			}
		})
	}
}

func TestSubAuthorCannotExceedParentPermissions(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	// Parent with only READ
	parent := makeAuthor("restricted-parent", nil, db.AuthorKindHuman, db.PermRead)
	if err := d.CreateAuthor(ctx, parent); err != nil {
		t.Fatalf("CreateAuthor parent: %v", err)
	}

	// Sub-author requests READ|COMMENT — but effective should be just READ
	sub := makeAuthor("greedy-sub", &parent.ID, db.AuthorKindLLM, db.PermRead|db.PermComment)
	if err := d.CreateAuthor(ctx, sub); err != nil {
		t.Fatalf("CreateAuthor sub: %v", err)
	}

	effective, err := d.ComputeEffectivePermissions(ctx, sub.ID)
	if err != nil {
		t.Fatalf("ComputeEffectivePermissions: %v", err)
	}
	if effective.Has(db.PermComment) {
		t.Error("sub-author effective permissions should not include COMMENT (parent lacks it)")
	}
	if !effective.Has(db.PermRead) {
		t.Error("sub-author effective permissions should include READ")
	}
	if effective != db.PermRead {
		t.Errorf("effective = %d (%s), want %d (%s)", effective, effective, db.PermRead, db.PermRead.String())
	}
}
