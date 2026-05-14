package parquetbuilder

import (
    "bufio"
    "os"
    "path/filepath"
    "strconv"
    "strings"
)

func ParseStagingFile(path string) ([]OptionRow, error) {

    file, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    expiry, err := ExtractExpiryFromFilename(path)
    if err != nil {
        return nil, err
    }

    scanner := bufio.NewScanner(file)

    // Larger line buffer
    buf := make([]byte, 0, 1024*1024)
    scanner.Buffer(buf, 10*1024*1024)

    var rows []OptionRow

    for scanner.Scan() {

        line := scanner.Text()

        parts := strings.Split(line, ",")

        if len(parts) != 9 {
            continue
        }

        strike, _ := strconv.ParseInt(parts[0], 10, 64)

        windowStart, _ := strconv.ParseInt(parts[2], 10, 64)

        openVal, _ := strconv.ParseFloat(parts[3], 32)
        highVal, _ := strconv.ParseFloat(parts[4], 32)
        lowVal, _ := strconv.ParseFloat(parts[5], 32)
        closeVal, _ := strconv.ParseFloat(parts[6], 32)

        volume, _ := strconv.ParseInt(parts[7], 10, 64)

        transactions, _ := strconv.ParseInt(parts[8], 10, 32)

        rows = append(rows, OptionRow{
            Expiry:       expiry,
            Strike:       strike,
            OptionType:   parts[1],
            WindowStart:  windowStart,
            Open:         float32(openVal),
            High:         float32(highVal),
            Low:          float32(lowVal),
            Close:        float32(closeVal),
            Volume:       volume,
            Transactions: int32(transactions),
        })
    }

    return rows, scanner.Err()
}

func ExtractExpiryFromFilename(path string) (int32, error) {

    base := filepath.Base(path)

    // AAPL_260515.csv

    parts := strings.Split(base, "_")

    expiryStr := strings.TrimSuffix(parts[1], ".csv")

    expiry, err := strconv.ParseInt(expiryStr, 10, 32)

    return int32(expiry), err
}
