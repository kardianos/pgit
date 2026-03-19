package remote_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/imgajeed76/pgit/v4/internal/testdb"
	"github.com/imgajeed76/pgit/v4/internal/util"
	"github.com/imgajeed76/pgit/v4/provider/remote"
	"github.com/imgajeed76/pgit/v4/server"
)

// startTestServer creates a test DB, server, author, and returns a remote
// provider plus the underlying *db.DB for seeding data.
func startTestServer(t *testing.T) (*remote.Provider, *db.DB) {
	t.Helper()

	d := testdb.Acquire(t)
	ctx := context.Background()

	// Create author with token.
	email := "remote-test@example.com"
	a := &db.Author{
		ID:          util.NewULID(),
		Name:        "remote-tester",
		Email:       &email,
		Kind:        db.AuthorKindHuman,
		Permissions: db.PermAll | db.PermAdmin,
		CreatedAt:   time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := d.CreateAuthor(ctx, a); err != nil {
		t.Fatal(err)
	}
	token, err := d.CreateAuthorToken(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}

	srv := server.New(d, ":0")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		if err := srv.StartOnListener(ln); err != nil && err != http.ErrServerClosed {
			// ignore
		}
	}()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	baseURL := fmt.Sprintf("http://%s", ln.Addr().String())
	p := remote.New(baseURL, token)
	return p, d
}

func TestRemoteProvider_SchemaVersion(t *testing.T) {
	p, _ := startTestServer(t)
	ctx := context.Background()

	v, err := p.GetSchemaVersion(ctx)
	if err != nil {
		t.Fatalf("GetSchemaVersion: %v", err)
	}
	if v < 1 {
		t.Errorf("expected schema version >= 1, got %d", v)
	}
}

