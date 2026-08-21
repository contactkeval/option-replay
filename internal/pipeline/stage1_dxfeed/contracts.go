package stage1_dxfeed

import (
	"os"
	"path/filepath"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
	"github.com/contactkeval/option-replay/internal/db"
	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func LoadContractsToSQLite(cfg config.Config) error {
	metadataDBPath := filepath.Join(cfg.MetadataRoot, "metadata.db")

	database, err := db.Open(db.Options{
		Path:    metadataDBPath,
		Schemas: db.SchemaContracts,
	})
	if err != nil {
		return err
	}
	defer database.Close()

	provider := data.NewMassiveDataProvider(os.Getenv("MASSIVE_API_KEY"))

	totalContracts := 0
	totalUnderlyings := 0
	today := time.Now()

	for underlying := range config.AllowedUnderlyings {
		totalUnderlyings++

		contracts, err := provider.GetContracts(
			underlying,
			0,
			time.Time{},
			time.Time{},
			false,
		)
		if err != nil {
			logger.Errorf("FAILED %s: %v", underlying, err)
			continue
		}

		logger.Infof("%-10s -> %6d contracts", underlying, len(contracts))

		tx, err := database.Begin()
		if err != nil {
			return err
		}

		for _, c := range contracts {
			if err := database.InsertContractIgnore(
				tx,
				underlying,
				c.ExpiryDate,
				c.Strike,
				c.Type,
				0,
				today,
			); err != nil {
				tx.Rollback()
				return err
			}

			totalContracts++
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	uniqueContracts, err := database.CountContracts()
	if err != nil {
		return err
	}

	logger.Infof("Underlyings processed : %d", totalUnderlyings)
	logger.Infof("Contracts returned    : %d", totalContracts)
	logger.Infof("Unique contracts      : %d", uniqueContracts)

	return nil
}
