// Command mkzip writes a reproducible zip archive for the Windows release:
// entries in sorted order under one top-level directory, every timestamp set
// to SOURCE_DATE_EPOCH, and fixed permissions. The golang image has no zip
// tool, and archive/zip from the pinned toolchain keeps the output identical
// across machines.
//
// Usage:
//
//	mkzip -o out.zip -epoch 1700000000 DIR
package main

import (
	"archive/zip"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"time"
)

func main() {
	out := flag.String("o", "", "output zip file")
	epoch := flag.Int64("epoch", -1, "timestamp for every entry, in Unix seconds (SOURCE_DATE_EPOCH)")
	flag.Parse()
	if *out == "" || *epoch < 0 || flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: mkzip -o out.zip -epoch SECONDS DIR")
		os.Exit(2)
	}
	if err := write(*out, flag.Arg(0), time.Unix(*epoch, 0).UTC()); err != nil {
		fmt.Fprintf(os.Stderr, "mkzip: %v\n", err)
		os.Exit(1)
	}
}

// write archives dir as out. Entry names start with dir's base name, like
// the tar archives, so extracting creates one directory. Files are read
// through an os.Root, so nothing outside dir can be archived.
func write(out, dir string, modified time.Time) (err error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("open %s: %w", dir, err)
	}
	defer root.Close()
	files, err := listFiles(root.FS())
	if err != nil {
		return fmt.Errorf("list %s: %w", dir, err)
	}

	f, err := os.Create(out) //nolint:gosec // G304: the output path is the tool's argument by design
	if err != nil {
		return fmt.Errorf("create %s: %w", out, err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()

	zw := zip.NewWriter(f)
	prefix := filepath.Base(dir)
	for _, rel := range files {
		if err := add(zw, root, rel, path.Join(prefix, rel), modified); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("finish %s: %w", out, err)
	}
	return nil
}

// listFiles returns the regular files under fsys as sorted slash paths.
func listFiles(fsys fs.FS) ([]string, error) {
	var files []string
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk: %w", err)
	}
	slices.Sort(files)
	return files, nil
}

func add(zw *zip.Writer, root *os.Root, rel, name string, modified time.Time) error {
	in, err := root.Open(filepath.FromSlash(rel))
	if err != nil {
		return fmt.Errorf("open %s: %w", rel, err)
	}
	defer in.Close()

	hdr := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: modified}
	hdr.SetMode(0o755)
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return fmt.Errorf("add %s: %w", name, err)
	}
	if _, err := io.Copy(w, in); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}
