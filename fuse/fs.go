// Package fuse implements a read-only FUSE filesystem that exposes a pgit
// commit as a mountable directory tree.  Directory listings are loaded once
// (from GetTreeMetadataAtCommit) and file content is fetched lazily on first
// read via GetFileAtCommit.
package fuse

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	bazfuse "bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/imgajeed76/pgit/v4/internal/db"
)

// FileReader is the minimal interface needed by the FUSE filesystem.
// It is satisfied by provider.Provider and by test mocks.
type FileReader interface {
	GetTreeMetadataAtCommit(ctx context.Context, commitID string) ([]*db.Blob, error)
	GetFileAtCommit(ctx context.Context, path, commitID string) (*db.Blob, error)
}

// FS implements fs.FS for a single pgit commit.
type FS struct {
	provider FileReader
	commitID string

	// tree maps every path in the commit to its metadata blob (no content).
	tree map[string]*db.Blob
	mu   sync.RWMutex

	// dirs caches the set of directory paths (including "") for /.
	dirs map[string]bool

	// children maps dir path -> immediate child names.
	children map[string][]string
}

// NewFS creates a filesystem for the given commit.  It fetches the tree
// metadata eagerly so that directory listings are fast.
func NewFS(p FileReader, commitID string) (*FS, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	blobs, err := p.GetTreeMetadataAtCommit(ctx, commitID)
	if err != nil {
		return nil, err
	}

	f := &FS{
		provider: p,
		commitID: commitID,
		tree:     make(map[string]*db.Blob, len(blobs)),
		dirs:     make(map[string]bool),
		children: make(map[string][]string),
	}

	f.buildTree(blobs)
	return f, nil
}

// NewFSFromBlobs creates a filesystem from a pre-built blob list.
// Used for testing without a real provider connection.
func NewFSFromBlobs(p FileReader, commitID string, blobs []*db.Blob) *FS {
	f := &FS{
		provider: p,
		commitID: commitID,
		tree:     make(map[string]*db.Blob, len(blobs)),
		dirs:     make(map[string]bool),
		children: make(map[string][]string),
	}
	f.buildTree(blobs)
	return f
}

// buildTree populates the dirs and children maps from the flat blob list.
func (f *FS) buildTree(blobs []*db.Blob) {
	// Root directory always exists.
	f.dirs[""] = true

	for _, b := range blobs {
		path := b.Path
		f.tree[path] = b

		// Ensure all ancestor directories exist.
		dir := parentDir(path)
		name := baseName(path)

		// Register this file as a child of its parent.
		f.children[dir] = append(f.children[dir], name)

		// Walk up to create intermediate directories.
		for dir != "" {
			if f.dirs[dir] {
				break // already created
			}
			f.dirs[dir] = true
			parent := parentDir(dir)
			f.children[parent] = append(f.children[parent], baseName(dir))
			dir = parent
		}
	}
}

// Root returns the root directory node.
func (f *FS) Root() (fs.Node, error) {
	return &Dir{fs: f, path: ""}, nil
}

// Dir represents a directory in the FUSE filesystem.
type Dir struct {
	fs   *FS
	path string // "" for root, "src", "src/pkg", etc.
}

var _ fs.Node = (*Dir)(nil)
var _ fs.HandleReadDirAller = (*Dir)(nil)
var _ fs.NodeStringLookuper = (*Dir)(nil)

// Attr sets the directory attributes.
func (d *Dir) Attr(ctx context.Context, a *bazfuse.Attr) error {
	a.Mode = os.ModeDir | 0o555
	return nil
}

// Lookup finds a child entry by name.
func (d *Dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	childPath := name
	if d.path != "" {
		childPath = d.path + "/" + name
	}

	d.fs.mu.RLock()
	defer d.fs.mu.RUnlock()

	// Check if it's a directory.
	if d.fs.dirs[childPath] {
		return &Dir{fs: d.fs, path: childPath}, nil
	}

	// Check if it's a file.
	if blob, ok := d.fs.tree[childPath]; ok {
		return &File{fs: d.fs, path: childPath, blob: blob}, nil
	}

	return nil, bazfuse.ENOENT
}

// ReadDirAll returns all entries in this directory.
func (d *Dir) ReadDirAll(ctx context.Context) ([]bazfuse.Dirent, error) {
	d.fs.mu.RLock()
	defer d.fs.mu.RUnlock()

	names := d.fs.children[d.path]
	entries := make([]bazfuse.Dirent, 0, len(names))

	for _, name := range names {
		childPath := name
		if d.path != "" {
			childPath = d.path + "/" + name
		}

		de := bazfuse.Dirent{Name: name}
		if d.fs.dirs[childPath] {
			de.Type = bazfuse.DT_Dir
		} else {
			de.Type = bazfuse.DT_File
		}
		entries = append(entries, de)
	}

	return entries, nil
}

// File represents a regular file in the FUSE filesystem.
type File struct {
	fs   *FS
	path string
	blob *db.Blob

	// Lazily fetched content (mutex-based, not sync.Once, so a failed
	// first fetch due to context cancellation can be retried).
	mu      sync.Mutex
	content []byte
	loaded  bool
}

var _ fs.Node = (*File)(nil)
var _ fs.HandleReadAller = (*File)(nil)

// Attr sets the file attributes.
func (f *File) Attr(ctx context.Context, a *bazfuse.Attr) error {
	a.Mode = 0o444
	if f.blob.Mode != 0 {
		// Strip write bits — this is a read-only filesystem.
		a.Mode = os.FileMode(f.blob.Mode) & 0o555
	}
	f.mu.Lock()
	if f.loaded {
		a.Size = uint64(len(f.content))
	}
	f.mu.Unlock()
	return nil
}

// ReadAll fetches the file content lazily from the provider.
// Uses a mutex instead of sync.Once so that a failed fetch (e.g. due
// to context cancellation) can be retried on the next read.
func (f *File) ReadAll(ctx context.Context) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.loaded {
		return f.content, nil
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	blob, err := f.fs.provider.GetFileAtCommit(fetchCtx, f.path, f.fs.commitID)
	if err != nil {
		return nil, err // don't cache errors — allow retry
	}
	f.content = blob.Content
	f.loaded = true
	return f.content, nil
}

// LookupDir finds a directory node for the given path.
// Exported for testing.
func (f *FS) LookupDir(path string) *Dir {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.dirs[path] {
		return &Dir{fs: f, path: path}
	}
	return nil
}

// LookupFile finds a file node for the given path.
// Exported for testing.
func (f *FS) LookupFile(path string) *File {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if blob, ok := f.tree[path]; ok {
		return &File{fs: f, path: path, blob: blob}
	}
	return nil
}

// GetChildren returns the immediate children of a directory.
// Exported for testing.
func (f *FS) GetChildren(dirPath string) []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.children[dirPath]
}

// IsDir reports whether the path is a known directory.
// Exported for testing.
func (f *FS) IsDir(path string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.dirs[path]
}

// --- path helpers ---

func parentDir(path string) string {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return ""
	}
	return path[:i]
}

func baseName(path string) string {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return path
	}
	return path[i+1:]
}
