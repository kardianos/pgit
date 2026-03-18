// Package testdb provides a shared test database pool with reference counting.
// A single pg-xpatch container is started on the first Acquire() call using
// the locally-built test image (pgit-xpatch-test:latest). Each test gets a
// unique database. The container shuts down 5 seconds after the last Release().
//
// Build the test image first:
//
//	./scripts/build-test-db.sh
package testdb

import (
	"context"
	"crypto/rand"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/imgajeed76/pgit/v4/internal/container"
	"github.com/imgajeed76/pgit/v4/internal/db"
)

const (
	// TestImage is the locally-built pg-xpatch image for testing.
	// Build it with: ./scripts/build-test-db.sh
	TestImage = "pgit-xpatch-test:latest"

	// TestContainerName is a dedicated container name that never
	// collides with the production pgit-local container.
	TestContainerName = "pgit-test-db"

	// TestVolumeName is a dedicated volume for test data.
	TestVolumeName = "pgit-test-data"
)

var (
	mu       sync.Mutex
	refCount int
	port     int
	runtime  container.Runtime
	timer    *time.Timer
)

// Acquire starts the shared test container if needed, creates a fresh database
// with a unique name, initialises the pgit schema, and returns a connected
// *db.DB. A t.Cleanup handler is registered that calls Release automatically,
// so callers never need to remember to clean up.
func Acquire(t testing.TB) *db.DB {
	t.Helper()

	mu.Lock()
	defer mu.Unlock()

	// Cancel any pending shutdown timer.
	if timer != nil {
		timer.Stop()
		timer = nil
	}

	// Start the test container on first use.
	if refCount == 0 {
		runtime = container.DetectRuntime()
		if runtime == container.RuntimeNone {
			t.Skip("no container runtime available (docker/podman); skipping integration test")
		}

		// Check the test image exists locally.
		if !testImageExists(runtime) {
			t.Skip("test image " + TestImage + " not found; run ./scripts/build-test-db.sh first")
		}

		port = container.FindAvailablePort(15433)
		if err := startTestContainer(runtime, port); err != nil {
			t.Fatalf("testdb: start test container: %v", err)
		}
		if err := waitForPostgres(runtime); err != nil {
			t.Fatalf("testdb: wait for postgres: %v", err)
		}
	}
	refCount++

	// Create a unique database for this test.
	dbName := uniqueDBName()
	if err := ensureDatabase(runtime, dbName); err != nil {
		refCount--
		t.Fatalf("testdb: create database %s: %v", dbName, err)
	}

	url := container.LocalConnectionURL(port, dbName)
	ctx := context.Background()
	d, err := db.Connect(ctx, url)
	if err != nil {
		_ = dropDatabase(runtime, dbName)
		refCount--
		t.Fatalf("testdb: connect to %s: %v", dbName, err)
	}

	if err := d.InitSchema(ctx); err != nil {
		d.Close()
		_ = dropDatabase(runtime, dbName)
		refCount--
		t.Fatalf("testdb: init schema in %s: %v", dbName, err)
	}

	// Register automatic cleanup.
	t.Cleanup(func() {
		Release(t, d, dbName)
	})

	return d
}

// Release closes the database connection, drops the test database,
// decrements the reference count, and schedules a container shutdown
// if no more tests are using the pool.
func Release(t testing.TB, d *db.DB, dbName string) {
	t.Helper()

	d.Close()

	mu.Lock()
	defer mu.Unlock()

	if err := dropDatabase(runtime, dbName); err != nil {
		t.Logf("testdb: drop database %s: %v (non-fatal)", dbName, err)
	}

	refCount--
	if refCount <= 0 {
		refCount = 0
		// Schedule container shutdown after 5 seconds of idle.
		timer = time.AfterFunc(5*time.Second, func() {
			mu.Lock()
			defer mu.Unlock()
			if refCount == 0 {
				_ = stopTestContainer(runtime)
			}
		})
	}
}

// FixedULID returns a deterministic ULID-like string for golden file stability.
// The sequence number is zero-padded to 26 characters (standard ULID length).
func FixedULID(seq int) string {
	return fmt.Sprintf("%026d", seq)
}

