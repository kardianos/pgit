# pgit Vision: Collaborative Development Platform

## Overview

pgit today is a Git-compatible VCS backed by PostgreSQL. The next phase
transforms it into a complete collaborative development platform by adding
FUSE-based virtual filesystems, server-side references, Gerrit-style change
sets, and integrated code review with first-class CLI and web interfaces.

---

## Core Principles

- **No branching model.** Changes are modeled as **change lists (CLs)**, not
  branches. The default submit strategy is cherry-pick onto HEAD. This
  eliminates merge conflicts caused by long-lived branches and keeps history
  linear.
- **Stacked CLs** are a first-class concept. A stack can be submitted
  atomically (all-at-once) or one CL at a time from the bottom up.
- **Instant workspaces.** FUSE makes checkout near-zero-cost — only files
  actually read are fetched. CI/CD benefits the most: a clean repo is
  available instantly without cloning.
- **Review is equal for humans and AI.** The review interface exposes the
  same primitives (diff, comment, vote, submit) through CLI, web UI, and
  programmatic API so automated reviewers are indistinguishable from human
  ones.
- **Hierarchical authors.** Every actor in the system — human, LLM, CI
  job — is an author. Authors can create sub-authors on the fly with
  equal or fewer permissions, forming a delegation chain
  ("LLM-B on behalf of LLM-A on behalf of user-1"). Permissions are
  computed at evaluation time via bitwise AND across the chain, so a
  sub-author can never exceed any ancestor's capabilities.

---

## Components

### 1. FUSE Virtual Filesystem

A FUSE provider that presents any CL, commit, or submitted HEAD as a
read-only (or copy-on-write) directory tree.

- **Lazy materialization.** Files are fetched from PostgreSQL on first
  `open(2)`; directory listings come from the `file_refs` table.
- **CI/CD asset cache.** Build artifacts and dependency caches are stored
  as addressable blobs. CI jobs mount the cache layer beneath the source
  layer — rebuilds only fetch changed inputs.
- **Checkout via FUSE.** `pgit mount <ref> <mountpoint>` exposes any
  revision. `pgit mount cl/<id>` mounts an in-flight change list for
  local building or testing without modifying the working tree.

### 2. Server-Side User Branches

Lightweight, per-user refs stored in PostgreSQL — analogous to
`refs/users/` in Gerrit or `refs/drafts/`.

- Every user gets an implicit namespace (`refs/users/<email>/`).
- Pushing to your namespace never conflicts with anyone else.
- CLs are created by pushing to your namespace; the review system picks
  them up automatically.
- Server-side refs enable resuming work from any machine without
  exchanging packfiles.

### 3. Change Lists (CLs)

The unit of review. Inspired by Gerrit change sets.

- A CL is one logical change (single commit or squashed patch) with
  metadata: title, description, reviewers, labels, CI status.
- **Server-side CL identity.** The CL ID is a server-assigned ULID
  stored in the `cl` table — it is never embedded in commit messages,
  trailers, or any client-side artifact. The mapping between a CL and
  its commits lives in the `patch_set` table. Amending a commit message,
  rewording, or rebasing cannot alter or orphan a CL. Clients track
  the CL association locally (e.g. in `.pgit/cl_head`) and the server
  resolves it by patch-set linkage, not by parsing message content.
- **First-class client support.** The pgit client is CL-aware
  throughout the workflow. Common git operations are CL-contextual:
  - `pgit commit` inside a CL automatically creates a new patch set.
  - `pgit commit --amend` updates the current patch set in place
    (rewriting the snapshot commit) rather than creating a new one.
  - `pgit reword` updates the CL title/description and the commit
    message together — the CL ID is unaffected.
  - `pgit status` shows the active CL, its patch-set number, and
    review state.
  - `pgit diff` defaults to diffing the current CL against its base.
  - `pgit push` uploads the latest patch set to the server.
- **Patch sets.** Each update to a CL creates a new numbered patch set.
  Reviewers can diff between any two patch sets.
- **Default submit strategy: cherry-pick on HEAD.** When a CL is
  submitted, its patch is cherry-picked onto the current tip. No merge
  commits, no branch integrations.
- **Stacked CLs.** A CL can declare a parent CL. The stack is displayed
  as a chain in the review UI. Submission options:
  - Submit the entire stack atomically (sequential cherry-picks).
  - Submit individually from the bottom, rebasing descendants
    automatically.
