package logging

import (
	"os"
	"path/filepath"
	"testing"
)

// Test rotateCompressed with various scenarios
func TestRotateCompressed(t *testing.T) {
	t.Run("empty path with no file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.log")
		
		err := rotateCompressed(path, 3)
		if err != nil {
			t.Errorf("rotateCompressed() error = %v", err)
		}
	})

	t.Run("single file no backups", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.log")
		
		// Create initial log file
		os.WriteFile(path, []byte("log content"), 0644)
		
		err := rotateCompressed(path, 0)
		if err != nil {
			t.Errorf("rotateCompressed() error = %v", err)
		}
		
		// Original file should be removed
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Error("Original file should be removed")
		}
	})

	t.Run("with backups rotation", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.log")
		
		// Create initial log file
		os.WriteFile(path, []byte("latest log"), 0644)
		
		err := rotateCompressed(path, 3)
		if err != nil {
			t.Errorf("rotateCompressed() error = %v", err)
		}
		
		// Check compressed backup exists
		backup1 := backupPath(path, 1, true)
		if !fileExists(backup1) {
			t.Error("Backup file should exist")
		}
	})

	t.Run("multiple rotations", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.log")
		
		// Rotate multiple times
		for i := 0; i < 5; i++ {
			os.WriteFile(path, []byte("log content"), 0644)
			err := rotateCompressed(path, 3)
			if err != nil {
				t.Errorf("rotateCompressed() iteration %d error = %v", i, err)
			}
		}
	})
}

// Test compressTo with various scenarios
func TestCompressTo(t *testing.T) {
	t.Run("valid file compression", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcPath := filepath.Join(tmpDir, "test.log")
		dstPath := filepath.Join(tmpDir, "test.log.1.gz")
		
		content := []byte("test log content for compression")
		os.WriteFile(srcPath, content, 0644)
		
		err := compressTo(srcPath, dstPath)
		if err != nil {
			t.Errorf("compressTo() error = %v", err)
		}
		
		// Check compressed file exists
		if !fileExists(dstPath) {
			t.Error("Compressed file should exist")
		}
		
		// Check source file still exists (caller should remove)
		if !fileExists(srcPath) {
			t.Error("Source file should still exist")
		}
	})

	t.Run("non-existent source", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcPath := filepath.Join(tmpDir, "nonexistent.log")
		dstPath := filepath.Join(tmpDir, "test.log.1.gz")
		
		err := compressTo(srcPath, dstPath)
		if err == nil {
			t.Error("compressTo should return error for non-existent source")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcPath := filepath.Join(tmpDir, "empty.log")
		dstPath := filepath.Join(tmpDir, "empty.log.1.gz")
		
		os.WriteFile(srcPath, []byte{}, 0644)
		
		err := compressTo(srcPath, dstPath)
		if err != nil {
			t.Errorf("compressTo() error = %v", err)
		}
		
		if !fileExists(dstPath) {
			t.Error("Compressed file should exist")
		}
	})

	t.Run("large file", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcPath := filepath.Join(tmpDir, "large.log")
		dstPath := filepath.Join(tmpDir, "large.log.1.gz")
		
		// Create 1MB content
		content := make([]byte, 1024*1024)
		for i := range content {
			content[i] = byte('a' + (i % 26))
		}
		os.WriteFile(srcPath, content, 0644)
		
		err := compressTo(srcPath, dstPath)
		if err != nil {
			t.Errorf("compressTo() error = %v", err)
		}
		
		if !fileExists(dstPath) {
			t.Error("Compressed file should exist")
		}
	})
}