func TestRemoteProvider_HeadAndCommits(t *testing.T) {
	p, d := startTestServer(t)
	ctx := context.Background()

	// Seed a commit and set HEAD via direct DB.
	commitID := util.NewULID()
	c := &db.Commit{
		ID:             commitID,
		TreeHash:       "abcdef01",
		Message:        "remote provider test",
		AuthorName:     "tester",
		AuthorEmail:    "tester@example.com",
		AuthoredAt:     time.Now().UTC().Truncate(time.Microsecond),
		CommitterName:  "tester",
		CommitterEmail: "tester@example.com",
		CommittedAt:    time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := d.CreateCommit(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := d.SetHead(ctx, commitID); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "GetHead",
			fn: func() error {
				head, err := p.GetHead(ctx)
				if err != nil {
					return err
				}
				if head != commitID {
					return fmt.Errorf("expected %s, got %s", commitID, head)
				}
				return nil
			},
		},
		{
			name: "GetCommit",
			fn: func() error {
				got, err := p.GetCommit(ctx, commitID)
				if err != nil {
					return err
				}
				if got.Message != "remote provider test" {
					return fmt.Errorf("wrong message: %s", got.Message)
				}
				return nil
			},
		},
		{
			name: "GetCommitLog",
			fn: func() error {
				commits, err := p.GetCommitLog(ctx, 10)
				if err != nil {
					return err
				}
				if len(commits) == 0 {
					return fmt.Errorf("expected at least 1 commit")
				}
				return nil
			},
		},
		{
			name: "CommitExists",
			fn: func() error {
				exists, err := p.CommitExists(ctx, commitID)
				if err != nil {
					return err
				}
				if !exists {
					return fmt.Errorf("expected commit to exist")
				}
				return nil
			},
		},
		{
			name: "SetHead",
			fn: func() error {
				return p.SetHead(ctx, commitID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRemoteProvider_Authors(t *testing.T) {
	p, d := startTestServer(t)
	ctx := context.Background()

	// Create author via remote provider.
	email := "remote-new@example.com"
	// Get the auth user (created by startTestServer) so we can make the new author a child
	authUser, err := d.GetAuthorByName(ctx, "remote-tester")
	if err != nil {
		t.Fatal(err)
	}
	authUserID := authUser.ID
	a := &db.Author{
		ID:          util.NewULID(),
		ParentID:    &authUserID, // child of the auth user
		Name:        "remote-created-author",
		Email:       &email,
		Kind:        db.AuthorKindService,
		Permissions: db.PermRead,
		CreatedAt:   time.Now().UTC().Truncate(time.Microsecond),
	}

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "CreateAuthor",
			fn: func() error {
				return p.CreateAuthor(ctx, a)
			},
		},
		{
			name: "GetAuthor",
			fn: func() error {
				got, err := p.GetAuthor(ctx, a.ID)
				if err != nil {
					return err
				}
				if got.Name != a.Name {
					return fmt.Errorf("expected name %s, got %s", a.Name, got.Name)
				}
				return nil
			},
		},
		{
			name: "ListAuthors",
			fn: func() error {
				authors, err := p.ListAuthors(ctx, false)
				if err != nil {
					return err
				}
				if len(authors) < 2 {
					return fmt.Errorf("expected at least 2 authors, got %d", len(authors))
				}
				return nil
			},
		},
		{
			name: "CreateAuthorToken",
			fn: func() error {
				token, err := p.CreateAuthorToken(ctx, a.ID)
				if err != nil {
					return err
				}
				if len(token) == 0 {
					return fmt.Errorf("expected non-empty token")
				}
				return nil
			},
		},
		{
			name: "SoftDeleteAuthor",
			fn: func() error {
				return p.SoftDeleteAuthor(ctx, a.ID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRemoteProvider_CLWorkflow(t *testing.T) {
	p, d := startTestServer(t)
	ctx := context.Background()

	// Create author for CL via direct DB.
	email := "cl-remote@example.com"
	author := &db.Author{
		ID:          util.NewULID(),
		Name:        "cl-remote-author",
		Email:       &email,
		Kind:        db.AuthorKindHuman,
		Permissions: db.PermAll | db.PermAdmin,
		CreatedAt:   time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := d.CreateAuthor(ctx, author); err != nil {
		t.Fatal(err)
	}

	clID := util.NewULID()
	now := time.Now().UTC().Truncate(time.Microsecond)

	cl := &db.CL{
		ID:          clID,
		AuthorID:    author.ID,
		Title:       "Remote CL Test",
		Description: "Testing CL via remote provider",
		Status:      db.CLStatusDraft,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "CreateCL",
			fn: func() error {
				return p.CreateCL(ctx, cl)
			},
		},
		{
			name: "GetCL",
			fn: func() error {
				got, err := p.GetCL(ctx, clID)
				if err != nil {
					return err
				}
				if got.Title != cl.Title {
					return fmt.Errorf("expected title %q, got %q", cl.Title, got.Title)
				}
				return nil
			},
		},
		{
			name: "ListCLs",
			fn: func() error {
				cls, err := p.ListCLs(ctx, db.CLStatusDraft, 10)
				if err != nil {
					return err
				}
				if len(cls) == 0 {
					return fmt.Errorf("expected at least 1 CL")
				}
				return nil
			},
		},
		{
			name: "UpdateCLStatus",
			fn: func() error {
				return p.UpdateCLStatus(ctx, clID, db.CLStatusActive)
			},
		},
		{
			name: "GetCL_after_status_update",
			fn: func() error {
				got, err := p.GetCL(ctx, clID)
				if err != nil {
					return err
				}
				if got.Status != db.CLStatusActive {
					return fmt.Errorf("expected status active, got %s", got.Status)
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRemoteProvider_Refs(t *testing.T) {
	p, d := startTestServer(t)
	ctx := context.Background()

	// Seed a commit and HEAD.
	commitID := util.NewULID()
	c := &db.Commit{
		ID:             commitID,
		TreeHash:       "beef0001",
		Message:        "refs test",
		AuthorName:     "tester",
		AuthorEmail:    "t@e.com",
		AuthoredAt:     time.Now().UTC().Truncate(time.Microsecond),
		CommitterName:  "tester",
		CommitterEmail: "t@e.com",
		CommittedAt:    time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := d.CreateCommit(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := d.SetHead(ctx, commitID); err != nil {
		t.Fatal(err)
	}

	refs, err := p.GetAllRefs(ctx)
	if err != nil {
		t.Fatalf("GetAllRefs: %v", err)
	}
	if len(refs) == 0 {
		t.Error("expected at least 1 ref (HEAD)")
	}
}

func TestRemoteProvider_ErrNotSupported(t *testing.T) {
	p, _ := startTestServer(t)
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"Exec", func() error { return p.Exec(ctx, "SELECT 1") }},
		{"WithTx", func() error { return p.WithTx(ctx, nil) }},
		{"InitSchema", func() error { return p.InitSchema(ctx) }},
		{"DropSchema", func() error { return p.DropSchema(ctx) }},
		{"CreateCommit", func() error { return p.CreateCommit(ctx, nil) }},
		{"CreateBlob", func() error { return p.CreateBlob(ctx, nil) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err != remote.ErrNotSupported {
				t.Errorf("expected ErrNotSupported, got %v", err)
			}
		})
	}
}
