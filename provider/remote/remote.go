// Package remote implements provider.Provider by making HTTP calls to a pgit server.
package remote

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotSupported is returned for operations that require a direct database connection.
var ErrNotSupported = errors.New("operation not supported on remote provider")

// Provider implements provider.Provider over HTTP.
type Provider struct {
	baseURL    string
	httpClient *http.Client
	token      string
}

// New creates a remote Provider targeting the given server URL.
func New(baseURL, token string) *Provider {
	return &Provider{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		token:      token,
	}
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func (p *Provider) doRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	u := p.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// apiResponse wraps the standard {"data": ...} / {"error": ...} envelope.
type apiResponse struct {
	Data  json.RawMessage `json:"data"`
	Error string          `json:"error"`
}

func (p *Provider) get(ctx context.Context, path string, result any) error {
	resp, err := p.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeResponse(resp, result)
}

func (p *Provider) post(ctx context.Context, path string, body, result any) error {
	resp, err := p.doRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeResponse(resp, result)
}

func (p *Provider) put(ctx context.Context, path string, body, result any) error {
	resp, err := p.doRequest(ctx, http.MethodPut, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeResponse(resp, result)
}

func (p *Provider) delete(ctx context.Context, path string, result any) error {
	resp, err := p.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeResponse(resp, result)
}

func decodeResponse(resp *http.Response, result any) error {
	if resp.StatusCode >= 400 {
		var env apiResponse
		_ = json.NewDecoder(resp.Body).Decode(&env)
		if env.Error != "" {
			return fmt.Errorf("server error (%d): %s", resp.StatusCode, env.Error)
		}
		return fmt.Errorf("server error: %d", resp.StatusCode)
	}

	var env apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if env.Error != "" {
		return fmt.Errorf("server error: %s", env.Error)
	}
	if result != nil {
		if err := json.Unmarshal(env.Data, result); err != nil {
			return fmt.Errorf("unmarshal data: %w", err)
		}
	}
	return nil
}

// getNDJSON reads newline-delimited JSON from a response body.
func (p *Provider) getNDJSON(ctx context.Context, path string, factory func() any, collect func(any)) error {
	resp, err := p.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var env apiResponse
		_ = json.NewDecoder(resp.Body).Decode(&env)
		if env.Error != "" {
			return fmt.Errorf("server error (%d): %s", resp.StatusCode, env.Error)
		}
		return fmt.Errorf("server error: %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		item := factory()
		if err := json.Unmarshal(line, item); err != nil {
			return fmt.Errorf("unmarshal ndjson line: %w", err)
		}
		collect(item)
	}
	return scanner.Err()
}

// ---------------------------------------------------------------------------
// CommitReader
// ---------------------------------------------------------------------------

func (p *Provider) GetCommit(ctx context.Context, id string) (*db.Commit, error) {
	var c db.Commit
	if err := p.get(ctx, "/api/v1/commits/"+id, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (p *Provider) GetHeadCommit(ctx context.Context) (*db.Commit, error) {
	// Get HEAD ref, then get that commit.
	var head struct {
		CommitID string `json:"commit_id"`
	}
	if err := p.get(ctx, "/api/v1/head", &head); err != nil {
		return nil, err
	}
	return p.GetCommit(ctx, head.CommitID)
}

func (p *Provider) GetCommitLog(ctx context.Context, limit int) ([]*db.Commit, error) {
	var commits []*db.Commit
	if err := p.get(ctx, fmt.Sprintf("/api/v1/commits?limit=%d", limit), &commits); err != nil {
		return nil, err
	}
	return commits, nil
}

func (p *Provider) GetCommitLogFrom(ctx context.Context, commitID string, limit int) ([]*db.Commit, error) {
	var commits []*db.Commit
	if err := p.get(ctx, fmt.Sprintf("/api/v1/commits?limit=%d&from=%s", limit, url.QueryEscape(commitID)), &commits); err != nil {
		return nil, err
	}
	return commits, nil
}

func (p *Provider) GetAllCommits(ctx context.Context) ([]*db.Commit, error) {
	return p.GetCommitLog(ctx, 1_000_000) // server caps at its max; we want all
}

func (p *Provider) GetCommitsAfter(ctx context.Context, afterID string) ([]*db.Commit, error) {
	return nil, ErrNotSupported
}

func (p *Provider) CountCommits(ctx context.Context) (int, error) {
	return 0, ErrNotSupported
}

func (p *Provider) CommitExists(ctx context.Context, id string) (bool, error) {
	_, err := p.GetCommit(ctx, id)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (p *Provider) GetLatestCommitID(ctx context.Context) (string, error) {
	var head struct {
		CommitID string `json:"commit_id"`
	}
	if err := p.get(ctx, "/api/v1/head", &head); err != nil {
		return "", err
	}
	return head.CommitID, nil
}

func (p *Provider) GetAllCommitIDsOrdered(ctx context.Context) ([]string, error) {
	return nil, ErrNotSupported
}

func (p *Provider) FindCommonAncestor(ctx context.Context, commitA, commitB string) (string, error) {
	return "", ErrNotSupported
}

func (p *Provider) FindCommitByPartialID(ctx context.Context, partialID string) (*db.Commit, error) {
	return nil, ErrNotSupported
}

func (p *Provider) GetCommitsBatch(ctx context.Context, ids []string) (map[string]*db.Commit, error) {
	return nil, ErrNotSupported
}

func (p *Provider) GetCommitsBatchByRange(ctx context.Context, ids []string) (map[string]*db.Commit, error) {
	return nil, ErrNotSupported
}

// ---------------------------------------------------------------------------
// CommitWriter
// ---------------------------------------------------------------------------

func (p *Provider) CreateCommit(ctx context.Context, c *db.Commit) error {
	return ErrNotSupported
}

func (p *Provider) CreateCommitsBatch(ctx context.Context, commits []*db.Commit) error {
	return ErrNotSupported
}

func (p *Provider) CreateCommitsBatchTx(ctx context.Context, tx pgx.Tx, commits []*db.Commit) error {
	return ErrNotSupported
}

func (p *Provider) DeleteCommits(ctx context.Context, commitIDs []string) error {
	return ErrNotSupported
}

// ---------------------------------------------------------------------------
// BlobReader
// ---------------------------------------------------------------------------

func (p *Provider) GetBlob(ctx context.Context, path, commitID string) (*db.Blob, error) {
	var b db.Blob
	if err := p.get(ctx, "/api/v1/blob/"+commitID+"/"+path, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

func (p *Provider) GetBlobsAtCommit(ctx context.Context, commitID string) ([]*db.Blob, error) {
	return p.GetTreeAtCommit(ctx, commitID)
}

func (p *Provider) GetTreeAtCommit(ctx context.Context, commitID string) ([]*db.Blob, error) {
	var blobs []*db.Blob
	err := p.getNDJSON(ctx, "/api/v1/tree/"+commitID, func() any {
		return &db.Blob{}
	}, func(item any) {
		blobs = append(blobs, item.(*db.Blob))
	})
	return blobs, err
}

func (p *Provider) GetCurrentTree(ctx context.Context) ([]*db.Blob, error) {
	head, err := p.GetLatestCommitID(ctx)
	if err != nil {
		return nil, err
	}
	return p.GetTreeAtCommit(ctx, head)
}

func (p *Provider) GetTreeMetadataAtCommit(ctx context.Context, commitID string) ([]*db.Blob, error) {
	return nil, ErrNotSupported
}

func (p *Provider) GetCurrentTreeMetadata(ctx context.Context) ([]*db.Blob, error) {
	return nil, ErrNotSupported
}

func (p *Provider) GetFileHistory(ctx context.Context, path string) ([]*db.Blob, error) {
	var blobs []*db.Blob
	err := p.getNDJSON(ctx, "/api/v1/history/"+path, func() any {
		return &db.Blob{}
	}, func(item any) {
		blobs = append(blobs, item.(*db.Blob))
	})
	return blobs, err
}

func (p *Provider) GetFileAtCommit(ctx context.Context, path, commitID string) (*db.Blob, error) {
	return p.GetBlob(ctx, path, commitID)
}

func (p *Provider) GetChangedFiles(ctx context.Context, fromCommit, toCommit string) ([]*db.Blob, error) {
	var blobs []*db.Blob
	if err := p.get(ctx, fmt.Sprintf("/api/v1/diff?from=%s&to=%s",
		url.QueryEscape(fromCommit), url.QueryEscape(toCommit)), &blobs); err != nil {
		return nil, err
	}
	return blobs, nil
}

func (p *Provider) GetChangedFilesMetadata(ctx context.Context, fromCommit, toCommit string) ([]*db.Blob, error) {
	return nil, ErrNotSupported
}

func (p *Provider) GetBlobsAtCommitMetadata(ctx context.Context, commitID string) ([]*db.Blob, error) {
	return nil, ErrNotSupported
}

func (p *Provider) GetAllPaths(ctx context.Context) ([]string, error) {
	return nil, ErrNotSupported
}

func (p *Provider) GetImportedPaths(ctx context.Context) (map[string]bool, error) {
	return nil, ErrNotSupported
}

func (p *Provider) BlobExists(ctx context.Context, path, commitID string) (bool, error) {
	_, err := p.GetBlob(ctx, path, commitID)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (p *Provider) FileExistsInTree(ctx context.Context, path, commitID string) (bool, error) {
	return p.BlobExists(ctx, path, commitID)
}

func (p *Provider) CountBlobs(ctx context.Context) (int64, error) {
	return 0, ErrNotSupported
}

func (p *Provider) GetPathIDAndGroupIDByPath(ctx context.Context, path string) (int32, int32, error) {
	return 0, 0, ErrNotSupported
}

func (p *Provider) GetGroupIDByPath(ctx context.Context, path string) (int32, error) {
	return 0, ErrNotSupported
}

func (p *Provider) GetFileRefHistory(ctx context.Context, pathID int32) ([]*db.FileRef, error) {
	return nil, ErrNotSupported
}

func (p *Provider) GetFileRefsAtCommit(ctx context.Context, commitID string) ([]*db.FileRef, error) {
	return nil, ErrNotSupported
}

func (p *Provider) GetAllContentForGroup(ctx context.Context, groupID int32, isBinary bool) ([]db.ContentVersionPair, error) {
	return nil, ErrNotSupported
}

func (p *Provider) GetContent(ctx context.Context, groupID, versionID int32, isBinary bool) ([]byte, error) {
	return nil, ErrNotSupported
}

// ---------------------------------------------------------------------------
// BlobWriter
// ---------------------------------------------------------------------------

func (p *Provider) CreateBlob(ctx context.Context, b *db.Blob) error {
	return ErrNotSupported
}

func (p *Provider) CreateBlobs(ctx context.Context, blobs []*db.Blob) error {
	return ErrNotSupported
}

func (p *Provider) CreateBlobsTx(ctx context.Context, tx pgx.Tx, blobs []*db.Blob) error {
	return ErrNotSupported
}

func (p *Provider) DeleteBlobsForCommits(ctx context.Context, commitIDs []string) error {
	return ErrNotSupported
}

// ---------------------------------------------------------------------------
// RefManager
// ---------------------------------------------------------------------------

func (p *Provider) GetRef(ctx context.Context, name string) (*db.Ref, error) {
	return nil, ErrNotSupported
}

func (p *Provider) SetRef(ctx context.Context, name, commitID string) error {
	return ErrNotSupported
}

func (p *Provider) DeleteRef(ctx context.Context, name string) error {
	return ErrNotSupported
}

func (p *Provider) GetAllRefs(ctx context.Context) ([]*db.Ref, error) {
	var refs []*db.Ref
	if err := p.get(ctx, "/api/v1/refs", &refs); err != nil {
		return nil, err
	}
	return refs, nil
}

func (p *Provider) GetHead(ctx context.Context) (string, error) {
	var head struct {
		CommitID string `json:"commit_id"`
	}
	if err := p.get(ctx, "/api/v1/head", &head); err != nil {
		return "", err
	}
	return head.CommitID, nil
}

func (p *Provider) SetHead(ctx context.Context, commitID string) error {
	var result map[string]string
	return p.put(ctx, "/api/v1/head", map[string]string{"commit_id": commitID}, &result)
}

func (p *Provider) RefExists(ctx context.Context, name string) (bool, error) {
	return false, ErrNotSupported
}

// ---------------------------------------------------------------------------
// MetadataManager
// ---------------------------------------------------------------------------

func (p *Provider) EnsureMetadataTable(ctx context.Context) error {
	return ErrNotSupported
}

func (p *Provider) GetMetadata(ctx context.Context, key string) (string, error) {
	return "", ErrNotSupported
}

func (p *Provider) SetMetadata(ctx context.Context, key, value string) error {
	return ErrNotSupported
}

func (p *Provider) DeleteMetadata(ctx context.Context, key string) error {
	return ErrNotSupported
}

func (p *Provider) GetRepoPath(ctx context.Context) string {
	return ""
}

func (p *Provider) SetRepoPath(ctx context.Context, path string) error {
	return ErrNotSupported
}

// ---------------------------------------------------------------------------
// SyncManager
// ---------------------------------------------------------------------------

func (p *Provider) GetSyncState(ctx context.Context, remoteName string) (*db.SyncState, error) {
	return nil, ErrNotSupported
}

func (p *Provider) SetSyncState(ctx context.Context, remoteName string, lastCommitID *string) error {
	return ErrNotSupported
}

func (p *Provider) DeleteSyncState(ctx context.Context, remoteName string) error {
	return ErrNotSupported
}

func (p *Provider) GetAllSyncStates(ctx context.Context) ([]*db.SyncState, error) {
	return nil, ErrNotSupported
}

// ---------------------------------------------------------------------------
// GraphReader
// ---------------------------------------------------------------------------

func (p *Provider) GetCommitGraphByID(ctx context.Context, id string) (*db.CommitGraphEntry, error) {
	return nil, ErrNotSupported
}

func (p *Provider) GetCommitGraphBySeq(ctx context.Context, seq int32) (*db.CommitGraphEntry, error) {
	return nil, ErrNotSupported
}

func (p *Provider) GetAncestorID(ctx context.Context, commitID string, n int) (string, error) {
	return "", ErrNotSupported
}

func (p *Provider) CommitExistsInGraph(ctx context.Context, id string) (bool, error) {
	return false, ErrNotSupported
}

func (p *Provider) FindCommitByPartialIDInGraph(ctx context.Context, partialID string) (string, error) {
	return "", ErrNotSupported
}

func (p *Provider) CountCommitsFromGraph(ctx context.Context) (int, error) {
	return 0, ErrNotSupported
}

// ---------------------------------------------------------------------------
// GraphWriter
// ---------------------------------------------------------------------------

func (p *Provider) CreateCommitGraphBatch(ctx context.Context, entries []db.CommitGraphEntry) error {
	return ErrNotSupported
}

// ---------------------------------------------------------------------------
// StatsReader
// ---------------------------------------------------------------------------

func (p *Provider) GetRepoStatsFast(ctx context.Context) (*db.RepoStats, error) {
	var stats db.RepoStats
	if err := p.get(ctx, "/api/v1/stats", &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

func (p *Provider) GetXpatchStats(ctx context.Context, tableName string) (*db.XpatchStats, error) {
	return nil, ErrNotSupported
}

func (p *Provider) GetDetailedTableSizes(ctx context.Context) (*db.DetailedTableSizes, error) {
	return nil, ErrNotSupported
}

func (p *Provider) GetCommitStats(ctx context.Context) (map[string]any, error) {
	return nil, ErrNotSupported
}

func (p *Provider) GetBlobStats(ctx context.Context) (map[string]any, error) {
	return nil, ErrNotSupported
}

// ---------------------------------------------------------------------------
// SchemaManager
// ---------------------------------------------------------------------------

func (p *Provider) InitSchema(ctx context.Context) error {
	return ErrNotSupported
}

func (p *Provider) SchemaExists(ctx context.Context) (bool, error) {
	return false, ErrNotSupported
}

func (p *Provider) GetSchemaVersion(ctx context.Context) (int, error) {
	var result struct {
		Version int `json:"version"`
	}
	if err := p.get(ctx, "/api/v1/schema/version", &result); err != nil {
		return 0, err
	}
	return result.Version, nil
}

func (p *Provider) SetSchemaVersion(ctx context.Context, version int) error {
	return ErrNotSupported
}

func (p *Provider) DropSchema(ctx context.Context) error {
	return ErrNotSupported
}

func (p *Provider) IsSchemaAtLeast(ctx context.Context, minVersion int) (bool, error) {
	v, err := p.GetSchemaVersion(ctx)
	if err != nil {
		return false, err
	}
	return v >= minVersion, nil
}

func (p *Provider) DropAllIndexes(ctx context.Context) error     { return ErrNotSupported }
func (p *Provider) CreateAllIndexes(ctx context.Context) error   { return ErrNotSupported }
func (p *Provider) DropBlobPhaseIndexes(ctx context.Context) error   { return ErrNotSupported }
func (p *Provider) CreateBlobPhaseIndexes(ctx context.Context) error { return ErrNotSupported }
func (p *Provider) DropCommitsIndexes(ctx context.Context) error     { return ErrNotSupported }
func (p *Provider) CreateCommitsIndexes(ctx context.Context) error   { return ErrNotSupported }
func (p *Provider) DropCommitGraphIndexes(ctx context.Context) error { return ErrNotSupported }
func (p *Provider) CreateCommitGraphIndexes(ctx context.Context) error {
	return ErrNotSupported
}
func (p *Provider) DropPathsIndexes(ctx context.Context) error   { return ErrNotSupported }
func (p *Provider) CreatePathsIndexes(ctx context.Context) error { return ErrNotSupported }
func (p *Provider) DropFileRefsIndexes(ctx context.Context) error   { return ErrNotSupported }
func (p *Provider) CreateFileRefsIndexes(ctx context.Context) error { return ErrNotSupported }
func (p *Provider) DropReviewIndexes(ctx context.Context) error     { return ErrNotSupported }
func (p *Provider) CreateReviewIndexes(ctx context.Context) error   { return ErrNotSupported }

// ---------------------------------------------------------------------------
// SearchProvider
// ---------------------------------------------------------------------------

func (p *Provider) SearchContent(ctx context.Context, opts db.SearchContentOptions) ([]*db.SearchContentResult, error) {
	q := url.Values{}
	q.Set("pattern", opts.Pattern)
	if opts.IgnoreCase {
		q.Set("ignore_case", "true")
	}
	if opts.PathPattern != "" {
		q.Set("path_pattern", opts.PathPattern)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}

	var results []*db.SearchContentResult
	err := p.getNDJSON(ctx, "/api/v1/search?"+q.Encode(), func() any {
		return &db.SearchContentResult{}
	}, func(item any) {
		results = append(results, item.(*db.SearchContentResult))
	})
	return results, err
}

func (p *Provider) SearchContentAtCommit(ctx context.Context, commitID string, opts db.SearchContentOptions) ([]*db.SearchContentResult, error) {
	return nil, ErrNotSupported
}

// ---------------------------------------------------------------------------
// SQLExecutor (not supported on remote)
// ---------------------------------------------------------------------------

func (p *Provider) Exec(ctx context.Context, sql string, args ...any) error {
	return ErrNotSupported
}

func (p *Provider) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, ErrNotSupported
}

func (p *Provider) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return nil
}

func (p *Provider) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return ErrNotSupported
}

func (p *Provider) Pool() *pgxpool.Pool {
	return nil
}

// ---------------------------------------------------------------------------
// AuthorManager
// ---------------------------------------------------------------------------

func (p *Provider) CreateAuthor(ctx context.Context, a *db.Author) error {
	return p.post(ctx, "/api/v1/authors", a, a)
}

func (p *Provider) GetAuthor(ctx context.Context, id string) (*db.Author, error) {
	var a db.Author
	if err := p.get(ctx, "/api/v1/authors/"+id, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (p *Provider) GetAuthorByName(ctx context.Context, name string) (*db.Author, error) {
	return nil, ErrNotSupported
}

func (p *Provider) ListAuthors(ctx context.Context, includeDeleted bool) ([]*db.Author, error) {
	path := "/api/v1/authors"
	if includeDeleted {
		path += "?include_deleted=true"
	}
	var authors []*db.Author
	if err := p.get(ctx, path, &authors); err != nil {
		return nil, err
	}
	return authors, nil
}

func (p *Provider) GetAuthorChildren(ctx context.Context, parentID string) ([]*db.Author, error) {
	return nil, ErrNotSupported
}

func (p *Provider) GetAuthorChain(ctx context.Context, id string) ([]*db.Author, error) {
	return nil, ErrNotSupported
}

func (p *Provider) ComputeEffectivePermissions(ctx context.Context, id string) (db.Permission, error) {
	return 0, ErrNotSupported
}

func (p *Provider) SoftDeleteAuthor(ctx context.Context, id string) error {
	var result map[string]string
	return p.delete(ctx, "/api/v1/authors/"+id, &result)
}

func (p *Provider) UpdateAuthorPermissions(ctx context.Context, id string, perms db.Permission) error {
	return ErrNotSupported
}

func (p *Provider) CreateAuthorToken(ctx context.Context, authorID string) (string, error) {
	var result struct {
		Token string `json:"token"`
	}
	if err := p.post(ctx, "/api/v1/authors/"+authorID+"/token", nil, &result); err != nil {
		return "", err
	}
	return result.Token, nil
}

func (p *Provider) ValidateAuthorToken(ctx context.Context, token string) (*db.Author, error) {
	return nil, ErrNotSupported
}

// ---------------------------------------------------------------------------
// CLManager
// ---------------------------------------------------------------------------

func (p *Provider) CreateCL(ctx context.Context, c *db.CL) error {
	return p.post(ctx, "/api/v1/cls", c, c)
}

func (p *Provider) GetCL(ctx context.Context, id string) (*db.CL, error) {
	var c db.CL
	if err := p.get(ctx, "/api/v1/cls/"+id, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (p *Provider) ListCLs(ctx context.Context, status db.CLStatus, limit int) ([]*db.CL, error) {
	q := url.Values{}
	if status != "" {
		q.Set("status", string(status))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := "/api/v1/cls"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var cls []*db.CL
	if err := p.get(ctx, path, &cls); err != nil {
		return nil, err
	}
	return cls, nil
}

func (p *Provider) UpdateCLStatus(ctx context.Context, id string, status db.CLStatus) error {
	var result map[string]string
	return p.put(ctx, "/api/v1/cls/"+id+"/status", map[string]string{"status": string(status)}, &result)
}

func (p *Provider) SubmitCL(ctx context.Context, id string, commitHash string) error {
	var result map[string]string
	return p.post(ctx, "/api/v1/cls/"+id+"/submit", map[string]string{"commit_hash": commitHash}, &result)
}

func (p *Provider) UpdateCL(ctx context.Context, c *db.CL) error {
	return ErrNotSupported
}

func (p *Provider) CreatePatchSet(ctx context.Context, ps *db.PatchSet) error {
	return p.post(ctx, "/api/v1/cls/"+ps.CLID+"/patchsets", ps, ps)
}

func (p *Provider) GetPatchSetsForCL(ctx context.Context, clID string) ([]*db.PatchSet, error) {
	var patchsets []*db.PatchSet
	if err := p.get(ctx, "/api/v1/cls/"+clID+"/patchsets", &patchsets); err != nil {
		return nil, err
	}
	return patchsets, nil
}

func (p *Provider) GetLatestPatchSet(ctx context.Context, clID string) (*db.PatchSet, error) {
	return nil, ErrNotSupported
}

func (p *Provider) CreateReviewComment(ctx context.Context, c *db.ReviewComment) error {
	return p.post(ctx, "/api/v1/cls/"+c.CLID+"/comments", c, c)
}

func (p *Provider) GetCommentsForCL(ctx context.Context, clID string) ([]*db.ReviewComment, error) {
	var comments []*db.ReviewComment
	if err := p.get(ctx, "/api/v1/cls/"+clID+"/comments", &comments); err != nil {
		return nil, err
	}
	return comments, nil
}

func (p *Provider) GetCommentsForPatchSet(ctx context.Context, clID string, patchSetNum int) ([]*db.ReviewComment, error) {
	return nil, ErrNotSupported
}

func (p *Provider) SetReviewVote(ctx context.Context, v *db.ReviewVote) error {
	var result db.ReviewVote
	return p.put(ctx, "/api/v1/cls/"+v.CLID+"/votes", v, &result)
}

func (p *Provider) GetVotesForCL(ctx context.Context, clID string) ([]*db.ReviewVote, error) {
	var votes []*db.ReviewVote
	if err := p.get(ctx, "/api/v1/cls/"+clID+"/votes", &votes); err != nil {
		return nil, err
	}
	return votes, nil
}

func (p *Provider) GetCLStack(ctx context.Context, clID string) ([]*db.CL, error) {
	return nil, ErrNotSupported
}

// ---------------------------------------------------------------------------
// Closer
// ---------------------------------------------------------------------------

func (p *Provider) Close() {
	// No resources to release.
}
