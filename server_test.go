package lfsd_test

import (
	"bytes"
	"log"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gorilla/handlers"
	"github.com/wzshiming/lfsd"
	"github.com/wzshiming/lfsd/content"
	"github.com/wzshiming/lfsd/meta"
)

func runCmd(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func TestServer(t *testing.T) {
	defer os.RemoveAll("lfs")

	contentStore, err := content.NewFS("lfs/content")
	if err != nil {
		t.Fatal(err)
	}

	metaStore, err := meta.NewBolt("lfs/meta.db")
	if err != nil {
		t.Fatal(err)
	}

	server := lfsd.NewServer(
		lfsd.WithContentStore(contentStore),
		lfsd.WithLocksStore(metaStore),
	)

	handler := handlers.LoggingHandler(log.Writer(), server)

	ts := httptest.NewServer(handler)
	defer ts.Close()

	tmpDir, err := os.MkdirTemp("", "lfs-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	bareRepoDir := filepath.Join(tmpDir, "bare.git")
	repoDir := filepath.Join(tmpDir, "repo")
	cloneDir := filepath.Join(tmpDir, "clone")

	// Create bare repository
	if err := runCmd(tmpDir, "git", "init", "--bare", bareRepoDir); err != nil {
		t.Fatal(err)
	}

	// Configure bare repository LFS URL
	if err := runCmd(bareRepoDir, "git", "config", "lfs.url", ts.URL); err != nil {
		t.Fatal(err)
	}

	// Clone bare repository
	if err := runCmd(tmpDir, "git", "clone", bareRepoDir, repoDir); err != nil {
		t.Fatal(err)
	}

	// Configure git user
	if err := runCmd(repoDir, "git", "config", "user.email", "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := runCmd(repoDir, "git", "config", "user.name", "Test User"); err != nil {
		t.Fatal(err)
	}

	// Install git lfs
	if err := runCmd(repoDir, "git", "lfs", "install"); err != nil {
		t.Fatal(err)
	}

	// Configure LFS server URL
	if err := runCmd(repoDir, "git", "config", "lfs.url", ts.URL); err != nil {
		t.Fatal(err)
	}

	// Track large files
	if err := runCmd(repoDir, "git", "lfs", "track", "*.bin"); err != nil {
		t.Fatal(err)
	}

	// Create a test file
	testFile := filepath.Join(repoDir, "test.bin")
	testContent := make([]byte, 1024*1024) // 1MB 文件
	for i := range testContent {
		testContent[i] = byte(i % 256)
	}
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Add and commit files
	if err := runCmd(repoDir, "git", "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := runCmd(repoDir, "git", "commit", "-m", "Add test file"); err != nil {
		t.Fatal(err)
	}

	// Push to remote repository (this will trigger LFS pre-push hook)
	t.Log("Pushing to remote (this will upload LFS objects)...")
	if err := runCmd(repoDir, "git", "push", "-u", "origin", "master"); err != nil {
		t.Fatal("git push failed:", err)
	}

	// Check LFS status
	if err := runCmd(repoDir, "git", "lfs", "status"); err != nil {
		t.Fatal(err)
	}

	// List LFS files
	t.Log("Listing LFS files...")
	if err := runCmd(repoDir, "git", "lfs", "ls-files"); err != nil {
		t.Fatal(err)
	}

	// Clone repository to test download
	t.Log("Cloning repository to test download...")

	// Do not use git clone --recursive because it automatically tries to fetch LFS objects
	// and at this point lfs.url is not configured yet
	if err := runCmd(tmpDir, "git", "clone", "--no-checkout", bareRepoDir, cloneDir); err != nil {
		t.Fatal(err)
	}

	// Configure LFS URL for the cloned repository
	if err := runCmd(cloneDir, "git", "config", "lfs.url", ts.URL); err != nil {
		t.Fatal(err)
	}

	// Perform checkout
	if err := runCmd(cloneDir, "git", "checkout", "master"); err != nil {
		t.Log("Checkout warning:", err)
	}

	// Pull LFS objects
	t.Log("Pulling LFS objects...")
	if err := runCmd(cloneDir, "git", "lfs", "pull"); err != nil {
		t.Fatal("LFS pull failed:", err)
	}

	// Verify file content
	clonedFile := filepath.Join(cloneDir, "test.bin")
	clonedContent, err := os.ReadFile(clonedFile)
	if err != nil {
		t.Fatal("Could not read cloned file:", err)
	}

	if !bytes.Equal(clonedContent, testContent) {
		t.Fatal("File content mismatch!")
	}

	t.Log("File content verified successfully!")
	t.Log("LFS server test completed successfully")
}
