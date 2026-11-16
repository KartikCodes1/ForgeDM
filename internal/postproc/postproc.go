// Package postproc runs the finishing steps after a download completes:
// checksum verification, antivirus scan and archive extraction.
package postproc

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bodgit/sevenzip"
	"github.com/nwaples/rardecode/v2"
)

// Hash computes the hex digest of a file.
func Hash(path, algo string) (string, error) {
	var h hash.Hash
	switch strings.ToLower(algo) {
	case "md5":
		h = md5.New()
	case "sha1":
		h = sha1.New()
	case "sha512":
		h = sha512.New()
	default:
		h = sha256.New()
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.CopyBuffer(h, f, make([]byte, 1<<20)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// IsArchive reports whether ForgeDM can extract the file.
func IsArchive(name string) bool {
	n := strings.ToLower(name)
	for _, s := range []string{".zip", ".7z", ".rar", ".tar", ".tar.gz", ".tgz", ".tar.bz2", ".tbz2"} {
		if strings.HasSuffix(n, s) {
			return true
		}
	}
	return false
}

func archiveStem(name string) string {
	n := name
	lower := strings.ToLower(n)
	for _, s := range []string{".tar.gz", ".tar.bz2", ".tgz", ".tbz2", ".zip", ".7z", ".rar", ".tar"} {
		if strings.HasSuffix(lower, s) {
			return n[:len(n)-len(s)]
		}
	}
	return strings.TrimSuffix(n, filepath.Ext(n))
}

// uniqueDir returns dir, or "dir (n)" if it already exists.
func uniqueDir(dir string) string {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return dir
	}
	for i := 1; i < 1000; i++ {
		c := fmt.Sprintf("%s (%d)", dir, i)
		if _, err := os.Stat(c); os.IsNotExist(err) {
			return c
		}
	}
	return dir
}

// safeJoin prevents archive entries from escaping the target folder.
func safeJoin(root, name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	p := filepath.Join(root, filepath.FromSlash(name))
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("unsafe path in archive: %s", name)
	}
	return p, nil
}

func writeEntry(root, name string, isDir bool, r io.Reader) error {
	p, err := safeJoin(root, name)
	if err != nil {
		return err
	}
	if isDir {
		return os.MkdirAll(p, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Extract unpacks an archive next to it and returns the folder it created.
func Extract(path string) (string, error) {
	dest := uniqueDir(filepath.Join(filepath.Dir(path), archiveStem(filepath.Base(path))))
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}
	lower := strings.ToLower(path)
	var err error
	switch {
	case strings.HasSuffix(lower, ".zip"):
		err = extractZip(path, dest)
	case strings.HasSuffix(lower, ".7z"):
		err = extract7z(path, dest)
	case strings.HasSuffix(lower, ".rar"):
		err = extractRar(path, dest)
	case strings.HasSuffix(lower, ".tar"):
		err = extractTarFile(path, dest, "")
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		err = extractTarFile(path, dest, "gz")
	case strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"):
		err = extractTarFile(path, dest, "bz2")
	default:
		err = errors.New("this archive type isn't supported")
	}
	if err != nil {
		_ = os.RemoveAll(dest)
		return "", err
	}
	return dest, nil
}

func extractZip(path, dest string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if f.Flags&0x1 != 0 {
			return errors.New("the archive is password-protected")
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = writeEntry(dest, f.Name, f.FileInfo().IsDir(), rc)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extract7z(path, dest string) error {
	r, err := sevenzip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = writeEntry(dest, f.Name, f.FileInfo().IsDir(), rc)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractRar(path, dest string) error {
	r, err := rardecode.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	for {
		h, err := r.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Encrypted {
			return errors.New("the archive is password-protected")
		}
		if err := writeEntry(dest, h.Name, h.IsDir, r); err != nil {
			return err
		}
	}
}

func extractTarFile(path, dest, comp string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var in io.Reader = f
	switch comp {
	case "gz":
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gz.Close()
		in = gz
	case "bz2":
		in = bzip2.NewReader(f)
	}
	tr := tar.NewReader(in)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := writeEntry(dest, h.Name, true, nil); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeEntry(dest, h.Name, false, tr); err != nil {
				return err
			}
		}
	}
}
