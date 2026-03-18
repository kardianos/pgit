package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// CreateAuthor inserts a new author into the database.
func (db *DB) CreateAuthor(ctx context.Context, a *Author) error {
	sql := `
	INSERT INTO author (id, parent_id, name, email, kind, permissions, token_hash, created_at, deleted_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	return db.Exec(ctx, sql,
		a.ID, a.ParentID, a.Name, a.Email, a.Kind,
		int64(a.Permissions), a.TokenHash, a.CreatedAt, a.DeletedAt,
	)
}

// GetAuthor retrieves an author by ID.
func (db *DB) GetAuthor(ctx context.Context, id string) (*Author, error) {
	sql := `
	SELECT id, parent_id, name, email, kind, permissions, token_hash, created_at, deleted_at
	FROM author WHERE id = $1 AND deleted_at IS NULL`

	a := &Author{}
	var perms int64
	err := db.QueryRow(ctx, sql, id).Scan(
		&a.ID, &a.ParentID, &a.Name, &a.Email, &a.Kind,
		&perms, &a.TokenHash, &a.CreatedAt, &a.DeletedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("author not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get author: %w", err)
	}
	a.Permissions = Permission(perms)
	return a, nil
}

// GetAuthorByName retrieves an author by name (unique index lookup).
func (db *DB) GetAuthorByName(ctx context.Context, name string) (*Author, error) {
	sql := `
	SELECT id, parent_id, name, email, kind, permissions, token_hash, created_at, deleted_at
	FROM author WHERE name = $1 AND deleted_at IS NULL`

	a := &Author{}
	var perms int64
	err := db.QueryRow(ctx, sql, name).Scan(
		&a.ID, &a.ParentID, &a.Name, &a.Email, &a.Kind,
		&perms, &a.TokenHash, &a.CreatedAt, &a.DeletedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("author not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("get author by name: %w", err)
	}
	a.Permissions = Permission(perms)
	return a, nil
}

// GetAuthorChildren returns the direct children of a given author.
func (db *DB) GetAuthorChildren(ctx context.Context, parentID string) ([]*Author, error) {
	sql := `
	SELECT id, parent_id, name, email, kind, permissions, token_hash, created_at, deleted_at
	FROM author WHERE parent_id = $1 AND deleted_at IS NULL
	ORDER BY created_at`

	rows, err := db.Query(ctx, sql, parentID)
	if err != nil {
		return nil, fmt.Errorf("get author children: %w", err)
	}
	defer rows.Close()

	var authors []*Author
	for rows.Next() {
		a := &Author{}
		var perms int64
		if err := rows.Scan(&a.ID, &a.ParentID, &a.Name, &a.Email, &a.Kind,
			&perms, &a.TokenHash, &a.CreatedAt, &a.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan author child: %w", err)
		}
		a.Permissions = Permission(perms)
		authors = append(authors, a)
	}
	return authors, rows.Err()
}

// ListAuthors returns all authors ordered by created_at.
// If includeDeleted is false, soft-deleted authors are excluded.
func (db *DB) ListAuthors(ctx context.Context, includeDeleted bool) ([]*Author, error) {
	var sql string
	if includeDeleted {
		sql = `SELECT id, parent_id, name, email, kind, permissions, token_hash, created_at, deleted_at
		       FROM author ORDER BY created_at`
	} else {
		sql = `SELECT id, parent_id, name, email, kind, permissions, token_hash, created_at, deleted_at
		       FROM author WHERE deleted_at IS NULL ORDER BY created_at`
	}

	rows, err := db.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("list authors: %w", err)
	}
	defer rows.Close()

	var authors []*Author
	for rows.Next() {
		a := &Author{}
		var perms int64
		if err := rows.Scan(&a.ID, &a.ParentID, &a.Name, &a.Email, &a.Kind,
			&perms, &a.TokenHash, &a.CreatedAt, &a.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan author: %w", err)
		}
		a.Permissions = Permission(perms)
		authors = append(authors, a)
	}
	return authors, rows.Err()
}

// GetAuthorChain walks from the given author up to the root, returning
// the chain [self, parent, grandparent, ..., root].
func (db *DB) GetAuthorChain(ctx context.Context, id string) ([]*Author, error) {
	var chain []*Author
	currentID := id
	seen := make(map[string]bool)

	for {
		if seen[currentID] {
			return nil, fmt.Errorf("cycle detected in author chain at %s", currentID)
		}
		seen[currentID] = true

		a, err := db.GetAuthor(ctx, currentID)
		if err != nil {
			return nil, fmt.Errorf("get author chain: %w", err)
		}
		chain = append(chain, a)

		if a.ParentID == nil {
			break // reached root
		}
		currentID = *a.ParentID
	}
	return chain, nil
}

// ComputeEffectivePermissions walks the author chain to the root and
// ANDs all permission bitmasks together. This means revoking a bit on
// any ancestor instantly restricts all descendants.
func (db *DB) ComputeEffectivePermissions(ctx context.Context, id string) (Permission, error) {
	chain, err := db.GetAuthorChain(ctx, id)
	if err != nil {
		return 0, err
	}

	effective := chain[0].Permissions
	for _, a := range chain[1:] {
		effective &= a.Permissions
	}
	return effective, nil
}

// SoftDeleteAuthor sets the deleted_at timestamp for the given author.
func (db *DB) SoftDeleteAuthor(ctx context.Context, id string) error {
	sql := `UPDATE author SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL`
	tag, err := db.pool.Exec(ctx, sql, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("soft delete author: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("author not found: %s", id)
	}
	return nil
}

// UpdateAuthorPermissions sets the permissions bitmask for the given author.
func (db *DB) UpdateAuthorPermissions(ctx context.Context, id string, perms Permission) error {
	sql := `UPDATE author SET permissions = $1 WHERE id = $2 AND deleted_at IS NULL`
	tag, err := db.pool.Exec(ctx, sql, int64(perms), id)
	if err != nil {
		return fmt.Errorf("update author permissions: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("author not found: %s", id)
	}
	return nil
}

// CreateAuthorToken generates a random token, stores its bcrypt hash on the
// author, and returns the plaintext token (hex-encoded, 64 chars).
func (db *DB) CreateAuthorToken(ctx context.Context, authorID string) (string, error) {
	// Generate 32 random bytes
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	plaintext := hex.EncodeToString(tokenBytes)

	// Hash with bcrypt
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash token: %w", err)
	}

	// Store hash — verify the author exists and is not deleted
	sql := `UPDATE author SET token_hash = $1 WHERE id = $2 AND deleted_at IS NULL`
	tag, err2 := db.pool.Exec(ctx, sql, hash, authorID)
	if err2 != nil {
		return "", fmt.Errorf("store token hash: %w", err2)
	}
	if tag.RowsAffected() == 0 {
		return "", fmt.Errorf("author not found: %s", authorID)
	}

	return plaintext, nil
}

// ValidateAuthorToken finds an author whose token_hash matches the given
// plaintext token. Returns the author if found, or an error if no match.
func (db *DB) ValidateAuthorToken(ctx context.Context, token string) (*Author, error) {
	// Scan all authors with a non-null token_hash and check bcrypt.
	// This is intentionally not indexed — token validation is rare and
	// the bcrypt comparison prevents timing attacks.
	sql := `
	SELECT id, parent_id, name, email, kind, permissions, token_hash, created_at, deleted_at
	FROM author WHERE token_hash IS NOT NULL AND deleted_at IS NULL`

	rows, err := db.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("query authors for token: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		a := &Author{}
		var perms int64
		if err := rows.Scan(&a.ID, &a.ParentID, &a.Name, &a.Email, &a.Kind,
			&perms, &a.TokenHash, &a.CreatedAt, &a.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan author: %w", err)
		}
		a.Permissions = Permission(perms)

		if bcrypt.CompareHashAndPassword(a.TokenHash, []byte(token)) == nil {
			return a, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate authors: %w", err)
	}
	return nil, fmt.Errorf("invalid token")
}
