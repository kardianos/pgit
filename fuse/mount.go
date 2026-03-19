package fuse

import (
	"fmt"
	"os"

	bazfuse "bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// Mount mounts the given commit as a read-only FUSE filesystem at
// mountpoint.  It blocks until the filesystem is unmounted.
func Mount(mountpoint string, p FileReader, commitID string) error {
	// Create the mountpoint directory if it doesn't exist.
	if err := os.MkdirAll(mountpoint, 0o755); err != nil {
		return fmt.Errorf("creating mountpoint: %w", err)
	}

	filesys, err := NewFS(p, commitID)
	if err != nil {
		return fmt.Errorf("building filesystem: %w", err)
	}

	c, err := bazfuse.Mount(mountpoint, bazfuse.ReadOnly(), bazfuse.FSName("pgit"), bazfuse.Subtype("pgit"))
	if err != nil {
		return fmt.Errorf("mounting FUSE: %w", err)
	}
	defer c.Close()

	if err := fs.Serve(c, filesys); err != nil {
		return fmt.Errorf("serving FUSE: %w", err)
	}

	return nil
}

// MountInBackground mounts the filesystem and starts serving in a
// goroutine.  The returned channel receives nil on clean unmount or an
// error if serving fails.
func MountInBackground(mountpoint string, p FileReader, commitID string) (done <-chan error, err error) {
	if err := os.MkdirAll(mountpoint, 0o755); err != nil {
		return nil, fmt.Errorf("creating mountpoint: %w", err)
	}

	filesys, err := NewFS(p, commitID)
	if err != nil {
		return nil, fmt.Errorf("building filesystem: %w", err)
	}

	c, err := bazfuse.Mount(mountpoint, bazfuse.ReadOnly(), bazfuse.FSName("pgit"), bazfuse.Subtype("pgit"))
	if err != nil {
		return nil, fmt.Errorf("mounting FUSE: %w", err)
	}

	ch := make(chan error, 1)
	go func() {
		ch <- fs.Serve(c, filesys)
		c.Close()
	}()

	return ch, nil
}

// Unmount unmounts the FUSE filesystem at mountpoint.
func Unmount(mountpoint string) error {
	return bazfuse.Unmount(mountpoint)
}
