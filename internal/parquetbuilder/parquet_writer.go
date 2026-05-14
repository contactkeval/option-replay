package parquetbuilder

import (
    "os"

    "github.com/parquet-go/parquet-go"
)

func WriteParquet(path string, rows []OptionRow) error {

    file, err := os.Create(path)
    if err != nil {
        return err
    }
    defer file.Close()

    writer := parquet.NewGenericWriter[OptionRow](file)

    defer writer.Close()

    _, err = writer.Write(rows)

    return err
}