- **No long-lived branches.** Feature flags and incremental delivery
  replace branches. If isolated work is truly needed, stacked CLs
  provide the same workflow without branch overhead.

### 4. Code Review

#### CLI (`pgit review`)

```
pgit cl new              # create a CL from staged changes
pgit cl update           # push a new patch set to an existing CL
pgit cl list             # list open CLs
pgit cl show <id>        # display CL diff, comments, status
pgit cl comment <id>     # add an inline or top-level comment
pgit cl vote <id> +1     # vote (approve / request changes)
pgit cl submit <id>      # cherry-pick CL onto HEAD
pgit cl mount <id>       # FUSE-mount the CL for local testing
pgit cl stack             # show the current CL stack
pgit cl stack submit      # submit the full stack
```

#### Web Interface

- Diff viewer with inline commenting and threading.
- CL dashboard: open, reviewed, submitted, abandoned.
- Stack visualization (dependency graph of related CLs).
- CI status and build logs inline.
- Identical data model as the CLI — anything done in the web UI is
  immediately visible from the CLI and vice versa.

#### Acting as a Sub-Author (`--account`)

There is no separate AI mode. Any command can be executed under a
sub-author's identity using the global `--account` flag or the
`PGIT_ACCOUNT` environment variable:

```
# Explicit flag — act as your "llm-reviewer" sub-author
pgit --account llm-reviewer cl comment 42 -m "nit: unused import"

# Environment — useful in CI or LLM agent wrappers
export PGIT_ACCOUNT=ci-builder
pgit cl show 42
```

The caller still authenticates as themselves (token, OAuth, etc.).
`--account` tells the server to attribute the action to the named
sub-author under the caller's author tree. The server verifies:

1. The sub-author exists and is a descendant of the authenticated
   caller.
2. The sub-author's effective permissions (bitwise AND up the chain)
   allow the requested operation.

If either check fails the request is rejected. This means an LLM
agent, a CI job, and a human colleague all use the exact same CLI —
the only difference is which `--account` they present. AI comments,
votes, and CL updates are attributed and filterable by author kind
but rendered inline with no separate pane.

### 5. CI/CD Integration

- CI jobs mount the repo via FUSE — no clone step.
- Asset cache is a shared blob store; jobs declare cache keys and the
  FUSE layer handles materialization.
- CI results (status, logs, artifacts) are written back to the CL as
  structured metadata, visible in both CLI and web review.

---

## Package Architecture & Provider Model

All data access is abstracted behind a **provider interface** so that
every CLI command works identically whether talking directly to
PostgreSQL or going through the pgit server.

### Layer Diagram

```
┌─────────────────────────────────────────────┐
│  CLI commands / Web handlers / FUSE layer   │
│           (consumers — provider-agnostic)    │
└──────────────────┬──────────────────────────┘
                   │ provider.Provider interface
          ┌────────┴────────┐
          ▼                 ▼
   ┌─────────────┐   ┌──────────────┐
   │  direct.New  │   │  remote.New   │
   │  (pgx → PG)  │   │  (→ pgit srv) │
   └──────┬──────┘   └──────┬───────┘
          │                  │
          ▼                  ▼
   ┌───────────┐      ┌───────────┐
   │ PostgreSQL │      │ pgit      │
   │            │      │ server    │──→ PostgreSQL
   └───────────┘      └───────────┘
```

### Go Packages

```
pkg/
  sqlcat/          SQL query catalog — .sql files embedded at build time.
                   Pure queries, no connection logic. Used by both the
                   direct provider and the server.

  provider/        Provider interface definition.
    provider.go    Interface: Repo, CL, Author, Blob, Ref, …
    direct/        Implementation that calls sqlcat queries over pgx
                   directly against PostgreSQL.
    remote/        Implementation that calls the pgit server API.
                   Handles auth (token, OAuth), streaming, and
                   client-side cancellation.

  server/          pgit server (Go, net/http or gRPC).
    auth/          Authentication: DB-based username/password, OAuth
                   (OIDC), token validation for sub-authors.
    api/           Request handlers — thin wrappers around sqlcat
                   queries executed server-side.
    stream/        Streaming support for large results (blob content,
                   diff output, log traversal). Sends incremental
                   chunks so clients can begin processing immediately.
    cancel/        In-band cancellation. Unlike pgwire protocol 3,
                   cancellation is part of the request stream — the
                   client sends a cancel frame on the same connection,
                   and the server aborts the backing PG query via
                   context cancellation. No out-of-band secret or
                   separate socket needed.

internal/
  cli/             CLI commands — accept a provider.Provider, never
                   import pgx or server packages directly.
  db/              (existing) Lower-level PG helpers, gradually migrated
                   into sqlcat + direct provider.
```

