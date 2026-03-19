package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	pgitfuse "github.com/imgajeed76/pgit/v4/fuse"
	"github.com/imgajeed76/pgit/v4/internal/repo"
	"github.com/spf13/cobra"
)

func newMountCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mount <ref> <mountpoint>",
		Short: "Mount a commit or CL as a read-only FUSE filesystem",
		Long: `Mount a pgit commit as a read-only FUSE directory tree.

The <ref> argument can be:
  HEAD           the current head commit
  <commit-id>    a specific commit (full or partial ID)
  cl/<cl-id>     the latest patch set of a change list

Files are fetched lazily from PostgreSQL on first read.

Examples:
  pgit mount HEAD /tmp/pgit-head
  pgit mount abc123 /tmp/pgit-abc
  pgit mount cl/42 /tmp/pgit-cl42`,
		Args: cobra.ExactArgs(2),
		RunE: runMount,
	}

	cmd.Flags().Bool("background", false, "Mount in background and return immediately")

	return cmd
}

func newUnmountCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unmount <mountpoint>",
		Short: "Unmount a FUSE filesystem",
		Long:  `Unmount a previously mounted pgit FUSE filesystem.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runUnmount,
	}
}

func runMount(cmd *cobra.Command, args []string) error {
	ref := args[0]
	mountpoint := args[1]

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r, err := repo.Open()
	if err != nil {
		return err
	}
	if err := r.Connect(ctx); err != nil {
		return err
	}
	// Note: we do NOT defer r.Close() here because the provider must stay
	// alive for the lifetime of the mount.  Close happens on unmount.

	commitID, err := resolveRef(ctx, r, ref)
	if err != nil {
		return err
	}

	bg, _ := cmd.Flags().GetBool("background")

	if bg {
		done, err := pgitfuse.MountInBackground(mountpoint, r.Provider, commitID)
		if err != nil {
			r.Close()
			return err
		}
		shortID := commitID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		fmt.Printf("Mounted %s at %s (commit %s)\n", ref, mountpoint, shortID)
		fmt.Println("Use 'pgit unmount", mountpoint+"' to unmount.")
		// Block until the mount is done (unmounted or error).
		// The FUSE serving runs in a background goroutine; we must
		// keep the process alive or the mount dies with us.
		<-done
		r.Close()
		return nil
	}

	shortID := commitID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	fmt.Printf("Mounting %s at %s (commit %s)\n", ref, mountpoint, shortID)
	fmt.Println("Press Ctrl+C or run 'pgit unmount", mountpoint+"' to unmount.")

	err = pgitfuse.Mount(mountpoint, r.Provider, commitID)
	r.Close()
	return err
}

func runUnmount(cmd *cobra.Command, args []string) error {
	mountpoint := args[0]
	if err := pgitfuse.Unmount(mountpoint); err != nil {
		return fmt.Errorf("unmounting %s: %w", mountpoint, err)
	}
	fmt.Printf("Unmounted %s\n", mountpoint)
	return nil
}

// resolveRef converts a user-supplied ref to a commit ID.
func resolveRef(ctx context.Context, r *repo.Repository, ref string) (string, error) {
	p := r.Provider

	switch {
	case ref == "HEAD":
		return p.GetHead(ctx)

	case strings.HasPrefix(ref, "cl/"):
		clID := strings.TrimPrefix(ref, "cl/")
		cl, err := p.GetCL(ctx, clID)
		if err != nil {
			return "", fmt.Errorf("looking up CL %s: %w", clID, err)
		}
		_ = cl // validate it exists
		ps, err := p.GetLatestPatchSet(ctx, clID)
		if err != nil {
			return "", fmt.Errorf("getting latest patch set for CL %s: %w", clID, err)
		}
		return ps.CommitHash, nil

	default:
		// Try as a full or partial commit ID.
		c, err := p.FindCommitByPartialID(ctx, ref)
		if err != nil {
			return "", fmt.Errorf("resolving ref %q: %w", ref, err)
		}
		return c.ID, nil
	}
}
