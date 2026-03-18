package db_test

import (
	"context"
	"testing"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/imgajeed76/pgit/v4/internal/testdb"
)

// T54: CreateCommitGraphBatch / GetCommitGraphByID — linear graph
func TestCreateCommitGraphBatchAndGetByID(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	// Build a linear graph: seq 1 -> 2 -> 3 -> 4
	// Binary lifting ancestors:
	//   seq=1 (depth=0): ancestors=[]
	//   seq=2 (depth=1): ancestors=[1]          (2^0 back = 1)
	//   seq=3 (depth=2): ancestors=[2, 1]       (2^0 back = 2, 2^1 back = 1)
	//   seq=4 (depth=3): ancestors=[3, 2]       (2^0 back = 3, 2^1 back = 2)
	entries := []db.CommitGraphEntry{
		{Seq: 1, ID: "01GRAPH0000000000000000001A", Depth: 0, Ancestors: []int32{}},
		{Seq: 2, ID: "01GRAPH0000000000000000002B", Depth: 1, Ancestors: []int32{1}},
		{Seq: 3, ID: "01GRAPH0000000000000000003C", Depth: 2, Ancestors: []int32{2, 1}},
		{Seq: 4, ID: "01GRAPH0000000000000000004D", Depth: 3, Ancestors: []int32{3, 2}},
	}

	tests := []struct {
		name string
	}{
		{name: "linear_graph_of_four"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := d.CreateCommitGraphBatch(ctx, entries); err != nil {
				t.Fatalf("CreateCommitGraphBatch: %v", err)
			}

			// Verify each entry round-trips
			for _, want := range entries {
				got, err := d.GetCommitGraphByID(ctx, want.ID)
				if err != nil {
					t.Fatalf("GetCommitGraphByID(%s): %v", want.ID, err)
				}
				if got == nil {
					t.Fatalf("GetCommitGraphByID(%s) returned nil", want.ID)
				}
				if got.Seq != want.Seq {
					t.Errorf("Seq = %d, want %d", got.Seq, want.Seq)
				}
				if got.Depth != want.Depth {
					t.Errorf("Depth = %d, want %d", got.Depth, want.Depth)
				}
				if len(got.Ancestors) != len(want.Ancestors) {
					t.Errorf("Ancestors len = %d, want %d", len(got.Ancestors), len(want.Ancestors))
				}
				for i := range want.Ancestors {
					if i < len(got.Ancestors) && got.Ancestors[i] != want.Ancestors[i] {
						t.Errorf("Ancestors[%d] = %d, want %d", i, got.Ancestors[i], want.Ancestors[i])
					}
				}
			}

			// Verify count
			count, err := d.CountCommitsFromGraph(ctx)
			if err != nil {
				t.Fatalf("CountCommitsFromGraph: %v", err)
			}
			if count != 4 {
				t.Errorf("CountCommitsFromGraph = %d, want 4", count)
			}
		})
	}
}

// T55: GetAncestorID — ancestor at depth N
func TestGetAncestorID(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	entries := []db.CommitGraphEntry{
		{Seq: 1, ID: "01ANCESTOR2000000000000001A", Depth: 0, Ancestors: []int32{}},
		{Seq: 2, ID: "01ANCESTOR2000000000000002B", Depth: 1, Ancestors: []int32{1}},
		{Seq: 3, ID: "01ANCESTOR2000000000000003C", Depth: 2, Ancestors: []int32{2, 1}},
		{Seq: 4, ID: "01ANCESTOR2000000000000004D", Depth: 3, Ancestors: []int32{3, 2}},
	}
	if err := d.CreateCommitGraphBatch(ctx, entries); err != nil {
		t.Fatalf("CreateCommitGraphBatch: %v", err)
	}

	tests := []struct {
		name     string
		commitID string
		n        int
		wantID   string
		wantErr  bool
	}{
		{name: "zero_steps", commitID: "01ANCESTOR2000000000000004D", n: 0, wantID: "01ANCESTOR2000000000000004D"},
		{name: "one_step_back", commitID: "01ANCESTOR2000000000000004D", n: 1, wantID: "01ANCESTOR2000000000000003C"},
		{name: "two_steps_back", commitID: "01ANCESTOR2000000000000004D", n: 2, wantID: "01ANCESTOR2000000000000002B"},
		{name: "three_steps_back", commitID: "01ANCESTOR2000000000000004D", n: 3, wantID: "01ANCESTOR2000000000000001A"},
		{name: "too_far_back", commitID: "01ANCESTOR2000000000000004D", n: 10, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.GetAncestorID(ctx, tt.commitID, tt.n)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("GetAncestorID: %v", err)
			}
			if got != tt.wantID {
				t.Errorf("got %q, want %q", got, tt.wantID)
			}
		})
	}
}

// T56: FindCommitByPartialIDInGraph — partial ID resolution
func TestFindCommitByPartialIDInGraph(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	entries := []db.CommitGraphEntry{
		{Seq: 1, ID: "01PARTGRAPH00000000000001AA", Depth: 0, Ancestors: []int32{}},
		{Seq: 2, ID: "01PARTGRAPH00000000000002BB", Depth: 1, Ancestors: []int32{1}},
	}
	if err := d.CreateCommitGraphBatch(ctx, entries); err != nil {
		t.Fatalf("CreateCommitGraphBatch: %v", err)
	}

	tests := []struct {
		name    string
		partial string
		wantID  string
		wantNil bool
	}{
		{name: "full_id", partial: "01PARTGRAPH00000000000001AA", wantID: "01PARTGRAPH00000000000001AA"},
		{name: "prefix_match_unique", partial: "01PARTGRAPH00000000000001", wantID: "01PARTGRAPH00000000000001AA"},
		{name: "no_match", partial: "99ZZZZZ", wantNil: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.FindCommitByPartialIDInGraph(ctx, tt.partial)
			if err != nil {
				t.Fatalf("FindCommitByPartialIDInGraph: %v", err)
			}
			if tt.wantNil {
				if got != "" {
					t.Errorf("expected empty string, got %q", got)
				}
				return
			}
			if got != tt.wantID {
				t.Errorf("got %q, want %q", got, tt.wantID)
			}
		})
	}
}
