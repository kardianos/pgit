package config

import (
	"os"
	"path/filepath"
	"testing"
)

// T18: Index Add/Delete/List operations
func TestIndexAddDeleteList(t *testing.T) {
	type op struct {
		action string // "add", "add_new", "delete", "remove"
		path   string
	}

	tests := []struct {
		name        string
		ops         []op
		wantEntries []IndexEntry
	}{
		{
			name: "add single new file",
			ops: []op{
				{action: "add_new", path: "file.txt"},
			},
			wantEntries: []IndexEntry{
				{Status: StatusAdded, Path: "file.txt"},
			},
		},
		{
			name: "add modified file",
			ops: []op{
				{action: "add", path: "existing.txt"},
			},
			wantEntries: []IndexEntry{
				{Status: StatusModified, Path: "existing.txt"},
			},
		},
		{
			name: "delete a file",
			ops: []op{
				{action: "delete", path: "removed.txt"},
			},
			wantEntries: []IndexEntry{
				{Status: StatusDeleted, Path: "removed.txt"},
			},
		},
		{
			name: "add then remove unstages",
			ops: []op{
				{action: "add_new", path: "file.txt"},
				{action: "remove", path: "file.txt"},
			},
			wantEntries: []IndexEntry{},
		},
		{
			name: "multiple files sorted by path",
			ops: []op{
				{action: "add_new", path: "c.txt"},
				{action: "add_new", path: "a.txt"},
				{action: "delete", path: "b.txt"},
			},
			wantEntries: []IndexEntry{
				{Status: StatusAdded, Path: "a.txt"},
				{Status: StatusDeleted, Path: "b.txt"},
				{Status: StatusAdded, Path: "c.txt"},
			},
		},
		{
			name: "overwrite status on same path",
			ops: []op{
				{action: "add_new", path: "file.txt"},
				{action: "delete", path: "file.txt"},
			},
			wantEntries: []IndexEntry{
				{Status: StatusDeleted, Path: "file.txt"},
			},
		},
		{
			name: "clear empties everything",
			ops: []op{
				{action: "add_new", path: "a.txt"},
				{action: "add_new", path: "b.txt"},
				{action: "clear"},
			},
			wantEntries: []IndexEntry{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := NewIndex()

			for _, o := range tt.ops {
				switch o.action {
				case "add_new":
					idx.Add(o.path, true)
				case "add":
					idx.Add(o.path, false)
				case "delete":
					idx.Delete(o.path)
				case "remove":
					idx.Remove(o.path)
				case "clear":
					idx.Clear()
				}
			}

			got := idx.List()
			if len(got) != len(tt.wantEntries) {
				t.Fatalf("List() returned %d entries, want %d", len(got), len(tt.wantEntries))
			}
			for i, want := range tt.wantEntries {
				if got[i].Status != want.Status || got[i].Path != want.Path {
					t.Errorf("entry[%d] = {%s %s}, want {%s %s}",
						i, got[i].Status, got[i].Path, want.Status, want.Path)
				}
			}
		})
	}
}

// T19: Index Save/Load round-trip
func TestIndexSaveLoadRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		entries []struct {
			path  string
			isNew bool
			isDel bool
		}
	}{
		{
			name: "empty index",
		},
		{
			name: "single added file",
			entries: []struct {
				path  string
				isNew bool
				isDel bool
			}{
				{path: "new_file.sql", isNew: true},
			},
		},
		{
			name: "mixed statuses",
			entries: []struct {
				path  string
				isNew bool
				isDel bool
			}{
				{path: "added.sql", isNew: true},
				{path: "modified.sql", isNew: false},
				{path: "deleted.sql", isDel: true},
			},
		},
		{
			name: "nested paths",
			entries: []struct {
				path  string
				isNew bool
				isDel bool
			}{
				{path: "schema/tables/users.sql", isNew: true},
				{path: "schema/views/report.sql", isNew: true},
				{path: "data/seed.sql", isNew: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, ".pgit"), 0755); err != nil {
				t.Fatal(err)
			}

			original := NewIndex()
			for _, e := range tt.entries {
				if e.isDel {
					original.Delete(e.path)
				} else {
					original.Add(e.path, e.isNew)
				}
			}

			if err := original.Save(dir); err != nil {
				t.Fatalf("Save: %v", err)
			}

			loaded, err := LoadIndex(dir)
			if err != nil {
				t.Fatalf("LoadIndex: %v", err)
			}

			origList := original.List()
			loadedList := loaded.List()

			if len(origList) != len(loadedList) {
				t.Fatalf("loaded %d entries, want %d", len(loadedList), len(origList))
			}

			for i, want := range origList {
				got := loadedList[i]
				if got.Status != want.Status || got.Path != want.Path {
					t.Errorf("entry[%d] = {%s %s}, want {%s %s}",
						i, got.Status, got.Path, want.Status, want.Path)
				}
			}
		})
	}
}
