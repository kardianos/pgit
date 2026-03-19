package cli

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// executeCommand runs a cobra command with the given args and captures
// stdout and stderr. Because the CLI writes to os.Stdout/os.Stderr
// directly (via fmt.Printf, not cmd.OutOrStdout()), we redirect the
// real file descriptors through a pipe.
func executeCommand(root *cobra.Command, args ...string) (stdout string, stderr string, err error) {
	// --- capture os.Stdout ---
	origStdout := os.Stdout
	rOut, wOut, pipeErr := os.Pipe()
	if pipeErr != nil {
		return "", "", pipeErr
	}
	os.Stdout = wOut

	// --- capture os.Stderr ---
	origStderr := os.Stderr
	rErr, wErr, pipeErr := os.Pipe()
	if pipeErr != nil {
		wOut.Close()
		rOut.Close()
		os.Stdout = origStdout
		return "", "", pipeErr
	}
	os.Stderr = wErr

	// Read from pipes concurrently to avoid deadlock when the command
	// writes more than the OS pipe buffer (typically 64KB).
	var outBytes, errBytes []byte
	done := make(chan struct{})
	go func() {
		outBytes, _ = io.ReadAll(rOut)
		rOut.Close()
		done <- struct{}{}
	}()
	go func() {
		errBytes, _ = io.ReadAll(rErr)
		rErr.Close()
		done <- struct{}{}
	}()

	// Run the command
	root.SetArgs(args)
	err = root.Execute()

	// Close write-ends so the readers see EOF
	wOut.Close()
	wErr.Close()

	// Restore originals
	os.Stdout = origStdout
	os.Stderr = origStderr

	// Wait for both readers to finish
	<-done
	<-done

	return string(outBytes), string(errBytes), err
}

// newTestRootCmd creates a fresh root command tree identical to the
// production one. We build a new tree for every test so that flag
// state from a previous invocation does not leak.
func newTestRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "pgit",
		Short:         "A Git-like version control system backed by PostgreSQL",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
	root.PersistentFlags().Bool("no-color", false, "Disable colored output")
	root.PersistentFlags().String("account", "", "Act as a sub-author (by name)")

	root.AddCommand(
		newVersionCmd(),
		newInitCmd(),
		newConfigCmd(),
		newAddCmd(),
		newRmCmd(),
		newMvCmd(),
		newStatusCmd(),
		newCommitCmd(),
		newLogCmd(),
		newDiffCmd(),
		newShowCmd(),
		newCheckoutCmd(),
		newBlameCmd(),
		newRemoteCmd(),
		newSearchCmd(),
		newGrepCmd(),
		newSQLCmd(),
		newAnalyzeCmd(),
		newAuthorCmd(),
		newCLCmd(),
		newRefCmd(),
		newCICmd(),
		newMountCmd(),
		newUnmountCmd(),
	)

	return root
}

// assertContains checks that output contains all expected substrings.
func assertContains(t *testing.T, output string, substrs []string) {
	t.Helper()
	for _, s := range substrs {
		if !strings.Contains(output, s) {
			t.Errorf("output missing expected substring %q\n--- output ---\n%s", s, output)
		}
	}
}

