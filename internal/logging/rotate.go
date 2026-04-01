package logging

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type rotatingFileWriter struct {
	mu         sync.Mutex
	path       string
	maxSize    int64
	maxBackups int
	compress   bool
	file       *os.File
	size       int64
}

func newRotatingFileWriter(path string, maxSize int64, maxBackups int, compress bool) (*rotatingFileWriter, error) {
	if maxSize <= 0 {
		maxSize = 100 * 1024 * 1024
	}
	if maxBackups <= 0 {
		maxBackups = 7
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	return &rotatingFileWriter{
		path:       path,
		maxSize:    maxSize,
		maxBackups: maxBackups,
		compress:   compress,
		file:       f,
		size:       info.Size(),
	}, nil
}

func (w *rotatingFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return 0, fmt.Errorf("writer is closed")
	}

	if w.size+int64(len(p)) > w.maxSize {
		if err := w.rotateLocked(); err != nil {
			return 0, err
		}
	}

	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	w.size = 0
	return err
}

func (w *rotatingFileWriter) rotateLocked() error {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
	}

	if w.compress {
		if err := rotateCompressed(w.path, w.maxBackups); err != nil {
			return err
		}
	} else {
		if err := rotatePlain(w.path, w.maxBackups); err != nil {
			return err
		}
	}

	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w.file = f
	w.size = 0
	return nil
}

func rotatePlain(path string, maxBackups int) error {
	if maxBackups > 0 {
		_ = os.Remove(backupPath(path, maxBackups, false))
		for i := maxBackups - 1; i >= 1; i-- {
			src := backupPath(path, i, false)
			dst := backupPath(path, i+1, false)
			if fileExists(src) {
				if err := os.Rename(src, dst); err != nil {
					return err
				}
			}
		}
	}

	if fileExists(path) {
		return os.Rename(path, backupPath(path, 1, false))
	}
	return nil
}

func rotateCompressed(path string, maxBackups int) error {
	if maxBackups > 0 {
		_ = os.Remove(backupPath(path, maxBackups, true))
		for i := maxBackups - 1; i >= 1; i-- {
			src := backupPath(path, i, true)
			dst := backupPath(path, i+1, true)
			if fileExists(src) {
				if err := os.Rename(src, dst); err != nil {
					return err
				}
			}
		}
	}

	if !fileExists(path) {
		return nil
	}

	tmp := backupPath(path, 1, false)
	if err := os.Rename(path, tmp); err != nil {
		return err
	}
	defer os.Remove(tmp)

	return compressTo(tmp, backupPath(path, 1, true))
}

func compressTo(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer dst.Close()

	gzw := gzip.NewWriter(dst)
	if _, err := io.Copy(gzw, src); err != nil {
		_ = gzw.Close()
		return err
	}
	return gzw.Close()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func backupPath(path string, idx int, compressed bool) string {
	if compressed {
		return fmt.Sprintf("%s.%d.gz", path, idx)
	}
	return fmt.Sprintf("%s.%d", path, idx)
}
