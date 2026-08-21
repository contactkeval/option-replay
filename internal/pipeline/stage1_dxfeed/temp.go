package stage1_dxfeed

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
	"github.com/contactkeval/option-replay/internal/pipeline/config"
)

func loadSymbols(path string) ([]string, error) {

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var symbols []string

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {

		s := strings.TrimSpace(scanner.Text())

		if s == "" {
			continue
		}

		symbols = append(symbols, s+"{=1m}")
	}

	return symbols, scanner.Err()
}

func Run3() {

	ctx := context.Background()

	dxLinkURL := "wss://tasty-openapi-dxlink-md-ws.dxfeed.com/realtime"
	dxToken := os.Getenv("dxFeed_Token")

	symbols, err := loadSymbols("D:\\Documents\\code\\go\\option-replay\\input\\data\\dxFeedSPY260606.txt")
	if err != nil {
		log.Fatal(err)
	}

	symbols = symbols[:250]
	logger.Infof(
		"Loaded %d symbols",
		len(symbols),
	)

	requested := make(map[string]struct{})
	received := make(map[string]struct{})

	for _, s := range symbols {
		requested[s] = struct{}{}
	}

	csvFile, err := os.Create("dxfeed_output.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer csvFile.Close()

	writer := csv.NewWriter(csvFile)
	defer writer.Flush()

	err = writer.Write([]string{
		"event_symbol",
		"time",
		"open",
		"high",
		"low",
		"close",
		"volume",
	})
	if err != nil {
		log.Fatal(err)
	}

	fromTime := time.Now().
		Add(-10 * 24 * time.Hour).
		UnixMilli()

	client, err := Connect(
		ctx,
		dxLinkURL,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	if err := client.Setup(); err != nil {
		log.Fatal(err)
	}

	if err := client.Auth(dxToken); err != nil {
		log.Fatal(err)
	}

	if err := client.WaitForAuth(ctx); err != nil {
		log.Fatal(err)
	}

	if err := client.OpenFeedChannel(); err != nil {
		log.Fatal(err)
	}

	if err := client.WaitForChannel(
		ctx,
		1,
	); err != nil {
		log.Fatal(err)
	}

	if err := client.SubscribeCandles(
		symbols,
		fromTime,
	); err != nil {
		log.Fatal(err)
	}

	logger.Infof("Waiting for data...")

	err = client.ReadLoop(
		ctx,
		func(candle config.Candle) error {

			received[candle.EventSymbol] = struct{}{}

			record := []string{
				candle.EventSymbol,
				fmt.Sprintf("%d", candle.Time),
				fmt.Sprintf("%f", candle.Open),
				fmt.Sprintf("%f", candle.High),
				fmt.Sprintf("%f", candle.Low),
				fmt.Sprintf("%f", candle.Close),
				fmt.Sprintf("%f", candle.Volume),
			}

			if err := writer.Write(record); err != nil {
				return err
			}

			if len(received)%100 == 0 {
				logger.Debugf(
					"Received symbols: %d",
					len(received),
				)
				// flush periodically so file isn't lost if process dies
				writer.Flush()

				if err := writer.Error(); err != nil {
					return err
				}
			}

			return nil
		},
	)

	if err != nil {
		logger.Warnf("ReadLoop ended: %v", err)
	}

	logger.Infof("Subscribed Symbols : %d", len(requested))
	logger.Infof("Returned Symbols   : %d", len(received))

	missing := 0

	for symbol := range requested {

		if _, ok := received[symbol]; !ok {
			missing++
		}
	}

	logger.Infof(
		"Missing Symbols    : %d",
		missing,
	)
}
