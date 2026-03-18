package config

import (
	"os"
	"path/filepath"
	"testing"
)

// helper: create a temp dir with .pgit subdirectory so Save/Load paths work
func setupTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".pgit"), 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// T13: Config Load/Save round-trip
func TestConfigLoadSaveRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() *Config
		verify func(t *testing.T, got *Config)
	}{
		{
			name: "default config with user info",
			setup: func() *Config {
				c := DefaultConfig("/some/path")
				c.User.Name = "Alice"
				c.User.Email = "alice@example.com"
				return c
			},
			verify: func(t *testing.T, got *Config) {
				if got.User.Name != "Alice" {
					t.Errorf("User.Name = %q, want %q", got.User.Name, "Alice")
				}
				if got.User.Email != "alice@example.com" {
					t.Errorf("User.Email = %q, want %q", got.User.Email, "alice@example.com")
				}
			},
		},
		{
			name: "config with remotes",
			setup: func() *Config {
				c := DefaultConfig("/repo")
				c.SetRemote("origin", "postgres://localhost/mydb")
				c.SetRemote("upstream", "postgres://remote/updb")
				return c
			},
			verify: func(t *testing.T, got *Config) {
				origin, ok := got.GetRemote("origin")
				if !ok {
					t.Fatal("origin remote missing")
				}
				if origin.URL != "postgres://localhost/mydb" {
					t.Errorf("origin URL = %q, want %q", origin.URL, "postgres://localhost/mydb")
				}
				upstream, ok := got.GetRemote("upstream")
				if !ok {
					t.Fatal("upstream remote missing")
				}
				if upstream.URL != "postgres://remote/updb" {
					t.Errorf("upstream URL = %q, want %q", upstream.URL, "postgres://remote/updb")
				}
			},
		},
		{
			name: "config with core settings round-trips LocalDB",
			setup: func() *Config {
				c := DefaultConfig("/myproject")
				c.User.Name = "Bob"
				return c
			},
			verify: func(t *testing.T, got *Config) {
				// LocalDB is derived from the repo path via HashPath; verify exact value survives round-trip
				want := DefaultConfig("/myproject")
				if got.Core.LocalDB != want.Core.LocalDB {
					t.Errorf("Core.LocalDB = %q, want %q", got.Core.LocalDB, want.Core.LocalDB)
				}
				if got.User.Name != "Bob" {
					t.Errorf("User.Name = %q, want %q", got.User.Name, "Bob")
				}
			},
		},
		{
			name: "config with no remotes round-trips empty Remotes",
			setup: func() *Config {
				c := DefaultConfig("/emptyremotes")
				c.User.Name = "NoRemote"
				return c
			},
			verify: func(t *testing.T, got *Config) {
				if len(got.Remotes) != 0 {
					t.Errorf("Remotes = %v, want empty", got.Remotes)
				}
				if got.User.Name != "NoRemote" {
					t.Errorf("User.Name = %q, want %q", got.User.Name, "NoRemote")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupTempRepo(t)
			original := tt.setup()

			if err := original.Save(dir); err != nil {
				t.Fatalf("Save: %v", err)
			}

			loaded, err := Load(dir)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			tt.verify(t, loaded)
		})
	}
}

// T14: Config GetValue/SetValue
func TestConfigGetSetValue(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantErr bool
	}{
		{name: "user.name", key: "user.name", value: "Carol"},
		{name: "user.email", key: "user.email", value: "carol@example.com"},
		{name: "invalid key single part", key: "bogus", value: "x", wantErr: true},
		{name: "invalid key unknown category", key: "foo.bar", value: "x", wantErr: true},
		{name: "readonly key core.local_db", key: "core.local_db", value: "x", wantErr: true},
	}

	// Verify GetValue returns ok=false for unknown keys
	t.Run("GetValue unknown key returns false", func(t *testing.T) {
		c := DefaultConfig("/test")
		_, ok := c.GetValue("bogus.key")
		if ok {
			t.Error("GetValue(\"bogus.key\") returned ok=true, want false")
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := DefaultConfig("/test")

			err := c.SetValue(tt.key, tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for key %q, got nil", tt.key)
				}
				return
			}
			if err != nil {
				t.Fatalf("SetValue(%q, %q): %v", tt.key, tt.value, err)
			}

			got, ok := c.GetValue(tt.key)
			if !ok {
				t.Fatalf("GetValue(%q) returned ok=false", tt.key)
			}
			if got != tt.value {
				t.Errorf("GetValue(%q) = %q, want %q", tt.key, got, tt.value)
			}
		})
	}
}

