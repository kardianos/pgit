package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/imgajeed76/pgit/v4/internal/testdb"
	"github.com/imgajeed76/pgit/v4/internal/util"
	"github.com/imgajeed76/pgit/v4/server"
)

// testServer starts a server on an ephemeral port and returns the base URL
// and a cleanup function. It also creates an author with a token for auth.
func testServer(t *testing.T) (baseURL, token string, d *db.DB) {
	t.Helper()

	d = testdb.Acquire(t)

	// Create test author with token.
	email := "server-test@example.com"
	a := &db.Author{
		ID:          util.NewULID(),
		Name:        "server-tester",
		Email:       &email,
		Kind:        db.AuthorKindHuman,
		Permissions: db.PermAll | db.PermAdmin,
		CreatedAt:   time.Now().UTC().Truncate(time.Microsecond),
	}
	ctx := context.Background()
	if err := d.CreateAuthor(ctx, a); err != nil {
		t.Fatal(err)
	}
	var err error
	token, err = d.CreateAuthorToken(ctx, a.ID)
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
			// Server stopped; ignore in tests.
		}
	}()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	baseURL = fmt.Sprintf("http://%s", ln.Addr().String())
	return baseURL, token, d
}

func doReq(t *testing.T, method, url, token string, body string) (*http.Response, []byte) {
	t.Helper()
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func TestServerRequiresAuth(t *testing.T) {
	baseURL, _, _ := testServer(t)

	tests := []struct {
		name string
		path string
	}{
		{"head", "/api/v1/head"},
		{"refs", "/api/v1/refs"},
		{"schema_version", "/api/v1/schema/version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, body := doReq(t, http.MethodGet, baseURL+tt.path, "", "")
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d; body=%s", resp.StatusCode, string(body))
			}
		})
	}
}

func TestServerSchemaVersion(t *testing.T) {
	baseURL, token, _ := testServer(t)

	resp, body := doReq(t, http.MethodGet, baseURL+"/api/v1/schema/version", token, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", resp.StatusCode, string(body))
	}

	var env struct {
		Data struct {
			Version int `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, string(body))
	}
	if env.Data.Version < 1 {
		t.Errorf("expected schema version >= 1, got %d", env.Data.Version)
	}
}

func TestServerAuthorCRUD(t *testing.T) {
	baseURL, token, _ := testServer(t)

	// Create author.
	email := "new-author@example.com"
	createBody := fmt.Sprintf(`{
		"ID": "%s",
		"Name": "new-test-author",
		"Email": %q,
		"Kind": "human",
		"Permissions": %d,
		"CreatedAt": "%s"
	}`, util.NewULID(), email, db.PermAll, time.Now().UTC().Format(time.RFC3339Nano))

	resp, body := doReq(t, http.MethodPost, baseURL+"/api/v1/authors", token, createBody)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create author: expected 201, got %d; body=%s", resp.StatusCode, string(body))
	}

	// List authors.
	resp, body = doReq(t, http.MethodGet, baseURL+"/api/v1/authors", token, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list authors: expected 200, got %d; body=%s", resp.StatusCode, string(body))
	}

	var listEnv struct {
		Data []*db.Author `json:"data"`
	}
	if err := json.Unmarshal(body, &listEnv); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Should have at least 2 authors (the server-tester + new-test-author).
	if len(listEnv.Data) < 2 {
		t.Errorf("expected at least 2 authors, got %d", len(listEnv.Data))
	}
}

func TestServerHeadAndRefs(t *testing.T) {
	baseURL, token, d := testServer(t)
	ctx := context.Background()

	// Create a commit and set HEAD.
	commitID := util.NewULID()
	c := &db.Commit{
		ID:             commitID,
		TreeHash:       "deadbeef",
		Message:        "test commit",
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

	// GET /head
	resp, body := doReq(t, http.MethodGet, baseURL+"/api/v1/head", token, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get head: expected 200, got %d; body=%s", resp.StatusCode, string(body))
	}

	var headEnv struct {
		Data struct {
			CommitID string `json:"commit_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &headEnv); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if headEnv.Data.CommitID != commitID {
		t.Errorf("expected head commit %s, got %s", commitID, headEnv.Data.CommitID)
	}

	// GET /commits/{id}
	resp, body = doReq(t, http.MethodGet, baseURL+"/api/v1/commits/"+commitID, token, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get commit: expected 200, got %d; body=%s", resp.StatusCode, string(body))
	}

	// GET /refs
	resp, body = doReq(t, http.MethodGet, baseURL+"/api/v1/refs", token, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get refs: expected 200, got %d; body=%s", resp.StatusCode, string(body))
	}
}

func TestServerCLWorkflow(t *testing.T) {
	baseURL, token, d := testServer(t)
	ctx := context.Background()

	// Create an author for the CL.
	email := "cl-author@example.com"
	author := &db.Author{
		ID:          util.NewULID(),
		Name:        "cl-author",
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

	// Create CL
	createBody := fmt.Sprintf(`{
		"ID": "%s",
		"AuthorID": "%s",
		"Title": "Test CL",
		"Description": "A test change list",
		"Status": "draft",
		"CreatedAt": "%s",
		"UpdatedAt": "%s"
	}`, clID, author.ID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))

	resp, body := doReq(t, http.MethodPost, baseURL+"/api/v1/cls", token, createBody)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create CL: expected 201, got %d; body=%s", resp.StatusCode, string(body))
	}

	// Get CL
	resp, body = doReq(t, http.MethodGet, baseURL+"/api/v1/cls/"+clID, token, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get CL: expected 200, got %d; body=%s", resp.StatusCode, string(body))
	}

	// List CLs
	resp, body = doReq(t, http.MethodGet, baseURL+"/api/v1/cls?status=draft", token, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list CLs: expected 200, got %d; body=%s", resp.StatusCode, string(body))
	}

	// Update CL status
	resp, body = doReq(t, http.MethodPut, baseURL+"/api/v1/cls/"+clID+"/status", token,
		`{"status":"active"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update CL status: expected 200, got %d; body=%s", resp.StatusCode, string(body))
	}
}
