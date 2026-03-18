package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/imgajeed76/pgit/v4/internal/config"
	"github.com/imgajeed76/pgit/v4/internal/testdb"
)

// ═══════════════════════════════════════════════════════════════════════════
// Shared integration test setup
// ═══════════════════════════════════════════════════════════════════════════

// setupIntegrationRepo creates a temp dir with .pgit, connects to a real
// test DB, and populates it with the pgit schema. Returns the repo root.
// The working directory is changed to the repo root.
//
// The repo's config is updated so that repo.Open() + Connect() will find
// the test container automatically (since testdb.Acquire starts the shared
// container via container.StartContainer).
func setupIntegrationRepo(t *testing.T) string {
	t.Helper()

	d := testdb.Acquire(t) // starts container, creates schema, registers cleanup

	dir := t.TempDir()
	pgitDir := filepath.Join(dir, ".pgit")
	if err := os.MkdirAll(pgitDir, 0755); err != nil {
		t.Fatalf("mkdir .pgit: %v", err)
	}

	cfg := config.DefaultConfig(dir)
	cfg.User.Name = "Test User"
	cfg.User.Email = "test@pgit.dev"
	cfg.Core.DatabaseURL = d.URL() // direct connection, bypasses container discovery
	if err := cfg.Save(dir); err != nil {
		t.Fatalf("save config: %v", err)
	}
	idx := config.NewIndex()
	if err := idx.Save(dir); err != nil {
		t.Fatalf("save index: %v", err)
	}

	// Prevent CLI from opening an interactive editor (e.g. commit without -m)
	t.Setenv("EDITOR", "true")
	t.Setenv("VISUAL", "true")

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	return dir
}

// ═══════════════════════════════════════════════════════════════════════════
// T83: pgit init → creates .pgit (full integration)
// ═══════════════════════════════════════════════════════════════════════════

func TestInitCreatesRepoIntegration(t *testing.T) {
	_ = testdb.Acquire(t) // ensure container is running

	tests := []struct {
		name     string
		contains []string
		wantErr  bool
	}{
		{
			name:     "init creates .pgit with container runtime",
			contains: []string{"Initialized", ".pgit"},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			root := newTestRootCmd()
			stdout, _, err := executeCommand(root, "init", dir)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			assertContains(t, stdout, tt.contains)

			// Verify .pgit/config.toml
			cfgPath := filepath.Join(dir, ".pgit", "config.toml")
			if _, statErr := os.Stat(cfgPath); os.IsNotExist(statErr) {
				t.Error(".pgit/config.toml not created")
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// T85-T87: pgit status
// ═══════════════════════════════════════════════════════════════════════════

func TestStatusIntegration(t *testing.T) {
	dir := setupIntegrationRepo(t)

	tests := []struct {
		name     string
		args     []string
		setup    func(t *testing.T) // optional additional setup
		contains []string
		wantErr  bool
	}{
		{
			name:     "T85: status on clean repo",
			args:     []string{"status"},
			contains: []string{"On branch", "main"},
			wantErr:  false,
		},
		{
			name: "T86: status with untracked file",
			args: []string{"status"},
			setup: func(t *testing.T) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, "newfile.txt"), []byte("hello\n"), 0644); err != nil {
					t.Fatal(err)
				}
			},
			contains: []string{"Untracked"},
			wantErr:  false,
		},
		{
			name: "T87a: status --short",
			args: []string{"status", "--short"},
			setup: func(t *testing.T) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, "shortfile.txt"), []byte("data\n"), 0644); err != nil {
					t.Fatal(err)
				}
			},
			// Short format outputs single-char codes
			contains: []string{},
			wantErr:  false,
		},
		{
			name:     "T87b: status --json",
			args:     []string{"status", "--json"},
			contains: []string{"branch", "staged", "unstaged"},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}
			root := newTestRootCmd()
			stdout, _, err := executeCommand(root, tt.args...)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			assertContains(t, stdout, tt.contains)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// T88-T92: pgit add, pgit rm, pgit mv
// ═══════════════════════════════════════════════════════════════════════════

func TestAddRmMvIntegration(t *testing.T) {
	dir := setupIntegrationRepo(t)

	tests := []struct {
		name     string
		args     []string
		setup    func(t *testing.T)
		contains []string
		wantErr  bool
	}{
		{
			name: "T88: add a single file",
			args: []string{"add", "hello.txt"},
			setup: func(t *testing.T) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world\n"), 0644); err != nil {
					t.Fatal(err)
				}
			},
			contains: []string{"Added", "1"},
			wantErr:  false,
		},
		{
			name: "T89: add all with dot",
			args: []string{"add", "."},
			setup: func(t *testing.T) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b\n"), 0644); err != nil {
					t.Fatal(err)
				}
			},
			contains: []string{},
			wantErr:  false,
		},
		{
			name: "T90: add -A",
			args: []string{"add", "-A"},
			setup: func(t *testing.T) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, "all.txt"), []byte("all\n"), 0644); err != nil {
					t.Fatal(err)
				}
			},
			contains: []string{},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}
			root := newTestRootCmd()
			stdout, _, err := executeCommand(root, tt.args...)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			assertContains(t, stdout, tt.contains)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// T93-T96: pgit commit, pgit log
