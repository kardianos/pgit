package util

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// T11: PgitError formatting
func TestPgitErrorError(t *testing.T) {
	tests := []struct {
		name  string
		err   *PgitError
		want  string
	}{
		{
			name: "simple title",
			err:  NewError("something broke"),
			want: "something broke",
		},
		{
			name: "with wrapped error",
			err:  NewError("connection failed").Wrap(fmt.Errorf("dial tcp: timeout")),
			want: "connection failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPgitErrorUnwrap(t *testing.T) {
	inner := fmt.Errorf("root cause")
	err := NewError("outer").Wrap(inner)

	if !errors.Is(err, inner) {
		t.Error("Unwrap() should allow errors.Is to find wrapped error")
	}

	noWrap := NewError("standalone")
	if noWrap.Unwrap() != nil {
		t.Error("Unwrap() on error with no wrap should return nil")
	}
}

func TestPgitErrorFormat(t *testing.T) {
	tests := []struct {
		name string
		err  *PgitError
		// We check substrings rather than exact match since Format() output is multi-line
		wantContains []string
	}{
		{
			name:         "title only",
			err:          NewError("simple error"),
			wantContains: []string{"Error: simple error"},
		},
		{
			name: "with message and context",
			err: NewError("not a repo").
				WithMessage("No .pgit directory found").
				WithContext("while running pgit log"),
			wantContains: []string{
				"Error: not a repo",
				"No .pgit directory found",
				"while running pgit log",
			},
		},
		{
			name: "with causes",
			err: NewError("connection failed").
				WithCauses("server down", "bad credentials"),
			wantContains: []string{
				"Possible causes:",
				"server down",
				"bad credentials",
			},
		},
		{
			name: "with suggestions",
			err: NewError("container not running").
				WithSuggestions("pgit local start", "pgit doctor"),
			wantContains: []string{
				"Try:",
				"$ pgit local start",
				"$ pgit doctor",
			},
		},
		{
			name: "full error with everything",
			err: NewError("database error").
				WithMessage("Cannot connect to PostgreSQL").
				WithContext("postgres://localhost:5433/mydb").
				WithCause("server is not running").
				WithCause("wrong port").
				WithSuggestion("pgit local start").
				WithSuggestion("pgit local status"),
			wantContains: []string{
				"Error: database error",
				"Cannot connect to PostgreSQL",
				"postgres://localhost:5433/mydb",
				"Possible causes:",
				"server is not running",
				"wrong port",
				"Try:",
				"$ pgit local start",
				"$ pgit local status",
			},
		},
	}

	var golden strings.Builder
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Format()
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("Format() missing %q in:\n%s", want, got)
				}
			}
		})
		fmt.Fprintf(&golden, "=== %s ===\n%s\n", tt.name, tt.err.Format())
	}

	updateGolden(t, "pgit_error_format", []byte(golden.String()))
}

func TestPgitErrorBuilderChaining(t *testing.T) {
	err := NewError("test").
		WithMessage("msg").
		WithContext("ctx").
		WithCause("c1").
		WithCauses("c2", "c3").
		WithSuggestion("s1").
		WithSuggestions("s2", "s3").
		Wrap(fmt.Errorf("inner"))

	if err.Title != "test" {
		t.Errorf("Title = %q", err.Title)
	}
	if err.Message != "msg" {
		t.Errorf("Message = %q", err.Message)
	}
	if err.Context != "ctx" {
		t.Errorf("Context = %q", err.Context)
	}
	if len(err.Causes) != 3 {
		t.Errorf("Causes count = %d, want 3", len(err.Causes))
	}
	if len(err.Suggestions) != 3 {
		t.Errorf("Suggestions count = %d, want 3", len(err.Suggestions))
	}
	if err.Err == nil {
		t.Error("wrapped error is nil")
	}
}

// Test pre-built error constructors verify constructor-specific content.
func TestPrebuiltErrors(t *testing.T) {
	tests := []struct {
		name         string
		err          *PgitError
		wantContains []string // strings that MUST appear in Format() output
	}{
		{
			name:         "NotARepoError",
			err:          NotARepoError(),
			wantContains: []string{"Not a pgit repository", "pgit init"},
		},
		{
			name:         "NoContainerError",
			err:          NoContainerError(),
			wantContains: []string{"No container runtime", "Docker", "Podman"},
		},
		{
			name:         "ContainerNotRunningError",
			err:          ContainerNotRunningError(),
			wantContains: []string{"not running", "pgit local start"},
		},
		{
			name:         "DatabaseConnectionError",
			err:          DatabaseConnectionError("postgres://localhost:5433/test", fmt.Errorf("dial refused")),
			wantContains: []string{"postgres://localhost:5433/test", "Cannot connect"},
		},
		{
			name:         "RemoteNotFoundError",
			err:          RemoteNotFoundError("origin"),
			wantContains: []string{"origin"},
		},
		{
			name:         "CommitNotFoundError",
			err:          CommitNotFoundError("abc1234"),
			wantContains: []string{"abc1234"},
		},
		{
			name:         "MissingArgumentError",
			err:          MissingArgumentError("path", "pgit add <path>"),
			wantContains: []string{"path", "pgit add <path>"},
		},
		{
			name:         "TooManyArgumentsError",
			err:          TooManyArgumentsError(1, 3),
			wantContains: []string{"1", "3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() == "" {
				t.Fatal("Error() should not be empty")
			}
			formatted := tt.err.Format()
			for _, want := range tt.wantContains {
				if !strings.Contains(formatted, want) {
					t.Errorf("Format() missing %q in:\n%s", want, formatted)
				}
			}
		})
	}
}

// Test sentinel errors
func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{name: "ErrNotARepository", err: ErrNotARepository, msg: "not a pgit repository"},
		{name: "ErrAlreadyInitialized", err: ErrAlreadyInitialized, msg: "already exists"},
		{name: "ErrNoContainerRuntime", err: ErrNoContainerRuntime, msg: "no container runtime"},
		{name: "ErrNoCommits", err: ErrNoCommits, msg: "no commits"},
		{name: "ErrNothingToCommit", err: ErrNothingToCommit, msg: "nothing to commit"},
		{name: "ErrNothingStaged", err: ErrNothingStaged, msg: "nothing staged"},
		{name: "ErrMergeConflict", err: ErrMergeConflict, msg: "merge conflict"},
		{name: "ErrNotConnected", err: ErrNotConnected, msg: "not connected"},
		{name: "ErrInvalidCommitID", err: ErrInvalidCommitID, msg: "invalid commit"},
		{name: "ErrCommitNotFound", err: ErrCommitNotFound, msg: "commit not found"},
		{name: "ErrFileNotFound", err: ErrFileNotFound, msg: "file not found"},
		{name: "ErrPathNotInRepo", err: ErrPathNotInRepo, msg: "outside repository"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.err.Error(), tt.msg) {
				t.Errorf("%s.Error() = %q, want to contain %q", tt.name, tt.err.Error(), tt.msg)
			}
		})
	}
}
