package scheduler

import (
	"os"
	"time"

	"github.com/contactkeval/option-replay/internal/data"
)

var (
	locNY *time.Location
	start time.Time
	end   time.Time

	dataProv data.Provider
)

func init() {
	var err error
	locNY, err = time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}

	start = time.Date(2025, 1, 1, 0, 0, 0, 0, locNY)
	end = time.Date(2026, 1, 1, 0, 0, 0, 0, locNY)

	dataProv = getMassiveDataProvider()
}

func getLocalFileDataProvider() data.Provider {
	dataProv = data.NewMassiveDataProvider(os.Getenv("POLYGON_API_KEY"))
	dataProv = data.NewLocalFileDataProvider("dir", dataProv) // Massive data provider as secondary
	return dataProv
}

func getMassiveDataProvider() data.Provider {
	return data.NewMassiveDataProvider(os.Getenv("POLYGON_API_KEY"))
}
