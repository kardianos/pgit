package repo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/imgajeed76/pgit/v4/internal/util"
)

// T60: Init creates .pgit directory
func TestInitCreatesPgitDirectory(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "creates .pgit/config.toml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			repo, err := Init(dir)
			if err != nil {
				// Init requires a container runtime; skip if unavailable
				if err == util.ErrNoContainerRuntime {
					t.Skip("no container runtime available")
				}
				t.Fatalf("Init(%q) error: %v", dir, err)
			}

			if repo.Root != dir {
				t.Errorf("repo.Root = %q, want %q", repo.Root, dir)
			}

			configPath := filepath.Join(dir, ".pgit", "config.toml")
			if _, err := os.Stat(configPath); os.IsNotExist(err) {
				t.Errorf(".pgit/config.toml does not exist after Init")
			}

			indexPath := filepath.Join(dir, ".pgit", "index")
			if _, err := os.Stat(indexPath); os.IsNotExist(err) {
				t.Errorf(".pgit/index does not exist after Init")
			}
		})
	}
}

// T61: Open / OpenAt
func TestOpenAndOpenAt(t *testing.T) {
	dir := t.TempDir()

	_, err := Init(dir)
	if err != nil {
		if err == util.ErrNoContainerRuntime {
			t.Skip("no container runtime available")
		}
		t.Fatalf("Init: %v", err)
	}

	tests := []struct {
		name   string
		openFn func() (*Repository, error)
	}{
		{
			name: "Open from within repo dir",
			openFn: func() (*Repository, error) {
				// Save and restore working dir
				orig, _ := os.Getwd()
				defer os.Chdir(orig)
				os.Chdir(dir)
				return Open()
			},
		},
		{
			name: "OpenAt from outside",
			openFn: func() (*Repository, error) {
				return OpenAt(dir)
			},
		},
		{
			name: "OpenAt from subdirectory",
			openFn: func() (*Repository, error) {
				sub := filepath.Join(dir, "subdir")
				os.MkdirAll(sub, 0755)
				return OpenAt(sub)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, err := tt.openFn()
			if err != nil {
				t.Fatalf("open error: %v", err)
			}
			if repo.Root != dir {
				t.Errorf("repo.Root = %q, want %q", repo.Root, dir)
			}
		})
	}
}

// T62: Open outside repo
func TestOpenOutsideRepo(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "dir with no .pgit returns error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			_, err := OpenAt(dir)
			if err == nil {
				t.Fatal("expected error opening non-repo dir, got nil")
			}
		})
	}
}
