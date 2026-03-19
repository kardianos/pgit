package cli

import (
	"bytes"
	"testing"
)

func TestServerCmd_Help(t *testing.T) {
	cmd := newServerCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("server --help failed: %v", err)
	}

	output := buf.String()

	tests := []struct {
		name     string
		contains string
	}{
		{"usage_line", "HTTP server"},
		{"addr_flag", "--addr"},
		{"database_url_flag", "--database-url"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !bytes.Contains([]byte(output), []byte(tt.contains)) {
				t.Errorf("expected help output to contain %q, got:\n%s", tt.contains, output)
			}
		})
	}
}
