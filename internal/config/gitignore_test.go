package config

import (
	"os"
	"path/filepath"
	"testing"
)

// T20: IgnorePatterns
func TestIgnorePatterns(t *testing.T) {
	tests := []struct {
		name          string
		pgitignore    string
		path          string
		isDir         bool
		wantIgnored   bool
	}{
		{
			name:        "wildcard *.log matches file",
			pgitignore:  "*.log\n",
			path:        "debug.log",
			isDir:       false,
			wantIgnored: true,
		},
		{
			name:        "wildcard *.log does not match .txt",
			pgitignore:  "*.log\n",
			path:        "readme.txt",
			isDir:       false,
			wantIgnored: false,
		},
		{
			name:        "directory rule build/ ignores directory",
			pgitignore:  "build/\n",
			path:        "build",
			isDir:       true,
			wantIgnored: true,
		},
		{
			name:        "directory rule build/ does not ignore file named build",
			pgitignore:  "build/\n",
			path:        "build",
			isDir:       false,
			wantIgnored: false,
		},
		{
			name:        "negation !important.log overrides *.log",
			pgitignore:  "*.log\n!important.log\n",
			path:        "important.log",
			isDir:       false,
			wantIgnored: false,
		},
		{
			name:        "negation does not affect other .log files",
			pgitignore:  "*.log\n!important.log\n",
			path:        "debug.log",
			isDir:       false,
			wantIgnored: true,
		},
		{
			name:        "wildcard matches nested basename",
			pgitignore:  "*.log\n",
			path:        "logs/debug.log",
			isDir:       false,
			wantIgnored: true,
		},
		{
			name:        "double-star **/*.go matches nested",
			pgitignore:  "**/*.go\n",
			path:        "pkg/main.go",
			isDir:       false,
			wantIgnored: true,
		},
		{
			name:        "double-star **/*.go matches top-level",
			pgitignore:  "**/*.go\n",
			path:        "main.go",
			isDir:       false,
			wantIgnored: true,
		},
		{
			name:        "comment lines are ignored",
			pgitignore:  "# this is a comment\n*.log\n",
			path:        "debug.log",
			isDir:       false,
			wantIgnored: true,
		},
		{
			name:        "empty lines are ignored",
			pgitignore:  "\n\n*.log\n\n",
			path:        "debug.log",
			isDir:       false,
			wantIgnored: true,
		},
		{
			name:        ".pgit directory always ignored",
			pgitignore:  "",
			path:        ".pgit",
			isDir:       true,
			wantIgnored: true,
		},
		{
			name:        "pattern with slash is rooted - positive",
			pgitignore:  "build/output\n",
			path:        "build/output",
			isDir:       false,
			wantIgnored: true,
		},
		{
			name:        "pattern with slash is rooted - negative (nested)",
			pgitignore:  "build/output\n",
			path:        "other/build/output",
			isDir:       false,
			wantIgnored: false,
		},
		{
			name:        "exact filename matches at any depth",
			pgitignore:  "debug.log\n",
			path:        "logs/debug.log",
			isDir:       false,
			wantIgnored: true,
		},
		{
			name:        "vendor/** also matches vendor directory itself",
			pgitignore:  "vendor/**\n",
			path:        "vendor",
			isDir:       true,
			wantIgnored: true, // implementation treats vendor/** as matching the vendor dir too
		},
		{
			name:        "vendor/** does not match notvendor path",
			pgitignore:  "vendor/**\n",
			path:        "notvendor/pkg/mod.go",
			isDir:       false,
			wantIgnored: false,
		},
		{
			name:        "exact filename match",
			pgitignore:  "Thumbs.db\n",
			path:        "Thumbs.db",
			isDir:       false,
			wantIgnored: true,
		},
		{
			name:        "question mark wildcard",
			pgitignore:  "file?.txt\n",
			path:        "file1.txt",
			isDir:       false,
			wantIgnored: true,
		},
		{
			name:        "prefix double-star vendor/**",
			pgitignore:  "vendor/**\n",
			path:        "vendor/pkg/mod.go",
			isDir:       false,
			wantIgnored: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			// Write .pgitignore
			if tt.pgitignore != "" {
				if err := os.WriteFile(filepath.Join(dir, ".pgitignore"), []byte(tt.pgitignore), 0644); err != nil {
					t.Fatal(err)
				}
			}

			ip, err := LoadIgnorePatterns(dir)
			if err != nil {
				t.Fatalf("LoadIgnorePatterns: %v", err)
			}

			got := ip.IsIgnored(tt.path, tt.isDir)
			if got != tt.wantIgnored {
				t.Errorf("IsIgnored(%q, isDir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.wantIgnored)
			}
		})
	}
}
