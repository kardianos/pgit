package repo

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// testdataDir returns the absolute path to the testdata directory.
func testdataDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "testdata")
}

// readGolden reads a golden file from testdata.
func readGolden(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(testdataDir(), name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file %q: %v", name, err)
	}
	return string(data)
}

// T72: FormatDiff new file golden
func TestFormatDiffNewFileGolden(t *testing.T) {
	tests := []struct {
		name       string
		result     DiffResult
		goldenFile string
	}{
		{
			name: "new file",
			result: DiffResult{
				Path:       "hello.txt",
				Status:     StatusNew,
				OldContent: "",
				NewContent: "line one\nline two\nline three\n",
				Hunks: GenerateHunks("", "line one\nline two\nline three\n", 3),
			},
			goldenFile: "format_diff_new_file.golden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDiff(tt.result, true)
			want := readGolden(t, tt.goldenFile)
			if got != want {
				t.Errorf("FormatDiff output mismatch.\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

// T73: FormatDiff modified file golden
func TestFormatDiffModifiedGolden(t *testing.T) {
	tests := []struct {
		name       string
		result     DiffResult
		goldenFile string
	}{
		{
			name: "modified file",
			result: DiffResult{
				Path:       "file.txt",
				Status:     StatusModified,
				OldContent: "line one\nline two\nline three\n",
				NewContent: "line one\nline TWO\nline three\n",
				Hunks: GenerateHunks(
					"line one\nline two\nline three\n",
					"line one\nline TWO\nline three\n",
					3,
				),
			},
			goldenFile: "format_diff_modified.golden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDiff(tt.result, true)
			want := readGolden(t, tt.goldenFile)
			if got != want {
				t.Errorf("FormatDiff output mismatch.\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

// T74: FormatDiff deleted file golden
func TestFormatDiffDeletedGolden(t *testing.T) {
	tests := []struct {
		name       string
		result     DiffResult
		goldenFile string
	}{
		{
			name: "deleted file",
			result: DiffResult{
				Path:       "gone.txt",
				Status:     StatusDeleted,
				OldContent: "goodbye\nworld\n",
				NewContent: "",
				Hunks:      GenerateHunks("goodbye\nworld\n", "", 3),
			},
			goldenFile: "format_diff_deleted.golden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDiff(tt.result, true)
			want := readGolden(t, tt.goldenFile)
			if got != want {
				t.Errorf("FormatDiff output mismatch.\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

// T75: GenerateHunks
func TestGenerateHunks(t *testing.T) {
	tests := []struct {
		name       string
		old        string
		new        string
		context    int
		wantHunks  int
		wantAdds   int
		wantDels   int
	}{
		{
			name:      "identical content produces no hunks",
			old:       "same\n",
			new:       "same\n",
			context:   3,
			wantHunks: 0,
		},
		{
			name:      "all new content",
			old:       "",
			new:       "a\nb\nc\n",
			context:   3,
			wantHunks: 1,
			wantAdds:  3,
			wantDels:  0,
		},
		{
			name:      "all deleted content",
			old:       "a\nb\n",
			new:       "",
			context:   3,
			wantHunks: 1,
			wantAdds:  0,
			wantDels:  2,
		},
		{
			name:      "single line change",
			old:       "alpha\nbeta\ngamma\n",
			new:       "alpha\nBETA\ngamma\n",
			context:   3,
			wantHunks: 1,
			wantAdds:  1,
			wantDels:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hunks := GenerateHunks(tt.old, tt.new, tt.context)
			if len(hunks) != tt.wantHunks {
				t.Errorf("got %d hunks, want %d", len(hunks), tt.wantHunks)
				return
			}

			var totalAdds, totalDels int
			for _, h := range hunks {
				for _, l := range h.Lines {
					switch l.Type {
					case DiffLineAdd:
						totalAdds++
					case DiffLineDelete:
						totalDels++
					}
				}
			}

			if totalAdds != tt.wantAdds {
				t.Errorf("got %d adds, want %d", totalAdds, tt.wantAdds)
			}
			if totalDels != tt.wantDels {
				t.Errorf("got %d deletes, want %d", totalDels, tt.wantDels)
			}
		})
	}
}

// T76: FormatDiff full output golden (combined test)
func TestFormatDiffFullOutput(t *testing.T) {
	tests := []struct {
		name   string
		result DiffResult
	}{
		{
			name: "binary file shows binary message",
			result: DiffResult{
				Path:     "image.png",
				Status:   StatusModified,
				IsBinary: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDiff(tt.result, true)

			// Binary file should contain the binary message
			if tt.result.IsBinary {
				if !strings.Contains(got, "Binary files") {
					t.Errorf("expected binary message in output, got:\n%s", got)
				}
			}
		})
	}
}
