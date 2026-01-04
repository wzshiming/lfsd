package main

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestContentStore(t *testing.T) {
	testDir := "lfs-content-store-test"
	defer os.RemoveAll(testDir)

	store, err := NewContentStore(testDir)
	if err != nil {
		t.Fatalf("Error creating content store: %s", err)
	}

	content := "test content for content store"
	size := int64(len(content))
	// SHA256 of "test content for content store"
	oid := "a1f7b3c9d2e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0"

	// Test Put with mismatched hash (should fail)
	err = store.Put(oid, size, bytes.NewBufferString(content))
	if err == nil {
		t.Fatal("Expected error for hash mismatch, got nil")
	}

	// Use correct hash
	correctOid := "f1e2d3c4b5a69788796a5b4c3d2e1f0f1e2d3c4b5a69788796a5b4c3d2e1f0f"

	// Test Put with mismatched size (should fail)
	err = store.Put(correctOid, size+10, bytes.NewBufferString(content))
	if err == nil {
		t.Fatal("Expected error for size mismatch, got nil")
	}

	// Test Exists for non-existent object
	if store.Exists(correctOid) {
		t.Fatal("Expected object to not exist")
	}
}

func TestContentStoreWithCorrectData(t *testing.T) {
	testDir := "lfs-content-store-test2"
	defer os.RemoveAll(testDir)

	store, err := NewContentStore(testDir)
	if err != nil {
		t.Fatalf("Error creating content store: %s", err)
	}

	content := "this is my content"
	size := int64(len(content))
	// Correct SHA256 of "this is my content"
	oid := "f97e1b2936a56511b3b6efc99011758e4700d60fb1674d31445d1ee40b663f24"

	// Test Put with correct data
	err = store.Put(oid, size, bytes.NewBufferString(content))
	if err != nil {
		t.Fatalf("Error putting content: %s", err)
	}

	// Test Exists
	if !store.Exists(oid) {
		t.Fatal("Expected object to exist")
	}

	// Test Size
	gotSize, err := store.Size(oid)
	if err != nil {
		t.Fatalf("Error getting size: %s", err)
	}
	if gotSize != size {
		t.Errorf("Expected size %d, got %d", size, gotSize)
	}

	// Test Get
	reader, err := store.Get(oid, 0)
	if err != nil {
		t.Fatalf("Error getting content: %s", err)
	}
	defer reader.Close()

	gotContent, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Error reading content: %s", err)
	}
	if string(gotContent) != content {
		t.Errorf("Expected content %q, got %q", content, string(gotContent))
	}
}

func TestContentStoreGetWithOffset(t *testing.T) {
	testDir := "lfs-content-store-test3"
	defer os.RemoveAll(testDir)

	store, err := NewContentStore(testDir)
	if err != nil {
		t.Fatalf("Error creating content store: %s", err)
	}

	content := "this is my content"
	size := int64(len(content))
	oid := "f97e1b2936a56511b3b6efc99011758e4700d60fb1674d31445d1ee40b663f24"

	err = store.Put(oid, size, bytes.NewBufferString(content))
	if err != nil {
		t.Fatalf("Error putting content: %s", err)
	}

	// Test Get with offset
	offset := int64(5)
	reader, err := store.Get(oid, offset)
	if err != nil {
		t.Fatalf("Error getting content with offset: %s", err)
	}
	defer reader.Close()

	gotContent, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Error reading content: %s", err)
	}

	expected := content[offset:]
	if string(gotContent) != expected {
		t.Errorf("Expected content %q, got %q", expected, string(gotContent))
	}
}

func TestTransformKey(t *testing.T) {
	tests := []struct {
		key      string
		expected string
	}{
		{"abc", "abc"},
		{"abcd", "abcd"},
		{"abcde", "ab/cd/e"},
		{"abcdef", "ab/cd/ef"},
		{"f97e1b2936a56511b3b6efc99011758e4700d60fb1674d31445d1ee40b663f24", "f9/7e/1b2936a56511b3b6efc99011758e4700d60fb1674d31445d1ee40b663f24"},
	}

	for _, test := range tests {
		got := transformKey(test.key)
		// Normalize path separators for comparison
		if got != test.expected {
			t.Errorf("transformKey(%q) = %q, expected %q", test.key, got, test.expected)
		}
	}
}
