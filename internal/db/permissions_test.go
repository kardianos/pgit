package db_test

import (
	"testing"

	"github.com/imgajeed76/pgit/v4/internal/db"
)

func TestPermissionHas(t *testing.T) {
	tests := []struct {
		name string
		perm db.Permission
		flag db.Permission
		want bool
	}{
		{"read_has_read", db.PermRead, db.PermRead, true},
		{"read_not_comment", db.PermRead, db.PermComment, false},
		{"all_has_read", db.PermAll, db.PermRead, true},
		{"all_has_comment", db.PermAll, db.PermComment, true},
		{"all_has_vote", db.PermAll, db.PermVote, true},
		{"all_has_cl_create", db.PermAll, db.PermCLCreate, true},
		{"all_has_cl_update", db.PermAll, db.PermCLUpdate, true},
		{"all_has_cl_submit", db.PermAll, db.PermCLSubmit, true},
		{"all_has_ci_trigger", db.PermAll, db.PermCITrigger, true},
		{"all_not_admin", db.PermAll, db.PermAdmin, false},
		{"admin_has_admin", db.PermAdmin, db.PermAdmin, true},
		{"admin_not_read", db.PermAdmin, db.PermRead, false},
		{"zero_has_nothing", db.Permission(0), db.PermRead, false},
		{"combined_has_both", db.PermRead | db.PermComment, db.PermComment, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.perm.Has(tt.flag)
			if got != tt.want {
				t.Errorf("Permission(%d).Has(%d) = %v, want %v", tt.perm, tt.flag, got, tt.want)
			}
		})
	}
}

func TestPermissionString(t *testing.T) {
	tests := []struct {
		name string
		perm db.Permission
		want string
	}{
		{"none", db.Permission(0), "none"},
		{"read_only", db.PermRead, "read"},
		{"read_comment", db.PermRead | db.PermComment, "read,comment"},
		{"all", db.PermAll, "read,comment,vote,cl_create,cl_update,cl_submit,ci_trigger"},
		{"admin_only", db.PermAdmin, "admin"},
		{"read_admin", db.PermRead | db.PermAdmin, "read,admin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.perm.String()
			if got != tt.want {
				t.Errorf("Permission(%d).String() = %q, want %q", tt.perm, got, tt.want)
			}
		})
	}
}

func TestParsePermissions(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    db.Permission
		wantErr bool
	}{
		{"empty", "", db.Permission(0), false},
		{"none", "none", db.Permission(0), false},
		{"read", "read", db.PermRead, false},
		{"read_comment", "read,comment", db.PermRead | db.PermComment, false},
		{"with_spaces", "read , comment", db.PermRead | db.PermComment, false},
		{"all_bits", "read,comment,vote,cl_create,cl_update,cl_submit,ci_trigger", db.PermAll, false},
		{"admin", "admin", db.PermAdmin, false},
		{"unknown", "bogus", db.Permission(0), true},
		{"partial_unknown", "read,bogus", db.Permission(0), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.ParsePermissions(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePermissions(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParsePermissions(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParsePermissionsRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		perm db.Permission
	}{
		{"none", db.Permission(0)},
		{"read", db.PermRead},
		{"all", db.PermAll},
		{"admin", db.PermAdmin},
		{"read_comment_vote", db.PermRead | db.PermComment | db.PermVote},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.perm.String()
			got, err := db.ParsePermissions(s)
			if err != nil {
				t.Fatalf("ParsePermissions(%q) error = %v", s, err)
			}
			if got != tt.perm {
				t.Errorf("round-trip: %d -> %q -> %d", tt.perm, s, got)
			}
		})
	}
}

func TestPermAllIncludesExpectedBits(t *testing.T) {
	expected := []db.Permission{
		db.PermRead, db.PermComment, db.PermVote,
		db.PermCLCreate, db.PermCLUpdate, db.PermCLSubmit,
		db.PermCITrigger,
	}

	for _, bit := range expected {
		if !db.PermAll.Has(bit) {
			t.Errorf("PermAll does not include bit %d", bit)
		}
	}

	if db.PermAll.Has(db.PermAdmin) {
		t.Error("PermAll should not include PermAdmin")
	}
}
