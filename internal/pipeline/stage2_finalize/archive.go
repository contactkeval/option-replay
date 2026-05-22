package stage2_finalize

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
)

func ArchiveStage2File(
	sourcePath string,
	archivePath string,
) error {

	if err := os.MkdirAll(
		filepath.Dir(archivePath),
		0755,
	); err != nil {
		return err
	}

	src, err := os.Open(sourcePath)
	if err != nil {
		return err
	}

	dst, err := os.Create(archivePath)
	if err != nil {
		src.Close()
		return err
	}

	gz := gzip.NewWriter(dst)

	if _, err := io.Copy(gz, src); err != nil {

		gz.Close()
		dst.Close()
		src.Close()

		return err
	}

	if err := gz.Close(); err != nil {
		dst.Close()
		src.Close()
		return err
	}

	if err := dst.Close(); err != nil {
		src.Close()
		return err
	}

	if err := src.Close(); err != nil {
		return err
	}

	return os.Remove(sourcePath)
}
