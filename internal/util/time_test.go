package util

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func hasFlag(name string) bool {
	for _, arg := range os.Args {
		if arg == name || strings.HasPrefix(arg, name+"=") {
			return true
		}
	}
	return false
}

func updateGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if os.Getenv("UPDATE_GOLDEN") == "1" || hasFlag("-update") {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(path, got, 0o644)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden file %s not found (run with UPDATE_GOLDEN=1 to create)", path)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output differs from golden file %s:\ngot:\n%s\nwant:\n%s", path, got, want)
	}
}

// T8: RelativeTime
func TestRelativeTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		time time.Time
		want string
	}{
		{name: "just now (0s)", time: now, want: "just now"},
		{name: "30 seconds ago", time: now.Add(-30 * time.Second), want: "just now"},
		{name: "1 minute ago", time: now.Add(-1 * time.Minute), want: "1 minute ago"},
		{name: "45 minutes ago", time: now.Add(-45 * time.Minute), want: "45 minutes ago"},
		{name: "1 hour ago", time: now.Add(-1 * time.Hour), want: "1 hour ago"},
		{name: "5 hours ago", time: now.Add(-5 * time.Hour), want: "5 hours ago"},
		{name: "23 hours ago", time: now.Add(-23 * time.Hour), want: "23 hours ago"},
		{name: "1 day ago", time: now.Add(-24 * time.Hour), want: "1 day ago"},
		{name: "3 days ago", time: now.Add(-3 * 24 * time.Hour), want: "3 days ago"},
		{name: "6 days ago", time: now.Add(-6 * 24 * time.Hour), want: "6 days ago"},
		{name: "1 week ago", time: now.Add(-7 * 24 * time.Hour), want: "1 week ago"},
		{name: "3 weeks ago", time: now.Add(-21 * 24 * time.Hour), want: "3 weeks ago"},
		{name: "old date", time: time.Date(2020, 6, 15, 0, 0, 0, 0, time.UTC), want: "Jun 15, 2020"},
	}

	var golden strings.Builder
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RelativeTime(tt.time)
			if got != tt.want {
				t.Errorf("RelativeTime() = %q, want %q", got, tt.want)
			}
		})
		fmt.Fprintf(&golden, "%-25s => %s\n", tt.name, tt.want)
	}

	updateGolden(t, "relative_time", []byte(golden.String()))
}

func TestRelativeTimeShort(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		time time.Time
		want string
	}{
		{name: "now", time: now, want: "now"},
		{name: "5 seconds ago", time: now.Add(-5 * time.Second), want: "now"},
		{name: "10 minutes ago", time: now.Add(-10 * time.Minute), want: "10m ago"},
		{name: "1 minute ago", time: now.Add(-1 * time.Minute), want: "1m ago"},
		{name: "3 hours ago", time: now.Add(-3 * time.Hour), want: "3h ago"},
		{name: "2 days ago", time: now.Add(-2 * 24 * time.Hour), want: "2d ago"},
		{name: "2 weeks ago", time: now.Add(-14 * 24 * time.Hour), want: "2w ago"},
		{name: "old date", time: time.Date(2023, 3, 15, 0, 0, 0, 0, time.UTC), want: "Mar 15"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RelativeTimeShort(tt.time)
			if got != tt.want {
				t.Errorf("RelativeTimeShort() = %q, want %q", got, tt.want)
			}
		})
	}
}
