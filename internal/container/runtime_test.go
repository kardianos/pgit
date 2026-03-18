package container

import (
	"fmt"
	"net"
	"testing"
)

// T77: DetectRuntime
func TestDetectRuntime(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "returns Docker or Podman or skips"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := DetectRuntime()
			if rt == RuntimeNone {
				t.Skip("no container runtime available")
			}
			if rt != RuntimeDocker && rt != RuntimePodman {
				t.Errorf("DetectRuntime() = %q, want docker or podman", rt)
			}
		})
	}
}

// T78: Container state inspection (requires running container)
func TestContainerStateInspection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	rt := DetectRuntime()
	if rt == RuntimeNone {
		t.Skip("no container runtime available")
	}
	if !IsContainerRunning(rt) {
		t.Skip("container not running — start with 'pgit local start' to run this test")
	}

	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "GetContainerPort returns positive port",
			fn: func(t *testing.T) {
				port, err := GetContainerPort(rt)
				if err != nil {
					t.Fatalf("GetContainerPort: %v", err)
				}
				if port <= 0 {
					t.Errorf("port = %d, want > 0", port)
				}
			},
		},
		{
			name: "ContainerExists is true when running",
			fn: func(t *testing.T) {
				if !ContainerExists(rt) {
					t.Error("ContainerExists = false, want true (container is running)")
				}
			},
		},
		{
			name: "GetContainerLogs returns non-empty",
			fn: func(t *testing.T) {
				logs, err := GetContainerLogs(rt, 5)
				if err != nil {
					t.Fatalf("GetContainerLogs: %v", err)
				}
				if logs == "" {
					t.Error("GetContainerLogs returned empty")
				}
			},
		},
		{
			name: "WaitForPostgres succeeds on running container",
			fn: func(t *testing.T) {
				if err := WaitForPostgres(rt, 5); err != nil {
					t.Errorf("WaitForPostgres: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.fn)
	}
}

// T79: EnsureDatabase / DropDatabase
func TestEnsureAndDropDatabase(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "ensure and drop test database"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := DetectRuntime()
			if rt == RuntimeNone {
				t.Skip("no container runtime available")
			}
			if !IsContainerRunning(rt) {
				t.Skip("container not running")
			}

			dbName := "pgit_test_ensure_drop"
			if err := EnsureDatabase(rt, dbName); err != nil {
				t.Fatalf("EnsureDatabase: %v", err)
			}

			// Idempotent
			if err := EnsureDatabase(rt, dbName); err != nil {
				t.Fatalf("EnsureDatabase (second call): %v", err)
			}

			if err := DropDatabase(rt, dbName); err != nil {
				t.Fatalf("DropDatabase: %v", err)
			}
		})
	}
}

// T80: LocalConnectionURL — pure function, no container needed
func TestLocalConnectionURL(t *testing.T) {
	tests := []struct {
		name     string
		port     int
		database string
		want     string
	}{
		{
			name:     "default port and db",
			port:     5433,
			database: "myrepo",
			want:     fmt.Sprintf("postgres://postgres:%s@localhost:5433/myrepo?sslmode=disable", DefaultPassword),
		},
		{
			name:     "custom port",
			port:     15433,
			database: "test_db",
			want:     fmt.Sprintf("postgres://postgres:%s@localhost:15433/test_db?sslmode=disable", DefaultPassword),
		},
		{
			name:     "standard postgres port",
			port:     5432,
			database: "pgit",
			want:     fmt.Sprintf("postgres://postgres:%s@localhost:5432/pgit?sslmode=disable", DefaultPassword),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LocalConnectionURL(tt.port, tt.database)
			if got != tt.want {
				t.Errorf("LocalConnectionURL(%d, %q) = %q, want %q", tt.port, tt.database, got, tt.want)
			}
		})
	}
}

// T81: FindAvailablePort
func TestFindAvailablePort(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "returned port is available"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := FindAvailablePort(30000)
			if port < 30000 || port >= 30100 {
				t.Errorf("FindAvailablePort(30000) = %d, want in [30000, 30100)", port)
			}

			// Verify the port is actually usable by trying to listen on it
			ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				t.Errorf("port %d reported available but cannot listen: %v", port, err)
			} else {
				ln.Close()
			}
		})
	}
}

// T82: IsPortAvailable — table-driven with occupied and free ports
func TestIsPortAvailable(t *testing.T) {
	// Occupy a port for testing
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on random port: %v", err)
	}
	defer ln.Close()

	occupiedPort := ln.Addr().(*net.TCPAddr).Port

	tests := []struct {
		name string
		port int
		want bool
	}{
		{
			name: "occupied port is not available",
			port: occupiedPort,
			want: false,
		},
		{
			name: "dynamically found free port is available",
			port: func() int {
				// Find a guaranteed-free port by binding and immediately closing
				l, _ := net.Listen("tcp", "127.0.0.1:0")
				p := l.Addr().(*net.TCPAddr).Port
				l.Close()
				return p
			}(),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPortAvailable(tt.port)
			if got != tt.want {
				t.Errorf("IsPortAvailable(%d) = %v, want %v", tt.port, got, tt.want)
			}
		})
	}
}
