package db_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/imgajeed76/pgit/v4/internal/testdb"
)

// T42: CreateContent / GetContent round-trip
func TestCreateGetContentRoundTrip(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		content  *db.Content
	}{
		{
			name: "text_content",
			content: &db.Content{
				GroupID:   1,
				VersionID: 1,
				Content:   []byte("hello world\n"),
				IsBinary:  false,
			},
		},
		{
			name: "binary_content",
			content: &db.Content{
				GroupID:   2,
				VersionID: 1,
				Content:   []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
				IsBinary:  true,
			},
		},
		{
			name: "empty_text_content",
			content: &db.Content{
				GroupID:   3,
				VersionID: 1,
				Content:   []byte{},
				IsBinary:  false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := d.CreateContent(ctx, tt.content); err != nil {
				t.Fatalf("CreateContent: %v", err)
			}

			got, err := d.GetContent(ctx, tt.content.GroupID, tt.content.VersionID, tt.content.IsBinary)
			if err != nil {
				t.Fatalf("GetContent: %v", err)
			}
			if got == nil {
				t.Fatal("GetContent returned nil")
			}
			if !bytes.Equal(got, tt.content.Content) {
				t.Errorf("content mismatch: got %q, want %q", got, tt.content.Content)
			}
		})
	}
}

// T43: Delta compression transparency — multiple versions should each return exact original
func TestDeltaCompressionTransparency(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	const groupID int32 = 100
	const numVersions = 10

	// Build 10 versions with incremental edits
	versions := make([][]byte, numVersions)
	for i := range numVersions {
		versions[i] = fmt.Appendf(nil, "line 1: original content\nline 2: version %d\nline 3: shared data across all versions\n", i+1)
	}

	tests := []struct {
		name string
	}{
		{name: "ten_incremental_versions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Insert all versions
			for i, v := range versions {
				c := &db.Content{
					GroupID:   groupID,
					VersionID: int32(i + 1),
					Content:   v,
					IsBinary:  false,
				}
				if err := d.CreateContent(ctx, c); err != nil {
					t.Fatalf("CreateContent version %d: %v", i+1, err)
				}
			}

			// Retrieve each version and verify exact match
			for i, want := range versions {
				got, err := d.GetContent(ctx, groupID, int32(i+1), false)
				if err != nil {
					t.Fatalf("GetContent version %d: %v", i+1, err)
				}
				if !bytes.Equal(got, want) {
					t.Errorf("version %d: content mismatch\ngot:  %q\nwant: %q", i+1, got, want)
				}
			}
		})
	}
}

// T44: GetContentsBatch
func TestGetContentsBatch(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	// Insert some content
	contents := []*db.Content{
		{GroupID: 200, VersionID: 1, Content: []byte("alpha"), IsBinary: false},
		{GroupID: 200, VersionID: 2, Content: []byte("beta"), IsBinary: false},
		{GroupID: 201, VersionID: 1, Content: []byte{0xFF, 0xFE}, IsBinary: true},
	}
	for _, c := range contents {
		if err := d.CreateContent(ctx, c); err != nil {
			t.Fatalf("CreateContent: %v", err)
		}
	}

	tests := []struct {
		name        string
		keys        []db.ContentKey
		isBinaryMap map[db.ContentKey]bool
		wantLen     int
	}{
		{
			name: "mixed_text_and_binary",
			keys: []db.ContentKey{
				{GroupID: 200, VersionID: 1},
				{GroupID: 200, VersionID: 2},
				{GroupID: 201, VersionID: 1},
			},
			isBinaryMap: map[db.ContentKey]bool{
				{GroupID: 201, VersionID: 1}: true,
			},
			wantLen: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := d.GetContentsBatch(ctx, tt.keys, tt.isBinaryMap)
			if err != nil {
				t.Fatalf("GetContentsBatch: %v", err)
			}
			if len(result) != tt.wantLen {
				t.Errorf("got %d results, want %d", len(result), tt.wantLen)
			}

			// Verify specific values
			if got := result[db.ContentKey{GroupID: 200, VersionID: 1}]; !bytes.Equal(got, []byte("alpha")) {
				t.Errorf("(200,1) = %q, want %q", got, "alpha")
			}
			if got := result[db.ContentKey{GroupID: 201, VersionID: 1}]; !bytes.Equal(got, []byte{0xFF, 0xFE}) {
				t.Errorf("(201,1) = %x, want fffe", got)
			}
		})
	}
}

// T45: GetAllContentForGroup
func TestGetAllContentForGroup(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	const groupID int32 = 300
	wantContents := []string{"v1 content", "v2 content", "v3 content"}

	for i, s := range wantContents {
		c := &db.Content{GroupID: groupID, VersionID: int32(i + 1), Content: []byte(s), IsBinary: false}
		if err := d.CreateContent(ctx, c); err != nil {
			t.Fatalf("CreateContent: %v", err)
		}
	}

	tests := []struct {
		name      string
		wantCount int
	}{
		{name: "three_versions_in_group", wantCount: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pairs, err := d.GetAllContentForGroup(ctx, groupID, false)
			if err != nil {
				t.Fatalf("GetAllContentForGroup: %v", err)
			}
			if len(pairs) != tt.wantCount {
				t.Fatalf("got %d pairs, want %d", len(pairs), tt.wantCount)
			}

			// Verify order and content
			for i, p := range pairs {
				wantVersionID := int32(i + 1)
				if p.VersionID != wantVersionID {
					t.Errorf("pair[%d].VersionID = %d, want %d", i, p.VersionID, wantVersionID)
				}
				if !bytes.Equal(p.Content, []byte(wantContents[i])) {
					t.Errorf("pair[%d].Content = %q, want %q", i, p.Content, wantContents[i])
				}
			}
		})
	}
}
