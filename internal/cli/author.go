package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/imgajeed76/pgit/v4/internal/repo"
	"github.com/imgajeed76/pgit/v4/internal/util"
	"github.com/spf13/cobra"
)

func newAuthorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "author",
		Short: "Manage authors (users, bots, service accounts)",
		Long: `Manage authors in the pgit code review system.

Authors can be root accounts (human users) or sub-authors (LLM agents,
service accounts) that inherit and are constrained by their parent's
permissions.`,
	}

	cmd.AddCommand(
		newAuthorCreateCmd(),
		newAuthorListCmd(),
		newAuthorShowCmd(),
		newAuthorDeleteCmd(),
		newAuthorTokenCmd(),
	)

	return cmd
}

func newAuthorCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new author",
		Long: `Create a new root or sub-author.

Root authors (no --parent) require --email. Sub-authors inherit their
parent's kind if --kind is not specified, and their parent's permissions
if --permissions is not specified.

Sub-author permissions cannot exceed their parent's effective permissions.`,
		Args: cobra.ExactArgs(1),
		RunE: runAuthorCreate,
	}

	cmd.Flags().String("kind", "", "Author kind: human, llm, or service")
	cmd.Flags().String("email", "", "Email address (required for root authors)")
	cmd.Flags().String("parent", "", "Parent author ID or name (creates a sub-author)")
	cmd.Flags().String("permissions", "", "Comma-separated permissions (e.g. read,comment,vote)")

	return cmd
}

func newAuthorListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List authors",
		Long:  `List all authors. By default excludes soft-deleted authors.`,
		RunE:  runAuthorList,
	}

	cmd.Flags().Bool("all", false, "Include soft-deleted authors")

	return cmd
}

func newAuthorShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id-or-name>",
		Short: "Show author details",
		Long:  `Show an author's details including delegation chain, effective permissions, and children.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runAuthorShow,
	}
}

func newAuthorDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id-or-name>",
		Short: "Soft-delete an author",
		Long:  `Mark an author as deleted. The author record is preserved but excluded from listings.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runAuthorDelete,
	}
}

func newAuthorTokenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "token <id-or-name>",
		Short: "Generate an auth token for an author",
		Long: `Generate a new authentication token for an author. The token is printed
once and cannot be retrieved again. The bcrypt hash is stored in the database.`,
		Args: cobra.ExactArgs(1),
		RunE: runAuthorToken,
	}
}

// resolveAuthor looks up an author by name first, then by ID.
func resolveAuthor(ctx context.Context, r *repo.Repository, nameOrID string) (*db.Author, error) {
	a, err := r.Provider.GetAuthorByName(ctx, nameOrID)
	if err == nil {
		return a, nil
	}
	// Try as ID
	a, err = r.Provider.GetAuthor(ctx, nameOrID)
	if err != nil {
		return nil, fmt.Errorf("author not found: %s", nameOrID)
	}
	return a, nil
}

func runAuthorCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	kind, _ := cmd.Flags().GetString("kind")
	email, _ := cmd.Flags().GetString("email")
	parentFlag, _ := cmd.Flags().GetString("parent")
	permsFlag, _ := cmd.Flags().GetString("permissions")

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

	a := &db.Author{
		ID:        util.NewULID(),
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}

	if parentFlag != "" {
		// Sub-author: resolve parent
		parent, err := resolveAuthor(ctx, r, parentFlag)
		if err != nil {
			return fmt.Errorf("parent: %w", err)
		}
		a.ParentID = &parent.ID

		// Inherit kind from parent if not specified
		if kind == "" {
			a.Kind = parent.Kind
		} else {
			a.Kind = db.AuthorKind(kind)
		}

		// Inherit permissions from parent if not specified
		if permsFlag == "" {
			a.Permissions = parent.Permissions
		} else {
			perms, err := db.ParsePermissions(permsFlag)
			if err != nil {
				return err
			}
			// Validate: sub-author permissions cannot exceed parent's effective permissions
			parentEffective, err := r.Provider.ComputeEffectivePermissions(ctx, parent.ID)
			if err != nil {
				return fmt.Errorf("compute parent permissions: %w", err)
			}
			excess := perms &^ parentEffective
			if excess != 0 {
				return fmt.Errorf("sub-author permissions (%s) exceed parent's effective permissions (%s)",
					perms.String(), parentEffective.String())
			}
			a.Permissions = perms
		}
	} else {
		// Root author: require email
		if email == "" {
			return fmt.Errorf("--email is required for root authors (no --parent)")
		}
		a.Email = &email

		if kind == "" {
			a.Kind = db.AuthorKindHuman
		} else {
			a.Kind = db.AuthorKind(kind)
		}

		if permsFlag == "" {
			a.Permissions = db.PermAll
		} else {
			perms, err := db.ParsePermissions(permsFlag)
			if err != nil {
				return err
			}
			a.Permissions = perms
		}
	}

	// Validate kind
	switch a.Kind {
	case db.AuthorKindHuman, db.AuthorKindLLM, db.AuthorKindService:
		// ok
	default:
		return fmt.Errorf("invalid kind: %q (must be human, llm, or service)", a.Kind)
	}

	if err := r.Provider.CreateAuthor(ctx, a); err != nil {
		return err
	}

	fmt.Printf("Created author %q (%s)\n", a.Name, a.ID)
	fmt.Printf("  kind:        %s\n", a.Kind)
	fmt.Printf("  permissions: %s\n", a.Permissions.String())
	if a.ParentID != nil {
		fmt.Printf("  parent:      %s\n", *a.ParentID)
	}
	if a.Email != nil {
		fmt.Printf("  email:       %s\n", *a.Email)
	}

	return nil
}

