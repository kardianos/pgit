package util

import (
	"bytes"
	"testing"
)

// T1: HashBytes (SHA256) and HashBytesBlake3
func TestHashBytes(t *testing.T) {
	tests := []struct {
		name       string
		input      []byte
		wantSHA    string // expected SHA256 hex (hardcoded known-good values)
		wantBLAKE3 string // expected BLAKE3 truncated hex (first 16 bytes)
	}{
		{
			name:       "empty input",
			input:      []byte{},
			wantSHA:    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			wantBLAKE3: "af1349b9f5f9a1a6a0404dea36dcc949",
		},
		{
			name:       "hello world",
			input:      []byte("hello world"),
			wantSHA:    "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
			wantBLAKE3: "d74981efa70a0c880b8d8c1985d075db",
		},
		{
			name:  "binary data with NUL bytes",
			input: []byte{0x00, 0xFF, 0x01, 0xFE},
			// Verify non-empty, correct length; actual values not hardcoded
			// because the point is exercising binary input paths.
		},
		{
			name:  "large repeated input",
			input: bytes.Repeat([]byte("a"), 100_000),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// SHA256
			gotSHA := HashBytes(tt.input)
			if len(gotSHA) != 64 {
				t.Fatalf("HashBytes() length = %d, want 64", len(gotSHA))
			}
			if tt.wantSHA != "" && gotSHA != tt.wantSHA {
				t.Errorf("HashBytes() = %q, want %q", gotSHA, tt.wantSHA)
			}

			// BLAKE3 — check actual value, not just length
			gotBlake := HashBytesBlake3(tt.input)
			if len(gotBlake) != ContentHashSize {
				t.Fatalf("HashBytesBlake3() length = %d, want %d", len(gotBlake), ContentHashSize)
			}

			gotHex := HashBytesBlake3Hex(tt.input)
			if len(gotHex) != ContentHashSize*2 {
				t.Fatalf("HashBytesBlake3Hex() length = %d, want %d", len(gotHex), ContentHashSize*2)
			}
			if tt.wantBLAKE3 != "" && gotHex != tt.wantBLAKE3 {
				t.Errorf("HashBytesBlake3Hex() = %q, want %q", gotHex, tt.wantBLAKE3)
			}
		})
	}
}

func TestHashBytesDeterministic(t *testing.T) {
	data := []byte("determinism check")
	h1 := HashBytesBlake3(data)
	h2 := HashBytesBlake3(data)
	if !bytes.Equal(h1, h2) {
		t.Errorf("HashBytesBlake3 not deterministic: %x != %x", h1, h2)
	}
}

func TestHashBytesDifferentInputsDifferentHashes(t *testing.T) {
	h1 := HashBytesBlake3([]byte("input A"))
	h2 := HashBytesBlake3([]byte("input B"))
	if bytes.Equal(h1, h2) {
		t.Error("HashBytesBlake3 produced same hash for different inputs")
	}
}

// T2: DetectBinary
func TestDetectBinary(t *testing.T) {
	tests := []struct {
		name   string
		input  []byte
		binary bool
	}{
		{name: "empty", input: []byte{}, binary: false},
		{name: "plain text", input: []byte("Hello, world!\n"), binary: false},
		{name: "utf8 multibyte", input: []byte("Grüße\n"), binary: false},
		{name: "NUL at start", input: []byte{0x00, 'a', 'b'}, binary: true},
		{name: "NUL at end", input: []byte{'a', 'b', 0x00}, binary: true},
		{name: "NUL in middle", input: []byte("hello\x00world"), binary: true},
		{name: "high bytes no NUL", input: []byte{0xFF, 0xFE, 0xFD}, binary: false},
		{name: "all NUL", input: []byte{0, 0, 0, 0}, binary: true},
		{name: "ELF header", input: []byte{0x7f, 'E', 'L', 'F', 0x00}, binary: true},
		{name: "tabs and newlines", input: []byte("col1\tcol2\nval1\tval2\n"), binary: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectBinary(tt.input)
			if got != tt.binary {
				t.Errorf("DetectBinary() = %v, want %v", got, tt.binary)
			}
		})
	}
}

