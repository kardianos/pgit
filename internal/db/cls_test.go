package db_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/imgajeed76/pgit/v4/internal/testdb"
	"github.com/imgajeed76/pgit/v4/internal/util"
)

// setupCLTestAuthor creates a root author for CL tests.
func setupCLTestAuthor(t *testing.T, d *db.DB, name string) *db.Author {
	t.Helper()
	a := &db.Author{
		ID:          util.NewULID(),
		Name:        name,
		Email:       emailPtr(name + "@example.com"),
		Kind:        db.AuthorKindHuman,
		Permissions: db.PermAll,
		CreatedAt:   time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := d.CreateAuthor(context.Background(), a); err != nil {
		t.Fatalf("setup author %s: %v", name, err)
	}
	return a
}

func makeCL(authorID string, title string, parentCLID *string) *db.CL {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &db.CL{
		ID:          util.NewULID(),
		AuthorID:    authorID,
		Title:       title,
		Description: "description for " + title,
		Status:      db.CLStatusDraft,
		ParentCLID:  parentCLID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func TestCreateGetCLRoundTrip(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()
	author := setupCLTestAuthor(t, d, "cl-author")

	tests := []struct {
		name  string
		title string
	}{
		{"simple_cl", "Add feature X"},
		{"with_description", "Fix bug Y"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cl := makeCL(author.ID, tt.title, nil)
			if err := d.CreateCL(ctx, cl); err != nil {
				t.Fatalf("CreateCL: %v", err)
			}

			got, err := d.GetCL(ctx, cl.ID)
			if err != nil {
				t.Fatalf("GetCL: %v", err)
			}

			if got.ID != cl.ID {
				t.Errorf("ID = %q, want %q", got.ID, cl.ID)
			}
			if got.Title != tt.title {
				t.Errorf("Title = %q, want %q", got.Title, tt.title)
			}
			if got.AuthorID != author.ID {
				t.Errorf("AuthorID = %q, want %q", got.AuthorID, author.ID)
			}
			if got.Status != db.CLStatusDraft {
				t.Errorf("Status = %q, want %q", got.Status, db.CLStatusDraft)
			}
		})
	}
}

func TestListCLsWithStatusFilter(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()
	author := setupCLTestAuthor(t, d, "list-author")

	// Create CLs with different statuses
	for _, status := range []db.CLStatus{db.CLStatusDraft, db.CLStatusDraft, db.CLStatusActive, db.CLStatusSubmitted} {
		cl := makeCL(author.ID, "CL-"+string(status), nil)
		cl.Status = status
		if err := d.CreateCL(ctx, cl); err != nil {
			t.Fatalf("CreateCL: %v", err)
		}
		// Small sleep to ensure distinct updated_at for ordering
		time.Sleep(time.Millisecond)
	}

	tests := []struct {
		name     string
		status   db.CLStatus
		wantLen  int
	}{
		{"all", "", 4},
		{"draft", db.CLStatusDraft, 2},
		{"active", db.CLStatusActive, 1},
		{"submitted", db.CLStatusSubmitted, 1},
		{"abandoned_none", db.CLStatusAbandoned, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cls, err := d.ListCLs(ctx, tt.status, 100)
			if err != nil {
				t.Fatalf("ListCLs: %v", err)
			}
			if len(cls) != tt.wantLen {
				t.Errorf("got %d CLs, want %d", len(cls), tt.wantLen)
			}
		})
	}
}

func TestUpdateCLStatus(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()
	author := setupCLTestAuthor(t, d, "status-author")

	cl := makeCL(author.ID, "Status test", nil)
	if err := d.CreateCL(ctx, cl); err != nil {
		t.Fatalf("CreateCL: %v", err)
	}

	tests := []struct {
		name   string
		status db.CLStatus
	}{
		{"to_active", db.CLStatusActive},
		{"to_submitted", db.CLStatusSubmitted},
		{"to_abandoned", db.CLStatusAbandoned},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := d.UpdateCLStatus(ctx, cl.ID, tt.status); err != nil {
				t.Fatalf("UpdateCLStatus: %v", err)
			}

			got, err := d.GetCL(ctx, cl.ID)
			if err != nil {
				t.Fatalf("GetCL: %v", err)
			}
			if got.Status != tt.status {
				t.Errorf("Status = %q, want %q", got.Status, tt.status)
			}
		})
	}
}