### Key Design Rules

1. **sqlcat is the single source of SQL.** Every query lives in
   `pkg/sqlcat/` as an embedded `.sql` file. Both the direct provider
   and the server import sqlcat — no duplicated queries.
2. **Commands never choose the transport.** A command receives a
   `provider.Provider` from the CLI root. Configuration
   (`pgit config set provider remote --server https://...`) determines
   which implementation is injected.
3. **The server adds capabilities, not logic.** Auth, streaming, and
   cancellation live in the server layer. The actual data operations
   delegate to the same sqlcat queries the direct provider uses.
4. **In-band cancellation.** The remote provider sends a cancel frame
   on the open request stream. The server catches it and cancels the
   `context.Context` backing the PG query. This avoids the pgwire
   protocol 3 limitation where cancel requires an out-of-band
   connection with a shared secret.
5. **Streaming by default.** Large responses (blob reads, log
   traversals, diffs) are streamed. The provider interface exposes
   iterator-style methods (`Next() bool`) so consumers never buffer
   entire result sets.

---

## Data Model Sketch

All table names use singular form.

### Author & Permissions

```
author
  id              ULID PK
  parent_id       ULID NULL FK(author) -- NULL = root (system-provisioned)
  name            TEXT
  email           TEXT NULL            -- required for root authors
  kind            ENUM (human, llm, service)
  permissions     BIGINT NOT NULL      -- bitmask of granted permissions
  token_hash      BYTEA NULL           -- auth token (sub-authors typically have one)
  created_at      TIMESTAMPTZ
  deleted_at      TIMESTAMPTZ NULL     -- soft delete
```

**Root authors** (parent_id IS NULL) are provisioned by the system —
only admins can create them. Any author can create **sub-authors** under
themselves with equal or fewer permission bits. The sub-author's
`permissions` field stores the *requested* grant; the **effective
permissions** are computed at evaluation time:

```
effective = author.permissions
            & parent.permissions
            & grandparent.permissions
            & …                        -- walk to root
```

Because the AND is applied at evaluation time, revoking a bit on any
ancestor instantly restricts all descendants — no fan-out update needed.

**Delegation chain display.** When an action is attributed to a
sub-author, the full chain is rendered:

```
LLM-B (on behalf of LLM-A (on behalf of user-1))
```

**Typical sub-author patterns:**

| Use case | Kind | Permissions |
|---|---|---|
| FUSE CI checkout | service | `READ` |
| LLM auto-review | llm | `READ \| COMMENT` |
| LLM auto-fix | llm | `READ \| COMMENT \| CL_CREATE \| CL_UPDATE` |
| Human colleague delegation | human | same or subset of parent |

### Permission Bits (initial set)

```
READ            = 1 << 0
COMMENT         = 1 << 1
VOTE            = 1 << 2
CL_CREATE       = 1 << 3
CL_UPDATE       = 1 << 4
CL_SUBMIT       = 1 << 5
CI_TRIGGER      = 1 << 6
ADMIN           = 1 << 63
```

### Change Lists & Review

```
cl
  id              ULID PK
  author_id       ULID FK(author)
  title           TEXT
  description     TEXT
  status          ENUM (draft, active, submitted, abandoned)
  parent_cl_id    ULID NULL            -- for stacked CLs
  submitted_as    COMMIT_HASH NULL     -- set on submit
  created_at      TIMESTAMPTZ
  updated_at      TIMESTAMPTZ

patch_set
  id              ULID PK
  cl_id           ULID FK(cl)
  number          INT
  commit_hash     COMMIT_HASH          -- snapshot commit
  created_at      TIMESTAMPTZ

review_comment
  id              ULID PK
  cl_id           ULID FK(cl)
  patch_set       INT NULL             -- NULL = top-level
  path            TEXT NULL             -- NULL = top-level
  line            INT NULL
  author_id       ULID FK(author)      -- full chain via parent_id walk
  body            TEXT
  parent_id       ULID NULL FK(review_comment) -- threading
  created_at      TIMESTAMPTZ

review_vote
  cl_id           ULID FK(cl)
  author_id       ULID FK(author)
  score           INT                  -- e.g. -1, 0, +1, +2
  updated_at      TIMESTAMPTZ
  PK(cl_id, author_id)

ci_result
  id              ULID PK
  cl_id           ULID FK(cl)
  patch_set       INT
  job_name        TEXT
  status          ENUM (pending, running, passed, failed)
  log_blob_hash   TEXT NULL
  artifacts       JSONB
  triggered_by    ULID FK(author)      -- CI service sub-author
  created_at      TIMESTAMPTZ
  updated_at      TIMESTAMPTZ
```

