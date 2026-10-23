package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func main() {
	root := flag.String("root", "", "folder to package")
	output := flag.String("output", "", "zip destination")
	flag.Parse()
	if err := pack(*root, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func pack(root, output string) error {
	if root == "" || output == "" {
		return fmt.Errorf("root and output are required")
	}
	parent := filepath.Dir(output)
	if e := os.MkdirAll(parent, 0755); e != nil {
		return e
	}
	f, e := os.CreateTemp(parent, ".portable-*.zip")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	z := zip.NewWriter(f)
	e = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported package entry %s", path)
		}
		rel, err := filepath.Rel(filepath.Dir(root), path)
		if err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		h.Method = zip.Deflate
		w, err := z.CreateHeader(h)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(w, src)
		return err
	})
	ze := z.Close()
	fe := f.Close()
	if e != nil {
		return e
	}
	if ze != nil {
		return ze
	}
	if fe != nil {
		return fe
	}
	return os.Rename(name, output)
}
