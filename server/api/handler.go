// Package api implements JSON-over-HTTP handlers for the pgit server.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/imgajeed76/pgit/v4/server/auth"
)

// Handler serves all /api/v1/ endpoints.
type Handler struct {
	db  *db.DB
	mux *http.ServeMux
}

// NewHandler creates a Handler and registers routes.
func NewHandler(d *db.DB) *Handler {
	h := &Handler{db: d, mux: http.NewServeMux()}
	h.registerRoutes()
	return h
}

// ServeHTTP delegates to the internal mux.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) registerRoutes() {
	// Commits
	h.mux.HandleFunc("GET /api/v1/commits/{id}", h.GetCommit)
	h.mux.HandleFunc("GET /api/v1/commits", h.GetCommitLog)
	h.mux.HandleFunc("GET /api/v1/head", h.GetHead)
	h.mux.HandleFunc("PUT /api/v1/head", h.SetHead)

	// Blobs
	h.mux.HandleFunc("GET /api/v1/tree/{commitID}", h.GetTreeAtCommit)
	h.mux.HandleFunc("GET /api/v1/blob/{commitID}/{path...}", h.GetFileAtCommit)
	h.mux.HandleFunc("GET /api/v1/diff", h.GetChangedFiles)
	h.mux.HandleFunc("GET /api/v1/history/{path...}", h.GetFileHistory)

	// Refs
	h.mux.HandleFunc("GET /api/v1/refs", h.GetAllRefs)

	// Search
	h.mux.HandleFunc("GET /api/v1/search", h.SearchContent)

	// Authors
	h.mux.HandleFunc("POST /api/v1/authors", h.CreateAuthor)
	h.mux.HandleFunc("GET /api/v1/authors", h.ListAuthors)
	h.mux.HandleFunc("GET /api/v1/authors/{id}", h.GetAuthor)
	h.mux.HandleFunc("DELETE /api/v1/authors/{id}", h.SoftDeleteAuthor)
	h.mux.HandleFunc("POST /api/v1/authors/{id}/token", h.CreateAuthorToken)

	// CLs
	h.mux.HandleFunc("POST /api/v1/cls", h.CreateCL)
	h.mux.HandleFunc("GET /api/v1/cls", h.ListCLs)
	h.mux.HandleFunc("GET /api/v1/cls/{id}", h.GetCL)
	h.mux.HandleFunc("PUT /api/v1/cls/{id}/status", h.UpdateCLStatus)
	h.mux.HandleFunc("POST /api/v1/cls/{id}/patchsets", h.CreatePatchSet)
	h.mux.HandleFunc("GET /api/v1/cls/{id}/patchsets", h.GetPatchSetsForCL)
	h.mux.HandleFunc("POST /api/v1/cls/{id}/comments", h.CreateReviewComment)
	h.mux.HandleFunc("GET /api/v1/cls/{id}/comments", h.GetCommentsForCL)
	h.mux.HandleFunc("PUT /api/v1/cls/{id}/votes", h.SetReviewVote)
	h.mux.HandleFunc("GET /api/v1/cls/{id}/votes", h.GetVotesForCL)
	h.mux.HandleFunc("POST /api/v1/cls/{id}/submit", h.SubmitCL)

	// User refs
	h.mux.HandleFunc("GET /api/v1/refs/user/{email}", h.GetUserRefs)
	h.mux.HandleFunc("PUT /api/v1/refs/user/{email}/{name}", h.SetUserRef)
	h.mux.HandleFunc("DELETE /api/v1/refs/user/{email}/{name}", h.DeleteUserRef)

	// CI results
	h.mux.HandleFunc("POST /api/v1/cls/{id}/ci", h.CreateCIResult)
	h.mux.HandleFunc("GET /api/v1/cls/{id}/ci", h.GetCIResultsForCL)
	h.mux.HandleFunc("PUT /api/v1/ci/{id}", h.UpdateCIResult)

	// Stats / Schema
	h.mux.HandleFunc("GET /api/v1/stats", h.GetRepoStats)
	h.mux.HandleFunc("GET /api/v1/schema/version", h.GetSchemaVersion)
}

// ---------------------------------------------------------------------------
// Response helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
}

func writeNDJSON(w http.ResponseWriter, items []any) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	for _, item := range items {
		_ = enc.Encode(item)
	}
}

// requireAuthor extracts the authenticated author from context.
func requireAuthor(w http.ResponseWriter, r *http.Request) *db.Author {
	a := auth.AuthorFromContext(r.Context())
	if a == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return nil
	}
	return a
}

