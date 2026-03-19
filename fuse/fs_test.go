package fuse

import (
	"context"
	"testing"

	"github.com/imgajeed76/pgit/v4/internal/db"
)

// mockProvider satisfies the FileReader interface just enough
// for NewFSFromBlobs (which doesn't call the provider at construction)
// and File.ReadAll (which calls GetFileAtCommit).
type mockProvider struct {
	files map[string]*db.Blob // path -> blob with content
}

func (m *mockProvider) GetTreeMetadataAtCommit(_ context.Context, _ string) ([]*db.Blob, error) {
	return nil, nil // unused in tests that use NewFSFromBlobs
}

func (m *mockProvider) GetFileAtCommit(_ context.Context, path, _ string) (*db.Blob, error) {
	if b, ok := m.files[path]; ok {
		return b, nil
	}
	return nil, context.DeadlineExceeded // stand-in for "not found"
}

func newTestBlobs() []*db.Blob {
	return []*db.Blob{
		{Path: "README.md", Mode: 0o644},
		{Path: "src/main.go", Mode: 0o644},
		{Path: "src/pkg/util.go", Mode: 0o644},
		{Path: "src/pkg/util_test.go", Mode: 0o644},
		{Path: "Makefile", Mode: 0o644},
	}
}

func newMockProvider() *mockProvider {
	return &mockProvider{
		files: map[string]*db.Blob{
			"README.md":            {Path: "README.md", Content: []byte("# Hello")},
			"src/main.go":         {Path: "src/main.go", Content: []byte("package main")},
			"src/pkg/util.go":     {Path: "src/pkg/util.go", Content: []byte("package pkg")},
			"src/pkg/util_test.go": {Path: "src/pkg/util_test.go", Content: []byte("package pkg_test")},
			"Makefile":            {Path: "Makefile", Content: []byte("all: build")},
		},
	}
}

func TestBuildTree_RootChildren(t *testing.T) {
	fs := NewFSFromBlobs(nil, "abc123", newTestBlobs())

	children := fs.GetChildren("")
	if len(children) != 3 {
		t.Fatalf("root should have 3 children (README.md, src, Makefile), got %d: %v", len(children), children)
	}

	has := make(map[string]bool)
	for _, c := range children {
		has[c] = true
	}
	for _, want := range []string{"README.md", "src", "Makefile"} {
		if !has[want] {
			t.Errorf("root missing child %q", want)
		}
	}
}

func TestBuildTree_NestedDirs(t *testing.T) {
	fs := NewFSFromBlobs(nil, "abc123", newTestBlobs())

	// src/ should be a directory
	if !fs.IsDir("src") {
		t.Error("src should be a directory")
	}

	// src/pkg/ should be a directory
	if !fs.IsDir("src/pkg") {
		t.Error("src/pkg should be a directory")
	}

	// src/ should have children: main.go, pkg
	srcChildren := fs.GetChildren("src")
	if len(srcChildren) != 2 {
		t.Fatalf("src/ should have 2 children, got %d: %v", len(srcChildren), srcChildren)
	}

	// src/pkg/ should have children: util.go, util_test.go
	pkgChildren := fs.GetChildren("src/pkg")
	if len(pkgChildren) != 2 {
		t.Fatalf("src/pkg/ should have 2 children, got %d: %v", len(pkgChildren), pkgChildren)
	}
}

func TestBuildTree_FilesAreNotDirs(t *testing.T) {
	fs := NewFSFromBlobs(nil, "abc123", newTestBlobs())

	if fs.IsDir("README.md") {
		t.Error("README.md should not be a directory")
	}
	if fs.IsDir("src/main.go") {
		t.Error("src/main.go should not be a directory")
	}
}

func TestLookupDir(t *testing.T) {
	fs := NewFSFromBlobs(nil, "abc123", newTestBlobs())

	root := fs.LookupDir("")
	if root == nil {
		t.Fatal("root dir should exist")
	}

	src := fs.LookupDir("src")
	if src == nil {
		t.Fatal("src dir should exist")
	}

	pkg := fs.LookupDir("src/pkg")
	if pkg == nil {
		t.Fatal("src/pkg dir should exist")
	}

	// Non-existent dir
	if fs.LookupDir("nonexistent") != nil {
		t.Error("nonexistent dir should return nil")
	}
}

