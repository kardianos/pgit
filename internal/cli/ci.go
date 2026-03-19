package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/imgajeed76/pgit/v4/internal/repo"
	"github.com/imgajeed76/pgit/v4/internal/util"
	"github.com/spf13/cobra"
)

func newCICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ci",
		Short: "Manage CI results for change lists",
		Long: `Manage CI (continuous integration) results associated with CLs.

CI results track job runs against specific patch sets, including
status (pending, running, passed, failed), logs, and artifacts.`,
	}

	cmd.AddCommand(
		newCIStatusCmd(),
		newCITriggerCmd(),
		newCIUpdateCmd(),
	)

	return cmd
}

func newCIStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <cl-id>",
		Short: "Show CI results for a CL",
		Long:  `Display all CI job results for a change list.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runCIStatus,
	}

	cmd.Flags().Int("patchset", 0, "Filter to a specific patch set number")

	return cmd
}

func newCITriggerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trigger <cl-id>",
		Short: "Create a pending CI result",
		Long:  `Create a new CI result in pending status for a CL's latest patch set.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runCITrigger,
	}

	cmd.Flags().String("job", "", "Job name (required)")
	_ = cmd.MarkFlagRequired("job")

	return cmd
}

func newCIUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <result-id>",
		Short: "Update a CI result",
		Long:  `Update the status and optionally attach a log file to a CI result.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runCIUpdate,
	}

	cmd.Flags().String("status", "", "New status: pending, running, passed, failed (required)")
	cmd.Flags().String("log", "", "Path to log file to attach")
	_ = cmd.MarkFlagRequired("status")

	return cmd
}

func runCIStatus(cmd *cobra.Command, args []string) error {
	clID := args[0]
	patchSetNum, _ := cmd.Flags().GetInt("patchset")

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

	var results []*db.CIResult
	if patchSetNum > 0 {
		results, err = r.Provider.GetCIResultsForPatchSet(ctx, clID, patchSetNum)
	} else {
		results, err = r.Provider.GetCIResultsForCL(ctx, clID)
	}
	if err != nil {
		return err
	}

	if len(results) == 0 {
		fmt.Println("No CI results found")
		return nil
	}

	for _, ci := range results {
		fmt.Printf("%-26s  PS#%-3d  %-10s  %-8s  %s\n",
			ci.ID, ci.PatchSet, ci.JobName, ci.Status,
			ci.CreatedAt.Format(time.RFC3339))
		if ci.LogBlobHash != nil {
			fmt.Printf("  log: %s\n", *ci.LogBlobHash)
		}
		if ci.Artifacts != nil {
			fmt.Printf("  artifacts: %s\n", string(ci.Artifacts))
		}
	}

	return nil
}

func runCITrigger(cmd *cobra.Command, args []string) error {
	clID := args[0]
	jobName, _ := cmd.Flags().GetString("job")

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

	// Get latest patch set number
	ps, err := r.Provider.GetLatestPatchSet(ctx, clID)
	if err != nil {
		return fmt.Errorf("get latest patch set: %w", err)
	}

	now := time.Now().UTC()
	ci := &db.CIResult{
		ID:          util.NewULID(),
		CLID:        clID,
		PatchSet:    ps.Number,
		JobName:     jobName,
		Status:      db.CIStatusPending,
		TriggeredBy: authorID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := r.Provider.CreateCIResult(ctx, ci); err != nil {
		return err
	}

	fmt.Printf("Created CI result %s\n", ci.ID)
	fmt.Printf("  CL:        %s\n", ci.CLID)
	fmt.Printf("  Patch set: #%d\n", ci.PatchSet)
	fmt.Printf("  Job:       %s\n", ci.JobName)
	fmt.Printf("  Status:    %s\n", ci.Status)

	return nil
}

func runCIUpdate(cmd *cobra.Command, args []string) error {
	resultID := args[0]
	statusStr, _ := cmd.Flags().GetString("status")
	logPath, _ := cmd.Flags().GetString("log")

	status := db.CIStatus(statusStr)
	switch status {
	case db.CIStatusPending, db.CIStatusRunning, db.CIStatusPassed, db.CIStatusFailed:
		// valid
	default:
		return fmt.Errorf("invalid status %q: must be pending, running, passed, or failed", statusStr)
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

	// Get existing CI result
	ci, err := r.Provider.GetCIResult(ctx, resultID)
	if err != nil {
		return err
	}

	ci.Status = status

	if logPath != "" {
		logData, readErr := os.ReadFile(logPath)
		if readErr != nil {
			return fmt.Errorf("read log file: %w", readErr)
		}
		// Store log content as a JSON artifact with the hash
		logStr := string(logData)
		ci.LogBlobHash = &logStr

		// Also store in artifacts as JSON
		artifacts := map[string]string{"log": logStr}
		artBytes, _ := json.Marshal(artifacts)
		ci.Artifacts = artBytes
	}

	if err := r.Provider.UpdateCIResult(ctx, ci); err != nil {
		return err
	}

	fmt.Printf("Updated CI result %s\n", ci.ID)
	fmt.Printf("  Status: %s\n", ci.Status)

	return nil
}
