package cli

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/imgajeed76/pgit/v4/internal/repo"
	"github.com/imgajeed76/pgit/v4/internal/util"
	"github.com/spf13/cobra"
)

func newCLCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cl",
		Short: "Manage change lists (code review)",
		Long: `Manage change lists (CLs) for code review.

A CL is similar to a pull request or merge request. It groups one or more
patch sets (snapshots of changes) with review comments and votes.`,
	}

	cmd.AddCommand(
		newCLNewCmd(),
		newCLUpdateCmd(),
		newCLListCmd(),
		newCLShowCmd(),
		newCLCommentCmd(),
		newCLVoteCmd(),
		newCLSubmitCmd(),
		newCLAbandonCmd(),
		newCLStackCmd(),
	)

	return cmd
}

func newCLNewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new",
		Short: "Create a new CL from staged changes",
		Long: `Create a new change list from the current staged changes.

This commits the staged changes and records them as the initial patch set.
The CL starts in "draft" status.`,
		RunE: runCLNew,
	}

	cmd.Flags().String("title", "", "CL title")
	cmd.Flags().String("description", "", "CL description")

	return cmd
}

func newCLUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [cl-id]",
		Short: "Push a new patch set to an existing CL",
		Long:  `Create a new patch set on an existing CL from the current staged changes.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runCLUpdate,
	}

	return cmd
}

func newCLListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List change lists",
		Long:  `List CLs with optional status filtering.`,
		RunE:  runCLList,
	}

	cmd.Flags().String("status", "", "Filter by status: draft, active, submitted, abandoned")

	return cmd
}

func newCLShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <cl-id>",
		Short: "Show CL details",
		Long:  `Show details of a change list including title, description, patch sets, comments, and votes.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runCLShow,
	}
}

func newCLCommentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comment <cl-id>",
		Short: "Add a comment to a CL",
		Long:  `Add a top-level or inline comment to a change list.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runCLComment,
	}

	cmd.Flags().StringP("message", "m", "", "Comment body (required)")
	cmd.Flags().String("path", "", "File path for inline comment")
	cmd.Flags().Int("line", 0, "Line number for inline comment")
	_ = cmd.MarkFlagRequired("message")

	return cmd
}

func newCLVoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "vote <cl-id> <score>",
		Short: "Vote on a CL",
		Long:  `Set a review vote score on a CL (e.g. +1, -1, +2).`,
		Args:  cobra.ExactArgs(2),
		RunE:  runCLVote,
	}
}

func newCLSubmitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "submit <cl-id>",
		Short: "Submit a CL (cherry-pick onto HEAD)",
		Long: `Submit a CL by cherry-picking the latest patch set's changes onto HEAD.

Creates a new commit with the CL's title as the commit message and marks
the CL as submitted.`,
		Args: cobra.ExactArgs(1),
		RunE: runCLSubmit,
	}
}

func newCLAbandonCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "abandon <cl-id>",
		Short: "Abandon a CL",
		Long:  `Mark a CL as abandoned.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runCLAbandon,
	}
}

func newCLStackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stack <cl-id>",
		Short: "Show the CL stack (parent chain)",
		Long:  `Show the chain of stacked CLs from the given CL to the root.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runCLStack,
	}
}

// getActingAuthorID resolves the acting author from the --account flag or
// falls back to looking up (or creating) an author matching config user.
func getActingAuthorID(cmd *cobra.Command, ctx context.Context, r *repo.Repository) (string, error) {
	accountName, _ := cmd.Root().Flags().GetString("account")
	if accountName != "" {
		a, err := r.Provider.GetAuthorByName(ctx, accountName)
		if err != nil {
			return "", fmt.Errorf("--account %q: %w", accountName, err)
		}
		if a.DeletedAt != nil {
			return "", fmt.Errorf("--account %q: author has been deleted", accountName)
		}
		return a.ID, nil
	}

	// Fall back to config user — try to find existing author matching config
	userName := r.Config.GetUserName()
	userEmail := r.Config.GetUserEmail()
	if userName == "" {
		return "", fmt.Errorf("no --account flag and user.name not configured")
	}

	a, err := r.Provider.GetAuthorByName(ctx, userName)
	if err == nil {
		return a.ID, nil
	}

	// Auto-create a root author for the config user
	a = &db.Author{
		ID:          util.NewULID(),
		Name:        userName,
		Email:       &userEmail,
		Kind:        db.AuthorKindHuman,
		Permissions: db.PermAll,
		CreatedAt:   time.Now().UTC(),
	}
	if err := r.Provider.CreateAuthor(ctx, a); err != nil {
		return "", fmt.Errorf("auto-create author: %w", err)
	}
	return a.ID, nil
}

func runCLNew(cmd *cobra.Command, args []string) error {
	title, _ := cmd.Flags().GetString("title")
	desc, _ := cmd.Flags().GetString("description")

	r, err := repo.Open()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := r.Connect(ctx); err != nil {
		return err
	}
	defer r.Close()

	authorID, err := getActingAuthorID(cmd, ctx, r)
	if err != nil {
		return err
	}

	// Create a commit from staged changes
	commitOpts := repo.CommitOptions{
		Message: title,
	}
	if title == "" {
		commitOpts.Message = "CL draft"
	}
	commit, err := r.Commit(ctx, commitOpts)
	if err != nil {
		return err
	}

	// Create the CL
	now := time.Now().UTC()
	cl := &db.CL{
		ID:          util.NewULID(),
		AuthorID:    authorID,
		Title:       title,
		Description: desc,
		Status:      db.CLStatusDraft,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if title == "" {
		cl.Title = "CL draft"
	}

	if err := r.Provider.CreateCL(ctx, cl); err != nil {
		return err
	}

	// Create initial patch set
	ps := &db.PatchSet{
		ID:         util.NewULID(),
		CLID:       cl.ID,
		Number:     1,
		CommitHash: commit.ID,
		CreatedAt:  now,
	}
	if err := r.Provider.CreatePatchSet(ctx, ps); err != nil {
		return err
	}

	accountName, _ := cmd.Root().Flags().GetString("account")

	fmt.Printf("Created CL %s\n", cl.ID)
	fmt.Printf("  Title:     %s\n", cl.Title)
	fmt.Printf("  Status:    %s\n", cl.Status)
	fmt.Printf("  Patch set: #%d (%s)\n", ps.Number, ps.CommitHash[:12])
	if accountName != "" {
		fmt.Printf("  (acting as %s)\n", accountName)
	}

	return nil
}

func runCLUpdate(cmd *cobra.Command, args []string) error {
	clID := args[0]

	r, err := repo.Open()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := r.Connect(ctx); err != nil {
		return err
	}
	defer r.Close()

	// Verify CL exists
	cl, err := r.Provider.GetCL(ctx, clID)
	if err != nil {
		return err
	}

	if cl.Status == db.CLStatusSubmitted || cl.Status == db.CLStatusAbandoned {
		return fmt.Errorf("cannot update CL in %s status", cl.Status)
	}

	// Create a commit from staged changes
	commit, err := r.Commit(ctx, repo.CommitOptions{
		Message: fmt.Sprintf("Patch set for CL %s", clID),
	})
	if err != nil {
		return err
	}

	// Get current highest patch set number
	latest, err := r.Provider.GetLatestPatchSet(ctx, clID)
	nextNum := 1
	if err == nil {
		nextNum = latest.Number + 1
	}

	ps := &db.PatchSet{
		ID:         util.NewULID(),
		CLID:       clID,
		Number:     nextNum,
		CommitHash: commit.ID,
		CreatedAt:  time.Now().UTC(),
	}
	if err := r.Provider.CreatePatchSet(ctx, ps); err != nil {
		return err
	}

	// Move to active if draft
	if cl.Status == db.CLStatusDraft {
		if err := r.Provider.UpdateCLStatus(ctx, clID, db.CLStatusActive); err != nil {
			return err
		}
	}

	accountName, _ := cmd.Root().Flags().GetString("account")

	fmt.Printf("Updated CL %s\n", cl.ID)
	fmt.Printf("  Patch set: #%d (%s)\n", ps.Number, ps.CommitHash[:12])
	if accountName != "" {
		fmt.Printf("  (acting as %s)\n", accountName)
	}

	return nil
}

func runCLList(cmd *cobra.Command, args []string) error {
	statusFilter, _ := cmd.Flags().GetString("status")

	r, err := repo.Open()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.Connect(ctx); err != nil {
		return err
	}
	defer r.Close()

	cls, err := r.Provider.ListCLs(ctx, db.CLStatus(statusFilter), 100)
	if err != nil {
		return err
	}

	if len(cls) == 0 {
		fmt.Println("No CLs found")
		return nil
	}

	for _, cl := range cls {
		submitted := ""
		if cl.SubmittedAs != nil {
			submitted = fmt.Sprintf(" -> %s", *cl.SubmittedAs)
		}
		fmt.Printf("%-26s  %-10s  %s%s\n", cl.ID, cl.Status, cl.Title, submitted)
	}

	return nil
}

func runCLShow(cmd *cobra.Command, args []string) error {
	clID := args[0]

	r, err := repo.Open()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.Connect(ctx); err != nil {
		return err
	}
	defer r.Close()

	cl, err := r.Provider.GetCL(ctx, clID)
	if err != nil {
		return err
	}

	fmt.Printf("CL %s\n", cl.ID)
	fmt.Printf("  Title:       %s\n", cl.Title)
	fmt.Printf("  Status:      %s\n", cl.Status)
	fmt.Printf("  Author:      %s\n", cl.AuthorID)
	if cl.Description != "" {
		fmt.Printf("  Description: %s\n", cl.Description)
	}
	fmt.Printf("  Created:     %s\n", cl.CreatedAt.Format(time.RFC3339))
	fmt.Printf("  Updated:     %s\n", cl.UpdatedAt.Format(time.RFC3339))
	if cl.SubmittedAs != nil {
		fmt.Printf("  Submitted:   %s\n", *cl.SubmittedAs)
	}
	if cl.ParentCLID != nil {
		fmt.Printf("  Parent CL:   %s\n", *cl.ParentCLID)
	}

	// Patch sets
	pss, err := r.Provider.GetPatchSetsForCL(ctx, clID)
	if err == nil && len(pss) > 0 {
		fmt.Println()
		fmt.Println("Patch sets:")
		for _, ps := range pss {
			fmt.Printf("  #%-4d  %s  %s\n", ps.Number, ps.CommitHash, ps.CreatedAt.Format(time.RFC3339))
		}
	}

	// Comments
	comments, err := r.Provider.GetCommentsForCL(ctx, clID)
	if err == nil && len(comments) > 0 {
		fmt.Println()
		fmt.Println("Comments:")
		for _, c := range comments {
			location := ""
			if c.Path != nil {
				location = fmt.Sprintf(" [%s", *c.Path)
				if c.Line != nil {
					location += fmt.Sprintf(":%d", *c.Line)
				}
				location += "]"
			}
			fmt.Printf("  %s by %s%s:\n    %s\n",
				c.CreatedAt.Format(time.RFC3339), c.AuthorID, location, c.Body)
		}
	}

	// Votes
	votes, err := r.Provider.GetVotesForCL(ctx, clID)
	if err == nil && len(votes) > 0 {
		fmt.Println()
		fmt.Println("Votes:")
		for _, v := range votes {
			sign := ""
			if v.Score > 0 {
				sign = "+"
			}
			fmt.Printf("  %s%d by %s\n", sign, v.Score, v.AuthorID)
		}
	}

	return nil
}

func runCLComment(cmd *cobra.Command, args []string) error {
	clID := args[0]
	body, _ := cmd.Flags().GetString("message")
	filePath, _ := cmd.Flags().GetString("path")
	line, _ := cmd.Flags().GetInt("line")

	r, err := repo.Open()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.Connect(ctx); err != nil {
		return err
	}
	defer r.Close()

	// Verify CL exists
	if _, err := r.Provider.GetCL(ctx, clID); err != nil {
		return err
	}

	authorID, err := getActingAuthorID(cmd, ctx, r)
	if err != nil {
		return err
	}

	comment := &db.ReviewComment{
		ID:        util.NewULID(),
		CLID:      clID,
		AuthorID:  authorID,
		Body:      body,
		CreatedAt: time.Now().UTC(),
	}

	if filePath != "" {
		comment.Path = &filePath
	}
	if line > 0 {
		comment.Line = &line
	}

	if err := r.Provider.CreateReviewComment(ctx, comment); err != nil {
		return err
	}

	accountName, _ := cmd.Root().Flags().GetString("account")

	fmt.Printf("Comment added to CL %s\n", clID)
	if accountName != "" {
		fmt.Printf("  (acting as %s)\n", accountName)
	}

	return nil
}

func runCLVote(cmd *cobra.Command, args []string) error {
	clID := args[0]
	score, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid score %q: must be an integer", args[1])
	}

	r, err := repo.Open()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.Connect(ctx); err != nil {
		return err
	}
	defer r.Close()

	// Verify CL exists
	if _, err := r.Provider.GetCL(ctx, clID); err != nil {
		return err
	}

	authorID, err := getActingAuthorID(cmd, ctx, r)
	if err != nil {
		return err
	}

	vote := &db.ReviewVote{
		CLID:      clID,
		AuthorID:  authorID,
		Score:     score,
		UpdatedAt: time.Now().UTC(),
	}

	if err := r.Provider.SetReviewVote(ctx, vote); err != nil {
		return err
	}

	accountName, _ := cmd.Root().Flags().GetString("account")

	sign := ""
	if score > 0 {
		sign = "+"
	}
	fmt.Printf("Voted %s%d on CL %s\n", sign, score, clID)
	if accountName != "" {
		fmt.Printf("  (acting as %s)\n", accountName)
	}

	return nil
}

func runCLSubmit(cmd *cobra.Command, args []string) error {
	clID := args[0]

	r, err := repo.Open()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := r.Connect(ctx); err != nil {
		return err
	}
	defer r.Close()

	// Verify CL exists and is in a submittable state
	cl, err := r.Provider.GetCL(ctx, clID)
	if err != nil {
		return err
	}

	if cl.Status == db.CLStatusSubmitted {
		return fmt.Errorf("CL already submitted")
	}
	if cl.Status == db.CLStatusAbandoned {
		return fmt.Errorf("cannot submit abandoned CL")
	}

	// Get the latest patch set
	ps, err := r.Provider.GetLatestPatchSet(ctx, clID)
	if err != nil {
		return fmt.Errorf("get latest patch set: %w", err)
	}

	// Get the patch set's commit to find what files it touched
	patchBlobs, err := r.Provider.GetBlobsAtCommit(ctx, ps.CommitHash)
	if err != nil {
		return fmt.Errorf("get patch set blobs: %w", err)
	}

	if len(patchBlobs) == 0 {
		return fmt.Errorf("patch set has no changes")
	}

	// Create a new commit with the CL's content applied to HEAD
	now := time.Now().UTC()
	commitID := util.NewULIDWithTime(now)

	headID, err := r.Provider.GetHead(ctx)
	if err != nil {
		return fmt.Errorf("get HEAD: %w", err)
	}

	var parentID *string
	if headID != "" {
		parentID = &headID
	}

	// Build blobs for the new commit — copy patch set's blobs with new commit ID
	var newBlobs []*db.Blob
	for _, b := range patchBlobs {
		newBlob := &db.Blob{
			Path:          b.Path,
			CommitID:      commitID,
			Content:       b.Content,
			ContentHash:   b.ContentHash,
			Mode:          b.Mode,
			IsBinary:      b.IsBinary,
			IsSymlink:     b.IsSymlink,
			SymlinkTarget: b.SymlinkTarget,
		}
		newBlobs = append(newBlobs, newBlob)
	}

	// Build tree entries for tree hash
	var treeEntries []util.TreeEntry
	for _, b := range newBlobs {
		if b.ContentHash != nil {
			treeEntries = append(treeEntries, util.TreeEntry{
				Mode:        b.Mode,
				Path:        b.Path,
				ContentHash: b.ContentHash,
			})
		}
	}
	treeHash := util.ComputeTreeHash(treeEntries)

	authorName := r.Config.GetUserName()
	authorEmail := r.Config.GetUserEmail()

	commit := &db.Commit{
		ID:             commitID,
		ParentID:       parentID,
		TreeHash:       treeHash,
		Message:        cl.Title,
		AuthorName:     authorName,
		AuthorEmail:    authorEmail,
		AuthoredAt:     now,
		CommitterName:  authorName,
		CommitterEmail: authorEmail,
		CommittedAt:    now,
	}

	// Create commit and blobs
	if err := r.Provider.CreateCommit(ctx, commit); err != nil {
		return fmt.Errorf("create commit: %w", err)
	}
	if err := r.Provider.CreateBlobs(ctx, newBlobs); err != nil {
		return fmt.Errorf("create blobs: %w", err)
	}

	// Update HEAD
	if err := r.Provider.SetHead(ctx, commitID); err != nil {
		return fmt.Errorf("set HEAD: %w", err)
	}

	// Mark CL as submitted
	if err := r.Provider.SubmitCL(ctx, clID, commitID); err != nil {
		return fmt.Errorf("mark CL submitted: %w", err)
	}

	fmt.Printf("Submitted CL %s\n", cl.ID)
	fmt.Printf("  Commit: %s\n", commitID)
	fmt.Printf("  Title:  %s\n", cl.Title)

	return nil
}

func runCLAbandon(cmd *cobra.Command, args []string) error {
	clID := args[0]

	r, err := repo.Open()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.Connect(ctx); err != nil {
		return err
	}
	defer r.Close()

	cl, err := r.Provider.GetCL(ctx, clID)
	if err != nil {
		return err
	}

	if cl.Status == db.CLStatusSubmitted {
		return fmt.Errorf("cannot abandon a submitted CL")
	}

	if err := r.Provider.UpdateCLStatus(ctx, clID, db.CLStatusAbandoned); err != nil {
		return err
	}

	fmt.Printf("Abandoned CL %s\n", cl.ID)
	return nil
}

func runCLStack(cmd *cobra.Command, args []string) error {
	clID := args[0]

	r, err := repo.Open()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.Connect(ctx); err != nil {
		return err
	}
	defer r.Close()

	stack, err := r.Provider.GetCLStack(ctx, clID)
	if err != nil {
		return err
	}

	fmt.Println("CL Stack:")
	for i, cl := range stack {
		marker := ""
		if i == 0 {
			marker = " (current)"
		}
		if cl.ParentCLID == nil {
			marker += " [root]"
		}
		fmt.Printf("  %s  %-10s  %s%s\n", cl.ID, cl.Status, cl.Title, marker)
	}

	return nil
}