// T15: Config remote management
func TestConfigRemoteManagement(t *testing.T) {
	type op struct {
		action string // "add", "get", "remove"
		name   string
		url    string // for add
	}

	tests := []struct {
		name       string
		ops        []op
		wantRemote map[string]string // expected remotes after all ops
	}{
		{
			name: "add single remote",
			ops: []op{
				{action: "add", name: "origin", url: "postgres://localhost/db1"},
			},
			wantRemote: map[string]string{"origin": "postgres://localhost/db1"},
		},
		{
			name: "add then remove",
			ops: []op{
				{action: "add", name: "origin", url: "postgres://localhost/db1"},
				{action: "remove", name: "origin"},
			},
			wantRemote: map[string]string{},
		},
		{
			name: "add multiple remotes",
			ops: []op{
				{action: "add", name: "origin", url: "postgres://host1/db"},
				{action: "add", name: "backup", url: "postgres://host2/db"},
			},
			wantRemote: map[string]string{
				"origin": "postgres://host1/db",
				"backup": "postgres://host2/db",
			},
		},
		{
			name: "overwrite existing remote",
			ops: []op{
				{action: "add", name: "origin", url: "postgres://old/db"},
				{action: "add", name: "origin", url: "postgres://new/db"},
			},
			wantRemote: map[string]string{"origin": "postgres://new/db"},
		},
		{
			name: "remove nonexistent returns false",
			ops: []op{
				{action: "remove", name: "nonexistent"},
			},
			wantRemote: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := DefaultConfig("/test")

			for _, o := range tt.ops {
				switch o.action {
				case "add":
					c.SetRemote(o.name, o.url)
				case "get":
					c.GetRemote(o.name)
				case "remove":
					got := c.RemoveRemote(o.name)
					// If the remote was previously added, removal should return true.
					// If it was never added, removal should return false.
					_, existed := tt.wantRemote[o.name]
					// wantRemote reflects state AFTER all ops, so check if we
					// added this name in a prior op.
					wasAdded := false
					for _, prev := range tt.ops {
						if prev.action == "add" && prev.name == o.name {
							wasAdded = true
							break
						}
					}
					if wasAdded && !got {
						t.Errorf("RemoveRemote(%q) = false, want true (was added)", o.name)
					}
					if !wasAdded && !existed && got {
						t.Errorf("RemoveRemote(%q) = true, want false (never added)", o.name)
					}
				}
			}

			// Verify expected remotes
			for name, wantURL := range tt.wantRemote {
				got, ok := c.GetRemote(name)
				if !ok {
					t.Errorf("remote %q not found", name)
					continue
				}
				if got.URL != wantURL {
					t.Errorf("remote %q URL = %q, want %q", name, got.URL, wantURL)
				}
			}

			// Check no extra remotes exist
			if len(c.Remotes) != len(tt.wantRemote) {
				t.Errorf("got %d remotes, want %d", len(c.Remotes), len(tt.wantRemote))
			}
		})
	}
}

