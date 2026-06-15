package stage4_compact

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/contactkeval/option-replay/internal/logger"
)

func RecoverCompactions(
	parquetRoot string,
) error {

	err := filepath.Walk(
		parquetRoot,
		func(
			path string,
			info os.FileInfo,
			err error,
		) error {

			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			// ---------------------------------
			// delete *.compacting
			// ---------------------------------

			if strings.HasSuffix(
				path,
				".parquet.compacting",
			) {

				logger.Infof(
					"recover delete compacting=%s",
					path,
				)

				return os.Remove(path)
			}

			// ---------------------------------
			// rename *.pending -> *.parquet
			// ---------------------------------

			if strings.HasSuffix(
				path,
				".parquet.pending",
			) {

				original := strings.TrimSuffix(
					path,
					".pending",
				)

				logger.Infof(
					"recover restore pending=%s",
					original,
				)

				return os.Rename(
					path,
					original,
				)
			}

			return nil
		},
	)

	if err != nil {
		return fmt.Errorf(
			"recover compactions: %w",
			err,
		)
	}

	return nil
}
