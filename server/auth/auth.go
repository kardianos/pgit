// Package auth provides authentication middleware for the pgit server.
package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/imgajeed76/pgit/v4/internal/db"
)

// contextKey is an unexported type for context keys in this package.
type contextKey int

const authorKey contextKey = 0

// AuthorFromContext extracts the authenticated author from the context.
// Returns nil if no author is present.
func AuthorFromContext(ctx context.Context) *db.Author {
	a, _ := ctx.Value(authorKey).(*db.Author)
	return a
}

// withAuthor returns a new context with the given author stored in it.
func withAuthor(ctx context.Context, a *db.Author) context.Context {
	return context.WithValue(ctx, authorKey, a)
}

// TokenAuth returns middleware that validates Bearer tokens using the database.
// Every request must include a valid token; unauthenticated requests receive 401.
func TokenAuth(d *db.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			token, ok := parseBearerToken(header)
			if !ok {
				http.Error(w, `{"error":"invalid authorization header format"}`, http.StatusUnauthorized)
				return
			}

			author, err := d.ValidateAuthorToken(r.Context(), token)
			if err != nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			ctx := withAuthor(r.Context(), author)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// parseBearerToken extracts the token from a "Bearer <token>" header value.
func parseBearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}