// T16: GlobalConfig Load/Save round-trip
func TestGlobalConfigLoadSaveRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() *GlobalConfig
		verify func(t *testing.T, got *GlobalConfig)
	}{
		{
			name: "defaults survive round-trip",
			setup: func() *GlobalConfig {
				return DefaultGlobalConfig()
			},
			verify: func(t *testing.T, got *GlobalConfig) {
				if got.Container.Port != 5433 {
					t.Errorf("Port = %d, want 5433", got.Container.Port)
				}
				if got.Container.ShmSize != "256m" {
					t.Errorf("ShmSize = %q, want %q", got.Container.ShmSize, "256m")
				}
				if got.Container.SharedBuffers != "256MB" {
					t.Errorf("SharedBuffers = %q, want %q", got.Container.SharedBuffers, "256MB")
				}
			},
		},
		{
			name: "custom values survive round-trip",
			setup: func() *GlobalConfig {
				c := DefaultGlobalConfig()
				c.Container.Port = 9999
				c.Container.ShmSize = "1g"
				c.User.Name = "Test User"
				c.User.Email = "test@example.com"
				c.Import.Workers = 8
				return c
			},
			verify: func(t *testing.T, got *GlobalConfig) {
				if got.Container.Port != 9999 {
					t.Errorf("Port = %d, want 9999", got.Container.Port)
				}
				if got.Container.ShmSize != "1g" {
					t.Errorf("ShmSize = %q, want %q", got.Container.ShmSize, "1g")
				}
				if got.User.Name != "Test User" {
					t.Errorf("User.Name = %q, want %q", got.User.Name, "Test User")
				}
				if got.User.Email != "test@example.com" {
					t.Errorf("User.Email = %q, want %q", got.User.Email, "test@example.com")
				}
				if got.Import.Workers != 8 {
					t.Errorf("Import.Workers = %d, want 8", got.Import.Workers)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a temp dir as XDG_CONFIG_HOME so Save/Load go to temp
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", dir)

			// Reset cache to avoid cross-test contamination
			original := tt.setup()

			if err := original.Save(); err != nil {
				t.Fatalf("Save: %v", err)
			}

			loaded, err := LoadGlobal()
			if err != nil {
				t.Fatalf("LoadGlobal: %v", err)
			}

			tt.verify(t, loaded)
		})
	}
}

// T17: GlobalConfig SetValue type coercion
func TestGlobalConfigSetValueTypeCoercion(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		strValue  string
		wantErr   bool
		verifyInt func(c *GlobalConfig) int
		verifyStr func(c *GlobalConfig) string
		wantInt   int
		wantStr   string
	}{
		{
			name:      "container.port int coercion",
			key:       "container.port",
			strValue:  "5555",
			verifyInt: func(c *GlobalConfig) int { return c.Container.Port },
			wantInt:   5555,
		},
		{
			name:      "container.max_connections int coercion",
			key:       "container.max_connections",
			strValue:  "200",
			verifyInt: func(c *GlobalConfig) int { return c.Container.MaxConnections },
			wantInt:   200,
		},
		{
			name:      "import.workers int coercion",
			key:       "import.workers",
			strValue:  "12",
			verifyInt: func(c *GlobalConfig) int { return c.Import.Workers },
			wantInt:   12,
		},
		{
			name:      "container.shm_size string passthrough",
			key:       "container.shm_size",
			strValue:  "2g",
			verifyStr: func(c *GlobalConfig) string { return c.Container.ShmSize },
			wantStr:   "2g",
		},
		{
			name:      "user.name string passthrough",
			key:       "user.name",
			strValue:  "Jane",
			verifyStr: func(c *GlobalConfig) string { return c.User.Name },
			wantStr:   "Jane",
		},
		{
			name:     "container.port invalid non-integer",
			key:      "container.port",
			strValue: "notanumber",
			wantErr:  true,
		},
		{
			name:     "container.port below min",
			key:      "container.port",
			strValue: "0",
			wantErr:  true,
		},
		{
			name:     "container.port above max",
			key:      "container.port",
			strValue: "99999",
			wantErr:  true,
		},
		{
			name:     "unknown key",
			key:      "nonexistent.key",
			strValue: "val",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := DefaultGlobalConfig()

			err := c.SetValue(tt.key, tt.strValue)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for key %q value %q, got nil", tt.key, tt.strValue)
				}
				return
			}
			if err != nil {
				t.Fatalf("SetValue(%q, %q): %v", tt.key, tt.strValue, err)
			}

			if tt.verifyInt != nil {
				got := tt.verifyInt(c)
				if got != tt.wantInt {
					t.Errorf("after SetValue(%q, %q): field = %d, want %d", tt.key, tt.strValue, got, tt.wantInt)
				}
			}
			if tt.verifyStr != nil {
				got := tt.verifyStr(c)
				if got != tt.wantStr {
					t.Errorf("after SetValue(%q, %q): field = %q, want %q", tt.key, tt.strValue, got, tt.wantStr)
				}
			}
		})
	}
}