---

## Testing Strategy

### Test Infrastructure: Shared Database Pool

A single pg-xpatch container is started on the **first test that
requests a database** and stays alive until all tests finish. Each test
gets an isolated, ephemeral database — no cross-test contamination.

```go
// internal/testdb/testdb.go

var pool struct {
    mu       sync.Mutex
    refCount int
    runtime  container.Runtime
    port     int
    idleT    *time.Timer   // 5-second idle shutdown
}

// Acquire returns a *db.DB connected to a fresh database.
// On first call, starts the container and waits for pg_isready.
// Increments refCount.
func Acquire(t testing.TB) *db.DB

// Release drops the database and decrements refCount.
// When refCount hits 0, starts the idle timer.
// If no Acquire within 5 seconds, stops the container.
// Called via t.Cleanup — never manually.
func Release(t testing.TB, d *db.DB)
```

Usage in every test:

```go
func TestSomething(t *testing.T) {
    d := testdb.Acquire(t) // container starts (or reuses), fresh DB created
    // ... test using d ...
    // t.Cleanup calls testdb.Release automatically
}
```

Database names are `test_<ulid>` — unique and collision-free even under
`-parallel`.

### Test Conventions

All tests follow these rules without exception:

1. **Table-driven.** Every test uses `[]struct{ name string; ... }` with
   named sub-tests via `t.Run(tt.name, ...)`.
2. **Golden files.** Any test that produces multi-line or structured
   output (diffs, log output, status, SQL results, CLI output) compares
   against `testdata/<test-name>.golden`. Run with `-update` flag to
   regenerate: `go test ./... -update`.
3. **No mocks for the database.** Tests hit real pg-xpatch. The merge
   package is the only unit that is pure-computation and needs no DB.
4. **Deterministic IDs.** Tests use `testdb.FixedULID(seq int)` to
   produce predictable ULIDs for golden file stability.

### Test Matrix

#### 1. `internal/util/` — Pure Functions (no DB)

- [ ] **T1: HashBytes** — BLAKE3 hash of known inputs matches expected hex strings.
- [ ] **T2: DetectBinary** — table of byte slices (UTF-8 text, null bytes, high-entropy binary, empty) → expected bool.
- [ ] **T3: IsBinaryFile** — table of temp files with known content → expected bool.
- [ ] **T4: RelativePath / AbsolutePath** — round-trip table: (repoRoot, input) → expected relative → back to absolute.
- [ ] **T5: FindRepoRoot** — table of directory layouts (has `.pgit/`, nested subdirs, no `.pgit/`) → expected root or error.
- [ ] **T6: ULID generation** — NewULID returns valid ULID; NewULIDWithTime encodes correct timestamp; ParseULID round-trips.
- [ ] **T7: ShortID** — table of full IDs → expected 8-char prefix.
- [ ] **T8: RelativeTime** — table of (now, then) pairs → expected human-readable string. Golden file for full output set.
- [ ] **T9: ToValidUTF8** — table of byte sequences (valid, invalid, mixed, empty) → expected sanitized string.
- [ ] **T10: ComputeTreeHash** — table of TreeEntry slices → expected deterministic hash. Verify order-independence.
- [ ] **T11: PgitError formatting** — build errors with WithMessage/WithCause/WithSuggestion chains → compare Format() against golden file.
- [ ] **T12: FileMode / IsSymlink / ReadSymlink** — table of temp files (regular, executable, symlink) → expected mode/bool/target.

#### 2. `internal/config/` — Configuration (no DB)