// ═══════════════════════════════════════════════════════════════════════════

func TestCommitLogIntegration(t *testing.T) {
	dir := setupIntegrationRepo(t)

	tests := []struct {
		name     string
		args     []string
		setup    func(t *testing.T)
		contains []string
		wantErr  bool
	}{
		{
			name: "T93: commit with message",
			args: []string{"commit", "-m", "initial commit"},
			setup: func(t *testing.T) {
				t.Helper()
				path := filepath.Join(dir, "commit_test.txt")
				if err := os.WriteFile(path, []byte("content\n"), 0644); err != nil {
					t.Fatal(err)
				}
				// Stage the file first
				root := newTestRootCmd()
				_, _, err := executeCommand(root, "add", "commit_test.txt")
				if err != nil {
					t.Fatalf("add: %v", err)
				}
			},
			contains: []string{"initial commit"},
			wantErr:  false,
		},
		{
			name:     "T94: commit with no staged changes fails",
			args:     []string{"commit", "-m", "nothing staged"},
			contains: []string{"nothing to commit"},
			wantErr:  false, // not an error, just a message
		},
		{
			name:     "T95: log --oneline",
			args:     []string{"log", "--oneline"},
			contains: []string{}, // At least should not error if commits exist
			wantErr:  false,
		},
		{
			name:     "T96: log --json",
			args:     []string{"log", "--json"},
			contains: []string{}, // JSON output
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}
			root := newTestRootCmd()
			stdout, _, err := executeCommand(root, tt.args...)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			assertContains(t, stdout, tt.contains)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// T97-T102: pgit show, pgit diff
// ═══════════════════════════════════════════════════════════════════════════

func TestShowDiffIntegration(t *testing.T) {
	t.Skip("TODO: commit via executeCommand hangs due to pgxpool/AfterConnect deadlock")
	dir := setupIntegrationRepo(t)

	// Create initial file, add, and commit so we have a HEAD
	setup := func(t *testing.T) {
		t.Helper()
		path := filepath.Join(dir, "showdiff.txt")
		if err := os.WriteFile(path, []byte("line1\nline2\n"), 0644); err != nil {
			t.Fatal(err)
		}
		root := newTestRootCmd()
		if _, _, err := executeCommand(root, "add", "showdiff.txt"); err != nil {
			t.Fatalf("add: %v", err)
		}
		root = newTestRootCmd()
		if _, _, err := executeCommand(root, "commit", "-m", "show diff setup"); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}
	setup(t)

	tests := []struct {
		name     string
		args     []string
		setup    func(t *testing.T)
		contains []string
		wantErr  bool
	}{
		{
			name:     "T97: show HEAD",
			args:     []string{"show"},
			contains: []string{"commit", "Author", "show diff setup"},
			wantErr:  false,
		},
		{
			name:     "T98: show --no-patch",
			args:     []string{"show", "--no-patch"},
			contains: []string{"commit", "Author"},
			wantErr:  false,
		},
		{
			name:     "T99: show --stat",
			args:     []string{"show", "--stat"},
			contains: []string{"file(s) changed"},
			wantErr:  false,
		},
		{
			name: "T100: diff with modification",
			args: []string{"diff"},
			setup: func(t *testing.T) {
				t.Helper()
				path := filepath.Join(dir, "showdiff.txt")
				if err := os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0644); err != nil {
					t.Fatal(err)
				}
			},
			contains: []string{},
			wantErr:  false,
		},
		{
			name:     "T101: diff --name-only",
			args:     []string{"diff", "--name-only"},
			contains: []string{},
			wantErr:  false,
		},
		{
			name:     "T102: diff --stat",
			args:     []string{"diff", "--stat"},
			contains: []string{},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(t)
			}
			root := newTestRootCmd()
			stdout, _, err := executeCommand(root, tt.args...)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			assertContains(t, stdout, tt.contains)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// T103: pgit blame
// ═══════════════════════════════════════════════════════════════════════════


func TestBlameIntegration(t *testing.T) {
	dir := setupIntegrationRepo(t)

	// Setup: create file and commit
	path := filepath.Join(dir, "blamed.txt")
	if err := os.WriteFile(path, []byte("first line\nsecond line\n"), 0644); err != nil {
		t.Fatal(err)
	}
	root := newTestRootCmd()
	if _, _, err := executeCommand(root, "add", "blamed.txt"); err != nil {
		t.Fatalf("add: %v", err)
	}
	root = newTestRootCmd()
	if _, _, err := executeCommand(root, "commit", "-m", "blame test"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	tests := []struct {
		name     string
		args     []string
		contains []string
		wantErr  bool
	}{
		{
			name:     "T103: blame shows line attribution",
			args:     []string{"blame", "blamed.txt"},
			contains: []string{"first line", "second line"},
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
				t.Errorf("unexpected error: %v", err)
			}
			assertContains(t, stdout, tt.contains)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// T107-T111: pgit search, pgit sql

// ═══════════════════════════════════════════════════════════════════════════

func TestSearchSQLIntegration(t *testing.T) {
	dir := setupIntegrationRepo(t)

	// Setup: create file with searchable content and commit
	path := filepath.Join(dir, "searchable.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc TODO() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	root := newTestRootCmd()
	if _, _, err := executeCommand(root, "add", "searchable.go"); err != nil {
		t.Fatalf("add: %v", err)
	}
	root = newTestRootCmd()
	if _, _, err := executeCommand(root, "commit", "-m", "searchable content"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	tests := []struct {
		name     string
		args     []string
		contains []string
		wantErr  bool
	}{
		{
			name:     "T107: search for pattern",
			args:     []string{"search", "TODO"},
			contains: []string{"TODO"},
			wantErr:  false,
		},
		{
			name:     "T108: search case-insensitive",
			args:     []string{"search", "-i", "todo"},
			contains: []string{"TODO"},
			wantErr:  false,
		},
		{
			name:     "T109: search no matches",
			args:     []string{"search", "NONEXISTENT_PATTERN_12345"},
			contains: []string{"No matches"},
			wantErr:  false,
		},
		{
			name:     "T110: sql schema subcommand",
			args:     []string{"sql", "schema"},
			contains: []string{"pgit_commits"},
			wantErr:  false,
		},
		{
			name:     "T111: sql select query",
			args:     []string{"sql", "--no-pager", "--raw", "SELECT COUNT(*) FROM pgit_commits"},
			contains: []string{},
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
				t.Errorf("unexpected error: %v", err)
			}
			assertContains(t, stdout, tt.contains)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// T112-T113: pgit config
// ═══════════════════════════════════════════════════════════════════════════

func TestConfigIntegration(t *testing.T) {
	_ = setupIntegrationRepo(t)

	tests := []struct {
		name     string
		args     []string
		contains []string
		wantErr  bool
	}{
		{
			name:     "T112a: config set user.name",
			args:     []string{"config", "user.name", "Test User"},
			contains: []string{"Set", "user.name"},
			wantErr:  false,
		},
		{
			name:     "T112b: config get user.name",
			args:     []string{"config", "user.name"},
			contains: []string{"Test User"},
			wantErr:  false,
		},
		{
			name:     "T112c: config set user.email",
			args:     []string{"config", "user.email", "test@example.com"},
			contains: []string{"Set", "user.email"},
			wantErr:  false,
		},
		{
			name:     "T112d: config get user.email",
			args:     []string{"config", "user.email"},
			contains: []string{"test@example.com"},
			wantErr:  false,
		},
		{
			name:     "T113a: config --list",
			args:     []string{"config", "--list"},
			contains: []string{"user.name", "user.email"},
			wantErr:  false,
		},
		{
			name:     "T113b: config unknown key",
			args:     []string{"config", "unknown.key"},
			contains: []string{},
			wantErr:  true,
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
				t.Errorf("unexpected error: %v", err)
			}
			assertContains(t, stdout, tt.contains)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// T121: pgit remote add/remove/set-url
// ═══════════════════════════════════════════════════════════════════════════

func TestRemoteIntegration(t *testing.T) {
	_ = setupIntegrationRepo(t)

	tests := []struct {
		name     string
		args     []string
		contains []string
		wantErr  bool
	}{
		{
			name:     "T121a: remote add origin",
			args:     []string{"remote", "add", "origin", "postgres://user@host/db"},
			contains: []string{"Added", "origin"},
			wantErr:  false,
		},
		{
			name:     "T121b: remote list shows origin",
			args:     []string{"remote"},
			contains: []string{"origin"},
			wantErr:  false,
		},
		{
			name:     "T121c: remote set-url",
			args:     []string{"remote", "set-url", "origin", "postgres://new@host/db"},
			contains: []string{"Updated"},
			wantErr:  false,
		},
		{
			name:     "T121d: remote add duplicate fails",
			args:     []string{"remote", "add", "origin", "postgres://dup@host/db"},
			contains: []string{},
			wantErr:  true,
		},
		{
			name:     "T121e: remote remove origin",
			args:     []string{"remote", "remove", "origin"},
			contains: []string{"Removed"},
			wantErr:  false,
		},
		{
			name:     "T121f: remote remove nonexistent fails",
			args:     []string{"remote", "remove", "nonexistent"},
			contains: []string{},
			wantErr:  true,
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
				t.Errorf("unexpected error: %v", err)
			}
			assertContains(t, stdout, tt.contains)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Group C: Heavy integration (checkout, analyze, push/pull/clone, doctor)
// These require a fully running database and are skipped without Docker.

// ═══════════════════════════════════════════════════════════════════════════

// T104-T106: pgit checkout
func TestCheckoutIntegration(t *testing.T) {
	t.Skip("blocked on pgxpool/AfterConnect shutdown deadlock in db.Connect")
	dir := setupIntegrationRepo(t)

	// Setup: create file, commit, modify, commit again
	path := filepath.Join(dir, "checkout.txt")
	if err := os.WriteFile(path, []byte("v1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	root := newTestRootCmd()
	if _, _, err := executeCommand(root, "add", "checkout.txt"); err != nil {
		t.Fatalf("add: %v", err)
	}
	root = newTestRootCmd()
	if _, _, err := executeCommand(root, "commit", "-m", "v1"); err != nil {
		t.Fatalf("commit v1: %v", err)
	}

	// Get the first commit hash from log
	root = newTestRootCmd()
	logOut, _, err := executeCommand(root, "log", "--oneline")
	if err != nil {
		t.Fatalf("log: %v", err)
	}

	// Make a second commit
	if err := os.WriteFile(path, []byte("v2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	root = newTestRootCmd()
	if _, _, err := executeCommand(root, "add", "checkout.txt"); err != nil {
		t.Fatalf("add: %v", err)
	}
	root = newTestRootCmd()
	if _, _, err := executeCommand(root, "commit", "-m", "v2"); err != nil {
		t.Fatalf("commit v2: %v", err)
	}

	_ = logOut // used to extract commit hash for checkout tests

	tests := []struct {
		name     string
		args     []string
		contains []string
		wantErr  bool
	}{
		{
			name:     "T104: checkout -- file restores from HEAD",
			args:     []string{"checkout", "--", "checkout.txt"},
			contains: []string{"Updated"},
			wantErr:  false,
		},
		{
			name:     "T105: checkout without args shows error",
			args:     []string{"checkout"},
			contains: []string{},
			wantErr:  true,
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
				t.Errorf("unexpected error: %v", err)
			}
			assertContains(t, stdout, tt.contains)
		})

	}
}

// T115-T120: pgit analyze subcommands
func TestAnalyzeIntegration(t *testing.T) {
	dir := setupIntegrationRepo(t)

	// Setup: create files and commit so analyze has data
	for _, name := range []string{"a.go", "b.go", "c.txt"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	root := newTestRootCmd()
	if _, _, err := executeCommand(root, "add", "-A"); err != nil {
		t.Fatalf("add: %v", err)
	}
	root = newTestRootCmd()
	if _, _, err := executeCommand(root, "commit", "-m", "analyze setup"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	tests := []struct {
		name     string
		args     []string
		contains []string
		wantErr  bool
	}{
		{
			name:     "T115: analyze churn",
			args:     []string{"analyze", "churn", "--no-pager", "--raw"},
			contains: []string{},
			wantErr:  false,
		},
		{
			name:     "T116: analyze authors",
			args:     []string{"analyze", "authors", "--no-pager", "--raw"},
			contains: []string{},
			wantErr:  false,
		},
		{
			name:     "T117: analyze hotspots",
			args:     []string{"analyze", "hotspots", "--no-pager", "--raw"},
			contains: []string{},
			wantErr:  false,
		},
		{
			name:     "T118: analyze coupling",
			args:     []string{"analyze", "coupling", "--no-pager", "--raw"},
			contains: []string{},
			wantErr:  false,
		},
		{
			name:     "T119: analyze activity",
			args:     []string{"analyze", "activity", "--no-pager", "--raw"},
			contains: []string{},
			wantErr:  false,
		},
		{
			name:     "T120: analyze bus-factor",
			args:     []string{"analyze", "bus-factor", "--no-pager", "--raw"},
			contains: []string{},
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
				t.Errorf("unexpected error: %v", err)
			}
			assertContains(t, stdout, tt.contains)
		})
	}
}

// T126: Connection error (bad remote)
func TestConnectionError(t *testing.T) {
	_ = setupIntegrationRepo(t)

	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "T126: log with bad remote fails",
			args:    []string{"log", "--oneline", "--remote", "nonexistent"},
			wantErr: true,
		},
		{
			name:    "T126: search with bad remote fails",
			args:    []string{"search", "pattern", "--remote", "nonexistent"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newTestRootCmd()
			_, _, err := executeCommand(root, tt.args...)
			if tt.wantErr && err == nil {
				t.Error("expected error for bad remote, got nil")
			}
		})
	}
}