// requirePerm checks that the authenticated author has the given permission.
func (h *Handler) requirePerm(w http.ResponseWriter, r *http.Request, perm db.Permission) *db.Author {
	a := requireAuthor(w, r)
	if a == nil {
		return nil
	}
	// Compute effective permissions (AND walk to root)
	effective, err := h.db.ComputeEffectivePermissions(r.Context(), a.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to compute permissions")
		return nil
	}
	if !effective.Has(perm) {
		writeError(w, http.StatusForbidden, fmt.Sprintf("insufficient permissions: requires %s", perm.String()))
		return nil
	}
	return a
}

// maxBodySize limits request body to 1MB.
const maxBodySize = 1 << 20

// readJSON decodes a JSON request body with a size limit.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Commits
// ---------------------------------------------------------------------------

func (h *Handler) GetCommit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := h.db.GetCommit(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no rows") {
			writeError(w, http.StatusNotFound, fmt.Sprintf("commit not found: %s", id))
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) GetCommitLog(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}

	from := r.URL.Query().Get("from")
	var commits []*db.Commit
	var err error
	if from != "" {
		commits, err = h.db.GetCommitLogFrom(r.Context(), from, limit)
	} else {
		commits, err = h.db.GetCommitLog(r.Context(), limit)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, commits)
}

func (h *Handler) GetHead(w http.ResponseWriter, r *http.Request) {
	commitID, err := h.db.GetHead(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"commit_id": commitID})
}