- [ ] **T13: Config Load/Save round-trip** — create Config, Save to temp dir, Load back, assert equal.
- [ ] **T14: Config GetValue/SetValue** — table of dotted keys (`user.name`, `remote.origin.url`, `container.port`) → set, get, verify.
- [ ] **T15: Config remote management** — SetRemote/GetRemote/RemoveRemote with table of (name, url) pairs.
- [ ] **T16: GlobalConfig Load/Save** — same round-trip pattern as local config.
- [ ] **T17: GlobalConfig SetValue type coercion** — table of (key, string value) → verify typed fields (int port, bool flags) parse correctly.
- [ ] **T18: Index Add/Delete/List** — table of staging operations → expected List() output after each step.
- [ ] **T19: Index Save/Load round-trip** — add entries, save to temp dir, load, assert identical.
- [ ] **T20: IgnorePatterns** — table of (pattern set, path, isDir) → expected IsIgnored bool. Cover: globs, negation, directory-only rules, nested `.pgitignore`.
- [ ] **T21: MergeState persistence** — AddConflict/RemoveConflict/HasConflicts, Save/Load round-trip, Clear deletes file.

#### 3. `internal/merge/` — Three-Way Merge (no DB)

- [ ] **T22: ThreeWay comprehensive** — extend existing tests into a single table-driven test. Each entry: `{name, base, local, remote, wantContent, wantConflict bool}`. Cover: identical edits, disjoint edits, overlapping conflicts, add/delete, empty files, trailing newlines. Golden file per case for conflict marker output.

#### 4. `internal/db/` — Database Layer (requires testdb)

##### Schema

- [ ] **T23: InitSchema creates all tables** — Acquire DB, InitSchema, query `pg_tables` and `pg_extension` → verify all pgit tables exist and pg_xpatch extension is loaded.
- [ ] **T24: InitSchema is idempotent** — call InitSchema twice, no error, same schema version.
- [ ] **T25: SchemaExists / GetSchemaVersion** — before init → (false, 0); after init → (true, current version).
- [ ] **T26: DropSchema removes everything** — init, drop, verify tables gone.
- [ ] **T27: Index management** — DropAllIndexes then CreateAllIndexes; verify via `pg_indexes` that expected indexes exist/absent.

##### Commits

- [ ] **T28: CreateCommit / GetCommit round-trip** — table of Commit structs with varying fields (with/without parent, different authors, messages with unicode) → create, get, assert equal.
- [ ] **T29: CreateCommitsBatch** — batch of 100 commits → CountCommits returns 100, GetAllCommits returns all in order.
- [ ] **T30: GetHeadCommit** — set HEAD ref, create commits, verify GetHeadCommit returns latest.
- [ ] **T31: GetCommitLog / GetCommitLogFrom** — create chain of 10 commits → GetCommitLog(5) returns 5 most recent; GetCommitLogFrom(mid, 3) returns 3 from midpoint.
- [ ] **T32: FindCommitByPartialID** — table of (full ID, prefix lengths 4–8) → all resolve to correct commit. Ambiguous prefix → error.
- [ ] **T33: DeleteCommits** — create 5, delete 2 by ID, verify remaining 3.
- [ ] **T34: GetCommitsBatch / GetCommitsBatchByRange** — create 10 commits, request subset by IDs → verify returned map keys and values.
- [ ] **T35: FindCommonAncestor** — build a diamond DAG (A→B, A→C, B→D, C→D) → FindCommonAncestor(B,C) returns A.

##### Paths & File Refs

