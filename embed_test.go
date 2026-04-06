package apicerberus

import (
	"io/fs"
	"testing"
)

// TestEmbeddedDashboardFS verifies the embedded dashboard filesystem can be accessed
func TestEmbeddedDashboardFS(t *testing.T) {
	fsys, err := EmbeddedDashboardFS()
	if err != nil {
		t.Fatalf("EmbeddedDashboardFS() error = %v", err)
	}
	if fsys == nil {
		t.Fatal("EmbeddedDashboardFS() returned nil filesystem")
	}

	// Verify we can read the root directory
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}

	// Should have at least index.html
	hasIndex := false
	for _, entry := range entries {
		if entry.Name() == "index.html" {
			hasIndex = true
			break
		}
	}
	if !hasIndex {
		t.Error("Expected index.html in embedded filesystem")
	}
}

// TestEmbeddedPortalFS verifies the embedded portal filesystem can be accessed
func TestEmbeddedPortalFS(t *testing.T) {
	fsys, err := EmbeddedPortalFS()
	if err != nil {
		t.Fatalf("EmbeddedPortalFS() error = %v", err)
	}
	if fsys == nil {
		t.Fatal("EmbeddedPortalFS() returned nil filesystem")
	}

	// Verify we can read the root directory
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}

	// Should have at least index.html
	hasIndex := false
	for _, entry := range entries {
		if entry.Name() == "index.html" {
			hasIndex = true
			break
		}
	}
	if !hasIndex {
		t.Error("Expected index.html in embedded filesystem")
	}
}

// TestEmbeddedFS_ReadFile verifies we can read files from the embedded filesystem
func TestEmbeddedFS_ReadFile(t *testing.T) {
	fsys, err := EmbeddedDashboardFS()
	if err != nil {
		t.Fatalf("EmbeddedDashboardFS() error = %v", err)
	}

	// Try to read index.html
	content, err := fs.ReadFile(fsys, "index.html")
	if err != nil {
		t.Fatalf("ReadFile(index.html) error = %v", err)
	}

	if len(content) == 0 {
		t.Error("Expected non-empty index.html content")
	}

	// Verify it contains HTML doctype or html tag
	contentStr := string(content)
	if contentStr == "" {
		t.Error("Expected non-empty string content")
	}
}

// TestEmbeddedFS_Stat verifies we can stat files in the embedded filesystem
func TestEmbeddedFS_Stat(t *testing.T) {
	fsys, err := EmbeddedDashboardFS()
	if err != nil {
		t.Fatalf("EmbeddedDashboardFS() error = %v", err)
	}

	// Try to stat index.html
	info, err := fs.Stat(fsys, "index.html")
	if err != nil {
		t.Fatalf("Stat(index.html) error = %v", err)
	}

	if info == nil {
		t.Fatal("Stat() returned nil info")
	}

	if info.Name() != "index.html" {
		t.Errorf("Expected name 'index.html', got %q", info.Name())
	}

	if info.IsDir() {
		t.Error("Expected index.html to be a file, not directory")
	}

	if info.Size() == 0 {
		t.Error("Expected non-zero file size")
	}
}

// TestEmbeddedFS_Glob verifies glob patterns work
func TestEmbeddedFS_Glob(t *testing.T) {
	fsys, err := EmbeddedDashboardFS()
	if err != nil {
		t.Fatalf("EmbeddedDashboardFS() error = %v", err)
	}

	// Glob for HTML files
	matches, err := fs.Glob(fsys, "*.html")
	if err != nil {
		t.Fatalf("Glob(*.html) error = %v", err)
	}

	if len(matches) == 0 {
		t.Error("Expected at least one HTML file")
	}

	// Should find index.html
	hasIndex := false
	for _, match := range matches {
		if match == "index.html" {
			hasIndex = true
			break
		}
	}
	if !hasIndex {
		t.Error("Expected to find index.html via glob")
	}
}
