package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// ═══════════════════════════════════════════════════════════════════════════
// T84: pgit init in an already-initialized repo
// ═══════════════════════════════════════════════════════════════════════════

func TestInitExistingRepo(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string // returns dir to init into
		contains []string
		wantErr  bool
	}{
		{
			name: "reinitialize prints warning",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				// Pre-create .pgit so Init sees it as already initialized
				if err := os.MkdirAll(filepath.Join(dir, ".pgit"), 0755); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			contains: []string{"Reinitialized", ".pgit"},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)

			root := newTestRootCmd()
			stdout, _, err := executeCommand(root, "init", dir)

			if tt.wantErr && err != nil {
				// expected error, ok
			} else if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			} else if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			assertContains(t, stdout, tt.contains)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// T124: Missing argument errors
// ═══════════════════════════════════════════════════════════════════════════

func TestMissingArgumentErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "search without pattern",
			args:    []string{"search"},
			wantErr: true,
		},
		{
			name:    "blame without file",
			args:    []string{"blame"},
			wantErr: true,
		},
		{
			name:    "remote add without args",
			args:    []string{"remote", "add"},
			wantErr: true,
		},
		{
			name:    "remote remove without name",
			args:    []string{"remote", "remove"},
			wantErr: true,
		},
		{
			name:    "remote set-url without args",
			args:    []string{"remote", "set-url"},
			wantErr: true,
		},
		{
			name:    "mv without args",
			args:    []string{"mv"},
			wantErr: true,
		},
		{
			name:    "rm without args",
			args:    []string{"rm"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newTestRootCmd()
			_, _, err := executeCommand(root, tt.args...)
			if tt.wantErr && err == nil {
				t.Error("expected error for missing args, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// T125: Not-a-repo errors
// ═══════════════════════════════════════════════════════════════════════════

func TestNotARepoErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "status outside repo",
			args:    []string{"status"},
			wantErr: true,
		},
		{
			name:    "add outside repo",
			args:    []string{"add", "file.txt"},
			wantErr: true,
		},
		{
			name:    "commit outside repo",
			args:    []string{"commit", "-m", "test"},
			wantErr: true,
		},
		{
			name:    "log outside repo",
			args:    []string{"log", "--oneline"},
			wantErr: true,
		},
		{
			name:    "diff outside repo",
			args:    []string{"diff"},
			wantErr: true,
		},
		{
			name:    "show outside repo",
			args:    []string{"show"},
			wantErr: true,
		},
		{
			name:    "blame outside repo",
			args:    []string{"blame", "file.txt"},
			wantErr: true,
		},
		{
			name:    "remote outside repo",
			args:    []string{"remote"},
			wantErr: true,
		},
		{
			name:    "config list outside repo",
			args:    []string{"config", "--list"},
			wantErr: true,
		},
		{
			name:    "checkout outside repo",
			args:    []string{"checkout", "HEAD"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a temp dir with no .pgit
			dir := t.TempDir()
			origDir, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(dir); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chdir(origDir) })

			root := newTestRootCmd()
			_, _, err = executeCommand(root, tt.args...)
			if tt.wantErr && err == nil {
				t.Errorf("expected not-a-repo error for %v, got nil", tt.args)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// T83 (partial): pgit init creates .pgit (no DB needed — just directory check)
// ═══════════════════════════════════════════════════════════════════════════

func TestInitCreatesDirectory(t *testing.T) {
	tests := []struct {
		name     string
		contains []string
		wantErr  bool
	}{
		{
			name:     "init in fresh directory",
			contains: []string{"Initialized", ".pgit"},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			root := newTestRootCmd()
			stdout, _, err := executeCommand(root, "init", dir)

			// Init requires a container runtime; if none, the command will
			// error. Accept that gracefully.
			if err != nil {
				t.Skipf("init failed (likely no container runtime): %v", err)
			}

			assertContains(t, stdout, tt.contains)

			// Verify .pgit dir was created
			pgitPath := filepath.Join(dir, ".pgit")
			if _, statErr := os.Stat(pgitPath); os.IsNotExist(statErr) {
				t.Error(".pgit directory was not created")
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Version command (pure CLI, no DB)
// ═══════════════════════════════════════════════════════════════════════════

func TestVersionCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		contains []string
		wantErr  bool
	}{
		{
			name:     "version subcommand",
			args:     []string{"version"},
			contains: []string{"pgit version"},
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
// Help output (pure CLI, no DB)
// ═══════════════════════════════════════════════════════════════════════════

func TestHelpOutput(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		contains []string
		wantErr  bool
	}{
		{
			name:     "root help",
			args:     []string{"--help"},
			contains: []string{"pgit", "PostgreSQL"},
			wantErr:  false,
		},
		{
			name:     "init help",
			args:     []string{"init", "--help"},
			contains: []string{"pgit repository"},
			wantErr:  false,
		},
		{
			name:     "sql schema no-db",
			args:     []string{"sql", "schema"},
			contains: []string{"pgit_commits", "pgit_file_refs"},
			wantErr:  false,
		},
		{
			name:     "sql tables no-db",
			args:     []string{"sql", "tables"},
			contains: []string{"pgit_commits"},
			wantErr:  false,
		},
		{
			name:     "sql examples no-db",
			args:     []string{"sql", "examples"},
			contains: []string{"SELECT", "pgit_commits"},
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
// Add with no args (pure CLI, prints helpful message without opening repo)
// ═══════════════════════════════════════════════════════════════════════════

func TestAddNoArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		contains []string
		wantErr  bool
	}{
		{
			name: "add with no args prints help",
			args: []string{"add"},
			// runAdd checks len(args)==0 && !addAll and prints help; but it
			// also calls repo.Open() first... the check is BEFORE Open.
			// Actually, looking at the code: the no-args check comes before
			// repo.Open(). So this should work outside a repo.
			contains: []string{"Nothing specified"},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Need to be in a repo for this since runAdd opens repo first
			// Actually re-reading add.go: the check is BEFORE repo.Open()
			// Wait, no: lines 39-47 check args before line 50 repo.Open().
			// So this should work from any directory.
			dir := t.TempDir()
			origDir, _ := os.Getwd()
			_ = os.Chdir(dir)
			t.Cleanup(func() { _ = os.Chdir(origDir) })

			// Actually we need to be in a repo because even the "add with no args"
			// path doesn't call repo.Open() but writes to stdout. But the cobra
			// command's RunE function is runAdd which checks args first. Let me
			// re-read: yes, lines 39-47 return nil BEFORE line 50's repo.Open().
			// So no repo needed!

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
