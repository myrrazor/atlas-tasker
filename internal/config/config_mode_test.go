package config

import (
	"os"
	"runtime"
	"testing"
)

func TestSaveWritesPrivateConfigMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix mode bits")
	}
	root := t.TempDir()
	cfg := defaultConfig()
	if err := Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(Path(root))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("config mode=%o want private", info.Mode().Perm())
	}
	if err := os.Chmod(Path(root), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(Path(root))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("config mode after re-save=%o want private", info.Mode().Perm())
	}
}
