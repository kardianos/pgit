package repo

import (
	"context"
	"os"
	"path/filepath"

	"strings"

	"github.com/imgajeed76/pgit/v4/internal/config"
	"github.com/imgajeed76/pgit/v4/internal/container"
	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/imgajeed76/pgit/v4/internal/util"
	"github.com/imgajeed76/pgit/v4/provider"
	"github.com/imgajeed76/pgit/v4/provider/direct"
	"github.com/imgajeed76/pgit/v4/provider/remote"
)

// Repository represents a pgit repository
type Repository struct {
	Root     string             // Repository root directory
	Config   *config.Config     // Repository configuration
	Provider provider.Provider  // Data access provider (direct or remote)
	Runtime  container.Runtime
}

// DB returns the underlying *db.DB if the provider is direct.
// Returns nil for remote providers.
func (r *Repository) DB() *db.DB {
	if dp, ok := r.Provider.(*direct.Provider); ok {
		return dp.DB()
	}
	return nil
}

// Open opens an existing repository
func Open() (*Repository, error) {
	return OpenAt("")
}

// OpenAt opens an existing repository at the given path
func OpenAt(path string) (*Repository, error) {
	var root string
	var err error

	if path == "" {
		root, err = util.FindRepoRoot()
	} else {
		root, err = util.FindRepoRootFrom(path)
	}
	if err != nil {
		return nil, err
	}

	cfg, err := config.Load(root)
	if err != nil {
		return nil, err
	}

	runtime := container.DetectRuntime()

	return &Repository{
		Root:    root,
		Config:  cfg,
		Runtime: runtime,
	}, nil
}

// Init initializes a new repository
func Init(path string) (*Repository, error) {
	// Resolve absolute path
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, err
		}
	}

	// Check if already a repository
	pgitPath := util.PgitPath(path)
	if _, err := os.Stat(pgitPath); err == nil {
		return nil, util.ErrAlreadyInitialized
	}

	// Create .pgit directory
	if err := os.MkdirAll(pgitPath, 0755); err != nil {
		return nil, err
	}

	// Detect runtime
	runtime := container.DetectRuntime()
	if runtime == container.RuntimeNone {
		// Clean up and return error
		os.RemoveAll(pgitPath)
		return nil, util.ErrNoContainerRuntime
	}

	// Create default config
	cfg := config.DefaultConfig(path)

	// Save config
	if err := cfg.Save(path); err != nil {
		os.RemoveAll(pgitPath)
		return nil, err
	}

	// Create empty index
	idx := config.NewIndex()
	if err := idx.Save(path); err != nil {
		os.RemoveAll(pgitPath)
		return nil, err
	}

	return &Repository{
		Root:    path,
		Config:  cfg,
		Runtime: runtime,
	}, nil
}

// Connect connects to the local database
func (r *Repository) Connect(ctx context.Context) error {
	if r.Provider != nil {
		if d := r.DB(); d != nil && d.IsConnected() {
			return nil // Already connected
		}
	}

	// If a server URL is configured, use the remote provider.
	if r.Config.Core.ServerURL != "" {
		r.Provider = remote.New(r.Config.Core.ServerURL, r.Config.Core.ServerToken)
		return nil
	}

	// If a direct database URL is configured, use it instead of
	// container discovery. Used by tests and direct-to-PG deployments.
	// Uses ConnectLite (MinConns=0, MaxConns=1) to avoid background
	// pool goroutines that deadlock on AfterConnect during shutdown.
	if r.Config.Core.DatabaseURL != "" {
		// If the URL starts with http:// or https://, treat it as a remote server.
		if strings.HasPrefix(r.Config.Core.DatabaseURL, "http://") || strings.HasPrefix(r.Config.Core.DatabaseURL, "https://") {
			r.Provider = remote.New(r.Config.Core.DatabaseURL, r.Config.Core.ServerToken)
			return nil
		}
		conn, err := db.ConnectLite(ctx, r.Config.Core.DatabaseURL)
		if err != nil {
			return err
		}
		r.Provider = direct.New(conn)
		return nil
	}

	// Ensure container is running
	if !container.IsContainerRunning(r.Runtime) {
		if err := r.StartContainer(); err != nil {
			return err
		}
	}

	// Ensure database exists
	if err := container.EnsureDatabase(r.Runtime, r.Config.Core.LocalDB); err != nil {
		return err
	}

	// Get port
	port, err := container.GetContainerPort(r.Runtime)
	if err != nil {
		return err
	}

	// Connect
	url := container.LocalConnectionURL(port, r.Config.Core.LocalDB)
	conn, err := db.Connect(ctx, url)
	if err != nil {
		return err
	}

	r.Provider = direct.New(conn)

	// Initialize schema if needed
	exists, err := r.Provider.SchemaExists(ctx)
	if err != nil {
		return err
	}
	if !exists {
		if err := r.Provider.InitSchema(ctx); err != nil {
			return err
		}
	}

	// Ensure repo path is stored in metadata (for pgit repos command)
	// Create metadata table if it doesn't exist (for old databases)
	_ = r.Provider.EnsureMetadataTable(ctx)
	_ = r.Provider.SetRepoPath(ctx, r.Root)

	return nil
}

// ConnectTo connects to a specific database URL (for remotes)
func (r *Repository) ConnectTo(ctx context.Context, url string) (*db.DB, error) {
	return db.Connect(ctx, url)
}

// Close closes the database connection
func (r *Repository) Close() {
	if r.Provider != nil {
		r.Provider.Close()
		r.Provider = nil
	}
}

// StartContainer starts the local container if not running
func (r *Repository) StartContainer() error {
	if container.IsContainerRunning(r.Runtime) {
		return nil
	}

	port := container.DefaultPort
	if !container.IsPortAvailable(port) {
		port = container.FindAvailablePort(port)
	}

	if err := container.StartContainer(r.Runtime, port); err != nil {
		return err
	}

	// Wait for PostgreSQL to be ready
	return container.WaitForPostgres(r.Runtime, 30)
}

// StopContainer stops the local container
func (r *Repository) StopContainer() error {
	return container.StopContainer(r.Runtime)
}

// LoadIndex loads the staging index
func (r *Repository) LoadIndex() (*config.Index, error) {
	return config.LoadIndex(r.Root)
}

// SaveIndex saves the staging index
func (r *Repository) SaveIndex(idx *config.Index) error {
	return idx.Save(r.Root)
}

// LoadIgnorePatterns loads gitignore patterns
func (r *Repository) LoadIgnorePatterns() (*config.IgnorePatterns, error) {
	return config.LoadIgnorePatterns(r.Root)
}

// AbsPath returns the absolute path for a relative path
func (r *Repository) AbsPath(relPath string) string {
	return util.AbsolutePath(r.Root, relPath)
}

// RelPath returns the relative path for an absolute path
func (r *Repository) RelPath(absPath string) (string, error) {
	return util.RelativePath(r.Root, absPath)
}

// SaveConfig saves the repository configuration
func (r *Repository) SaveConfig() error {
	return r.Config.Save(r.Root)
}