func (h *Handler) SetHead(w http.ResponseWriter, r *http.Request) {
	if h.requirePerm(w, r, db.PermCLSubmit) == nil {
		return
	}
	var body struct {
		CommitID string `json:"commit_id"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if body.CommitID == "" {
		writeError(w, http.StatusBadRequest, "commit_id required")
		return
	}
	if err := h.db.SetHead(r.Context(), body.CommitID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"commit_id": body.CommitID})
}

// ---------------------------------------------------------------------------
// Blobs
// ---------------------------------------------------------------------------

func (h *Handler) GetTreeAtCommit(w http.ResponseWriter, r *http.Request) {
	commitID := r.PathValue("commitID")
	blobs, err := h.db.GetTreeAtCommit(r.Context(), commitID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Stream as NDJSON for potentially large results.
	items := make([]any, len(blobs))
	for i, b := range blobs {
		items[i] = b
	}
	writeNDJSON(w, items)
}

func (h *Handler) GetFileAtCommit(w http.ResponseWriter, r *http.Request) {
	commitID := r.PathValue("commitID")
	path := r.PathValue("path")
	blob, err := h.db.GetFileAtCommit(r.Context(), path, commitID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no rows") {
			writeError(w, http.StatusNotFound, fmt.Sprintf("file not found: %s at %s", path, commitID))
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, blob)
}

func (h *Handler) GetChangedFiles(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" || to == "" {
		writeError(w, http.StatusBadRequest, "from and to query parameters required")
		return
	}
	blobs, err := h.db.GetChangedFiles(r.Context(), from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, blobs)
}

func (h *Handler) GetFileHistory(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	blobs, err := h.db.GetFileHistory(r.Context(), path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]any, len(blobs))
	for i, b := range blobs {
		items[i] = b
	}
	writeNDJSON(w, items)
}

// ---------------------------------------------------------------------------
// Refs
// ---------------------------------------------------------------------------

func (h *Handler) GetAllRefs(w http.ResponseWriter, r *http.Request) {
	refs, err := h.db.GetAllRefs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, refs)
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

func (h *Handler) SearchContent(w http.ResponseWriter, r *http.Request) {
	pattern := r.URL.Query().Get("pattern")
	if pattern == "" {
		writeError(w, http.StatusBadRequest, "pattern query parameter required")
		return
	}

	opts := db.SearchContentOptions{
		Pattern:    pattern,
		IgnoreCase: r.URL.Query().Get("ignore_case") == "true",
	}
	if pp := r.URL.Query().Get("path_pattern"); pp != "" {
		opts.PathPattern = pp
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			opts.Limit = n
		}
	}

	results, err := h.db.SearchContent(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]any, len(results))
	for i, res := range results {
		items[i] = res
	}
	writeNDJSON(w, items)
}

// ---------------------------------------------------------------------------
// Authors
// ---------------------------------------------------------------------------

func (h *Handler) CreateAuthor(w http.ResponseWriter, r *http.Request) {
	caller := h.requirePerm(w, r, db.PermAdmin)
	if caller == nil {
		return
	}
	var a db.Author
	if !readJSON(w, r, &a) {
		return
	}
	// Prevent privilege escalation: new author's permissions cannot exceed caller's effective permissions.
	callerEffective, _ := h.db.ComputeEffectivePermissions(r.Context(), caller.ID)
	if a.Permissions & ^callerEffective != 0 {
		writeError(w, http.StatusForbidden, "cannot grant permissions you don't have")
		return
	}
	if err := h.db.CreateAuthor(r.Context(), &a); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create author")
		return
	}
	writeJSON(w, http.StatusCreated, &a)
}

func (h *Handler) ListAuthors(w http.ResponseWriter, r *http.Request) {
	includeDeleted := r.URL.Query().Get("include_deleted") == "true"
	authors, err := h.db.ListAuthors(r.Context(), includeDeleted)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, authors)
}

func (h *Handler) GetAuthor(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, err := h.db.GetAuthor(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *Handler) SoftDeleteAuthor(w http.ResponseWriter, r *http.Request) {
	if h.requirePerm(w, r, db.PermAdmin) == nil {
		return
	}
	id := r.PathValue("id")
	if err := h.db.SoftDeleteAuthor(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) CreateAuthorToken(w http.ResponseWriter, r *http.Request) {
	caller := h.requirePerm(w, r, db.PermAdmin)
	if caller == nil {
		return
	}
	id := r.PathValue("id")
	// Can only create tokens for yourself or your descendants
	chain, err := h.db.GetAuthorChain(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "author not found")
		return
	}
	isDescendant := false
	for _, a := range chain {
		if a.ID == caller.ID {
			isDescendant = true
			break
		}
	}
	if !isDescendant {
		writeError(w, http.StatusForbidden, "can only create tokens for yourself or your descendants")
		return
	}
	token, err := h.db.CreateAuthorToken(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// ---------------------------------------------------------------------------
// CLs
// ---------------------------------------------------------------------------

func (h *Handler) CreateCL(w http.ResponseWriter, r *http.Request) {
	if h.requirePerm(w, r, db.PermCLCreate) == nil {
		return
	}
	var c db.CL
	if !readJSON(w, r, &c) {
		return
	}
	if err := h.db.CreateCL(r.Context(), &c); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create CL")
		return
	}
	writeJSON(w, http.StatusCreated, &c)
}

func (h *Handler) ListCLs(w http.ResponseWriter, r *http.Request) {
	status := db.CLStatus(r.URL.Query().Get("status"))
	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	cls, err := h.db.ListCLs(r.Context(), status, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cls)
}

func (h *Handler) GetCL(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := h.db.GetCL(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) UpdateCLStatus(w http.ResponseWriter, r *http.Request) {
	if h.requirePerm(w, r, db.PermCLUpdate) == nil {
		return
	}
	id := r.PathValue("id")
	var body struct {
		Status db.CLStatus `json:"status"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := h.db.UpdateCLStatus(r.Context(), id, body.Status); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update CL status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": string(body.Status)})
}

func (h *Handler) CreatePatchSet(w http.ResponseWriter, r *http.Request) {
	if h.requirePerm(w, r, db.PermCLUpdate) == nil {
		return
	}
	clID := r.PathValue("id")
	var ps db.PatchSet
	if !readJSON(w, r, &ps) {
		return
	}
	ps.CLID = clID
	if err := h.db.CreatePatchSet(r.Context(), &ps); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create patch set")
		return
	}
	writeJSON(w, http.StatusCreated, &ps)
}

func (h *Handler) GetPatchSetsForCL(w http.ResponseWriter, r *http.Request) {
	clID := r.PathValue("id")
	patchsets, err := h.db.GetPatchSetsForCL(r.Context(), clID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get patch sets")
		return
	}
	writeJSON(w, http.StatusOK, patchsets)
}

func (h *Handler) CreateReviewComment(w http.ResponseWriter, r *http.Request) {
	if h.requirePerm(w, r, db.PermComment) == nil {
		return
	}
	clID := r.PathValue("id")
	var c db.ReviewComment
	if !readJSON(w, r, &c) {
		return
	}
	c.CLID = clID
	if err := h.db.CreateReviewComment(r.Context(), &c); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create comment")
		return
	}
	writeJSON(w, http.StatusCreated, &c)
}

func (h *Handler) GetCommentsForCL(w http.ResponseWriter, r *http.Request) {
	clID := r.PathValue("id")
	comments, err := h.db.GetCommentsForCL(r.Context(), clID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get comments")
		return
	}
	writeJSON(w, http.StatusOK, comments)
}