func runAuthorList(cmd *cobra.Command, args []string) error {
	includeAll, _ := cmd.Flags().GetBool("all")

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

	authors, err := r.Provider.ListAuthors(ctx, includeAll)
	if err != nil {
		return err
	}

	if len(authors) == 0 {
		fmt.Println("No authors found")
		return nil
	}

	for _, a := range authors {
		deleted := ""
		if a.DeletedAt != nil {
			deleted = " [deleted]"
		}
		parent := ""
		if a.ParentID != nil {
			parent = fmt.Sprintf(" (parent: %s)", *a.ParentID)
		}
		fmt.Printf("%-26s  %-20s  %-8s  %s%s%s\n",
			a.ID, a.Name, a.Kind, a.Permissions.String(), parent, deleted)
	}

	return nil
}

func runAuthorShow(cmd *cobra.Command, args []string) error {
	nameOrID := args[0]

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

	a, err := resolveAuthor(ctx, r, nameOrID)
	if err != nil {
		return err
	}

	fmt.Printf("Author: %s\n", a.Name)
	fmt.Printf("  ID:          %s\n", a.ID)
	fmt.Printf("  Kind:        %s\n", a.Kind)
	if a.Email != nil {
		fmt.Printf("  Email:       %s\n", *a.Email)
	}
	fmt.Printf("  Permissions: %s\n", a.Permissions.String())
	fmt.Printf("  Created:     %s\n", a.CreatedAt.Format(time.RFC3339))
	if a.DeletedAt != nil {
		fmt.Printf("  Deleted:     %s\n", a.DeletedAt.Format(time.RFC3339))
	}

	// Effective permissions
	effective, err := r.Provider.ComputeEffectivePermissions(ctx, a.ID)
	if err == nil {
		fmt.Printf("  Effective:   %s\n", effective.String())
	}

	// Delegation chain
	chain, err := r.Provider.GetAuthorChain(ctx, a.ID)
	if err == nil && len(chain) > 1 {
		fmt.Println()
		fmt.Println("Delegation chain:")
		for i, link := range chain {
			indent := strings.Repeat("  ", i)
			marker := ""
			if i == 0 {
				marker = " (this author)"
			}
			if link.ParentID == nil {
				marker += " [root]"
			}
			fmt.Printf("  %s%s (%s)%s\n", indent, link.Name, link.ID, marker)
		}
	}

	// Children
	children, err := r.Provider.GetAuthorChildren(ctx, a.ID)
	if err == nil && len(children) > 0 {
		fmt.Println()
		fmt.Println("Children:")
		for _, child := range children {
			deleted := ""
			if child.DeletedAt != nil {
				deleted = " [deleted]"
			}
			fmt.Printf("  %s (%s) %s %s%s\n",
				child.Name, child.ID, child.Kind, child.Permissions.String(), deleted)
		}
	}

	return nil
}

func runAuthorDelete(cmd *cobra.Command, args []string) error {
	nameOrID := args[0]

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

	a, err := resolveAuthor(ctx, r, nameOrID)
	if err != nil {
		return err
	}

	if err := r.Provider.SoftDeleteAuthor(ctx, a.ID); err != nil {
		return err
	}

	fmt.Printf("Deleted author %q (%s)\n", a.Name, a.ID)
	return nil
}

func runAuthorToken(cmd *cobra.Command, args []string) error {
	nameOrID := args[0]

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

	a, err := resolveAuthor(ctx, r, nameOrID)
	if err != nil {
		return err
	}

	token, err := r.Provider.CreateAuthorToken(ctx, a.ID)
	if err != nil {
		return err
	}

	fmt.Printf("Token for %q (shown once, store it safely):\n", a.Name)
	fmt.Println(token)
	return nil
}
