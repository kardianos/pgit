package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/imgajeed76/pgit/v4/internal/testdb"
	"github.com/imgajeed76/pgit/v4/internal/util"
	"github.com/imgajeed76/pgit/v4/server/auth"
)

func makeTestAuthor(t *testing.T, d *db.DB) (*db.Author, string) {
	t.Helper()
	ctx := context.Background()

	email := "test@example.com"
	a := &db.Author{
		ID:          util.NewULID(),
		Name:        "test-author",
		Email:       &email,
		Kind:        db.AuthorKindHuman,
		Permissions: db.PermAll,
		CreatedAt:   time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := d.CreateAuthor(ctx, a); err != nil {
		t.Fatal(err)
	}

	token, err := d.CreateAuthorToken(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	return a, token
}

func TestTokenAuth(t *testing.T) {
	d := testdb.Acquire(t)
	author, token := makeTestAuthor(t, d)

	handler := auth.TokenAuth(d)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a := auth.AuthorFromContext(r.Context())
		if a == nil {
			t.Error("expected author in context, got nil")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if a.ID != author.ID {
			t.Errorf("expected author ID %s, got %s", author.ID, a.ID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{
			name:       "valid_token",
			authHeader: "Bearer " + token,
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing_header",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid_format_no_bearer",
			authHeader: "Token " + token,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid_token",
			authHeader: "Bearer deadbeef",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "empty_bearer",
			authHeader: "Bearer ",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestAuthorFromContext_NoAuthor(t *testing.T) {
	ctx := context.Background()
	a := auth.AuthorFromContext(ctx)
	if a != nil {
		t.Errorf("expected nil author from empty context, got %+v", a)
	}
}
