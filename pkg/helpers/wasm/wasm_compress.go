package helpers

import (
	"compress/gzip"
	"fmt"
	"os"

	"github.com/andybalholm/brotli"
)

// Compression utilities: wrap a built .wasm into .wasm.gz or .wasm.br, plus
// size-formatting helpers used by the build pipeline's progress output.

// compressionExt returns the suffix appended after ".wasm" (".gz" or ".br").
func compressionExt(c WasmCompression) string {
	if c == WasmCompressionBrotli {
		return ".br"
	}
	return ".gz"
}

func compressionLabel(c WasmCompression) string {
	if c == WasmCompressionBrotli {
		return "brotli"
	}
	return "gzip"
}

func (h *WasmHelper) compressWasmWith(src, dst string, c WasmCompression) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	if c == WasmCompressionBrotli {
		w := brotli.NewWriterLevel(f, brotli.BestCompression)
		if _, err := w.Write(in); err != nil {
			w.Close()
			return err
		}
		return w.Close()
	}
	w, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		return err
	}
	if _, err := w.Write(in); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

func (h *WasmHelper) fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (h *WasmHelper) formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	return fmt.Sprintf("%dKB", n/1024)
}
