package pkg

import (
	"encoding/csv"
	"fmt"
	"os"
)

func LoadSeedURLs(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read seed file %v", err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read seed file %v", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("seed file is empty")
	}
	var domainIDX = -1
	header := records[0]
	for i, col := range header {
		if col == "Domain" {
			domainIDX = i
			break
		}
	}
	if domainIDX == -1 {
		return nil, fmt.Errorf("failed to find the domain col in seed file")
	}
	var urls []string
	for _, row := range records[1:] {
		if len(row) > domainIDX {
			urls = append(urls, row[domainIDX])
		}
	}
	return urls, nil
}
