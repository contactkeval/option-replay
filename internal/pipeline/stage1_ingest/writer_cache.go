package stage1_ingest

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

type WriterEntry struct {
	File   *os.File
	Writer *bufio.Writer
}

type WriterCache struct {
	root    string
	writers map[string]*WriterEntry
}

func NewWriterCache(
	root string,
	maxOpenFiles int,
) *WriterCache {

	return &WriterCache{
		root:    root,
		writers: make(map[string]*WriterEntry),
	}
}

func (c *WriterCache) Write(
	t config.ParsedTicker,
	line string,
) error {

	dir := filepath.Join(c.root, t.Underlying)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filename := fmt.Sprintf(
		"%s_%s.csv",
		t.Underlying,
		t.ExpiryDate.Format("20060102"),
	)

	path := filepath.Join(dir, filename)

	entry, ok := c.writers[path]

	if !ok {

		file, err := os.OpenFile(
			path,
			os.O_CREATE|os.O_WRONLY|os.O_APPEND,
			0644,
		)

		if err != nil {
			return err
		}

		entry = &WriterEntry{
			File:   file,
			Writer: bufio.NewWriterSize(file, 64*1024),
		}

		c.writers[path] = entry
	}

	_, err := entry.Writer.WriteString(line + "\n")

	return err
}

func (c *WriterCache) CloseAll() error {

	for key, entry := range c.writers {

		if err := entry.Writer.Flush(); err != nil {
			return fmt.Errorf(
				"flush failed for %s: %w",
				key,
				err,
			)
		}

		if err := entry.File.Close(); err != nil {
			return fmt.Errorf(
				"close failed for %s: %w",
				key,
				err,
			)
		}
	}

	c.writers = make(map[string]*WriterEntry)

	return nil
}
