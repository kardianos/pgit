package db

import "time"

// AuthorKind enumerates the types of author accounts.
type AuthorKind string

const (
	AuthorKindHuman   AuthorKind = "human"
	AuthorKindLLM     AuthorKind = "llm"
	AuthorKindService AuthorKind = "service"
)

// Author represents a user, bot, or service account in the code review system.
type Author struct {
	ID          string
	ParentID    *string    // nil = root (system-provisioned)
	Name        string
	Email       *string    // required for root authors
	Kind        AuthorKind // "human", "llm", "service"
	Permissions Permission
	TokenHash   []byte     // bcrypt hash of auth token
	CreatedAt   time.Time
	DeletedAt   *time.Time // soft delete
}
