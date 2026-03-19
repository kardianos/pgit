package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/imgajeed76/pgit/v4/internal/repo"
	"github.com/spf13/cobra"
)

func newRefCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ref",
		Short: "Manage refs (branches, tags, user refs)",
		Long: `Manage named references to commits.

Refs can be global (e.g. refs/heads/main) or per-user
(stored under refs/users/<email>/).`,
	}

	cmd.AddCommand(
		newRefListCmd(),
		newRefSetCmd(),
		newRefDeleteCmd(),
	)

	return cmd
}

func newRefListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List refs",
		Long:  `List all refs, optionally filtered to a user namespace.`,
		RunE:  runRefList,
	}

	cmd.Flags().String("user", "", "Filter to user namespace (email)")

	return cmd
}

func newRefSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <name> <commit>",
		Short: "Set a ref to point to a commit",
		Long: `Create or update a ref. If --account is set, the ref is stored
in the user's namespace (refs/users/<email>/<name>).`,
		Args: cobra.ExactArgs(2),
		RunE: runRefSet,
	}

	return cmd
}

func newRefDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a ref",
		Long: `Delete a ref. If --account is set, deletes from the user's
namespace (refs/users/<email>/<name>).`,
		Args: cobra.ExactArgs(1),
		RunE: runRefDelete,
	}
}

func runRefList(cmd *cobra.Command, args []string) error {
	userEmail, _ := cmd.Flags().GetString("user")

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

	if userEmail != "" {
		refs, err := r.Provider.GetUserRefs(ctx, userEmail)
		if err != nil {
			return err
		}
		if len(refs) == 0 {
			fmt.Printf("No refs found for user %s\n", userEmail)
			return nil
		}
		for _, ref := range refs {
			fmt.Printf("%-60s  %s\n", ref.Name, ref.CommitID)
		}
		return nil
	}

	refs, err := r.Provider.GetAllRefs(ctx)
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		fmt.Println("No refs found")
		return nil
	}
	for _, ref := range refs {
		fmt.Printf("%-60s  %s\n", ref.Name, ref.CommitID)
	}

	return nil
}

func runRefSet(cmd *cobra.Command, args []string) error {
	name := args[0]
	commitID := args[1]

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

	accountName, _ := cmd.Root().Flags().GetString("account")
	if accountName != "" {
		// Resolve account to get email for user namespace
		a, aErr := r.Provider.GetAuthorByName(ctx, accountName)
		if aErr != nil {
			return fmt.Errorf("--account %q: %w", accountName, aErr)
		}
		if a.Email == nil {
			return fmt.Errorf("--account %q: author has no email", accountName)
		}
		if err := r.Provider.SetUserRef(ctx, *a.Email, name, commitID); err != nil {
			return err
		}
		fmt.Printf("Set refs/users/%s/%s -> %s\n", *a.Email, name, commitID)
		return nil
	}

	if err := r.Provider.SetRef(ctx, name, commitID); err != nil {
		return err
	}
	fmt.Printf("Set %s -> %s\n", name, commitID)

	return nil
}

func runRefDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

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

	accountName, _ := cmd.Root().Flags().GetString("account")
	if accountName != "" {
		a, aErr := r.Provider.GetAuthorByName(ctx, accountName)
		if aErr != nil {
			return fmt.Errorf("--account %q: %w", accountName, aErr)
		}
		if a.Email == nil {
			return fmt.Errorf("--account %q: author has no email", accountName)
		}
		if err := r.Provider.DeleteUserRef(ctx, *a.Email, name); err != nil {
			return err
		}
		fmt.Printf("Deleted refs/users/%s/%s\n", *a.Email, name)
		return nil
	}

	if err := r.Provider.DeleteRef(ctx, name); err != nil {
		return err
	}
	fmt.Printf("Deleted %s\n", name)

	return nil
}
