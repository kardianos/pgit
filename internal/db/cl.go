package db

import "time"

// CLStatus enumerates the lifecycle states of a change list.
type CLStatus string

const (
	CLStatusDraft     CLStatus = "draft"
	CLStatusActive    CLStatus = "active"
	CLStatusSubmitted CLStatus = "submitted"
	CLStatusAbandoned CLStatus = "abandoned"
)

// CIStatus enumerates the states of a CI job.
type CIStatus string

const (
	CIStatusPending CIStatus = "pending"
	CIStatusRunning CIStatus = "running"
	CIStatusPassed  CIStatus = "passed"
	CIStatusFailed  CIStatus = "failed"
)

// CL represents a change list (similar to a pull request).
type CL struct {
	ID          string
	AuthorID    string
	Title       string
	Description string
	Status      CLStatus
	ParentCLID  *string // for stacked CLs
	SubmittedAs *string // commit hash, set on submit
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PatchSet links a CL to a snapshot commit.
type PatchSet struct {
	ID         string
	CLID       string
	Number     int
	CommitHash string
	CreatedAt  time.Time
}

// ReviewComment is a threaded comment on a CL or patch set.
type ReviewComment struct {
	ID          string
	CLID        string
	PatchSet    *int    // nil = top-level comment
	Path        *string // nil = top-level comment
	Line        *int
	AuthorID    string
	Body        string
	ParentID    *string // threading: parent comment ID
	CreatedAt   time.Time
}

// ReviewVote is a per-CL per-author score.
type ReviewVote struct {
	CLID      string
	AuthorID  string
	Score     int
	UpdatedAt time.Time
}

// CIResult records a CI job run against a patch set.
type CIResult struct {
	ID          string
	CLID        string
	PatchSet    int
	JobName     string
	Status      CIStatus
	LogBlobHash *string
	Artifacts   []byte // JSONB
	TriggeredBy string // author ID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