func TestUpdateCL(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()
	author := setupCLTestAuthor(t, d, "update-author")

	cl := makeCL(author.ID, "Original title", nil)
	if err := d.CreateCL(ctx, cl); err != nil {
		t.Fatalf("CreateCL: %v", err)
	}

	cl.Title = "Updated title"
	cl.Description = "Updated description"
	if err := d.UpdateCL(ctx, cl); err != nil {
		t.Fatalf("UpdateCL: %v", err)
	}

	got, err := d.GetCL(ctx, cl.ID)
	if err != nil {
		t.Fatalf("GetCL: %v", err)
	}
	if got.Title != "Updated title" {
		t.Errorf("Title = %q, want %q", got.Title, "Updated title")
	}
	if got.Description != "Updated description" {
		t.Errorf("Description = %q, want %q", got.Description, "Updated description")
	}
}

func TestPatchSetCreateListLatest(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()
	author := setupCLTestAuthor(t, d, "ps-author")

	cl := makeCL(author.ID, "PatchSet test", nil)
	if err := d.CreateCL(ctx, cl); err != nil {
		t.Fatalf("CreateCL: %v", err)
	}

	// Create 3 patch sets
	for i := 1; i <= 3; i++ {
		ps := &db.PatchSet{
			ID:         util.NewULID(),
			CLID:       cl.ID,
			Number:     i,
			CommitHash: "abc123" + string(rune('0'+i)),
			CreatedAt:  time.Now().UTC().Truncate(time.Microsecond),
		}
		if err := d.CreatePatchSet(ctx, ps); err != nil {
			t.Fatalf("CreatePatchSet %d: %v", i, err)
		}
	}

	t.Run("list", func(t *testing.T) {
		pss, err := d.GetPatchSetsForCL(ctx, cl.ID)
		if err != nil {
			t.Fatalf("GetPatchSetsForCL: %v", err)
		}
		if len(pss) != 3 {
			t.Fatalf("got %d patch sets, want 3", len(pss))
		}
		for i, ps := range pss {
			if ps.Number != i+1 {
				t.Errorf("ps[%d].Number = %d, want %d", i, ps.Number, i+1)
			}
		}
	})

	t.Run("latest", func(t *testing.T) {
		ps, err := d.GetLatestPatchSet(ctx, cl.ID)
		if err != nil {
			t.Fatalf("GetLatestPatchSet: %v", err)
		}
		if ps.Number != 3 {
			t.Errorf("latest.Number = %d, want 3", ps.Number)
		}
	})
}

