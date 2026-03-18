package util

import (
	"os"
	"path/filepath"
	"testing"
)

// T3: IsBinaryFile
func TestIsBinaryFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	textFile := filepath.Join(tmpDir, "text.txt")
	os.WriteFile(textFile, []byte("hello world\nline two\n"), 0o644)

	binaryFile := filepath.Join(tmpDir, "binary.bin")
	os.WriteFile(binaryFile, []byte{0x89, 'P', 'N', 'G', 0x00, 0x0D, 0x0A}, 0o644)

	emptyFile := filepath.Join(tmpDir, "empty")
	os.WriteFile(emptyFile, []byte{}, 0o644)

	highBytesFile := filepath.Join(tmpDir, "highbytes")
	os.WriteFile(highBytesFile, []byte{0xFF, 0xFE, 0xFD, 0xFC}, 0o644)

	// Large text file (> 8KB) with NUL after the 8KB window
	largeText := make([]byte, 9000)
	for i := range largeText {
		largeText[i] = 'A'
	}
	largeText[8500] = 0x00 // NUL beyond 8KB read buffer
	largeTextFile := filepath.Join(tmpDir, "largetext.txt")
	os.WriteFile(largeTextFile, largeText, 0o644)

	// File with NUL at position 8191 (last byte of 8KB buffer)
	edgeCase := make([]byte, 8192)
	for i := range edgeCase {
		edgeCase[i] = 'x'
	}
	edgeCase[8191] = 0x00
	edgeFile := filepath.Join(tmpDir, "edge.bin")
	os.WriteFile(edgeFile, edgeCase, 0o644)

	tests := []struct {
		name    string
		path    string
		want    bool
		wantErr bool
	}{
		{name: "text file", path: textFile, want: false},
		{name: "binary with NUL", path: binaryFile, want: true},
		{name: "empty file returns io.EOF", path: emptyFile, want: false, wantErr: true},
		{name: "high bytes no NUL", path: highBytesFile, want: false},
		{name: "NUL beyond 8KB window", path: largeTextFile, want: false},
		{name: "NUL at edge of buffer", path: edgeFile, want: true},
		{name: "nonexistent file", path: filepath.Join(tmpDir, "nope"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsBinaryFile(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsBinaryFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("IsBinaryFile() = %v, want %v", got, tt.want)
			}
		})
	}
}

