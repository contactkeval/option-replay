package stage2_expiry_rollover

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func Run(cfg config.Config) error {

	today := time.Now().Format("060102")

	return filepath.Walk(cfg.Stage2Root, func(path string, info os.FileInfo, err error) error {

		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".csv") {
			return nil
		}

		base := filepath.Base(path)

		parts := strings.Split(base, "_")

		if len(parts) < 2 {
			return nil
		}

		expiry := strings.TrimSuffix(parts[1], ".csv")

		if expiry >= today {
			return nil
		}

		rel, err := filepath.Rel(cfg.Stage2Root, path)
		if err != nil {
			return err
		}

		destination := filepath.Join(cfg.Stage3Root, rel)

		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			return err
		}

		fmt.Printf("MOVING %s -> %s\n", path, destination)

		return os.Rename(path, destination)
	})
}
