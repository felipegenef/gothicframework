package helpers

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Archive extraction utilities used by EnsureBinary to unpack the TinyGo
// release archives (tar.gz on linux/darwin, zip on windows).

func (h *WasmHelper) extractArchive(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	magic := make([]byte, 4)
	_, err = f.Read(magic)
	f.Close()
	if err != nil {
		return fmt.Errorf("read magic bytes: %w", err)
	}
	switch {
	case magic[0] == 0x1f && magic[1] == 0x8b:
		return h.extractTarGz(archivePath, destDir)
	case magic[0] == 'P' && magic[1] == 'K':
		return h.extractZip(archivePath, destDir)
	default:
		return fmt.Errorf("unknown archive format (magic: %x)", magic[:2])
	}
}

func (h *WasmHelper) safeDest(destDir, entryName string) (string, error) {
	if entryName == "" {
		return "", fmt.Errorf("empty entry name")
	}
	dest := filepath.Join(destDir, filepath.FromSlash(entryName))
	prefix := filepath.Clean(destDir) + string(os.PathSeparator)
	if !strings.HasPrefix(dest+string(os.PathSeparator), prefix) {
		return "", fmt.Errorf("path traversal rejected: %q", entryName)
	}
	return dest, nil
}

func (h *WasmHelper) extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar next: %w", err)
		}
		dest, err := h.safeDest(destDir, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			mode := hdr.FileInfo().Mode()
			if mode == 0 {
				mode = 0644
			}
			if err := h.writeFileFromReader(dest, tr, mode); err != nil {
				return fmt.Errorf("write %s: %w", hdr.Name, err)
			}
		case tar.TypeSymlink:
			if filepath.IsAbs(hdr.Linkname) {
				return fmt.Errorf("absolute symlink rejected: %q → %q", hdr.Name, hdr.Linkname)
			}
			os.Remove(dest)
			if err := os.Symlink(hdr.Linkname, dest); err != nil {
				return fmt.Errorf("symlink %s: %w", hdr.Name, err)
			}
		case tar.TypeLink:
			linkSrc, err := h.safeDest(destDir, hdr.Linkname)
			if err != nil {
				return fmt.Errorf("hard link source: %w", err)
			}
			os.Remove(dest)
			if err := os.Link(linkSrc, dest); err != nil {
				return fmt.Errorf("hard link %s: %w", hdr.Name, err)
			}
		}
	}
	return nil
}

func (h *WasmHelper) extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()
	for _, f := range r.File {
		dest, err := h.safeDest(destDir, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dest, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		mode := f.Mode()
		if mode == 0 {
			mode = 0644
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %s: %w", f.Name, err)
		}
		writeErr := h.writeFileFromReader(dest, rc, mode)
		rc.Close()
		if writeErr != nil {
			return fmt.Errorf("write %s: %w", f.Name, writeErr)
		}
	}
	return nil
}

func (h *WasmHelper) writeFileFromReader(dest string, r io.Reader, mode os.FileMode) error {
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, r)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
