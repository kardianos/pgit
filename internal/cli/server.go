package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/imgajeed76/pgit/v4/server"
	"github.com/spf13/cobra"
)

func newServerCmd() *cobra.Command {
	var addr string
	var databaseURL string

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start the pgit HTTP server",
		Long: `Start an HTTP server that exposes the pgit API.

The server connects to a PostgreSQL database and serves JSON-over-HTTP
endpoints for commits, blobs, refs, authors, CLs, and more.

Authentication is via Bearer tokens in the Authorization header.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Determine database URL.
			if databaseURL == "" {
				databaseURL = os.Getenv("PGIT_DATABASE_URL")
			}
			if databaseURL == "" {
				return fmt.Errorf("database URL required: use --database-url or PGIT_DATABASE_URL")
			}

			// Connect to database.
			d, err := db.Connect(ctx, databaseURL)
			if err != nil {
				return fmt.Errorf("connect to database: %w", err)
			}
			defer d.Close()

			// Start server.
			srv := server.New(d, addr)
			fmt.Fprintf(os.Stderr, "pgit server listening on %s\n", addr)

			// Graceful shutdown on signal.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			errCh := make(chan error, 1)
			go func() {
				if err := srv.Start(); err != nil && err != http.ErrServerClosed {
					errCh <- err
				}
				close(errCh)
			}()

			select {
			case sig := <-sigCh:
				fmt.Fprintf(os.Stderr, "\nreceived %s, shutting down...\n", sig)
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5e9) // 5 seconds
				defer shutdownCancel()
				return srv.Shutdown(shutdownCtx)
			case err := <-errCh:
				return err
			}
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8080", "Listen address (host:port)")
	cmd.Flags().StringVar(&databaseURL, "database-url", "", "PostgreSQL connection URL")

	return cmd
}
