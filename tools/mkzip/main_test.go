package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestWriteIsReproducible(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "ollametry_1.0.0_windows_amd64")
	for name, body := range map[string]string{
		"ollametry.exe": "binary",
		"LICENSE":       "MIT",
		"README.md":     "# ollametry",
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	epoch := time.Unix(1_700_000_000, 0).UTC()

	first, second := filepath.Join(t.TempDir(), "a.zip"), filepath.Join(t.TempDir(), "b.zip")
	if err := write(first, dir, epoch); err != nil {
		t.Fatalf("first write: %v", err)
	}
	// A different on-disk mtime must not change the archive.
	if err := os.Chtimes(filepath.Join(dir, "LICENSE"), time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := write(second, dir, epoch); err != nil {
		t.Fatalf("second write: %v", err)
	}
	a, _ := os.ReadFile(first)
	b, _ := os.ReadFile(second)
	if !bytes.Equal(a, b) {
		t.Fatal("two archives of the same files differ")
	}

	zr, err := zip.NewReader(bytes.NewReader(a), int64(len(a)))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
		if !f.Modified.Equal(epoch) {
			t.Errorf("%s modified %v, want %v", f.Name, f.Modified, epoch)
		}
	}
	want := []string{
		"ollametry_1.0.0_windows_amd64/LICENSE",
		"ollametry_1.0.0_windows_amd64/README.md",
		"ollametry_1.0.0_windows_amd64/ollametry.exe",
	}
	if !slices.Equal(names, want) {
		t.Errorf("entries = %v, want %v (sorted, under one directory)", names, want)
	}
}