// ContentHash helper tests
func TestContentHashEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b []byte
		want bool
	}{
		{name: "both nil", a: nil, b: nil, want: true},
		{name: "a nil b not", a: nil, b: []byte{1}, want: false},
		{name: "a not b nil", a: []byte{1}, b: nil, want: false},
		{name: "equal", a: []byte{1, 2, 3}, b: []byte{1, 2, 3}, want: true},
		{name: "different length", a: []byte{1, 2}, b: []byte{1, 2, 3}, want: false},
		{name: "same length different", a: []byte{1, 2, 3}, b: []byte{1, 2, 4}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContentHashEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("ContentHashEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContentHashToFromHex(t *testing.T) {
	tests := []struct {
		name    string
		hash    []byte
		hex     string
		wantErr bool
	}{
		{name: "nil hash", hash: nil, hex: ""},
		{name: "16 byte hash", hash: []byte{0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}, hex: "abcdef01234567890011223344556677"},
	}
	for _, tt := range tests {
		t.Run(tt.name+" to hex", func(t *testing.T) {
			got := ContentHashToHex(tt.hash)
			if got != tt.hex {
				t.Errorf("ContentHashToHex() = %q, want %q", got, tt.hex)
			}
		})
		t.Run(tt.name+" from hex", func(t *testing.T) {
			got, err := ContentHashFromHex(tt.hex)
			if (err != nil) != tt.wantErr {
				t.Errorf("ContentHashFromHex() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !bytes.Equal(got, tt.hash) {
				t.Errorf("ContentHashFromHex() = %x, want %x", got, tt.hash)
			}
		})
	}
}

func TestContentHashFromHexInvalid(t *testing.T) {
	_, err := ContentHashFromHex("zzzz")
	if err == nil {
		t.Error("ContentHashFromHex() expected error for invalid hex")
	}
}

// T10: ComputeTreeHash
func TestComputeTreeHash(t *testing.T) {
	hashA := HashBytesBlake3([]byte("content A"))
	hashB := HashBytesBlake3([]byte("content B"))
	hashC := HashBytesBlake3([]byte("content C"))

	tests := []struct {
		name       string
		entries    []TreeEntry
		wantSame   []TreeEntry // entries that should produce the same hash
		wantDiffer []TreeEntry // entries that should produce a different hash
	}{
		{
			name:    "empty tree",
			entries: []TreeEntry{},
		},
		{
			name: "single file",
			entries: []TreeEntry{
				{Mode: 0o100644, Path: "main.go", ContentHash: hashA},
			},
		},
		{
			name: "sort order invariance",
			entries: []TreeEntry{
				{Mode: 0o100644, Path: "b.go", ContentHash: hashB},
				{Mode: 0o100644, Path: "a.go", ContentHash: hashA},
			},
			wantSame: []TreeEntry{
				{Mode: 0o100644, Path: "a.go", ContentHash: hashA},
				{Mode: 0o100644, Path: "b.go", ContentHash: hashB},
			},
		},
		{
			name: "mode affects hash",
			entries: []TreeEntry{
				{Mode: 0o100644, Path: "script.sh", ContentHash: hashA},
			},
			wantDiffer: []TreeEntry{
				{Mode: 0o100755, Path: "script.sh", ContentHash: hashA},
			},
		},
		{
			name: "content affects hash",
			entries: []TreeEntry{
				{Mode: 0o100644, Path: "file.txt", ContentHash: hashA},
			},
			wantDiffer: []TreeEntry{
				{Mode: 0o100644, Path: "file.txt", ContentHash: hashB},
			},
		},
		{
			name: "three files deep paths",
			entries: []TreeEntry{
				{Mode: 0o100644, Path: "src/pkg/main.go", ContentHash: hashA},
				{Mode: 0o100644, Path: "README.md", ContentHash: hashB},
				{Mode: 0o100755, Path: "scripts/build.sh", ContentHash: hashC},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeTreeHash(tt.entries)
			if got == "" {
				t.Error("ComputeTreeHash() returned empty string")
			}

			// Determinism
			got2 := ComputeTreeHash(tt.entries)
			if got != got2 {
				t.Errorf("ComputeTreeHash() not deterministic: %q != %q", got, got2)
			}

			// Check same-hash expectation (sort order invariance)
			if tt.wantSame != nil {
				gotSame := ComputeTreeHash(tt.wantSame)
				if got != gotSame {
					t.Errorf("expected same hash for reordered entries: %q != %q", got, gotSame)
				}
			}

			// Check different-hash expectation
			if tt.wantDiffer != nil {
				gotDiff := ComputeTreeHash(tt.wantDiffer)
				if got == gotDiff {
					t.Errorf("expected different hash but got same: %q", got)
				}
			}
		})
	}
}

// T9: ToValidUTF8
func TestToValidUTF8(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "valid ASCII",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "valid UTF-8 with multibyte",
			input: "日本語テスト",
			want:  "日本語テスト",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "latin1 umlaut bytes",
			input: string([]byte{0xC4, 0xD6, 0xDC}), // Ä Ö Ü in Latin-1
			want:  "ÄÖÜ",
		},
		{
			name:  "latin1 accented chars",
			input: string([]byte{0xE9, 0xE8, 0xEA}), // é è ê in Latin-1
			want:  "éèê",
		},
		{
			name:  "mixed valid utf8 and latin1",
			input: "abc" + string([]byte{0xFC}) + "def", // 0xFC = ü in Latin-1
			want:  "abcüdef",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToValidUTF8(tt.input)
			if got != tt.want {
				t.Errorf("ToValidUTF8() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestToValidUTF8Bytes(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{name: "valid utf8 bytes", input: []byte("hello"), want: "hello"},
		{name: "latin1 bytes", input: []byte{0xE4, 0xF6, 0xFC}, want: "äöü"}, // ä ö ü
		{name: "nil input", input: nil, want: ""},
		{name: "empty", input: []byte{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToValidUTF8Bytes(tt.input)
			if string(got) != tt.want {
				t.Errorf("ToValidUTF8Bytes() = %q, want %q", got, tt.want)
			}
		})
	}
}
