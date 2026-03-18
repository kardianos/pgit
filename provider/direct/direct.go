// Package direct implements provider.Provider by delegating to *db.DB.
// This is a thin wrapper — every method is a one-liner that forwards to the
// underlying database connection.
package direct

import (
	"context"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Provider wraps a *db.DB to satisfy provider.Provider.
type Provider struct {
	db *db.DB
}

// New creates a direct Provider from a *db.DB.
func New(d *db.DB) *Provider {
	return &Provider{db: d}
}

// DB returns the underlying *db.DB for operations that need it
// (import pipeline, raw SQL, etc.)
func (p *Provider) DB() *db.DB {
	return p.db
}

// ---------------------------------------------------------------------------
// CommitReader
// ---------------------------------------------------------------------------

func (p *Provider) GetCommit(ctx context.Context, id string) (*db.Commit, error) {
	return p.db.GetCommit(ctx, id)
}

func (p *Provider) GetHeadCommit(ctx context.Context) (*db.Commit, error) {
	return p.db.GetHeadCommit(ctx)
}

func (p *Provider) GetCommitLog(ctx context.Context, limit int) ([]*db.Commit, error) {
	return p.db.GetCommitLog(ctx, limit)
}

func (p *Provider) GetCommitLogFrom(ctx context.Context, commitID string, limit int) ([]*db.Commit, error) {
	return p.db.GetCommitLogFrom(ctx, commitID, limit)
}

func (p *Provider) GetAllCommits(ctx context.Context) ([]*db.Commit, error) {
	return p.db.GetAllCommits(ctx)
}

func (p *Provider) GetCommitsAfter(ctx context.Context, afterID string) ([]*db.Commit, error) {
	return p.db.GetCommitsAfter(ctx, afterID)
}

func (p *Provider) CountCommits(ctx context.Context) (int, error) {
	return p.db.CountCommits(ctx)
}

func (p *Provider) CommitExists(ctx context.Context, id string) (bool, error) {
	return p.db.CommitExists(ctx, id)
}

func (p *Provider) GetLatestCommitID(ctx context.Context) (string, error) {
	return p.db.GetLatestCommitID(ctx)
}

func (p *Provider) GetAllCommitIDsOrdered(ctx context.Context) ([]string, error) {
	return p.db.GetAllCommitIDsOrdered(ctx)
}

func (p *Provider) FindCommonAncestor(ctx context.Context, commitA, commitB string) (string, error) {
	return p.db.FindCommonAncestor(ctx, commitA, commitB)
}

func (p *Provider) FindCommitByPartialID(ctx context.Context, partialID string) (*db.Commit, error) {
	return p.db.FindCommitByPartialID(ctx, partialID)
}

func (p *Provider) GetCommitsBatch(ctx context.Context, ids []string) (map[string]*db.Commit, error) {
	return p.db.GetCommitsBatch(ctx, ids)
}

func (p *Provider) GetCommitsBatchByRange(ctx context.Context, ids []string) (map[string]*db.Commit, error) {
	return p.db.GetCommitsBatchByRange(ctx, ids)
}

// ---------------------------------------------------------------------------
// CommitWriter
// ---------------------------------------------------------------------------

func (p *Provider) CreateCommit(ctx context.Context, c *db.Commit) error {
	return p.db.CreateCommit(ctx, c)
}

func (p *Provider) CreateCommitsBatch(ctx context.Context, commits []*db.Commit) error {
	return p.db.CreateCommitsBatch(ctx, commits)
}

func (p *Provider) CreateCommitsBatchTx(ctx context.Context, tx pgx.Tx, commits []*db.Commit) error {
	return p.db.CreateCommitsBatchTx(ctx, tx, commits)
}

func (p *Provider) DeleteCommits(ctx context.Context, commitIDs []string) error {
	return p.db.DeleteCommits(ctx, commitIDs)
}

// ---------------------------------------------------------------------------
// BlobReader
// ---------------------------------------------------------------------------

func (p *Provider) GetBlob(ctx context.Context, path, commitID string) (*db.Blob, error) {
	return p.db.GetBlob(ctx, path, commitID)
}

func (p *Provider) GetBlobsAtCommit(ctx context.Context, commitID string) ([]*db.Blob, error) {
	return p.db.GetBlobsAtCommit(ctx, commitID)
}

func (p *Provider) GetTreeAtCommit(ctx context.Context, commitID string) ([]*db.Blob, error) {
	return p.db.GetTreeAtCommit(ctx, commitID)
}

func (p *Provider) GetCurrentTree(ctx context.Context) ([]*db.Blob, error) {
	return p.db.GetCurrentTree(ctx)
}

func (p *Provider) GetTreeMetadataAtCommit(ctx context.Context, commitID string) ([]*db.Blob, error) {
	return p.db.GetTreeMetadataAtCommit(ctx, commitID)
}

func (p *Provider) GetCurrentTreeMetadata(ctx context.Context) ([]*db.Blob, error) {
	return p.db.GetCurrentTreeMetadata(ctx)
}

func (p *Provider) GetFileHistory(ctx context.Context, path string) ([]*db.Blob, error) {
	return p.db.GetFileHistory(ctx, path)
}

func (p *Provider) GetFileAtCommit(ctx context.Context, path, commitID string) (*db.Blob, error) {
	return p.db.GetFileAtCommit(ctx, path, commitID)
}

func (p *Provider) GetChangedFiles(ctx context.Context, fromCommit, toCommit string) ([]*db.Blob, error) {
	return p.db.GetChangedFiles(ctx, fromCommit, toCommit)
}

func (p *Provider) GetChangedFilesMetadata(ctx context.Context, fromCommit, toCommit string) ([]*db.Blob, error) {
	return p.db.GetChangedFilesMetadata(ctx, fromCommit, toCommit)
}

func (p *Provider) GetBlobsAtCommitMetadata(ctx context.Context, commitID string) ([]*db.Blob, error) {
	return p.db.GetBlobsAtCommitMetadata(ctx, commitID)
}

func (p *Provider) GetAllPaths(ctx context.Context) ([]string, error) {
	return p.db.GetAllPaths(ctx)
}

func (p *Provider) GetImportedPaths(ctx context.Context) (map[string]bool, error) {
	return p.db.GetImportedPaths(ctx)
}

func (p *Provider) BlobExists(ctx context.Context, path, commitID string) (bool, error) {
	return p.db.BlobExists(ctx, path, commitID)
}

func (p *Provider) FileExistsInTree(ctx context.Context, path, commitID string) (bool, error) {
	return p.db.FileExistsInTree(ctx, path, commitID)
}

func (p *Provider) CountBlobs(ctx context.Context) (int64, error) {
	return p.db.CountBlobs(ctx)
}

func (p *Provider) GetPathIDAndGroupIDByPath(ctx context.Context, path string) (int32, int32, error) {
	return p.db.GetPathIDAndGroupIDByPath(ctx, path)
}

func (p *Provider) GetGroupIDByPath(ctx context.Context, path string) (int32, error) {
	return p.db.GetGroupIDByPath(ctx, path)
}

func (p *Provider) GetFileRefHistory(ctx context.Context, pathID int32) ([]*db.FileRef, error) {
	return p.db.GetFileRefHistory(ctx, pathID)
}

func (p *Provider) GetFileRefsAtCommit(ctx context.Context, commitID string) ([]*db.FileRef, error) {
	return p.db.GetFileRefsAtCommit(ctx, commitID)
}

func (p *Provider) GetAllContentForGroup(ctx context.Context, groupID int32, isBinary bool) ([]db.ContentVersionPair, error) {
	return p.db.GetAllContentForGroup(ctx, groupID, isBinary)
}

func (p *Provider) GetContent(ctx context.Context, groupID, versionID int32, isBinary bool) ([]byte, error) {
	return p.db.GetContent(ctx, groupID, versionID, isBinary)
}

// ---------------------------------------------------------------------------
// BlobWriter
// ---------------------------------------------------------------------------

func (p *Provider) CreateBlob(ctx context.Context, b *db.Blob) error {
	return p.db.CreateBlob(ctx, b)
}

func (p *Provider) CreateBlobs(ctx context.Context, blobs []*db.Blob) error {
	return p.db.CreateBlobs(ctx, blobs)
}

func (p *Provider) CreateBlobsTx(ctx context.Context, tx pgx.Tx, blobs []*db.Blob) error {
	return p.db.CreateBlobsTx(ctx, tx, blobs)
}

func (p *Provider) DeleteBlobsForCommits(ctx context.Context, commitIDs []string) error {
	return p.db.DeleteBlobsForCommits(ctx, commitIDs)
}

// ---------------------------------------------------------------------------
// RefManager
// ---------------------------------------------------------------------------

func (p *Provider) GetRef(ctx context.Context, name string) (*db.Ref, error) {
	return p.db.GetRef(ctx, name)
}

func (p *Provider) SetRef(ctx context.Context, name, commitID string) error {
	return p.db.SetRef(ctx, name, commitID)
}

func (p *Provider) DeleteRef(ctx context.Context, name string) error {
	return p.db.DeleteRef(ctx, name)
}

func (p *Provider) GetAllRefs(ctx context.Context) ([]*db.Ref, error) {
	return p.db.GetAllRefs(ctx)
}

func (p *Provider) GetHead(ctx context.Context) (string, error) {
	return p.db.GetHead(ctx)
}

func (p *Provider) SetHead(ctx context.Context, commitID string) error {
	return p.db.SetHead(ctx, commitID)
}

func (p *Provider) RefExists(ctx context.Context, name string) (bool, error) {
	return p.db.RefExists(ctx, name)
}

// ---------------------------------------------------------------------------
// MetadataManager
// ---------------------------------------------------------------------------

func (p *Provider) EnsureMetadataTable(ctx context.Context) error {
	return p.db.EnsureMetadataTable(ctx)
}

func (p *Provider) GetMetadata(ctx context.Context, key string) (string, error) {
	return p.db.GetMetadata(ctx, key)
}

func (p *Provider) SetMetadata(ctx context.Context, key, value string) error {
	return p.db.SetMetadata(ctx, key, value)
}

func (p *Provider) DeleteMetadata(ctx context.Context, key string) error {
	return p.db.DeleteMetadata(ctx, key)
}

func (p *Provider) GetRepoPath(ctx context.Context) string {
	return p.db.GetRepoPath(ctx)
}

func (p *Provider) SetRepoPath(ctx context.Context, path string) error {
	return p.db.SetRepoPath(ctx, path)
}

// ---------------------------------------------------------------------------
// SyncManager
// ---------------------------------------------------------------------------

func (p *Provider) GetSyncState(ctx context.Context, remoteName string) (*db.SyncState, error) {
	return p.db.GetSyncState(ctx, remoteName)
}

func (p *Provider) SetSyncState(ctx context.Context, remoteName string, lastCommitID *string) error {
	return p.db.SetSyncState(ctx, remoteName, lastCommitID)
}

func (p *Provider) DeleteSyncState(ctx context.Context, remoteName string) error {
	return p.db.DeleteSyncState(ctx, remoteName)
}

func (p *Provider) GetAllSyncStates(ctx context.Context) ([]*db.SyncState, error) {
	return p.db.GetAllSyncStates(ctx)
}

// ---------------------------------------------------------------------------
// GraphReader
// ---------------------------------------------------------------------------

func (p *Provider) GetCommitGraphByID(ctx context.Context, id string) (*db.CommitGraphEntry, error) {
	return p.db.GetCommitGraphByID(ctx, id)
}

func (p *Provider) GetCommitGraphBySeq(ctx context.Context, seq int32) (*db.CommitGraphEntry, error) {
	return p.db.GetCommitGraphBySeq(ctx, seq)
}

func (p *Provider) GetAncestorID(ctx context.Context, commitID string, n int) (string, error) {
	return p.db.GetAncestorID(ctx, commitID, n)
}

func (p *Provider) CommitExistsInGraph(ctx context.Context, id string) (bool, error) {
	return p.db.CommitExistsInGraph(ctx, id)
}

func (p *Provider) FindCommitByPartialIDInGraph(ctx context.Context, partialID string) (string, error) {
	return p.db.FindCommitByPartialIDInGraph(ctx, partialID)
}

func (p *Provider) CountCommitsFromGraph(ctx context.Context) (int, error) {
	return p.db.CountCommitsFromGraph(ctx)
}

// ---------------------------------------------------------------------------
// GraphWriter
// ---------------------------------------------------------------------------

func (p *Provider) CreateCommitGraphBatch(ctx context.Context, entries []db.CommitGraphEntry) error {
	return p.db.CreateCommitGraphBatch(ctx, entries)
}

// ---------------------------------------------------------------------------
// StatsReader
// ---------------------------------------------------------------------------

func (p *Provider) GetRepoStatsFast(ctx context.Context) (*db.RepoStats, error) {
	return p.db.GetRepoStatsFast(ctx)
}

func (p *Provider) GetXpatchStats(ctx context.Context, tableName string) (*db.XpatchStats, error) {
	return p.db.GetXpatchStats(ctx, tableName)
}

func (p *Provider) GetDetailedTableSizes(ctx context.Context) (*db.DetailedTableSizes, error) {
	return p.db.GetDetailedTableSizes(ctx)
}

func (p *Provider) GetCommitStats(ctx context.Context) (map[string]any, error) {
	return p.db.GetCommitStats(ctx)
}

func (p *Provider) GetBlobStats(ctx context.Context) (map[string]any, error) {
	return p.db.GetBlobStats(ctx)
}

// ---------------------------------------------------------------------------
// SchemaManager
// ---------------------------------------------------------------------------

func (p *Provider) InitSchema(ctx context.Context) error {
	return p.db.InitSchema(ctx)
}

func (p *Provider) SchemaExists(ctx context.Context) (bool, error) {
	return p.db.SchemaExists(ctx)
}

func (p *Provider) GetSchemaVersion(ctx context.Context) (int, error) {
	return p.db.GetSchemaVersion(ctx)
}

func (p *Provider) SetSchemaVersion(ctx context.Context, version int) error {
	return p.db.SetSchemaVersion(ctx, version)
}

func (p *Provider) DropSchema(ctx context.Context) error {
	return p.db.DropSchema(ctx)
}

func (p *Provider) IsSchemaAtLeast(ctx context.Context, minVersion int) (bool, error) {
	return p.db.IsSchemaAtLeast(ctx, minVersion)
}

func (p *Provider) DropAllIndexes(ctx context.Context) error {
	return p.db.DropAllIndexes(ctx)
}

func (p *Provider) CreateAllIndexes(ctx context.Context) error {
	return p.db.CreateAllIndexes(ctx)
}

func (p *Provider) DropBlobPhaseIndexes(ctx context.Context) error {
	return p.db.DropBlobPhaseIndexes(ctx)
}

func (p *Provider) CreateBlobPhaseIndexes(ctx context.Context) error {
	return p.db.CreateBlobPhaseIndexes(ctx)
}

func (p *Provider) DropCommitsIndexes(ctx context.Context) error {
	return p.db.DropCommitsIndexes(ctx)
}

func (p *Provider) CreateCommitsIndexes(ctx context.Context) error {
	return p.db.CreateCommitsIndexes(ctx)
}

func (p *Provider) DropCommitGraphIndexes(ctx context.Context) error {
	return p.db.DropCommitGraphIndexes(ctx)
}

func (p *Provider) CreateCommitGraphIndexes(ctx context.Context) error {
	return p.db.CreateCommitGraphIndexes(ctx)
}

func (p *Provider) DropPathsIndexes(ctx context.Context) error {
	return p.db.DropPathsIndexes(ctx)
}

func (p *Provider) CreatePathsIndexes(ctx context.Context) error {
	return p.db.CreatePathsIndexes(ctx)
}

func (p *Provider) DropFileRefsIndexes(ctx context.Context) error {
	return p.db.DropFileRefsIndexes(ctx)
}

func (p *Provider) CreateFileRefsIndexes(ctx context.Context) error {
	return p.db.CreateFileRefsIndexes(ctx)
}

// ---------------------------------------------------------------------------
// SearchProvider
// ---------------------------------------------------------------------------

func (p *Provider) SearchContent(ctx context.Context, opts db.SearchContentOptions) ([]*db.SearchContentResult, error) {
	return p.db.SearchContent(ctx, opts)
}

func (p *Provider) SearchContentAtCommit(ctx context.Context, commitID string, opts db.SearchContentOptions) ([]*db.SearchContentResult, error) {
	return p.db.SearchContentAtCommit(ctx, commitID, opts)
}

// ---------------------------------------------------------------------------
// SQLExecutor
// ---------------------------------------------------------------------------

func (p *Provider) Exec(ctx context.Context, sql string, args ...any) error {
	return p.db.Exec(ctx, sql, args...)
}

func (p *Provider) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return p.db.Query(ctx, sql, args...)
}

func (p *Provider) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return p.db.QueryRow(ctx, sql, args...)
}

func (p *Provider) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return p.db.WithTx(ctx, fn)
}

func (p *Provider) Pool() *pgxpool.Pool {
	return p.db.Pool()
}

// ---------------------------------------------------------------------------
// Closer
// ---------------------------------------------------------------------------

func (p *Provider) Close() {
	p.db.Close()
}
