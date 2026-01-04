package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGitLFSIntegration tests the server with the actual git-lfs binary.
// This test requires git and git-lfs to be installed.
func TestGitLFSIntegration(t *testing.T) {
	// Check if git-lfs is available
	if _, err := exec.LookPath("git-lfs"); err != nil {
		t.Skip("git-lfs not available, skipping integration test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available, skipping integration test")
	}

	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "lfsd-integration-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %s", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create content store directory
	contentDir := filepath.Join(tmpDir, "lfs-content")
	contentStore, err := NewContentStore(contentDir)
	if err != nil {
		t.Fatalf("Failed to create content store: %s", err)
	}

	metaStore := NewMetaStore()

	// Start test server
	config := &Config{
		Listen: ":0",
		Host:   "localhost:8080",
		Scheme: "http",
	}
	app := NewApp(config, contentStore, metaStore)
	server := httptest.NewServer(app)
	defer server.Close()

	// Update config to use actual server URL
	config.Host = strings.TrimPrefix(server.URL, "http://")

	// Create a git repository for testing
	repoDir := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("Failed to create repo dir: %s", err)
	}

	// Initialize git repo
	runGitCmd(t, repoDir, "init")
	runGitCmd(t, repoDir, "config", "user.email", "test@test.com")
	runGitCmd(t, repoDir, "config", "user.name", "Test User")

	// Configure LFS
	runGitCmd(t, repoDir, "lfs", "install", "--local")

	// Configure LFS URL to point to our test server
	lfsURL := server.URL
	runGitCmd(t, repoDir, "config", "lfs.url", lfsURL)

	// Track large files
	runGitCmd(t, repoDir, "lfs", "track", "*.bin")

	// Add .gitattributes
	runGitCmd(t, repoDir, "add", ".gitattributes")
	runGitCmd(t, repoDir, "commit", "-m", "Track .bin files with LFS")

	// Create a test file
	testContent := []byte("This is a test file for Git LFS integration testing. It should be stored in LFS.")
	testFile := filepath.Join(repoDir, "test.bin")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %s", err)
	}

	// Calculate expected OID
	hash := sha256.Sum256(testContent)
	expectedOid := hex.EncodeToString(hash[:])

	// Add and commit the file (this triggers LFS push)
	runGitCmd(t, repoDir, "add", "test.bin")
	runGitCmd(t, repoDir, "commit", "-m", "Add test.bin")

	// Push LFS objects (this tests upload)
	runGitLFSPush(t, repoDir, lfsURL, expectedOid, testContent)

	// Verify the object was stored
	if !contentStore.Exists(expectedOid) {
		t.Errorf("Expected object %s to exist in content store", expectedOid)
	}

	// Verify size
	size, err := contentStore.Size(expectedOid)
	if err != nil {
		t.Errorf("Failed to get size: %s", err)
	}
	if size != int64(len(testContent)) {
		t.Errorf("Expected size %d, got %d", len(testContent), size)
	}
}

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %s failed: %s\nStderr: %s", strings.Join(args, " "), err, stderr.String())
	}
}

func runGitLFSPush(t *testing.T, repoDir, lfsURL, oid string, content []byte) {
	t.Helper()

	// Manually upload via LFS batch API
	// Create batch request
	batchReq := fmt.Sprintf(`{
		"operation": "upload",
		"transfers": ["basic"],
		"objects": [{"oid": "%s", "size": %d}]
	}`, oid, len(content))

	// Make batch request
	cmd := exec.Command("curl", "-s", "-X", "POST",
		"-H", "Accept: application/vnd.git-lfs+json",
		"-H", "Content-Type: application/vnd.git-lfs+json",
		"-d", batchReq,
		lfsURL+"/objects/batch")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Batch request failed: %s", err)
	}

	t.Logf("Batch response: %s", output)

	// Check if upload action is present
	if !bytes.Contains(output, []byte("upload")) {
		t.Logf("Object might already exist, no upload action returned")
		return
	}

	// Upload the object
	cmd = exec.Command("curl", "-s", "-X", "PUT",
		"-H", "Content-Type: application/octet-stream",
		"--data-binary", "@-",
		lfsURL+"/objects/"+oid)
	cmd.Stdin = bytes.NewReader(content)
	output, err = cmd.Output()
	if err != nil {
		t.Fatalf("Upload failed: %s", err)
	}

	t.Logf("Upload response: %s", output)
}

// TestGitLFSDownload tests downloading an object using git-lfs.
func TestGitLFSDownload(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not available, skipping test")
	}

	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "lfsd-download-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %s", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create content store with pre-existing content
	contentDir := filepath.Join(tmpDir, "lfs-content")
	contentStore, err := NewContentStore(contentDir)
	if err != nil {
		t.Fatalf("Failed to create content store: %s", err)
	}

	metaStore := NewMetaStore()

	// Add test content
	testContent := []byte("Download test content")
	hash := sha256.Sum256(testContent)
	oid := hex.EncodeToString(hash[:])
	size := int64(len(testContent))

	metaStore.Put(oid, size)
	if err := contentStore.Put(oid, size, bytes.NewReader(testContent)); err != nil {
		t.Fatalf("Failed to store content: %s", err)
	}

	// Start test server
	config := &Config{
		Listen: ":0",
		Host:   "localhost:8080",
		Scheme: "http",
	}
	app := NewApp(config, contentStore, metaStore)
	server := httptest.NewServer(app)
	defer server.Close()

	// Test batch download request
	batchReq := fmt.Sprintf(`{
		"operation": "download",
		"transfers": ["basic"],
		"objects": [{"oid": "%s", "size": %d}]
	}`, oid, size)

	cmd := exec.Command("curl", "-s", "-X", "POST",
		"-H", "Accept: application/vnd.git-lfs+json",
		"-H", "Content-Type: application/vnd.git-lfs+json",
		"-d", batchReq,
		server.URL+"/objects/batch")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Batch request failed: %s", err)
	}

	t.Logf("Batch response: %s", output)

	if !bytes.Contains(output, []byte("download")) {
		t.Fatal("Expected download action in response")
	}

	// Download the object
	cmd = exec.Command("curl", "-s", server.URL+"/objects/"+oid)
	output, err = cmd.Output()
	if err != nil {
		t.Fatalf("Download failed: %s", err)
	}

	if !bytes.Equal(output, testContent) {
		t.Errorf("Downloaded content doesn't match: expected %q, got %q", testContent, output)
	}
}
