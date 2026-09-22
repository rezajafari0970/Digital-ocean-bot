package sanaei

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
)

var ErrInvalidTemplate = errors.New("invalid x-ui database template")

type DatabaseTemplate struct {
	ID      string
	Name    string
	Version int
	Path    string
	SHA256  string
	Size    int64
}

func InspectTemplate(path string) (DatabaseTemplate, error) {
	f, err := os.Open(path)
	if err != nil {
		return DatabaseTemplate{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() < 16 {
		return DatabaseTemplate{}, ErrInvalidTemplate
	}
	header := make([]byte, 16)
	if _, err := io.ReadFull(f, header); err != nil {
		return DatabaseTemplate{}, ErrInvalidTemplate
	}
	if string(header) != "SQLite format 3\x00" {
		return DatabaseTemplate{}, ErrInvalidTemplate
	}
	if _, err := f.Seek(0, 0); err != nil {
		return DatabaseTemplate{}, err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return DatabaseTemplate{}, err
	}
	return DatabaseTemplate{Path: path, SHA256: hex.EncodeToString(h.Sum(nil)), Size: info.Size()}, nil
}
