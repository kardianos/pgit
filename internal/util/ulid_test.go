package util

import (
	"testing"
	"time"
)

// T6: ULID generation and parsing
func TestNewULID(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "generates valid ULID"},
		{name: "second call also valid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := NewULID()
			if len(id) != 26 {
				t.Errorf("NewULID() length = %d, want 26", len(id))
			}
			if !ValidateULID(id) {
				t.Errorf("NewULID() returned invalid ULID: %q", id)
			}
		})
	}
}

func TestNewULIDUniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := range 1000 {
		id := NewULID()
		if seen[id] {
			t.Fatalf("NewULID() duplicate at iteration %d: %q", i, id)
		}
		seen[id] = true
	}
}

func TestNewULIDWithTime(t *testing.T) {
	tests := []struct {
		name string
		time time.Time
	}{
		{name: "epoch", time: time.Unix(0, 0)},
		{name: "2024-01-01", time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{name: "far future", time: time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)},
		{name: "now", time: time.Now()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := NewULIDWithTime(tt.time)
			if len(id) != 26 {
				t.Errorf("NewULIDWithTime() length = %d, want 26", len(id))
			}
			if !ValidateULID(id) {
				t.Errorf("NewULIDWithTime() returned invalid ULID: %q", id)
			}

			// Parse back and verify timestamp is within 1ms
			parsed, err := ParseULID(id)
			if err != nil {
				t.Fatalf("ParseULID() error: %v", err)
			}
			diff := parsed.Sub(tt.time)
			if diff < 0 {
				diff = -diff
			}
			if diff > time.Millisecond {
				t.Errorf("timestamp mismatch: parsed=%v, input=%v, diff=%v", parsed, tt.time, diff)
			}
		})
	}
}

func TestParseULID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid ULID", input: NewULID(), wantErr: false},
		{name: "empty string", input: "", wantErr: true},
		{name: "too short", input: "01234", wantErr: true},
		{name: "overflow value", input: "80000000000000000000000000", wantErr: true}, // exceeds max ULID value
		{name: "too long", input: "012345678901234567890123456", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseULID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseULID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateULID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "valid generated", input: NewULID(), want: true},
		{name: "all zeros", input: "00000000000000000000000000", want: true},
		{name: "empty", input: "", want: false},
		{name: "random garbage", input: "not-a-ulid-at-all", want: false},
		{name: "26 lowercase", input: "01aryz6s41yy1r5gj1v96xg000", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateULID(tt.input)
			if got != tt.want {
				t.Errorf("ValidateULID(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestULIDTimeSortable(t *testing.T) {
	t1 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	id1 := NewULIDWithTime(t1)
	id2 := NewULIDWithTime(t2)

	if id1 >= id2 {
		t.Errorf("ULIDs not time-sorted: %q (2020) >= %q (2025)", id1, id2)
	}
}
