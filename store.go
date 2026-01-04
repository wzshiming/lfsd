package lfsd

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

// Download retrieves the content from the store.
// If fromByte > 0, the reader starts from that byte.
func (s *ContentStore) Download(w http.ResponseWriter, r *http.Request, oid string) {
	path := filepath.Join(s.basePath, transformKey(oid))
	info, err := os.Stat(path)
	if err != nil {
		http.Error(w, "Object not found", http.StatusNotFound)
		return
	}

	file, err := os.Open(path)
	if err != nil {
		http.Error(w, "Object not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	http.ServeContent(w, r, oid, info.ModTime(), file)
}

// Upload writes the content to the store, verifying hash and size.
func (s *ContentStore) Upload(w http.ResponseWriter, r *http.Request, oid string) {
	if r.ContentLength <= 0 {
		http.Error(w, "Invalid content length", http.StatusBadRequest)
		return
	}

	path := filepath.Join(s.basePath, transformKey(oid))
	tmpPath := path + ".tmp"

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		http.Error(w, "Failed to create directory", http.StatusInternalServerError)
		return
	}

	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0640)
	if err != nil {
		http.Error(w, "Failed to create file", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpPath)

	hash := sha256.New()
	hw := io.MultiWriter(hash, file)

	written, err := io.Copy(hw, r.Body)
	if err != nil {
		file.Close()
		http.Error(w, "Failed to write content", http.StatusInternalServerError)
		return
	}
	file.Close()

	if written != r.ContentLength {
		http.Error(w, "Content length mismatch", http.StatusBadRequest)
		return
	}

	shaStr := hex.EncodeToString(hash.Sum(nil))
	if shaStr != oid {
		http.Error(w, "Content hash does not match OID", http.StatusBadRequest)
		return
	}

	if err := os.Rename(tmpPath, path); err != nil {
		http.Error(w, "Failed to rename file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *ContentStore) Verify(w http.ResponseWriter, r *http.Request, oid string, size int64) {
	path := filepath.Join(s.basePath, transformKey(oid))
	info, err := os.Stat(path)
	if err != nil {
		http.Error(w, "Object not found", http.StatusNotFound)
		return
	}

	// Verify that the object exists and has correct size
	if !s.Exists(oid) {
		http.Error(w, "Object not found", http.StatusNotFound)
		return
	}

	if info.Size() != size {
		http.Error(w, "Size mismatch", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// Exists returns true if the object exists in the content store.
func (s *ContentStore) Exists(oid string) bool {
	path := filepath.Join(s.basePath, transformKey(oid))
	_, err := os.Stat(path)
	return err == nil
}

// transformKey transforms an OID into a path with directory structure.
func transformKey(key string) string {
	if len(key) < 5 {
		return key
	}
	return filepath.Join(key[0:2], key[2:4], key[4:])
}
