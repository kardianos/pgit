package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// CreateCL inserts a new change list.
func (db *DB) CreateCL(ctx context.Context, c *CL) error {
	sql := `
	INSERT INTO cl (id, author_id, title, description, status, parent_cl_id, submitted_as, created_at, updated_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	return db.Exec(ctx, sql,
		c.ID, c.AuthorID, c.Title, c.Description, c.Status,
		c.ParentCLID, c.SubmittedAs, c.CreatedAt, c.UpdatedAt,
	)
}

// GetCL retrieves a change list by ID.
func (db *DB) GetCL(ctx context.Context, id string) (*CL, error) {
	sql := `
	SELECT id, author_id, title, description, status, parent_cl_id, submitted_as, created_at, updated_at
	FROM cl WHERE id = $1`

	c := &CL{}
	err := db.QueryRow(ctx, sql, id).Scan(
		&c.ID, &c.AuthorID, &c.Title, &c.Description, &c.Status,
		&c.ParentCLID, &c.SubmittedAs, &c.CreatedAt, &c.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("cl not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get cl: %w", err)
	}
	return c, nil
}

// ListCLs returns CLs optionally filtered by status.
// Pass empty string for status to list all. Results are ordered by updated_at DESC.
func (db *DB) ListCLs(ctx context.Context, status CLStatus, limit int) ([]*CL, error) {
	var sql string
	var args []any

	if status == "" {
		sql = `SELECT id, author_id, title, description, status, parent_cl_id, submitted_as, created_at, updated_at
		       FROM cl ORDER BY updated_at DESC LIMIT $1`
		args = []any{limit}
	} else {
		sql = `SELECT id, author_id, title, description, status, parent_cl_id, submitted_as, created_at, updated_at
		       FROM cl WHERE status = $1 ORDER BY updated_at DESC LIMIT $2`
		args = []any{status, limit}
	}

	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("list cls: %w", err)
	}
	defer rows.Close()

	var cls []*CL
	for rows.Next() {
		c := &CL{}
		if err := rows.Scan(&c.ID, &c.AuthorID, &c.Title, &c.Description, &c.Status,
			&c.ParentCLID, &c.SubmittedAs, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan cl: %w", err)
		}
		cls = append(cls, c)
	}
	return cls, rows.Err()
}

// UpdateCLStatus updates the status of a CL.
func (db *DB) UpdateCLStatus(ctx context.Context, id string, status CLStatus) error {
	sql := `UPDATE cl SET status = $1, updated_at = NOW() WHERE id = $2`
	return db.Exec(ctx, sql, status, id)
}

// SubmitCL marks a CL as submitted and records the commit hash.
func (db *DB) SubmitCL(ctx context.Context, id string, commitHash string) error {
	sql := `UPDATE cl SET status = $1, submitted_as = $2, updated_at = NOW() WHERE id = $3`
	return db.Exec(ctx, sql, CLStatusSubmitted, commitHash, id)
}

// UpdateCL updates the title and description of a CL.
func (db *DB) UpdateCL(ctx context.Context, c *CL) error {
	sql := `UPDATE cl SET title = $1, description = $2, updated_at = NOW() WHERE id = $3`
	return db.Exec(ctx, sql, c.Title, c.Description, c.ID)
}

// ---------------------------------------------------------------------------
// Patch sets
// ---------------------------------------------------------------------------

// CreatePatchSet inserts a new patch set.
func (db *DB) CreatePatchSet(ctx context.Context, ps *PatchSet) error {
	sql := `
	INSERT INTO patch_set (id, cl_id, number, commit_hash, created_at)
	VALUES ($1, $2, $3, $4, $5)`

	return db.Exec(ctx, sql, ps.ID, ps.CLID, ps.Number, ps.CommitHash, ps.CreatedAt)
}

// GetPatchSetsForCL returns all patch sets for a CL, ordered by number.
func (db *DB) GetPatchSetsForCL(ctx context.Context, clID string) ([]*PatchSet, error) {
	sql := `
	SELECT id, cl_id, number, commit_hash, created_at
	FROM patch_set WHERE cl_id = $1 ORDER BY number`

	rows, err := db.Query(ctx, sql, clID)
	if err != nil {
		return nil, fmt.Errorf("get patch sets: %w", err)
	}
	defer rows.Close()

	var pss []*PatchSet
	for rows.Next() {
		ps := &PatchSet{}
		if err := rows.Scan(&ps.ID, &ps.CLID, &ps.Number, &ps.CommitHash, &ps.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan patch set: %w", err)
		}
		pss = append(pss, ps)
	}
	return pss, rows.Err()
}

// GetLatestPatchSet returns the patch set with the highest number for a CL.
func (db *DB) GetLatestPatchSet(ctx context.Context, clID string) (*PatchSet, error) {
	sql := `
	SELECT id, cl_id, number, commit_hash, created_at
	FROM patch_set WHERE cl_id = $1 ORDER BY number DESC LIMIT 1`

	ps := &PatchSet{}
	err := db.QueryRow(ctx, sql, clID).Scan(
		&ps.ID, &ps.CLID, &ps.Number, &ps.CommitHash, &ps.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("no patch sets for cl: %s", clID)
	}
	if err != nil {
		return nil, fmt.Errorf("get latest patch set: %w", err)
	}
	return ps, nil
}

// ---------------------------------------------------------------------------
// Comments
// ---------------------------------------------------------------------------

// CreateReviewComment inserts a new review comment.
func (db *DB) CreateReviewComment(ctx context.Context, c *ReviewComment) error {
	sql := `
	INSERT INTO review_comment (id, cl_id, patch_set, path, line, author_id, body, parent_id, created_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	return db.Exec(ctx, sql,
		c.ID, c.CLID, c.PatchSet, c.Path, c.Line,
		c.AuthorID, c.Body, c.ParentID, c.CreatedAt,
	)
}

// GetCommentsForCL returns all comments for a CL, ordered by created_at.
func (db *DB) GetCommentsForCL(ctx context.Context, clID string) ([]*ReviewComment, error) {
	sql := `
	SELECT id, cl_id, patch_set, path, line, author_id, body, parent_id, created_at
	FROM review_comment WHERE cl_id = $1 ORDER BY created_at`

	rows, err := db.Query(ctx, sql, clID)
	if err != nil {
		return nil, fmt.Errorf("get comments for cl: %w", err)
	}
	defer rows.Close()

	return scanComments(rows)
}

// GetCommentsForPatchSet returns comments for a specific patch set of a CL.
func (db *DB) GetCommentsForPatchSet(ctx context.Context, clID string, patchSetNum int) ([]*ReviewComment, error) {
	sql := `
	SELECT id, cl_id, patch_set, path, line, author_id, body, parent_id, created_at
	FROM review_comment WHERE cl_id = $1 AND patch_set = $2 ORDER BY created_at`

	rows, err := db.Query(ctx, sql, clID, patchSetNum)
	if err != nil {
		return nil, fmt.Errorf("get comments for patch set: %w", err)
	}
	defer rows.Close()

	return scanComments(rows)
}

func scanComments(rows pgx.Rows) ([]*ReviewComment, error) {
	var comments []*ReviewComment
	for rows.Next() {
		c := &ReviewComment{}
		if err := rows.Scan(&c.ID, &c.CLID, &c.PatchSet, &c.Path, &c.Line,
			&c.AuthorID, &c.Body, &c.ParentID, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan review comment: %w", err)
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

// ---------------------------------------------------------------------------
// Votes
// ---------------------------------------------------------------------------

// SetReviewVote upserts a review vote (score) for a CL by an author.
func (db *DB) SetReviewVote(ctx context.Context, v *ReviewVote) error {
	sql := `
	INSERT INTO review_vote (cl_id, author_id, score, updated_at)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (cl_id, author_id) DO UPDATE SET score = EXCLUDED.score, updated_at = EXCLUDED.updated_at`

	return db.Exec(ctx, sql, v.CLID, v.AuthorID, v.Score, v.UpdatedAt)
}

// GetVotesForCL returns all votes for a CL.
func (db *DB) GetVotesForCL(ctx context.Context, clID string) ([]*ReviewVote, error) {
	sql := `
	SELECT cl_id, author_id, score, updated_at
	FROM review_vote WHERE cl_id = $1 ORDER BY updated_at`

	rows, err := db.Query(ctx, sql, clID)
	if err != nil {
		return nil, fmt.Errorf("get votes for cl: %w", err)
	}
	defer rows.Close()

	var votes []*ReviewVote
	for rows.Next() {
		v := &ReviewVote{}
		if err := rows.Scan(&v.CLID, &v.AuthorID, &v.Score, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan review vote: %w", err)
		}
		votes = append(votes, v)
	}
	return votes, rows.Err()
}

// ---------------------------------------------------------------------------
// CL stacking
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// CI Results
// ---------------------------------------------------------------------------

// CreateCIResult inserts a new CI result.
func (db *DB) CreateCIResult(ctx context.Context, r *CIResult) error {
	sql := `
	INSERT INTO ci_result (id, cl_id, patch_set, job_name, status, log_blob_hash, artifacts, triggered_by, created_at, updated_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	return db.Exec(ctx, sql,
		r.ID, r.CLID, r.PatchSet, r.JobName, r.Status,
		r.LogBlobHash, r.Artifacts, r.TriggeredBy, r.CreatedAt, r.UpdatedAt,
	)
}

// GetCIResultsForCL returns all CI results for a CL, ordered by created_at.
func (db *DB) GetCIResultsForCL(ctx context.Context, clID string) ([]*CIResult, error) {
	sql := `
	SELECT id, cl_id, patch_set, job_name, status, log_blob_hash, artifacts, triggered_by, created_at, updated_at
	FROM ci_result WHERE cl_id = $1 ORDER BY created_at`

	rows, err := db.Query(ctx, sql, clID)
	if err != nil {
		return nil, fmt.Errorf("get ci results for cl: %w", err)
	}
	defer rows.Close()

	return scanCIResults(rows)
}

// GetCIResultsForPatchSet returns CI results for a specific patch set of a CL.
func (db *DB) GetCIResultsForPatchSet(ctx context.Context, clID string, patchSet int) ([]*CIResult, error) {
	sql := `
	SELECT id, cl_id, patch_set, job_name, status, log_blob_hash, artifacts, triggered_by, created_at, updated_at
	FROM ci_result WHERE cl_id = $1 AND patch_set = $2 ORDER BY created_at`

	rows, err := db.Query(ctx, sql, clID, patchSet)
	if err != nil {
		return nil, fmt.Errorf("get ci results for patch set: %w", err)
	}
	defer rows.Close()

	return scanCIResults(rows)
}

// UpdateCIResult updates the status, log_blob_hash, and artifacts of a CI result.
func (db *DB) UpdateCIResult(ctx context.Context, r *CIResult) error {
	sql := `UPDATE ci_result SET status = $1, log_blob_hash = $2, artifacts = $3, updated_at = NOW() WHERE id = $4`
	return db.Exec(ctx, sql, r.Status, r.LogBlobHash, r.Artifacts, r.ID)
}

// GetCIResult retrieves a single CI result by ID.
func (db *DB) GetCIResult(ctx context.Context, id string) (*CIResult, error) {
	sql := `
	SELECT id, cl_id, patch_set, job_name, status, log_blob_hash, artifacts, triggered_by, created_at, updated_at
	FROM ci_result WHERE id = $1`

	r := &CIResult{}
	err := db.QueryRow(ctx, sql, id).Scan(
		&r.ID, &r.CLID, &r.PatchSet, &r.JobName, &r.Status,
		&r.LogBlobHash, &r.Artifacts, &r.TriggeredBy, &r.CreatedAt, &r.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("ci result not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get ci result: %w", err)
	}
	return r, nil
}

func scanCIResults(rows pgx.Rows) ([]*CIResult, error) {
	var results []*CIResult
	for rows.Next() {
		r := &CIResult{}
		if err := rows.Scan(&r.ID, &r.CLID, &r.PatchSet, &r.JobName, &r.Status,
			&r.LogBlobHash, &r.Artifacts, &r.TriggeredBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan ci result: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// GetCLStack walks the parent_cl_id chain from the given CL to the root,
// returning [self, parent, grandparent, ...].
func (db *DB) GetCLStack(ctx context.Context, clID string) ([]*CL, error) {
	var stack []*CL
	currentID := clID
	seen := make(map[string]bool)

	for {
		if seen[currentID] {
			return nil, fmt.Errorf("cycle detected in CL stack at %s", currentID)
		}
		seen[currentID] = true

		c, err := db.GetCL(ctx, currentID)
		if err != nil {
			return nil, fmt.Errorf("get cl stack: %w", err)
		}
		stack = append(stack, c)

		if c.ParentCLID == nil {
			break
		}
		currentID = *c.ParentCLID
	}
	return stack, nil
}