// ---------------------------------------------------------------------------
// Container management — uses dedicated test container, not pgit-local
// ---------------------------------------------------------------------------

func testImageExists(rt container.Runtime) bool {
	cmd := exec.Command(string(rt), "image", "inspect", TestImage)
	return cmd.Run() == nil
}

func startTestContainer(rt container.Runtime, p int) error {
	// If the test container already exists, check if it's running.
	exists := exec.Command(string(rt), "container", "inspect", TestContainerName)
	if exists.Run() == nil {
		running := exec.Command(string(rt), "inspect", "-f", "{{.State.Running}}", TestContainerName)
		out, _ := running.Output()
		if string(out) == "true\n" {
			// Already running (likely from another test package in the
			// same `go test ./...` run). Read the actual host port from
			// the container rather than using the requested port, which
			// may differ because FindAvailablePort skipped the bound port.
			actualPort, err := getContainerHostPort(rt)
			if err != nil {
				return fmt.Errorf("container running but cannot read port: %w", err)
			}
			port = actualPort
			return nil
		}
		_ = exec.Command(string(rt), "rm", "-f", TestContainerName).Run()
	}

	// Remove stale test volume to start from a clean state.
	_ = exec.Command(string(rt), "volume", "rm", "-f", TestVolumeName).Run()

	args := []string{
		"run", "-d",
		"--name", TestContainerName,
		"-p", fmt.Sprintf("127.0.0.1:%d:5432", p),
		"-v", fmt.Sprintf("%s:/var/lib/postgresql/data", TestVolumeName),
		"--shm-size", "256m",
		"-e", "POSTGRES_PASSWORD=" + container.DefaultPassword,
		TestImage,
		// Minimal PG config for fast test startup
		"-c", "shared_buffers=128MB",
		"-c", "max_connections=50",
		"-c", "fsync=off",
		"-c", "synchronous_commit=off",
		"-c", "full_page_writes=off",
	}

	cmd := exec.Command(string(rt), args...)
	return cmd.Run()
}

// getContainerHostPort reads the published host port for the 5432/tcp binding
// from a running container.
func getContainerHostPort(rt container.Runtime) (int, error) {
	cmd := exec.Command(string(rt), "inspect", "-f",
		"{{(index (index .NetworkSettings.Ports \"5432/tcp\") 0).HostPort}}",
		TestContainerName)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	var p int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &p); err != nil {
		return 0, fmt.Errorf("parse port %q: %w", string(out), err)
	}
	return p, nil
}

func stopTestContainer(rt container.Runtime) error {
	_ = exec.Command(string(rt), "stop", TestContainerName).Run()
	_ = exec.Command(string(rt), "rm", "-f", TestContainerName).Run()
	_ = exec.Command(string(rt), "volume", "rm", "-f", TestVolumeName).Run()
	return nil
}

func waitForPostgres(rt container.Runtime) error {
	for i := range 30 {
		cmd := exec.Command(string(rt), "exec", TestContainerName,
			"pg_isready", "-U", "postgres")
		if cmd.Run() == nil {
			return nil
		}
		if i < 29 {
			time.Sleep(time.Second)
		}
	}
	return fmt.Errorf("postgres did not become ready in 30 seconds")
}

func ensureDatabase(rt container.Runtime, dbName string) error {
	cmd := exec.Command(string(rt), "exec", TestContainerName,
		"psql", "-U", "postgres", "-c",
		fmt.Sprintf("CREATE DATABASE %s", dbName))
	return cmd.Run()
}

func dropDatabase(rt container.Runtime, dbName string) error {
	cmd := exec.Command(string(rt), "exec", TestContainerName,
		"psql", "-U", "postgres", "-c",
		fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", dbName))
	return cmd.Run()
}

// dbSeq is an atomic counter that guarantees unique database names even
// when tests call Acquire in the same nanosecond.
var dbSeq atomic.Uint64

// uniqueDBName generates a collision-free database name by combining a
// monotonic counter with random bytes.
func uniqueDBName() string {
	seq := dbSeq.Add(1)
	var buf [4]byte
	_, _ = rand.Read(buf[:])
	return fmt.Sprintf("test_%d_%x", seq, buf)
}