func TestLookupFile(t *testing.T) {
	fs := NewFSFromBlobs(nil, "abc123", newTestBlobs())

	f := fs.LookupFile("README.md")
	if f == nil {
		t.Fatal("README.md should exist")
	}
	if f.path != "README.md" {
		t.Errorf("path = %q, want %q", f.path, "README.md")
	}

	f2 := fs.LookupFile("src/pkg/util.go")
	if f2 == nil {
		t.Fatal("src/pkg/util.go should exist")
	}

	// Non-existent file
	if fs.LookupFile("nonexistent") != nil {
		t.Error("nonexistent file should return nil")
	}
}

func TestReadDirAll_Root(t *testing.T) {
	fsys := NewFSFromBlobs(nil, "abc123", newTestBlobs())
	root := &Dir{fs: fsys, path: ""}

	entries, err := root.ReadDirAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 3 {
		t.Fatalf("root should have 3 entries, got %d", len(entries))
	}
}

func TestReadDirAll_SubDir(t *testing.T) {
	fsys := NewFSFromBlobs(nil, "abc123", newTestBlobs())
	srcDir := &Dir{fs: fsys, path: "src/pkg"}

	entries, err := srcDir.ReadDirAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("src/pkg should have 2 entries, got %d", len(entries))
	}

	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	for _, want := range []string{"util.go", "util_test.go"} {
		if !names[want] {
			t.Errorf("src/pkg/ missing entry %q", want)
		}
	}
}

func TestLookup_ViaDir(t *testing.T) {
	fsys := NewFSFromBlobs(nil, "abc123", newTestBlobs())
	root := &Dir{fs: fsys, path: ""}

	// Lookup a file in root
	node, err := root.Lookup(context.Background(), "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := node.(*File); !ok {
		t.Error("README.md should be a File node")
	}

	// Lookup a directory in root
	node, err = root.Lookup(context.Background(), "src")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := node.(*Dir); !ok {
		t.Error("src should be a Dir node")
	}

	// Lookup nonexistent
	_, err = root.Lookup(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected ENOENT for nonexistent entry")
	}
}

func TestReadAll_LazyFetch(t *testing.T) {
	mp := newMockProvider()
	fsys := NewFSFromBlobs(mp, "abc123", newTestBlobs())

	f := fsys.LookupFile("README.md")
	if f == nil {
		t.Fatal("README.md should exist")
	}

	data, err := f.ReadAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# Hello" {
		t.Errorf("content = %q, want %q", string(data), "# Hello")
	}

	// Reading again should return cached content (same result).
	data2, err := f.ReadAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(data2) != "# Hello" {
		t.Errorf("second read content = %q, want %q", string(data2), "# Hello")
	}
}

func TestEmptyTree(t *testing.T) {
	fsys := NewFSFromBlobs(nil, "abc123", nil)

	// Root should still exist.
	if !fsys.IsDir("") {
		t.Error("root should be a directory even with empty tree")
	}

	children := fsys.GetChildren("")
	if len(children) != 0 {
		t.Errorf("root should have no children, got %v", children)
	}
}

func TestSingleFile(t *testing.T) {
	blobs := []*db.Blob{{Path: "hello.txt", Mode: 0o644}}
	fsys := NewFSFromBlobs(nil, "abc123", blobs)

	children := fsys.GetChildren("")
	if len(children) != 1 || children[0] != "hello.txt" {
		t.Errorf("root children = %v, want [hello.txt]", children)
	}

	if fsys.IsDir("hello.txt") {
		t.Error("hello.txt should not be a directory")
	}
}

func TestDeeplyNested(t *testing.T) {
	blobs := []*db.Blob{{Path: "a/b/c/d/e.txt", Mode: 0o644}}
	fsys := NewFSFromBlobs(nil, "abc123", blobs)

	for _, dir := range []string{"a", "a/b", "a/b/c", "a/b/c/d"} {
		if !fsys.IsDir(dir) {
			t.Errorf("%s should be a directory", dir)
		}
	}

	f := fsys.LookupFile("a/b/c/d/e.txt")
	if f == nil {
		t.Error("a/b/c/d/e.txt should exist as a file")
	}
}
