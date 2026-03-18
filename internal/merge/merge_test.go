package merge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper to build file content from lines
func lines(ls ...string) []byte {
	return []byte(strings.Join(ls, "\n") + "\n")
}

// helper to build file content from lines without trailing newline
func linesNoTrail(ls ...string) []byte {
	return []byte(strings.Join(ls, "\n"))
}

// T22: Consolidated table-driven three-way merge tests
func TestThreeWayMerge(t *testing.T) {
	tests := []struct {
		name             string
		base             []byte
		local            []byte
		remote           []byte
		remoteName       string
		wantConflicts    bool
		wantNumConflicts int    // only checked when wantConflicts == true
		wantAutoResolved int    // -1 means don't check; 0 means check for exactly 0
		wantContent      []byte // nil means don't check exact content
		wantContains     []string
		wantNotContains  []string
		goldenFile       string // if set, compare content against testdata/<file>
	}{
		{
			name:             "no changes",
			base:             lines("line1", "line2", "line3"),
			local:            lines("line1", "line2", "line3"),
			remote:           lines("line1", "line2", "line3"),
			remoteName:       "origin",
			wantConflicts:    false,
			wantAutoResolved: 0, // no changes means no auto-resolutions
			wantContent:      lines("line1", "line2", "line3"),
		},
		{
			name:             "only local changed",
			base:             lines("line1", "line2", "line3"),
			local:            lines("line1", "MODIFIED", "line3"),
			remote:           lines("line1", "line2", "line3"),
			remoteName:       "origin",
			wantConflicts:    false,
			wantAutoResolved: 1,
			wantContent:      lines("line1", "MODIFIED", "line3"),
		},
		{
			name:             "only remote changed",
			base:             lines("line1", "line2", "line3"),
			local:            lines("line1", "line2", "line3"),
			remote:           lines("line1", "REMOTE", "line3"),
			remoteName:       "origin",
			wantConflicts:    false,
			wantAutoResolved: 1,
			wantContent:      lines("line1", "REMOTE", "line3"),
		},
		{
			name:             "both changed different regions",
			base:             lines("aaa", "bbb", "ccc", "ddd", "eee"),
			local:            lines("LOCAL", "bbb", "ccc", "ddd", "eee"),
			remote:           lines("aaa", "bbb", "ccc", "ddd", "REMOTE"),
			remoteName:       "origin",
			wantConflicts:    false,
			wantAutoResolved: 2,
			wantContent:      lines("LOCAL", "bbb", "ccc", "ddd", "REMOTE"),
		},
		{
			name:             "both changed same line - conflict",
			base:             lines("line1", "line2", "line3"),
			local:            lines("line1", "LOCAL EDIT", "line3"),
			remote:           lines("line1", "REMOTE EDIT", "line3"),
			remoteName:       "origin",
			wantConflicts:    true,
			wantNumConflicts: 1,
			wantAutoResolved: 0,
			wantContains: []string{
				"<<<<<<< LOCAL", "=======", ">>>>>>> REMOTE (origin)",
				"LOCAL EDIT", "REMOTE EDIT", "line1", "line3",
			},
		},
		{
			name:             "both changed identically",
			base:             lines("line1", "line2", "line3"),
			local:            lines("line1", "SAME EDIT", "line3"),
			remote:           lines("line1", "SAME EDIT", "line3"),
			remoteName:       "origin",
			wantConflicts:    false,
			wantAutoResolved: -1,
			wantContent:      lines("line1", "SAME EDIT", "line3"),
		},
		{
			name:             "local adds lines",
			base:             lines("line1", "line3"),
			local:            lines("line1", "NEW LINE", "line3"),
			remote:           lines("line1", "line3"),
			remoteName:       "origin",
			wantConflicts:    false,
			wantAutoResolved: -1,
			wantContent:      lines("line1", "NEW LINE", "line3"),
		},
		{
			name:             "remote deletes lines",
			base:             lines("line1", "TO DELETE", "line3"),
			local:            lines("line1", "TO DELETE", "line3"),
			remote:           lines("line1", "line3"),
			remoteName:       "origin",
			wantConflicts:    false,
			wantAutoResolved: -1,
			wantContent:      lines("line1", "line3"),
		},
		{
			name:             "conflict only affects conflicting region",
			base:             lines("top1", "top2", "top3", "middle", "bot1", "bot2", "bot3"),
			local:            lines("top1", "top2", "top3", "LOCAL MIDDLE", "bot1", "bot2", "bot3"),
			remote:           lines("top1", "top2", "top3", "REMOTE MIDDLE", "bot1", "bot2", "bot3"),
			remoteName:       "origin",
			wantConflicts:    true,
			wantNumConflicts: 1,
			wantAutoResolved: -1,
			goldenFile:       "conflict_middle_region.txt",
		},
		{
			name:             "multiple non-overlapping edits",
			base:             lines("header", "section-a", "divider", "section-b", "footer"),
			local:            lines("header", "LOCAL-A", "divider", "section-b", "footer"),
			remote:           lines("header", "section-a", "divider", "REMOTE-B", "footer"),
			remoteName:       "origin",
			wantConflicts:    false,
			wantAutoResolved: 2,
			wantContent:      lines("header", "LOCAL-A", "divider", "REMOTE-B", "footer"),
		},
		{
			name:             "conflict with adjacent clean edit",
			base:             lines("a", "b", "c", "d", "e"),
			local:            lines("a", "LOCAL-B", "c", "d", "e"),
			remote:           lines("a", "REMOTE-B", "c", "REMOTE-D", "e"),
			remoteName:       "origin",
			wantConflicts:    true,
			wantNumConflicts: 1,
			wantContains:     []string{"REMOTE-D", "LOCAL-B", "REMOTE-B", "<<<<<<< LOCAL"},
			wantNotContains:  []string{}, // REMOTE-D should be outside conflict markers (cleanly merged)
		},
		{
			name:             "empty base both add - conflict",
			base:             []byte{},
			local:            lines("new local content"),
			remote:           lines("new remote content"),
			remoteName:       "origin",
			wantConflicts:    true,
			wantNumConflicts: 1,
			wantAutoResolved: -1,
			wantContains:     []string{"new local content", "new remote content", "<<<<<<< LOCAL"},
		},
		{
			name:             "empty base only local adds",
			base:             []byte{},
			local:            lines("new content"),
			remote:           []byte{},
			remoteName:       "origin",
			wantConflicts:    false,
			wantAutoResolved: -1,
			wantContent:      lines("new content"),
		},
		{
			name:             "local deletes all",
			base:             lines("line1", "line2", "line3"),
			local:            []byte{},
			remote:           lines("line1", "line2", "line3"),
			remoteName:       "origin",
			wantConflicts:    false,
			wantAutoResolved: -1,
			wantContent:      []byte{},
		},
		{
			name:             "multi-line edit",
			base:             lines("a", "b", "c", "d", "e"),
			local:            lines("a", "X", "Y", "Z", "e"),
			remote:           lines("a", "b", "c", "d", "e"),
			remoteName:       "origin",
			wantConflicts:    false,
			wantAutoResolved: -1,
			wantContent:      lines("a", "X", "Y", "Z", "e"),
		},
		{
			name:             "real-world readme both edit same location",
			base:             lines("# Tokio", "", "A runtime for writing reliable, asynchronous, and slim applications with", "the Rust programming language."),
			local:            lines("# Tokio", "", "Edited by User B.", "", "A runtime for writing reliable, asynchronous, and slim applications with", "the Rust programming language."),
			remote:           lines("# Tokio", "", "Edited by User A.", "", "A runtime for writing reliable, asynchronous, and slim applications with", "the Rust programming language."),
			remoteName:       "origin",
			wantConflicts:    true,
			wantAutoResolved: -1,
			wantContains:     []string{"<<<<<<< LOCAL", "Edited by User B.", "Edited by User A."},
		},
		{
			name:             "real-world different sections auto-merge",
			base:             lines("# Project", "", "Description here.", "", "## Installation", "", "Run: npm install", "", "## Usage", "", "Run: npm start"),
			local:            lines("# Project", "", "Description here.", "", "## Installation", "", "Run: npm install", "", "## Usage", "", "Run: npm run dev"),
			remote:           lines("# My Project", "", "Description here.", "", "## Installation", "", "Run: npm install", "", "## Usage", "", "Run: npm start"),
			remoteName:       "origin",
			wantConflicts:    false,
			wantAutoResolved: 2,
			wantContent:      lines("# My Project", "", "Description here.", "", "## Installation", "", "Run: npm install", "", "## Usage", "", "Run: npm run dev"),
		},
		{
			name:             "no trailing newline preserved",
			base:             linesNoTrail("line1", "line2"),
			local:            linesNoTrail("line1", "LOCAL"),
			remote:           linesNoTrail("line1", "line2"),
			remoteName:       "origin",
			wantConflicts:    false,
			wantAutoResolved: -1,
			wantContent:      linesNoTrail("line1", "LOCAL"),
		},
		{
			name:             "both add lines at end - conflict",
			base:             lines("line1", "line2"),
			local:            lines("line1", "line2", "local-added"),
			remote:           lines("line1", "line2", "remote-added"),
			remoteName:       "origin",
			wantConflicts:    true,
			wantAutoResolved: -1,
			wantContains:     []string{"local-added", "remote-added"},
		},
		{
			name:             "both add identical lines at end",
			base:             lines("line1", "line2"),
			local:            lines("line1", "line2", "same-line"),
			remote:           lines("line1", "line2", "same-line"),
			wantAutoResolved: -1,
			remoteName:    "origin",
			wantConflicts: false,
			wantContent:   lines("line1", "line2", "same-line"),
		},
	}

	// Ensure testdata directory exists for golden files
	if err := os.MkdirAll("testdata", 0755); err != nil {
		t.Fatalf("creating testdata dir: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ThreeWay(tt.base, tt.local, tt.remote, tt.remoteName)

			if result.HasConflicts != tt.wantConflicts {
				t.Fatalf("HasConflicts = %v, want %v", result.HasConflicts, tt.wantConflicts)
			}

			if tt.wantConflicts && tt.wantNumConflicts > 0 {
				if len(result.Conflicts) != tt.wantNumConflicts {
					t.Errorf("got %d conflicts, want %d", len(result.Conflicts), tt.wantNumConflicts)
				}
			}

			if tt.wantAutoResolved >= 0 {
				if result.AutoResolved != tt.wantAutoResolved {
					t.Errorf("AutoResolved = %d, want %d", result.AutoResolved, tt.wantAutoResolved)
				}
			}

			if tt.wantContent != nil {
				if string(result.Content) != string(tt.wantContent) {
					t.Errorf("content mismatch:\n  got:  %q\n  want: %q", string(result.Content), string(tt.wantContent))
				}
			}

			content := string(result.Content)
			for _, s := range tt.wantContains {
				if !strings.Contains(content, s) {
					t.Errorf("content missing %q\n  got: %s", s, content)
				}
			}

			for _, s := range tt.wantNotContains {
				if strings.Contains(content, s) {
					t.Errorf("content should not contain %q\n  got: %s", s, content)
				}
			}

			// Verify conflict marker ordering: LOCAL content between <<<<<<< and =======,
			// REMOTE content between ======= and >>>>>>>.
			if tt.wantConflicts {
				localMarker := strings.Index(content, "<<<<<<< LOCAL")
				separator := strings.Index(content, "=======")
				remoteMarker := strings.Index(content, ">>>>>>>")
				if localMarker >= 0 && separator >= 0 && remoteMarker >= 0 {
					if !(localMarker < separator && separator < remoteMarker) {
						t.Errorf("conflict markers out of order: <<<<<<< at %d, ======= at %d, >>>>>>> at %d",
							localMarker, separator, remoteMarker)
					}
				}
			}

			// Golden file comparison
			if tt.goldenFile != "" {
				goldenPath := filepath.Join("testdata", tt.goldenFile)
				if _, err := os.Stat(goldenPath); os.IsNotExist(err) {
					// Write golden file on first run
					if err := os.WriteFile(goldenPath, result.Content, 0644); err != nil {
						t.Fatalf("writing golden file: %v", err)
					}
					t.Logf("wrote golden file %s", goldenPath)
				} else {
					golden, err := os.ReadFile(goldenPath)
					if err != nil {
						t.Fatalf("reading golden file: %v", err)
					}
					if string(result.Content) != string(golden) {
						t.Errorf("content does not match golden file %s:\n  got:\n%s\n  want:\n%s",
							tt.goldenFile, string(result.Content), string(golden))
					}
				}
			}
		})
	}
}

func TestCountLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{name: "empty", input: "", want: 0},
		{name: "one line with newline", input: "foo\n", want: 1},
		{name: "two lines with newline", input: "foo\nbar\n", want: 2},
		{name: "one line no newline", input: "foo", want: 1},
		{name: "two lines no newline", input: "foo\nbar", want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countLines(tt.input)
			if got != tt.want {
				t.Errorf("countLines(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "empty", input: "", want: []string{}},
		{name: "two lines with newline", input: "a\nb\n", want: []string{"a", "b"}},
		{name: "two lines no newline", input: "a\nb", want: []string{"a", "b"}},
		{name: "single word", input: "single", want: []string{"single"}},
		{name: "single word with newline", input: "single\n", want: []string{"single"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("splitLines(%q): len=%d, want len=%d\n  got:  %v\n  want: %v",
					tt.input, len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitLines(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}