// T4: RelativePath and AbsolutePath
func TestRelativePath(t *testing.T) {
	tests := []struct {
		name     string
		root     string
		absPath  string
		want     string
		wantErr  bool
	}{
		{name: "simple child", root: "/repo", absPath: "/repo/src/main.go", want: "src/main.go"},
		{name: "root itself", root: "/repo", absPath: "/repo", want: "."},
		{name: "deep nesting", root: "/repo", absPath: "/repo/a/b/c/d.txt", want: "a/b/c/d.txt"},
		{name: "outside repo", root: "/repo", absPath: "/other/file.txt", want: "../other/file.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RelativePath(tt.root, tt.absPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("RelativePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("RelativePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAbsolutePath(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		relPath string
		want    string
	}{
		{name: "simple", root: "/repo", relPath: "src/main.go", want: "/repo/src/main.go"},
		{name: "dot path", root: "/repo", relPath: ".", want: "/repo"},
		{name: "forward slash input", root: "/repo", relPath: "a/b/c.txt", want: "/repo/a/b/c.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AbsolutePath(tt.root, tt.relPath)
			if got != tt.want {
				t.Errorf("AbsolutePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

// T5: FindRepoRootFrom
func TestFindRepoRootFrom(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a .pgit directory in a nested structure
	repoRoot := filepath.Join(tmpDir, "myproject")
	pgitDir := filepath.Join(repoRoot, ".pgit")
	nestedDir := filepath.Join(repoRoot, "src", "pkg", "deep")
	os.MkdirAll(pgitDir, 0o755)
	os.MkdirAll(nestedDir, 0o755)

	tests := []struct {
		name    string
		start   string
		want    string
		wantErr bool
	}{
		{name: "from repo root", start: repoRoot, want: repoRoot},
		{name: "from nested dir", start: nestedDir, want: repoRoot},
		{name: "from src dir", start: filepath.Join(repoRoot, "src"), want: repoRoot},
		{name: "no repo found", start: tmpDir, wantErr: true},
		{name: "filesystem root", start: "/", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FindRepoRootFrom(tt.start)
			if (err != nil) != tt.wantErr {
				t.Errorf("FindRepoRootFrom(%q) error = %v, wantErr %v", tt.start, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("FindRepoRootFrom(%q) = %q, want %q", tt.start, got, tt.want)
			}
		})
	}
}

// T12: FileMode and IsSymlink
func TestFileMode(t *testing.T) {
	tmpDir := t.TempDir()

	regularFile := filepath.Join(tmpDir, "regular.txt")
	os.WriteFile(regularFile, []byte("data"), 0o644)

	execFile := filepath.Join(tmpDir, "exec.sh")
	os.WriteFile(execFile, []byte("#!/bin/sh"), 0o755)

	readOnlyFile := filepath.Join(tmpDir, "readonly.txt")
	os.WriteFile(readOnlyFile, []byte("locked"), 0o444)

	tests := []struct {
		name     string
		path     string
		wantPerm os.FileMode
		wantErr  bool
	}{
		{name: "regular 644", path: regularFile, wantPerm: 0o644},
		{name: "executable 755", path: execFile, wantPerm: 0o755},
		{name: "read-only 444", path: readOnlyFile, wantPerm: 0o444},
		{name: "nonexistent", path: filepath.Join(tmpDir, "gone"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FileMode(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("FileMode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				gotPerm := os.FileMode(got).Perm()
				if gotPerm != tt.wantPerm {
					t.Errorf("FileMode() perm = %o, want %o", gotPerm, tt.wantPerm)
				}
			}
		})
	}
}

func TestIsSymlink(t *testing.T) {
	tmpDir := t.TempDir()

	regularFile := filepath.Join(tmpDir, "regular.txt")
	os.WriteFile(regularFile, []byte("data"), 0o644)

	symlinkFile := filepath.Join(tmpDir, "link.txt")
	os.Symlink(regularFile, symlinkFile)

	dirPath := filepath.Join(tmpDir, "subdir")
	os.MkdirAll(dirPath, 0o755)

	symlinkDir := filepath.Join(tmpDir, "dirlink")
	os.Symlink(dirPath, symlinkDir)

	tests := []struct {
		name    string
		path    string
		want    bool
		wantErr bool
	}{
		{name: "regular file", path: regularFile, want: false},
		{name: "symlink to file", path: symlinkFile, want: true},
		{name: "directory", path: dirPath, want: false},
		{name: "symlink to dir", path: symlinkDir, want: true},
		{name: "nonexistent", path: filepath.Join(tmpDir, "gone"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsSymlink(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsSymlink() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("IsSymlink() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test IsInsideRepo
func TestIsInsideRepo(t *testing.T) {
	tests := []struct {
		name string
		root string
		path string
		want bool
	}{
		{name: "inside", root: "/repo", path: "/repo/src/main.go", want: true},
		{name: "repo root", root: "/repo", path: "/repo/file.txt", want: true},
		{name: "outside", root: "/repo", path: "/other/file.txt", want: false},
		{name: "inside .pgit", root: "/repo", path: "/repo/.pgit/config", want: false},
		{name: ".pgit dir itself", root: "/repo", path: "/repo/.pgit", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsInsideRepo(tt.root, tt.path)
			if got != tt.want {
				t.Errorf("IsInsideRepo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHashPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string // hardcoded expected SHA256[:8] hex
	}{
		{name: "simple path", path: "src/main.go", want: "9e185f29fa355d7d"},
		{name: "root file", path: "README.md", want: "b335630551682c19"},
		{name: "deep path", path: "a/b/c/d/e/f/g.txt", want: "ab68be87aad4424c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HashPath(tt.path)
			if got != tt.want {
				t.Errorf("HashPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
			if len(got) != 16 {
				t.Errorf("HashPath() length = %d, want 16", len(got))
			}
		})
	}
	// Different paths produce different hashes
	if HashPath("a.go") == HashPath("b.go") {
		t.Error("HashPath() collision for different paths")
	}
}

func TestPgitPaths(t *testing.T) {
	root := "/home/user/project"
	if got := PgitPath(root); got != "/home/user/project/.pgit" {
		t.Errorf("PgitPath() = %q", got)
	}
	if got := ConfigPath(root); got != "/home/user/project/.pgit/config.toml" {
		t.Errorf("ConfigPath() = %q", got)
	}
	if got := IndexPath(root); got != "/home/user/project/.pgit/index" {
		t.Errorf("IndexPath() = %q", got)
	}
	if got := HeadPath(root); got != "/home/user/project/.pgit/HEAD" {
		t.Errorf("HeadPath() = %q", got)
	}
}
