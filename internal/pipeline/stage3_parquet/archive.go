package stage3_parquet

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
)

func ArchiveStage3File(
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

	gz.Close()
	dst.Close()
	src.Close()

	return os.Remove(sourcePath)
}
