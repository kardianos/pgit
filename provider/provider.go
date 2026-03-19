// Package provider defines the interface for accessing pgit repository data.
// Commands interact with Provider rather than *db.DB directly, enabling
// both direct PostgreSQL connections (Phase 1) and remote server access (Phase 3).
package provider

import (
	"context"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Provider is the full interface that all pgit commands can use.
// It is composed of domain-specific sub-interfaces.
type Provider interface {
	CommitReader
	CommitWriter
	BlobReader
	BlobWriter
	RefManager
	MetadataManager
	SyncManager
	GraphReader
	GraphWriter
	StatsReader
	SchemaManager
	SearchProvider
	SQLExecutor
	AuthorManager
	CLManager
	Closer
}

// CommitReader provides read access to commits.
type CommitReader interface {
	GetCommit(ctx context.Context, id string) (*db.Commit, error)
	GetHeadCommit(ctx context.Context) (*db.Commit, error)
	GetCommitLog(ctx context.Context, limit int) ([]*db.Commit, error)
	GetCommitLogFrom(ctx context.Context, commitID string, limit int) ([]*db.Commit, error)
	GetAllCommits(ctx context.Context) ([]*db.Commit, error)
	GetCommitsAfter(ctx context.Context, afterID string) ([]*db.Commit, error)
	CountCommits(ctx context.Context) (int, error)
	CommitExists(ctx context.Context, id string) (bool, error)
	GetLatestCommitID(ctx context.Context) (string, error)
	GetAllCommitIDsOrdered(ctx context.Context) ([]string, error)
	FindCommonAncestor(ctx context.Context, commitA, commitB string) (string, error)
	FindCommitByPartialID(ctx context.Context, partialID string) (*db.Commit, error)
	GetCommitsBatch(ctx context.Context, ids []string) (map[string]*db.Commit, error)
	GetCommitsBatchByRange(ctx context.Context, ids []string) (map[string]*db.Commit, error)
}

// CommitWriter provides write access to commits.
type CommitWriter interface {
	CreateCommit(ctx context.Context, c *db.Commit) error
	CreateCommitsBatch(ctx context.Context, commits []*db.Commit) error
	CreateCommitsBatchTx(ctx context.Context, tx pgx.Tx, commits []*db.Commit) error
	DeleteCommits(ctx context.Context, commitIDs []string) error
}

// BlobReader provides read access to blobs and file content.
type BlobReader interface {
	GetBlob(ctx context.Context, path, commitID string) (*db.Blob, error)
	GetBlobsAtCommit(ctx context.Context, commitID string) ([]*db.Blob, error)
	GetTreeAtCommit(ctx context.Context, commitID string) ([]*db.Blob, error)
	GetCurrentTree(ctx context.Context) ([]*db.Blob, error)
	GetTreeMetadataAtCommit(ctx context.Context, commitID string) ([]*db.Blob, error)
	GetCurrentTreeMetadata(ctx context.Context) ([]*db.Blob, error)
	GetFileHistory(ctx context.Context, path string) ([]*db.Blob, error)
	GetFileAtCommit(ctx context.Context, path, commitID string) (*db.Blob, error)
	GetChangedFiles(ctx context.Context, fromCommit, toCommit string) ([]*db.Blob, error)
	GetChangedFilesMetadata(ctx context.Context, fromCommit, toCommit string) ([]*db.Blob, error)
	GetBlobsAtCommitMetadata(ctx context.Context, commitID string) ([]*db.Blob, error)
	GetAllPaths(ctx context.Context) ([]string, error)
	GetImportedPaths(ctx context.Context) (map[string]bool, error)
	BlobExists(ctx context.Context, path, commitID string) (bool, error)
	FileExistsInTree(ctx context.Context, path, commitID string) (bool, error)
	CountBlobs(ctx context.Context) (int64, error)

	// Path operations
	GetPathIDAndGroupIDByPath(ctx context.Context, path string) (int32, int32, error)
	GetGroupIDByPath(ctx context.Context, path string) (int32, error)

	// FileRef operations
	GetFileRefHistory(ctx context.Context, pathID int32) ([]*db.FileRef, error)
	GetFileRefsAtCommit(ctx context.Context, commitID string) ([]*db.FileRef, error)

	// Content operations
	GetAllContentForGroup(ctx context.Context, groupID int32, isBinary bool) ([]db.ContentVersionPair, error)
	GetContent(ctx context.Context, groupID, versionID int32, isBinary bool) ([]byte, error)
}

// BlobWriter provides write access to blobs and file content.
type BlobWriter interface {
	CreateBlob(ctx context.Context, b *db.Blob) error
	CreateBlobs(ctx context.Context, blobs []*db.Blob) error
	CreateBlobsTx(ctx context.Context, tx pgx.Tx, blobs []*db.Blob) error
	DeleteBlobsForCommits(ctx context.Context, commitIDs []string) error
}

// RefManager provides read/write access to refs (HEAD, branches, tags).
type RefManager interface {
	GetRef(ctx context.Context, name string) (*db.Ref, error)
	SetRef(ctx context.Context, name, commitID string) error
	DeleteRef(ctx context.Context, name string) error
	GetAllRefs(ctx context.Context) ([]*db.Ref, error)
	GetHead(ctx context.Context) (string, error)
	SetHead(ctx context.Context, commitID string) error
	RefExists(ctx context.Context, name string) (bool, error)

	// User ref namespaces
	SetUserRef(ctx context.Context, email, name, commitID string) error
	GetUserRef(ctx context.Context, email, name string) (*db.Ref, error)
	GetUserRefs(ctx context.Context, email string) ([]*db.Ref, error)
	DeleteUserRef(ctx context.Context, email, name string) error
}

// MetadataManager provides read/write access to repository metadata.
type MetadataManager interface {
	EnsureMetadataTable(ctx context.Context) error
	GetMetadata(ctx context.Context, key string) (string, error)
	SetMetadata(ctx context.Context, key, value string) error
	DeleteMetadata(ctx context.Context, key string) error
	GetRepoPath(ctx context.Context) string
	SetRepoPath(ctx context.Context, path string) error
}

// SyncManager provides read/write access to sync state.
type SyncManager interface {
	GetSyncState(ctx context.Context, remoteName string) (*db.SyncState, error)
	SetSyncState(ctx context.Context, remoteName string, lastCommitID *string) error
	DeleteSyncState(ctx context.Context, remoteName string) error
	GetAllSyncStates(ctx context.Context) ([]*db.SyncState, error)
}

// GraphReader provides read access to the commit graph (binary lifting table).
type GraphReader interface {
	GetCommitGraphByID(ctx context.Context, id string) (*db.CommitGraphEntry, error)
	GetCommitGraphBySeq(ctx context.Context, seq int32) (*db.CommitGraphEntry, error)
	GetAncestorID(ctx context.Context, commitID string, n int) (string, error)
	CommitExistsInGraph(ctx context.Context, id string) (bool, error)
	FindCommitByPartialIDInGraph(ctx context.Context, partialID string) (string, error)
	CountCommitsFromGraph(ctx context.Context) (int, error)
}

// GraphWriter provides write access to the commit graph.
type GraphWriter interface {
	CreateCommitGraphBatch(ctx context.Context, entries []db.CommitGraphEntry) error
}

// StatsReader provides access to repository statistics.
type StatsReader interface {
	GetRepoStatsFast(ctx context.Context) (*db.RepoStats, error)
	GetXpatchStats(ctx context.Context, tableName string) (*db.XpatchStats, error)
	GetDetailedTableSizes(ctx context.Context) (*db.DetailedTableSizes, error)
	GetCommitStats(ctx context.Context) (map[string]any, error)
	GetBlobStats(ctx context.Context) (map[string]any, error)
}

// SchemaManager provides schema lifecycle operations.
type SchemaManager interface {
	InitSchema(ctx context.Context) error
	SchemaExists(ctx context.Context) (bool, error)
	GetSchemaVersion(ctx context.Context) (int, error)
	SetSchemaVersion(ctx context.Context, version int) error
	DropSchema(ctx context.Context) error
	IsSchemaAtLeast(ctx context.Context, minVersion int) (bool, error)

	// Index management for import
	DropAllIndexes(ctx context.Context) error
	CreateAllIndexes(ctx context.Context) error
	DropBlobPhaseIndexes(ctx context.Context) error
	CreateBlobPhaseIndexes(ctx context.Context) error
	DropCommitsIndexes(ctx context.Context) error
	CreateCommitsIndexes(ctx context.Context) error
	DropCommitGraphIndexes(ctx context.Context) error
	CreateCommitGraphIndexes(ctx context.Context) error
	DropPathsIndexes(ctx context.Context) error
	CreatePathsIndexes(ctx context.Context) error
	DropFileRefsIndexes(ctx context.Context) error
	CreateFileRefsIndexes(ctx context.Context) error
	DropReviewIndexes(ctx context.Context) error
	CreateReviewIndexes(ctx context.Context) error
}

// SearchProvider provides content search capabilities.
type SearchProvider interface {
	SearchContent(ctx context.Context, opts db.SearchContentOptions) ([]*db.SearchContentResult, error)
	SearchContentAtCommit(ctx context.Context, commitID string, opts db.SearchContentOptions) ([]*db.SearchContentResult, error)
}

// SQLExecutor provides raw SQL access for pgit sql and analyze commands.
// Only available on direct providers.
type SQLExecutor interface {
	Exec(ctx context.Context, sql string, args ...any) error
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error
	Pool() *pgxpool.Pool
}

// AuthorManager provides CRUD operations for authors.
type AuthorManager interface {
	CreateAuthor(ctx context.Context, a *db.Author) error
	GetAuthor(ctx context.Context, id string) (*db.Author, error)
	GetAuthorByName(ctx context.Context, name string) (*db.Author, error)
	ListAuthors(ctx context.Context, includeDeleted bool) ([]*db.Author, error)
	GetAuthorChildren(ctx context.Context, parentID string) ([]*db.Author, error)
	GetAuthorChain(ctx context.Context, id string) ([]*db.Author, error)
	ComputeEffectivePermissions(ctx context.Context, id string) (db.Permission, error)
	SoftDeleteAuthor(ctx context.Context, id string) error
	UpdateAuthorPermissions(ctx context.Context, id string, perms db.Permission) error
	CreateAuthorToken(ctx context.Context, authorID string) (string, error)
	ValidateAuthorToken(ctx context.Context, token string) (*db.Author, error)
}

// CLManager provides CRUD operations for change lists, patch sets,
// review comments, and review votes.
type CLManager interface {
	CreateCL(ctx context.Context, c *db.CL) error
	GetCL(ctx context.Context, id string) (*db.CL, error)
	ListCLs(ctx context.Context, status db.CLStatus, limit int) ([]*db.CL, error)
	UpdateCLStatus(ctx context.Context, id string, status db.CLStatus) error
	SubmitCL(ctx context.Context, id string, commitHash string) error
	UpdateCL(ctx context.Context, c *db.CL) error

	CreatePatchSet(ctx context.Context, ps *db.PatchSet) error
	GetPatchSetsForCL(ctx context.Context, clID string) ([]*db.PatchSet, error)
	GetLatestPatchSet(ctx context.Context, clID string) (*db.PatchSet, error)

	CreateReviewComment(ctx context.Context, c *db.ReviewComment) error
	GetCommentsForCL(ctx context.Context, clID string) ([]*db.ReviewComment, error)
	GetCommentsForPatchSet(ctx context.Context, clID string, patchSetNum int) ([]*db.ReviewComment, error)

	SetReviewVote(ctx context.Context, v *db.ReviewVote) error
	GetVotesForCL(ctx context.Context, clID string) ([]*db.ReviewVote, error)

	GetCLStack(ctx context.Context, clID string) ([]*db.CL, error)

	// CI results
	CreateCIResult(ctx context.Context, r *db.CIResult) error
	GetCIResultsForCL(ctx context.Context, clID string) ([]*db.CIResult, error)
	GetCIResultsForPatchSet(ctx context.Context, clID string, patchSet int) ([]*db.CIResult, error)
	UpdateCIResult(ctx context.Context, r *db.CIResult) error
	GetCIResult(ctx context.Context, id string) (*db.CIResult, error)
}

// Closer releases resources held by the provider.
type Closer interface {
	Close()
}

// ImportProvider extends Provider with methods needed only during import.
// These are available via the direct provider's DB() accessor.
type ImportProvider interface {
	SetImportGUCs(ctx context.Context) error
	ResetImportGUCs(ctx context.Context) error
}