func TestReviewCommentCreateListThreading(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()
	author := setupCLTestAuthor(t, d, "comment-author")

	cl := makeCL(author.ID, "Comment test", nil)
	if err := d.CreateCL(ctx, cl); err != nil {
		t.Fatalf("CreateCL: %v", err)
	}

	// Top-level comment
	topComment := &db.ReviewComment{
		ID:        util.NewULID(),
		CLID:      cl.ID,
		AuthorID:  author.ID,
		Body:      "This is a top-level comment",
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := d.CreateReviewComment(ctx, topComment); err != nil {
		t.Fatalf("CreateReviewComment top: %v", err)
	}

	// Reply to the top-level comment
	reply := &db.ReviewComment{
		ID:        util.NewULID(),
		CLID:      cl.ID,
		AuthorID:  author.ID,
		Body:      "This is a reply",
		ParentID:  &topComment.ID,
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := d.CreateReviewComment(ctx, reply); err != nil {
		t.Fatalf("CreateReviewComment reply: %v", err)
	}

	// Inline comment on patch set 1
	psNum := 1
	path := "main.go"
	line := 42
	inline := &db.ReviewComment{
		ID:        util.NewULID(),
		CLID:      cl.ID,
		PatchSet:  &psNum,
		Path:      &path,
		Line:      &line,
		AuthorID:  author.ID,
		Body:      "Inline comment on line 42",
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := d.CreateReviewComment(ctx, inline); err != nil {
		t.Fatalf("CreateReviewComment inline: %v", err)
	}

	t.Run("all_comments", func(t *testing.T) {
		comments, err := d.GetCommentsForCL(ctx, cl.ID)
		if err != nil {
			t.Fatalf("GetCommentsForCL: %v", err)
		}
		if len(comments) != 3 {
			t.Fatalf("got %d comments, want 3", len(comments))
		}
	})

	t.Run("patch_set_comments", func(t *testing.T) {
		comments, err := d.GetCommentsForPatchSet(ctx, cl.ID, 1)
		if err != nil {
			t.Fatalf("GetCommentsForPatchSet: %v", err)
		}
		if len(comments) != 1 {
			t.Fatalf("got %d comments, want 1", len(comments))
		}
		if comments[0].Body != "Inline comment on line 42" {
			t.Errorf("Body = %q", comments[0].Body)
		}
	})

	t.Run("threading", func(t *testing.T) {
		comments, err := d.GetCommentsForCL(ctx, cl.ID)
		if err != nil {
			t.Fatalf("GetCommentsForCL: %v", err)
		}

		// Find the reply
		for _, c := range comments {
			if c.ID == reply.ID {
				if c.ParentID == nil || *c.ParentID != topComment.ID {
					t.Errorf("reply.ParentID = %v, want %v", c.ParentID, topComment.ID)
				}
				return
			}
		}
		t.Error("reply not found in comments")
	})
}

func TestReviewVoteUpsert(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()
	author := setupCLTestAuthor(t, d, "vote-author")

	cl := makeCL(author.ID, "Vote test", nil)
	if err := d.CreateCL(ctx, cl); err != nil {
		t.Fatalf("CreateCL: %v", err)
	}

	// Initial vote
	v := &db.ReviewVote{
		CLID:      cl.ID,
		AuthorID:  author.ID,
		Score:     1,
		UpdatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := d.SetReviewVote(ctx, v); err != nil {
		t.Fatalf("SetReviewVote initial: %v", err)
	}

	// Update the vote
	v.Score = 2
	v.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
	if err := d.SetReviewVote(ctx, v); err != nil {
		t.Fatalf("SetReviewVote update: %v", err)
	}

	votes, err := d.GetVotesForCL(ctx, cl.ID)
	if err != nil {
		t.Fatalf("GetVotesForCL: %v", err)
	}
	if len(votes) != 1 {
		t.Fatalf("got %d votes, want 1 (upsert should not duplicate)", len(votes))
	}
	if votes[0].Score != 2 {
		t.Errorf("Score = %d, want 2", votes[0].Score)
	}
}

// ---------------------------------------------------------------------------
// CI Results
// ---------------------------------------------------------------------------

func makeCIResult(clID, authorID, jobName string, patchSet int) *db.CIResult {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &db.CIResult{
		ID:          util.NewULID(),
		CLID:        clID,
		PatchSet:    patchSet,
		JobName:     jobName,
		Status:      db.CIStatusPending,
		TriggeredBy: authorID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func TestCreateGetCIResultRoundTrip(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()
	author := setupCLTestAuthor(t, d, "ci-author")

	cl := makeCL(author.ID, "CI test", nil)
	if err := d.CreateCL(ctx, cl); err != nil {
		t.Fatalf("CreateCL: %v", err)
	}

	tests := []struct {
		name    string
		jobName string
	}{
		{"lint_job", "lint"},
		{"test_job", "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ci := makeCIResult(cl.ID, author.ID, tt.jobName, 1)
			if err := d.CreateCIResult(ctx, ci); err != nil {
				t.Fatalf("CreateCIResult: %v", err)
			}

			results, err := d.GetCIResultsForCL(ctx, cl.ID)
			if err != nil {
				t.Fatalf("GetCIResultsForCL: %v", err)
			}

			found := false
			for _, r := range results {
				if r.ID == ci.ID {
					found = true
					if r.JobName != tt.jobName {
						t.Errorf("JobName = %q, want %q", r.JobName, tt.jobName)
					}
					if r.Status != db.CIStatusPending {
						t.Errorf("Status = %q, want %q", r.Status, db.CIStatusPending)
					}
				}
			}
			if !found {
				t.Error("created CI result not found in GetCIResultsForCL")
			}
		})
	}
}

func TestMultipleCIResultsPerPatchSet(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()
	author := setupCLTestAuthor(t, d, "ci-multi-author")

	cl := makeCL(author.ID, "CI multi test", nil)
	if err := d.CreateCL(ctx, cl); err != nil {
		t.Fatalf("CreateCL: %v", err)
	}

	jobs := []string{"lint", "test", "build"}
	for _, job := range jobs {
		ci := makeCIResult(cl.ID, author.ID, job, 1)
		if err := d.CreateCIResult(ctx, ci); err != nil {
			t.Fatalf("CreateCIResult %s: %v", job, err)
		}
		time.Sleep(time.Millisecond) // ensure distinct created_at
	}

	tests := []struct {
		name     string
		patchSet int
		wantLen  int
	}{
		{"ps1_has_three", 1, 3},
		{"ps2_has_none", 2, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := d.GetCIResultsForPatchSet(ctx, cl.ID, tt.patchSet)
			if err != nil {
				t.Fatalf("GetCIResultsForPatchSet: %v", err)
			}
			if len(results) != tt.wantLen {
				t.Errorf("got %d results, want %d", len(results), tt.wantLen)
			}
		})
	}
}

func TestUpdateCIResultStatus(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()
	author := setupCLTestAuthor(t, d, "ci-update-author")

	cl := makeCL(author.ID, "CI update test", nil)
	if err := d.CreateCL(ctx, cl); err != nil {
		t.Fatalf("CreateCL: %v", err)
	}

	ci := makeCIResult(cl.ID, author.ID, "build", 1)
	if err := d.CreateCIResult(ctx, ci); err != nil {
		t.Fatalf("CreateCIResult: %v", err)
	}

	tests := []struct {
		name   string
		status db.CIStatus
	}{
		{"to_running", db.CIStatusRunning},
		{"to_passed", db.CIStatusPassed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ci.Status = tt.status
			if err := d.UpdateCIResult(ctx, ci); err != nil {
				t.Fatalf("UpdateCIResult: %v", err)
			}

			got, err := d.GetCIResult(ctx, ci.ID)
			if err != nil {
				t.Fatalf("GetCIResult: %v", err)
			}
			if got.Status != tt.status {
				t.Errorf("Status = %q, want %q", got.Status, tt.status)
			}
		})
	}
}

func TestGetCIResultsForPatchSetFiltering(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()
	author := setupCLTestAuthor(t, d, "ci-filter-author")

	cl := makeCL(author.ID, "CI filter test", nil)
	if err := d.CreateCL(ctx, cl); err != nil {
		t.Fatalf("CreateCL: %v", err)
	}

	// Create results across different patch sets
	for ps := 1; ps <= 3; ps++ {
		for j := 0; j < ps; j++ {
			ci := makeCIResult(cl.ID, author.ID, fmt.Sprintf("job-%d-%d", ps, j), ps)
			if err := d.CreateCIResult(ctx, ci); err != nil {
				t.Fatalf("CreateCIResult ps%d job%d: %v", ps, j, err)
			}
			time.Sleep(time.Millisecond)
		}
	}

	tests := []struct {
		name     string
		patchSet int
		wantLen  int
	}{
		{"ps1_one_result", 1, 1},
		{"ps2_two_results", 2, 2},
		{"ps3_three_results", 3, 3},
		{"ps4_none", 4, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := d.GetCIResultsForPatchSet(ctx, cl.ID, tt.patchSet)
			if err != nil {
				t.Fatalf("GetCIResultsForPatchSet: %v", err)
			}
			if len(results) != tt.wantLen {
				t.Errorf("got %d results, want %d", len(results), tt.wantLen)
			}
		})
	}
}

func TestCLStack(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()
	author := setupCLTestAuthor(t, d, "stack-author")

	// Create 3-CL stack: cl1 -> cl2 -> cl3
	cl1 := makeCL(author.ID, "Stack base", nil)
	if err := d.CreateCL(ctx, cl1); err != nil {
		t.Fatalf("CreateCL cl1: %v", err)
	}

	cl2 := makeCL(author.ID, "Stack middle", &cl1.ID)
	if err := d.CreateCL(ctx, cl2); err != nil {
		t.Fatalf("CreateCL cl2: %v", err)
	}

	cl3 := makeCL(author.ID, "Stack top", &cl2.ID)
	if err := d.CreateCL(ctx, cl3); err != nil {
		t.Fatalf("CreateCL cl3: %v", err)
	}

	tests := []struct {
		name     string
		startID  string
		wantLen  int
		wantLast string
	}{
		{"from_top", cl3.ID, 3, cl1.ID},
		{"from_middle", cl2.ID, 2, cl1.ID},
		{"from_base", cl1.ID, 1, cl1.ID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stack, err := d.GetCLStack(ctx, tt.startID)
			if err != nil {
				t.Fatalf("GetCLStack: %v", err)
			}
			if len(stack) != tt.wantLen {
				t.Fatalf("stack len = %d, want %d", len(stack), tt.wantLen)
			}
			if stack[len(stack)-1].ID != tt.wantLast {
				t.Errorf("last in stack = %q, want %q", stack[len(stack)-1].ID, tt.wantLast)
			}
		})
	}
}