func (h *Handler) SetReviewVote(w http.ResponseWriter, r *http.Request) {
	if h.requirePerm(w, r, db.PermVote) == nil {
		return
	}
	clID := r.PathValue("id")
	var v db.ReviewVote
	if !readJSON(w, r, &v) {
		return
	}
	v.CLID = clID
	if err := h.db.SetReviewVote(r.Context(), &v); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set vote")
		return
	}
	writeJSON(w, http.StatusOK, &v)
}

func (h *Handler) GetVotesForCL(w http.ResponseWriter, r *http.Request) {
	clID := r.PathValue("id")
	votes, err := h.db.GetVotesForCL(r.Context(), clID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get votes")
		return
	}
	writeJSON(w, http.StatusOK, votes)
}

func (h *Handler) SubmitCL(w http.ResponseWriter, r *http.Request) {
	if h.requirePerm(w, r, db.PermCLSubmit) == nil {
		return
	}
	id := r.PathValue("id")
	var body struct {
		CommitHash string `json:"commit_hash"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := h.db.SubmitCL(r.Context(), id, body.CommitHash); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to submit CL")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "submitted"})
}

// ---------------------------------------------------------------------------
// Stats / Schema
// ---------------------------------------------------------------------------

func (h *Handler) GetRepoStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.db.GetRepoStatsFast(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) GetSchemaVersion(w http.ResponseWriter, r *http.Request) {
	v, err := h.db.GetSchemaVersion(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"version": v})
}

// ---------------------------------------------------------------------------
// User Refs
// ---------------------------------------------------------------------------

func (h *Handler) GetUserRefs(w http.ResponseWriter, r *http.Request) {
	email := r.PathValue("email")
	refs, err := h.db.GetUserRefs(r.Context(), email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, refs)
}

func (h *Handler) SetUserRef(w http.ResponseWriter, r *http.Request) {
	if h.requirePerm(w, r, db.PermCLUpdate) == nil {
		return
	}
	email := r.PathValue("email")
	name := r.PathValue("name")
	var body struct {
		CommitID string `json:"commit_id"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if body.CommitID == "" {
		writeError(w, http.StatusBadRequest, "commit_id required")
		return
	}
	if err := h.db.SetUserRef(r.Context(), email, name, body.CommitID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"name":      fmt.Sprintf("refs/users/%s/%s", email, name),
		"commit_id": body.CommitID,
	})
}

func (h *Handler) DeleteUserRef(w http.ResponseWriter, r *http.Request) {
	if h.requirePerm(w, r, db.PermCLUpdate) == nil {
		return
	}
	email := r.PathValue("email")
	name := r.PathValue("name")
	if err := h.db.DeleteUserRef(r.Context(), email, name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ---------------------------------------------------------------------------
// CI Results
// ---------------------------------------------------------------------------

func (h *Handler) CreateCIResult(w http.ResponseWriter, r *http.Request) {
	if h.requirePerm(w, r, db.PermCITrigger) == nil {
		return
	}
	clID := r.PathValue("id")
	var ci db.CIResult
	if !readJSON(w, r, &ci) {
		return
	}
	ci.CLID = clID
	if err := h.db.CreateCIResult(r.Context(), &ci); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create CI result")
		return
	}
	writeJSON(w, http.StatusCreated, &ci)
}

func (h *Handler) GetCIResultsForCL(w http.ResponseWriter, r *http.Request) {
	clID := r.PathValue("id")

	patchSetStr := r.URL.Query().Get("patchset")
	if patchSetStr != "" {
		patchSet, pErr := strconv.Atoi(patchSetStr)
		if pErr != nil {
			writeError(w, http.StatusBadRequest, "invalid patchset number")
			return
		}
		results, err := h.db.GetCIResultsForPatchSet(r.Context(), clID, patchSet)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, results)
		return
	}

	results, err := h.db.GetCIResultsForCL(r.Context(), clID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *Handler) UpdateCIResult(w http.ResponseWriter, r *http.Request) {
	if h.requirePerm(w, r, db.PermCITrigger) == nil {
		return
	}
	id := r.PathValue("id")
	var ci db.CIResult
	if !readJSON(w, r, &ci) {
		return
	}
	ci.ID = id
	if err := h.db.UpdateCIResult(r.Context(), &ci); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update CI result")
		return
	}
	writeJSON(w, http.StatusOK, &ci)
}
