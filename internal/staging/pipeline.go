package staging

import "fmt"

func Run(inputRoot string, outputRoot string) error {

	files, err := DiscoverFiles(inputRoot)
	if err != nil {
		return err
	}

	fmt.Printf("Found %d files\n", len(files))

	for _, file := range files {

		cache := NewWriterCache(outputRoot)

		if err := ProcessFile(file, cache); err != nil {
			fmt.Printf("ERROR %s: %v\n", file, err)
		}

		if err := cache.Close(); err != nil {
			fmt.Printf("CLOSE ERROR: %v\n", err)
		}
	}

	return nil
}
