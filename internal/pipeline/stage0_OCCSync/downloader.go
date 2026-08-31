package stage0_occ

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/contactkeval/option-replay/internal/logger"
)

const (
	occSeriesURL = "https://marketdata.theocc.com/series-download" +
		"?exchanges=18,26,25,01,35,23,17,19,16,06,02,04,22,08,55,12,11,03,20,13,07,05,09" +
		"&downloadType=%s&dates=%s"

	maxDownloadAttempts = 3
	retryDelay          = time.Second
	emptyBodyMarker     = "No record(s) found"
)

type Downloader struct {
	httpClient *http.Client
	baseDir    string
}

func NewDownloader(baseDir string) *Downloader {
	return &Downloader{
		baseDir: baseDir,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (d *Downloader) buildURL(importDate time.Time, downloadType string) string {
	return fmt.Sprintf(
		occSeriesURL,
		strings.ToUpper(downloadType),
		importDate.Format("01/02/2006"),
	)
}

func (d *Downloader) buildFilename(importDate time.Time, downloadType string) string {
	return filepath.Join(
		d.baseDir,
		fmt.Sprintf("%04d", importDate.Year()),
		fmt.Sprintf(
			"%s_%s.txt",
			importDate.Format("20060102"),
			strings.ToUpper(downloadType),
		),
	)
}

// Download fetches one OCC series file for the given date and download type (A/D/M).
// If the remote has no records, it returns ("", nil).
// Existing local files are reused.
func (d *Downloader) Download(
	ctx context.Context,
	importDate time.Time,
	downloadType string,
) (string, error) {
	downloadType = strings.ToUpper(strings.TrimSpace(downloadType))
	switch downloadType {
	case ActionAdd, ActionDelete, ActionModify, ActionBoth:
	default:
		return "", fmt.Errorf("invalid download type %q", downloadType)
	}

	filename := d.buildFilename(importDate, downloadType)

	if info, err := os.Stat(filename); err == nil {
		if info.Size() == 0 {
			return "", nil
		}
		logger.Infof("OCC file already exists: %s", filename)
		return filename, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	url := d.buildURL(importDate, downloadType)

	var lastErr error
	for attempt := 1; attempt <= maxDownloadAttempts; attempt++ {
		if attempt > 1 {
			logger.Warnf(
				"download failed (%v); retrying (%d/%d)",
				lastErr,
				attempt,
				maxDownloadAttempts,
			)
		}

		path, empty, err := d.downloadToFile(ctx, url, filename)
		if err == nil {
			if empty {
				logger.Infof(
					"OCC %s for %s: no records",
					downloadType,
					importDate.Format("2006-01-02"),
				)
				return "", nil
			}
			return path, nil
		}

		lastErr = err
		if attempt < maxDownloadAttempts {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(retryDelay):
			}
		}
	}

	return "", lastErr
}

func (d *Downloader) downloadToFile(
	ctx context.Context,
	url string,
	filename string,
) (path string, empty bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", false, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "OptionReplay/1.0")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, fmt.Errorf("read body: %w", err)
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" || trimmed == emptyBodyMarker {
		return "", true, nil
	}

	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		return "", false, fmt.Errorf("create directory: %w", err)
	}

	tmpFilename := filename + ".tmp"
	if err := os.WriteFile(tmpFilename, body, 0644); err != nil {
		return "", false, fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tmpFilename, filename); err != nil {
		_ = os.Remove(tmpFilename)
		return "", false, fmt.Errorf("rename temp file: %w", err)
	}

	logger.Infof("downloaded %d bytes to %s", len(body), filename)
	return filename, false, nil
}
