package s3sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Config struct {
	Bucket              string
	Prefix              string
	LocalDir            string
	Region              string
	ConcurrentDownloads int
}

func Sync(cfg Config) error {
	ctx := context.Background()

	awsCfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(cfg.Region),
	)

	if err != nil {
		return err
	}

	client := s3.NewFromConfig(awsCfg)

	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: &cfg.Bucket,
		Prefix: &cfg.Prefix,
	})

	downloader := manager.NewDownloader(client)

	sem := make(chan struct{}, cfg.ConcurrentDownloads)

	var wg sync.WaitGroup

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}

		for _, obj := range page.Contents {
			key := *obj.Key
			size := obj.Size

			localPath := filepath.Join(cfg.LocalDir, key)

			// Skip if same size already exists
			if fileExistsWithSameSize(localPath, *size) {
				fmt.Printf("SKIP %s\n", key)
				continue
			}

			wg.Add(1)

			go func(key string, localPath string) {
				defer wg.Done()

				sem <- struct{}{}
				defer func() { <-sem }()

				if err := downloadFile(ctx, downloader, cfg.Bucket, key, localPath); err != nil {
					fmt.Printf("ERROR %s: %v\n", key, err)
					return
				}

				fmt.Printf("DOWNLOADED %s\n", key)

			}(key, localPath)
		}
	}

	wg.Wait()

	return nil
}

func fileExistsWithSameSize(path string, remoteSize int64) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return info.Size() == remoteSize
}

func downloadFile(
	ctx context.Context,
	downloader *manager.Downloader,
	bucket string,
	key string,
	localPath string,
) error {

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}

	file, err := os.Create(localPath)
	if err != nil {
		return err
	}

	defer file.Close()

	_, err = downloader.Download(ctx, file, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})

	if err != nil {
		return err
	}

	return file.Sync()
}
