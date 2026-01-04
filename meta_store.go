package main

import (
	"errors"
	"sync"
)

var (
	errObjectNotFound = errors.New("object not found")
)

// MetaObject is object metadata.
type MetaObject struct {
	Oid  string `json:"oid"`
	Size int64  `json:"size"`
}

// MetaStore implements an in-memory metadata storage.
type MetaStore struct {
	mu      sync.RWMutex
	objects map[string]*MetaObject
}

// NewMetaStore creates a new MetaStore.
func NewMetaStore() *MetaStore {
	return &MetaStore{
		objects: make(map[string]*MetaObject),
	}
}

// Get retrieves the metadata for an object.
func (s *MetaStore) Get(oid string) (*MetaObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	meta, ok := s.objects[oid]
	if !ok {
		return nil, errObjectNotFound
	}
	return meta, nil
}

// Put stores the metadata for an object.
// Returns true if the object already existed.
func (s *MetaStore) Put(oid string, size int64) (*MetaObject, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.objects[oid]; ok {
		return existing, true
	}

	meta := &MetaObject{Oid: oid, Size: size}
	s.objects[oid] = meta
	return meta, false
}

// Delete removes the metadata for an object.
func (s *MetaStore) Delete(oid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, oid)
}
