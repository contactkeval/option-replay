package staging

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type CachedWriter struct {
	File   *os.File
	Writer *bufio.Writer
}

type WriterCache struct {
	RootDir string
	mu      sync.Mutex
	writers map[string]*CachedWriter
}

func NewWriterCache(root string) *WriterCache {
	return &WriterCache{
		RootDir: root,
		writers: make(map[string]*CachedWriter),
	}
}

func (wc *WriterCache) getPath(t *ParsedTicker) string {

	dir := filepath.Join(
		wc.RootDir,
		t.Underlying,
	)

	filename := fmt.Sprintf(
		"%s_%s.csv",
		t.Underlying,
		t.Expiry,
	)

	return filepath.Join(dir, filename)
}

func (wc *WriterCache) Write(t *ParsedTicker, line string) error {

	wc.mu.Lock()
	defer wc.mu.Unlock()

	path := wc.getPath(t)

	cw, exists := wc.writers[path]

	if !exists {

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		file, err := os.OpenFile(
			path,
			os.O_CREATE|os.O_WRONLY|os.O_APPEND,
			0644,
		)

		if err != nil {
			return err
		}

		writer := bufio.NewWriter(file)

		cw = &CachedWriter{
			File:   file,
			Writer: writer,
		}

		wc.writers[path] = cw
	}

	_, err := cw.Writer.WriteString(line + "\n")

	return err
}

func (wc *WriterCache) Close() error {

	wc.mu.Lock()
	defer wc.mu.Unlock()

	for _, cw := range wc.writers {

		if err := cw.Writer.Flush(); err != nil {
			return err
		}

		if err := cw.File.Close(); err != nil {
			return err
		}
	}

	wc.writers = make(map[string]*CachedWriter)
	return nil
}
