package cli

import (
	"strings"
	"testing"
)

func TestMountHelp(t *testing.T) {
	root := newTestRootCmd()
	stdout, _, err := executeCommand(root, "mount", "--help")
	if err != nil {
		t.Fatalf("mount --help failed: %v", err)
	}

	for _, want := range []string{"mount", "ref", "mountpoint", "FUSE", "read-only"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("mount help missing %q\n--- output ---\n%s", want, stdout)
		}
	}
}

func TestUnmountHelp(t *testing.T) {
	root := newTestRootCmd()
	stdout, _, err := executeCommand(root, "unmount", "--help")
	if err != nil {
		t.Fatalf("unmount --help failed: %v", err)
	}

	for _, want := range []string{"unmount", "mountpoint", "Unmount"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("unmount help missing %q\n--- output ---\n%s", want, stdout)
		}
	}
}

func TestMountMissingArgs(t *testing.T) {
	root := newTestRootCmd()
	_, _, err := executeCommand(root, "mount")
	if err == nil {
		t.Error("mount with no args should error")
	}
}

func TestMountTooFewArgs(t *testing.T) {
	root := newTestRootCmd()
	_, _, err := executeCommand(root, "mount", "HEAD")
	if err == nil {
		t.Error("mount with one arg should error")
	}
}

func TestUnmountMissingArgs(t *testing.T) {
	root := newTestRootCmd()
	_, _, err := executeCommand(root, "unmount")
	if err == nil {
		t.Error("unmount with no args should error")
	}
}
