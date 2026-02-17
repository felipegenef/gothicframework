package helpers

import (
	"bytes"
	"compress/gzip"
	"io"

	"github.com/andybalholm/brotli"
)

func compressData(data []byte, method CompressionMethod) ([]byte, error) {
	var buf bytes.Buffer
	switch method {
	case BROTLI:
		w := brotli.NewWriter(&buf)
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
	default: // GZIP
		w := gzip.NewWriter(&buf)
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func decompressData(data []byte, method CompressionMethod) ([]byte, error) {
	switch method {
	case BROTLI:
		r := brotli.NewReader(bytes.NewReader(data))
		return io.ReadAll(r)
	default: // GZIP
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(r)
	}
}
