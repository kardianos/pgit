package db_test

import (
	"context"
	"testing"

	"github.com/imgajeed76/pgit/v4/internal/db"
	"github.com/imgajeed76/pgit/v4/internal/testdb"
)

// T23: InitSchema creates all tables
func TestInitSchemaCreatesAllTables(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	expectedTables := []string{
		"pgit_metadata",
		"pgit_commits",
		"pgit_paths",
		"pgit_file_refs",
		"pgit_text_content",
		"pgit_binary_content",
		"pgit_refs",
		"pgit_sync_state",
		"pgit_commit_graph",
		// Review tables (v5)
		"author",
		"cl",
		"patch_set",
		"review_comment",
		"review_vote",
		"ci_result",
	}

	for _, table := range expectedTables {
		t.Run(table, func(t *testing.T) {
			var exists bool
			err := d.QueryRow(ctx,
				`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = $1)`,
				table,
			).Scan(&exists)
			if err != nil {
				t.Fatalf("query for table %s: %v", table, err)
			}
			if !exists {
				t.Errorf("table %s does not exist after InitSchema", table)
			}
		})
	}

	// Verify pg_xpatch extension exists
	t.Run("pg_xpatch_extension", func(t *testing.T) {
		var exists bool
		err := d.QueryRow(ctx,
			`SELECT EXISTS (SELECT FROM pg_extension WHERE extname = 'pg_xpatch')`,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("query for extension: %v", err)
		}
		if !exists {
			t.Error("pg_xpatch extension not found after InitSchema")
		}
	})
}

// T24: InitSchema is idempotent
func TestInitSchemaIdempotent(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	// InitSchema was already called by Acquire. Call it again.
	if err := d.InitSchema(ctx); err != nil {
		t.Fatalf("second InitSchema call failed: %v", err)
	}

	// And a third time for good measure.
	if err := d.InitSchema(ctx); err != nil {
		t.Fatalf("third InitSchema call failed: %v", err)
	}
}

// T25: SchemaExists / GetSchemaVersion
func TestSchemaExistsAndVersion(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	tests := []struct {
		name            string
		wantExists      bool
		wantVersion     int
	}{
		{
			name:        "schema_exists_after_init",
			wantExists:  true,
			wantVersion: db.SchemaVersion,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists, err := d.SchemaExists(ctx)
			if err != nil {
				t.Fatalf("SchemaExists: %v", err)
			}
			if exists != tt.wantExists {
				t.Errorf("SchemaExists = %v, want %v", exists, tt.wantExists)
			}

			version, err := d.GetSchemaVersion(ctx)
			if err != nil {
				t.Fatalf("GetSchemaVersion: %v", err)
			}
			if version != tt.wantVersion {
				t.Errorf("GetSchemaVersion = %d, want %d", version, tt.wantVersion)
			}
		})
	}
}

// T26: DropSchema removes everything
func TestDropSchemaRemovesEverything(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	tests := []struct {
		name string
	}{
		{name: "drop_then_verify_gone"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := d.DropSchema(ctx); err != nil {
				t.Fatalf("DropSchema: %v", err)
			}

			exists, err := d.SchemaExists(ctx)
			if err != nil {
				t.Fatalf("SchemaExists after drop: %v", err)
			}
			if exists {
				t.Error("SchemaExists returned true after DropSchema")
			}

			// Verify specific tables are gone
			tables := []string{"pgit_commits", "pgit_paths", "pgit_file_refs",
				"pgit_text_content", "pgit_binary_content", "pgit_refs", "pgit_metadata",
				"pgit_sync_state", "pgit_commit_graph",
				"author", "cl", "patch_set", "review_comment", "review_vote", "ci_result"}
			for _, table := range tables {
				var found bool
				err := d.QueryRow(ctx,
					`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = $1)`,
					table,
				).Scan(&found)
				if err != nil {
					t.Fatalf("query for table %s: %v", table, err)
				}
				if found {
					t.Errorf("table %s still exists after DropSchema", table)
				}
			}
		})
	}
}

// T27: Index management — DropAllIndexes then CreateAllIndexes
func TestIndexManagement(t *testing.T) {
	d := testdb.Acquire(t)
	ctx := context.Background()

	tests := []struct {
		name string
	}{
		{name: "drop_and_recreate_indexes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Count indexes before drop
			var countBefore int
			err := d.QueryRow(ctx,
				`SELECT COUNT(*) FROM pg_indexes WHERE indexname LIKE 'idx_%'`,
			).Scan(&countBefore)
			if err != nil {
				t.Fatalf("count indexes before: %v", err)
			}
			if countBefore == 0 {
				t.Fatal("expected at least some secondary indexes before drop")
			}

			// Drop all indexes
			if err := d.DropAllIndexes(ctx); err != nil {
				t.Fatalf("DropAllIndexes: %v", err)
			}

			// Verify indexes are gone
			var countAfterDrop int
			err = d.QueryRow(ctx,
				`SELECT COUNT(*) FROM pg_indexes WHERE indexname LIKE 'idx_%'`,
			).Scan(&countAfterDrop)
			if err != nil {
				t.Fatalf("count indexes after drop: %v", err)
			}
			if countAfterDrop != 0 {
				t.Errorf("expected 0 secondary indexes after drop, got %d", countAfterDrop)
			}

			// Recreate all indexes
			if err := d.CreateAllIndexes(ctx); err != nil {
				t.Fatalf("CreateAllIndexes: %v", err)
			}

			// Verify indexes are back
			var countAfterCreate int
			err = d.QueryRow(ctx,
				`SELECT COUNT(*) FROM pg_indexes WHERE indexname LIKE 'idx_%'`,
			).Scan(&countAfterCreate)
			if err != nil {
				t.Fatalf("count indexes after create: %v", err)
			}
			if countAfterCreate != countBefore {
				t.Errorf("expected %d secondary indexes after recreate, got %d", countBefore, countAfterCreate)
			}
		})
	}
}
