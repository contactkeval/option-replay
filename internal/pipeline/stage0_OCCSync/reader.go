package stage0_occ

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/contactkeval/option-replay/internal/db"
)

// ParseErrorHandler is called for lines that fail to parse.
// Return a non-nil error to abort the read; return nil to skip and continue.
type ParseErrorHandler func(lineNo int, line string, err error) error

func ReadFile(
	filename string,
	handler func(db.OCCRecord) error,
	onParseErr ParseErrorHandler,
) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("open OCC file: %w", err)
	}
	defer file.Close()

	return Read(file, handler, onParseErr)
}

func Read(
	r io.Reader,
	handler func(db.OCCRecord) error,
	onParseErr ParseErrorHandler,
) error {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 1024)
	scanner.Buffer(buf, 64*1024)

	lineNo := 0
	for scanner.Scan() {
		lineNo++

		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		record, err := ParseRecord(line)
		if err != nil {
			if onParseErr == nil {
				return fmt.Errorf("line %d: %w", lineNo, err)
			}
			if herr := onParseErr(lineNo, line, err); herr != nil {
				return herr
			}
			continue
		}

		if err := handler(record); err != nil {
			return fmt.Errorf("line %d: %w", lineNo, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan OCC file: %w", err)
	}

	return nil
}
