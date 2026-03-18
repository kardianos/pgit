package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ═══════════════════════════════════════════════════════════════════════════
// Author CRUD via CLI
// ═══════════════════════════════════════════════════════════════════════════

func TestAuthorCRUDIntegration(t *testing.T) {
	_ = setupIntegrationRepo(t)

	// We run tests sequentially because they build on each other (create then list, etc.)
	tests := []struct {
		name     string
		args     []string
		contains []string
		wantErr  bool
	}{
		{
			name:     "create root author",
			args:     []string{"author", "create", "alice", "--email", "alice@test.dev", "--kind", "human"},
			contains: []string{"Created author", "alice", "human"},
			wantErr:  false,
		},
		{
			name:     "create sub-author under alice",
			args:     []string{"author", "create", "alice-bot", "--parent", "alice", "--kind", "llm", "--permissions", "read,comment"},
			contains: []string{"Created author", "alice-bot", "llm"},
			wantErr:  false,
		},
		{
			name:     "list authors shows both",
			args:     []string{"author", "list"},
			contains: []string{"alice", "alice-bot"},
			wantErr:  false,
		},
		{
			name:     "show alice shows details",
			args:     []string{"author", "show", "alice"},
			contains: []string{"alice", "human", "alice@test.dev", "Effective"},
			wantErr:  false,
		},
		{
			name:     "show alice-bot shows chain",
			args:     []string{"author", "show", "alice-bot"},
			contains: []string{"alice-bot", "Delegation chain", "alice"},
			wantErr:  false,
		},
		{
			name:     "show alice shows children",
			args:     []string{"author", "show", "alice"},
			contains: []string{"Children", "alice-bot"},
			wantErr:  false,
		},
		{
			name:     "delete alice-bot",
			args:     []string{"author", "delete", "alice-bot"},
			contains: []string{"Deleted", "alice-bot"},
			wantErr:  false,
		},
		{
			name:     "list excludes deleted by default",
			args:     []string{"author", "list"},
			contains: []string{"alice"},
			wantErr:  false,
		},
		{
			name:     "list --all includes deleted",
			args:     []string{"author", "list", "--all"},
			contains: []string{"alice-bot", "deleted"},
			wantErr:  false,
		},
		{
			name:     "token for alice",
			args:     []string{"author", "token", "alice"},
			contains: []string{"Token for", "alice"},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newTestRootCmd()
			stdout, _, err := executeCommand(root, tt.args...)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v\nstdout: %s", err, stdout)
			}
			assertContains(t, stdout, tt.contains)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Author validation rules
// ═══════════════════════════════════════════════════════════════════════════

func TestAuthorValidationIntegration(t *testing.T) {
	_ = setupIntegrationRepo(t)

	// Create a root author with restricted permissions first
	root := newTestRootCmd()
	_, _, err := executeCommand(root, "author", "create", "restricted-parent",
		"--email", "rp@test.dev", "--permissions", "read")
	if err != nil {
		t.Fatalf("setup: create restricted-parent: %v", err)
	}

	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "root without email fails",
			args:    []string{"author", "create", "no-email-root"},
			wantErr: true,
		},
		{
			name:    "invalid kind fails",
			args:    []string{"author", "create", "bad-kind", "--email", "x@x.com", "--kind", "robot"},
			wantErr: true,
		},
		{
			name:    "sub-author exceeding parent permissions fails",
			args:    []string{"author", "create", "greedy-sub", "--parent", "restricted-parent", "--permissions", "read,comment,vote"},
			wantErr: true,
		},
		{
			name:    "sub-author within parent permissions succeeds",
			args:    []string{"author", "create", "good-sub", "--parent", "restricted-parent", "--permissions", "read"},
			wantErr: false,
		},
		{
			name:    "show nonexistent author fails",
			args:    []string{"author", "show", "nobody"},
			wantErr: true,
		},
		{
			name:    "delete nonexistent author fails",
			args:    []string{"author", "delete", "nobody"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newTestRootCmd()
			_, _, err := executeCommand(root, tt.args...)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// CL lifecycle via CLI
// ═══════════════════════════════════════════════════════════════════════════

func TestCLLifecycleIntegration(t *testing.T) {
	dir := setupIntegrationRepo(t)

	// Setup: create a file and stage it for the CL
	path := filepath.Join(dir, "feature.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc Feature() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	root := newTestRootCmd()
	if _, _, err := executeCommand(root, "add", "feature.go"); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Create a CL
	root = newTestRootCmd()
	stdout, _, err := executeCommand(root, "cl", "new", "--title", "Add feature")
	if err != nil {
		t.Fatalf("cl new: %v\nstdout: %s", err, stdout)
	}
	assertContains(t, stdout, []string{"Created CL", "Add feature", "Patch set: #1"})

	// Extract the CL ID from output: "Created CL <id>"
	clID := extractCLID(t, stdout)

	// Stage more changes for update test
	if err := os.WriteFile(path, []byte("package main\n\nfunc Feature() { return }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	root = newTestRootCmd()
	if _, _, err := executeCommand(root, "add", "feature.go"); err != nil {
		t.Fatalf("add for update: %v", err)
	}

	tests := []struct {
		name     string
		args     []string
		contains []string
		wantErr  bool
	}{
		{
			name:     "cl list shows the new CL",
			args:     []string{"cl", "list"},
			contains: []string{clID, "Add feature"},
			wantErr:  false,
		},
		{
			name:     "cl list with draft filter",
			args:     []string{"cl", "list", "--status", "draft"},
			contains: []string{clID},
			wantErr:  false,
		},
		{
			name:     "cl show displays details",
			args:     []string{"cl", "show", clID},
			contains: []string{clID, "Add feature", "draft", "Patch sets", "#1"},
			wantErr:  false,
		},
		{
			name:     "cl update pushes patch set #2",
			args:     []string{"cl", "update", clID},
			contains: []string{"Updated CL", "Patch set: #2"},
			wantErr:  false,
		},
		{
			name:     "cl comment adds a comment",
			args:     []string{"cl", "comment", clID, "-m", "Looks good!"},
			contains: []string{"Comment added"},
			wantErr:  false,
		},
		{
			name:     "cl comment with path and line",
			args:     []string{"cl", "comment", clID, "-m", "nit: rename", "--path", "feature.go", "--line", "3"},
			contains: []string{"Comment added"},
			wantErr:  false,
		},
		{
			name:     "cl vote +1",
			args:     []string{"cl", "vote", clID, "+1"},
			contains: []string{"Voted +1"},
			wantErr:  false,
		},
		{
			name:     "cl show shows comments and votes",
			args:     []string{"cl", "show", clID},
			contains: []string{"Comments", "Looks good!", "Votes", "+1"},
			wantErr:  false,
		},
		{
			name:     "cl abandon",
			args:     []string{"cl", "abandon", clID},
			contains: []string{"Abandoned CL"},
			wantErr:  false,
		},
		{
			name:     "cl list with abandoned filter",
			args:     []string{"cl", "list", "--status", "abandoned"},
			contains: []string{clID},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newTestRootCmd()
			stdout, _, err := executeCommand(root, tt.args...)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v\nstdout: %s", err, stdout)
			}
			assertContains(t, stdout, tt.contains)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// CL submit via CLI
// ═══════════════════════════════════════════════════════════════════════════

func TestCLSubmitIntegration(t *testing.T) {
	dir := setupIntegrationRepo(t)

	// Create and stage a file
	path := filepath.Join(dir, "submit.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	root := newTestRootCmd()
	if _, _, err := executeCommand(root, "add", "submit.go"); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Create a CL
	root = newTestRootCmd()
	stdout, _, err := executeCommand(root, "cl", "new", "--title", "Submit test")
	if err != nil {
		t.Fatalf("cl new: %v\nstdout: %s", err, stdout)
	}
	clID := extractCLID(t, stdout)

	tests := []struct {
		name     string
		args     []string
		contains []string
		wantErr  bool
	}{
		{
			name:     "submit CL",
			args:     []string{"cl", "submit", clID},
			contains: []string{"Submitted CL", "Commit", "Submit test"},
			wantErr:  false,
		},
		{
			name:    "submit already-submitted CL fails",
			args:    []string{"cl", "submit", clID},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newTestRootCmd()
			stdout, _, err := executeCommand(root, tt.args...)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v\nstdout: %s", err, stdout)
			}
			assertContains(t, stdout, tt.contains)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// --account flag integration
// ═══════════════════════════════════════════════════════════════════════════

func TestAccountFlagIntegration(t *testing.T) {
	dir := setupIntegrationRepo(t)

	// Create root author and sub-author
	root := newTestRootCmd()
	if _, _, err := executeCommand(root, "author", "create", "main-user", "--email", "main@test.dev"); err != nil {
		t.Fatalf("create main-user: %v", err)
	}
	root = newTestRootCmd()
	if _, _, err := executeCommand(root, "author", "create", "bot-agent", "--parent", "main-user", "--kind", "llm", "--permissions", "read,comment,vote,cl_create,cl_update"); err != nil {
		t.Fatalf("create bot-agent: %v", err)
	}

	// Stage a file
	filePath := filepath.Join(dir, "bot-work.txt")
	if err := os.WriteFile(filePath, []byte("bot content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	root = newTestRootCmd()
	if _, _, err := executeCommand(root, "add", "bot-work.txt"); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Create a CL as bot-agent
	root = newTestRootCmd()
	stdout, _, err := executeCommand(root, "--account", "bot-agent", "cl", "new", "--title", "Bot CL")
	if err != nil {
		t.Fatalf("cl new as bot-agent: %v\nstdout: %s", err, stdout)
	}

	tests := []struct {
		name     string
		args     []string
		contains []string
		wantErr  bool
	}{
		{
			name:     "cl new as bot-agent shows acting-as",
			args:     nil, // already executed above
			contains: []string{"acting as bot-agent"},
			wantErr:  false,
		},
		{
			name:    "account flag with nonexistent author fails",
			args:    []string{"--account", "ghost", "cl", "comment", "fake-cl", "-m", "test"},
			wantErr: true,
		},
	}

	// Check the first test case against the already-captured stdout
	t.Run(tests[0].name, func(t *testing.T) {
		assertContains(t, stdout, tests[0].contains)
	})

	// Run the remaining test cases
	for _, tt := range tests[1:] {
		t.Run(tt.name, func(t *testing.T) {
			root := newTestRootCmd()
			stdout, _, err := executeCommand(root, tt.args...)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v\nstdout: %s", err, stdout)
			}
			if tt.contains != nil {
				assertContains(t, stdout, tt.contains)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// CL error cases
// ═══════════════════════════════════════════════════════════════════════════

func TestCLErrorCasesIntegration(t *testing.T) {
	_ = setupIntegrationRepo(t)

	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "show nonexistent CL",
			args:    []string{"cl", "show", "nonexistent-cl-id"},
			wantErr: true,
		},
		{
			name:    "abandon nonexistent CL",
			args:    []string{"cl", "abandon", "nonexistent-cl-id"},
			wantErr: true,
		},
		{
			name:    "vote on nonexistent CL",
			args:    []string{"cl", "vote", "nonexistent-cl-id", "+1"},
			wantErr: true,
		},
		{
			name:    "vote with invalid score",
			args:    []string{"cl", "vote", "some-id", "abc"},
			wantErr: true,
		},
		{
			name:    "comment on nonexistent CL",
			args:    []string{"cl", "comment", "nonexistent-cl-id", "-m", "hello"},
			wantErr: true,
		},
		{
			name:    "stack on nonexistent CL",
			args:    []string{"cl", "stack", "nonexistent-cl-id"},
			wantErr: true,
		},
		{
			name:    "cl list with no CLs",
			args:    []string{"cl", "list"},
			wantErr: false, // prints "No CLs found"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newTestRootCmd()
			_, _, err := executeCommand(root, tt.args...)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// extractCLID extracts the CL ID from "Created CL <id>" output.
func extractCLID(t *testing.T, stdout string) string {
	t.Helper()
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "Created CL ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Created CL "))
		}
	}
	t.Fatalf("could not extract CL ID from output:\n%s", stdout)
	return ""
}
