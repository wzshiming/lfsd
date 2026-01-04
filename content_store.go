package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
)

var (
	errHashMismatch = errors.New("content hash does not match OID")
	errSizeMismatch = errors.New("content size does not match")
)

// ContentStore provides a simple file system based storage.
type ContentStore struct {
	basePath string
}

// NewContentStore creates a ContentStore at the base directory.
func NewContentStore(base string) (*ContentStore, error) {
	if err := os.MkdirAll(base, 0750); err != nil {
		return nil, err
	}
	return &ContentStore{basePath: base}, nil
}

// Get retrieves the content from the store.
// If fromByte > 0, the reader starts from that byte.
func (s *ContentStore) Get(oid string, fromByte int64) (io.ReadCloser, error) {
	path := filepath.Join(s.basePath, transformKey(oid))

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if fromByte > 0 {
		_, err = f.Seek(fromByte, io.SeekCurrent)
		if err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

// Put writes the content to the store, verifying hash and size.
func (s *ContentStore) Put(oid string, size int64, r io.Reader) error {
	path := filepath.Join(s.basePath, transformKey(oid))
	tmpPath := path + ".tmp"

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}

	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0640)
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath)

	hash := sha256.New()
	hw := io.MultiWriter(hash, file)

	written, err := io.Copy(hw, r)
	if err != nil {
		file.Close()
		return err
	}
	file.Close()

	if written != size {
		return errSizeMismatch
	}

	shaStr := hex.EncodeToString(hash.Sum(nil))
	if shaStr != oid {
		return errHashMismatch
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return nil
}

// Exists returns true if the object exists in the content store.
func (s *ContentStore) Exists(oid string) bool {
	path := filepath.Join(s.basePath, transformKey(oid))
	_, err := os.Stat(path)
	return err == nil
}

// Size returns the size of the object in bytes.
func (s *ContentStore) Size(oid string) (int64, error) {
	path := filepath.Join(s.basePath, transformKey(oid))
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// transformKey transforms an OID into a path with directory structure.
func transformKey(key string) string {
	if len(key) < 5 {
		return key
	}
	return filepath.Join(key[0:2], key[2:4], key[4:])
}