- [ ] **T36: GetOrCreatePath** — table of paths → first call creates, second call returns same IDs. Verify with GetPathByPathID.
- [ ] **T37: GetOrCreatePathsBatch** — batch of 50 paths → all registered; re-register → same IDs.
- [ ] **T38: CreateFileRef / GetFileRefsAtCommit** — create commit with 5 file refs → GetFileRefsAtCommit returns all 5 with correct fields.
- [ ] **T39: GetChangedFileRefs** — commit A has files {a,b,c}, commit B has {a,b',d} → changed = {b(modified), c(deleted), d(added)}.
- [ ] **T40: GetFileRefHistory** — file modified across 5 commits → history returns all 5 refs in order.
- [ ] **T41: GetTreeRefsAtCommit** — verify tree reconstruction includes latest version of each file at that commit, not just files changed in that commit.

##### Content (Delta-Compressed)

- [ ] **T42: CreateContent / GetContent round-trip** — table of (groupID, versionID, data bytes, isBinary) → store, retrieve, assert equal. Cover: text, binary, empty, large (1MB).
- [ ] **T43: Delta compression is transparent** — store 10 versions of a file with incremental edits in same group → each GetContent returns exact original bytes.
- [ ] **T44: GetContentsBatch** — store 20 content items across 5 groups → batch-fetch 10 of them → verify all correct.
- [ ] **T45: GetAllContentForGroup** — store 5 versions in a group → returns all 5 in version order with correct data.

##### Blobs (High-Level Content Access)

- [ ] **T46: CreateBlob / GetBlob round-trip** — table of Blob structs (text files, binary files, symlinks, deleted markers) → create, get, assert equal.
- [ ] **T47: GetBlobsAtCommit / GetTreeAtCommit** — build a 3-commit history with file additions and modifications → verify tree at each commit reflects cumulative state.
- [ ] **T48: GetFileHistory** — file modified in 5 commits → returns 5 blobs in chronological order.
- [ ] **T49: GetChangedFiles** — two commits → returns only files that differ.
- [ ] **T50: SearchContent** — insert files with known content → table of (pattern, opts) → expected matching paths/lines. Golden file for formatted results.

##### Refs

- [ ] **T51: SetRef / GetRef / DeleteRef** — table of ref names (HEAD, refs/heads/main, refs/tags/v1) → CRUD round-trip.
- [ ] **T52: GetAllRefs** — create 5 refs → GetAllRefs returns all.
- [ ] **T53: SetHead / GetHead** — set to commit A, verify, set to commit B, verify.

##### Commit Graph

- [ ] **T54: CreateCommitGraphBatch / GetCommitGraphByID** — build linear graph of 10 entries → each retrievable by ID with correct parent/seq.
- [ ] **T55: GetAncestorID** — linear chain of 10 → GetAncestorID(tip, 3) returns correct ancestor.
- [ ] **T56: FindCommitByPartialIDInGraph** — partial ID resolution through the graph table.

##### Stats & Metadata

- [ ] **T57: GetRepoStatsFast** — after importing test data, verify counts (commits, paths, blobs) are non-zero and consistent.
- [ ] **T58: Metadata CRUD** — table of (key, value) pairs → Set, Get, Delete, Get-after-delete returns empty.
- [ ] **T59: SyncState CRUD** — Set/Get/Delete/GetAll for multiple remotes.

#### 5. `internal/repo/` — Repository Operations (requires testdb + temp filesystem)

##### Initialization & Discovery

- [ ] **T60: Init creates .pgit directory** — Init in temp dir → `.pgit/config.toml` exists, schema initialized in DB.
- [ ] **T61: Open / OpenAt** — Init, then Open from within repo dir; OpenAt from outside → both succeed and point to same root.
- [ ] **T62: Open outside repo** — Open from `/tmp` with no `.pgit` → returns NotARepoError.

##### Staging

- [ ] **T63: StageFile / GetStagedChanges** — table of file operations (create, modify, delete) → stage each → GetStagedChanges returns expected entries with correct status.
- [ ] **T64: UnstageFile / UnstageAll** — stage 3 files, unstage 1 → verify 2 remain; unstage all → verify empty.
- [ ] **T65: StageAll** — create/modify/delete several files → StageAll → verify all detected with correct statuses.
- [ ] **T66: StageDelete** — stage a deletion for an existing tracked file → verify appears as deleted in staged changes.

##### Commits

- [ ] **T67: Commit creates commit and clears index** — stage files, Commit with message/author → verify commit in DB, HEAD updated, index empty.
- [ ] **T68: Commit with no staged changes** — returns error.
- [ ] **T69: Sequential commits** — 3 commits with file changes → log shows all 3, tree at each commit is correct.

##### Working Tree Changes

- [ ] **T70: GetWorkingTreeChanges** — table of filesystem states (untracked, modified, deleted) vs last commit → expected FileChange list.
- [ ] **T71: GetUnstagedChanges** — modify files after staging → GetUnstagedChanges returns only the post-stage modifications.

##### Diffs

- [ ] **T72: Diff working tree** — modify a file → Diff(staged=false) → golden file comparison of hunks.
- [ ] **T73: Diff staged** — stage a modification → Diff(staged=true) → golden file.
- [ ] **T74: Diff between commits** — two commits → Diff(commitA..commitB) → golden file.
- [ ] **T75: GenerateHunks** — table of (oldContent, newContent, contextLines) → expected hunk count, line ranges. Golden files for formatted output.
- [ ] **T76: FormatDiff** — DiffResult with known hunks → golden file for colored and no-color output.

#### 6. `internal/container/` — Container Runtime (integration, real Docker)

- [ ] **T77: DetectRuntime** — verify returns Docker or Podman (skip if neither available).
- [ ] **T78: StartContainer / IsContainerRunning / StopContainer** — full lifecycle. Verify WaitForPostgres succeeds after start.
- [ ] **T79: EnsureDatabase / DropDatabase** — create DB, connect, drop, verify gone.
- [ ] **T80: LocalConnectionURL** — table of (port, dbname) → expected URL format.
- [ ] **T81: FindAvailablePort** — verify returned port is actually available (net.Listen test).
- [ ] **T82: IsPortAvailable** — table of (listen on port then check → false; free port → true).

#### 7. `internal/cli/` — CLI End-to-End (requires testdb + temp filesystem)

These tests invoke cobra commands directly via `cmd.ExecuteContext()`
with captured stdout/stderr. All output compared against golden files.

##### Repository Lifecycle

- [ ] **T83: `pgit init`** — init in temp dir → golden output, `.pgit/` created.
- [ ] **T84: `pgit init` in existing repo** — error message matches golden.
- [ ] **T85: `pgit status` clean** — init, no changes → golden "nothing to commit" output.
- [ ] **T86: `pgit status` with changes** — create/modify/delete files → golden output showing all categories. Test both long and `--short` format.
- [ ] **T87: `pgit status --json`** — same scenario → golden JSON output.

##### Add / Rm / Mv

- [ ] **T88: `pgit add <file>`** — create file, add → status shows staged. Golden output.
- [ ] **T89: `pgit add --all`** — multiple changes → all staged.
- [ ] **T90: `pgit rm <file>`** — tracked file → removed from working tree and staged for deletion.
- [ ] **T91: `pgit rm --cached`** — unstage without deleting working tree file.
- [ ] **T92: `pgit mv <src> <dst>`** — rename → old path deleted, new path added in staging.

##### Commit & Log

- [ ] **T93: `pgit commit -m "msg"`** — stage files, commit → golden output with commit hash.
- [ ] **T94: `pgit log --oneline`** — 3 commits → golden 3-line output.
- [ ] **T95: `pgit log --json`** — same → golden JSON array.
- [ ] **T96: `pgit log --max-count 2`** — 5 commits → only 2 shown.

##### Show & Diff

- [ ] **T97: `pgit show <commit>`** — golden output with commit metadata and diff.
- [ ] **T98: `pgit show <commit>:<path>`** — golden file content output.
- [ ] **T99: `pgit diff`** — working tree changes → golden diff output.
- [ ] **T100: `pgit diff --staged`** — staged changes → golden.
- [ ] **T101: `pgit diff <A>..<B>`** — between two commits → golden.
- [ ] **T102: `pgit diff --stat`** — golden stat summary.
- [ ] **T103: `pgit blame <file>`** — golden annotated output.

##### Checkout

- [ ] **T104: `pgit checkout -- <file>`** — modify file, checkout → file restored to HEAD content.
- [ ] **T105: `pgit checkout <commit> -- <file>`** — restore file to content at specific commit.
- [ ] **T106: `pgit checkout <commit>`** — full tree checkout → all files match that commit's tree.

##### Search & SQL

- [ ] **T107: `pgit search <pattern>`** — insert files with known content → golden search results.
- [ ] **T108: `pgit search --ignore-case`** — case-insensitive match → golden.
- [ ] **T109: `pgit sql "SELECT count(*) FROM pgit_commits"`** — golden output after known number of commits.
- [ ] **T110: `pgit sql tables`** — golden list of all pgit tables.
- [ ] **T111: `pgit sql schema`** — golden schema documentation output.

##### Config & Doctor

- [ ] **T112: `pgit config user.name`** — get/set round-trip → golden.
- [ ] **T113: `pgit config --list`** — golden config listing.
- [ ] **T114: `pgit doctor`** — in healthy repo → golden "all checks passed" output.

##### Analyze

- [ ] **T115: `pgit analyze churn`** — after importing test history → golden churn table.
- [ ] **T116: `pgit analyze authors`** — golden author stats.
- [ ] **T117: `pgit analyze coupling`** — golden coupling pairs.
- [ ] **T118: `pgit analyze hotspots`** — golden directory aggregation.
- [ ] **T119: `pgit analyze activity`** — golden time-series output.
- [ ] **T120: `pgit analyze bus-factor`** — golden bus-factor table.

##### Remote Operations

- [ ] **T121: `pgit remote add/remove/set-url`** — table of operations → verify config state after each.
- [ ] **T122: `pgit push` / `pgit pull`** — two repos with shared DB, push from A, pull from B → verify B has A's commits. Golden output for both commands.
- [ ] **T123: `pgit clone`** — clone from remote URL → repo initialized with full history.

##### Error Handling

- [ ] **T124: Missing argument errors** — table of commands with missing required args → each produces expected PgitError. Golden error output.
- [ ] **T125: Not-a-repo errors** — run repo-requiring commands outside a repo → golden error.
- [ ] **T126: Connection errors** — run commands with unreachable DB → golden error with suggestions.

#### 8. Golden File Management

```
testdata/
  util/
    relative_time.golden
    pgit_error_format.golden
  config/
    ignore_patterns.golden
  merge/
    conflict_markers_overlapping.golden
    conflict_markers_add_delete.golden
    ...
  db/
    search_results.golden
  repo/
    diff_working_tree.golden
    diff_staged.golden
    diff_between_commits.golden
    hunk_format.golden
    ...
  cli/
    init_output.golden
    init_existing_error.golden
    status_clean.golden
    status_changes.golden
    status_short.golden
    status_json.golden
    log_oneline.golden
    log_json.golden
    show_commit.golden
    show_file.golden
    diff_output.golden
    diff_staged.golden
    diff_stat.golden
    blame_output.golden
    search_results.golden
    sql_tables.golden
    sql_schema.golden
    doctor_healthy.golden
    analyze_churn.golden
    analyze_authors.golden
    analyze_coupling.golden
    analyze_hotspots.golden
    analyze_activity.golden
    analyze_bus_factor.golden
    error_missing_arg.golden
    error_not_a_repo.golden
    error_connection.golden
    ...
```

Update golden files:

```bash
go test ./... -update          # regenerate all golden files
git diff testdata/             # review changes
```

---

## Implementation Phases

### Phase 1 — Provider Abstraction & sqlcat
- Extract all SQL into `pkg/sqlcat/` as embedded `.sql` files.
- Define `provider.Provider` interface.
- Implement `provider/direct` (pgx, current behavior).
- Refactor CLI commands to accept a `Provider` instead of a `*pgx.Conn`.

### Phase 2 — Authors, Change Lists & Review CLI
- Author table with hierarchical sub-authors and bitwise permission model.
- Sub-author creation via CLI (`pgit author create --parent <id> --permissions read,comment`).
- Token-based auth for sub-authors.
- CL creation, update, listing, and submission (cherry-pick strategy).
- Patch set tracking.
- Inline commenting and voting via CLI.
- Stacked CL support.

### Phase 3 — pgit Server & Remote Provider
- `pgit server` command (Go, net/http or gRPC).
- Auth layer: DB username/password, OAuth/OIDC, sub-author token validation.
- Streaming responses for blobs, diffs, and log traversal.
- In-band cancellation (cancel frame on request stream → context cancellation).
- Implement `provider/remote` client.

### Phase 4 — FUSE Provider
- Read-only FUSE mount of any commit or CL.
- Lazy blob fetching via provider (works with both direct and remote).
- `pgit mount` / `pgit unmount` commands.
- CI asset cache layer.

### Phase 5 — Server-Side User Refs & CI Integration
- Per-user ref namespaces.
- CI status and artifact metadata on CLs.

### Phase 6 — Web Interface
- CL dashboard and diff viewer.
- Inline commenting and voting.
- Stack visualization.
- CI log and artifact viewer.

### Phase 7 — AI Review Tooling
- LLM agent harness that wraps the standard CLI with
  `PGIT_ACCOUNT=<llm-sub-author>`.
- Default review prompt and integration hooks for common LLM providers.
- Inline AI comments with attribution (no special rendering — same as
  any sub-author).
