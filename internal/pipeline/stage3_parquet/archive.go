package stage3_parquet

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func ArchiveFile(
	sourcePath string,
	archivePath string,
) error {

	if err := os.MkdirAll(
		filepath.Dir(archivePath),
		0755,
	); err != nil {

		return fmt.Errorf(
			"create archive directory %s: %w",
			filepath.Dir(archivePath),
			err,
		)
	}

	src, err := os.Open(sourcePath)
	if err != nil {

		return fmt.Errorf(
			"open source file %s: %w",
			sourcePath,
			err,
		)
	}

	dst, err := os.Create(archivePath)
	if err != nil {

		src.Close()

		return fmt.Errorf(
			"create archive file %s: %w",
			archivePath,
			err,
		)
	}

	gz := gzip.NewWriter(dst)
	if _, err := io.Copy(gz, src); err != nil {

		gz.Close()
		dst.Close()
		src.Close()

		return fmt.Errorf(
			"gzip copy source=%s archive=%s: %w",
			sourcePath,
			archivePath,
			err,
		)
	}

	if err := gz.Close(); err != nil {
		dst.Close()
		src.Close()
		return fmt.Errorf(
			"close gzip writer archive=%s: %w",
			archivePath,
			err,
		)
	}

	if err := dst.Close(); err != nil {
		src.Close()
		return fmt.Errorf(
			"close archive file %s: %w",
			archivePath,
			err,
		)
	}

	if err := src.Close(); err != nil {
		return fmt.Errorf(
			"close source file %s: %w",
			sourcePath,
			err,
		)
	}

	if err := os.Remove(sourcePath); err != nil {
		return fmt.Errorf(
			"remove source file %s: %w",
			sourcePath,
			err,
		)
	}

	return nil
}
