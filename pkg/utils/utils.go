package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CSVToText checks for an existing .txt alongside the given .csv file.
// If the .txt exists, it simply returns its path.
// Otherwise it reads the CSV and writes its contents into a new .txt file.
func CSVToText(inputPath string) (string, error) {
	// 1. Determine actual CSV path
	ext := filepath.Ext(inputPath)
	var csvPath string
	if ext == "" {
		csvPath = inputPath + ".csv"
	} else {
		csvPath = inputPath
	}

	// 2. Verify the CSV file exists
	if info, err := os.Stat(csvPath); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("CSV file not found: %s", csvPath)
		}
		return "", fmt.Errorf("checking CSV file: %w", err)
	} else if info.IsDir() {
		return "", fmt.Errorf("expected a file but found a directory: %s", csvPath)
	}

	// 3. Derive the .txt path
	base := strings.TrimSuffix(csvPath, filepath.Ext(csvPath))
	txtPath := base + ".txt"

	// 4. If .txt already exists, return it immediately
	if _, err := os.Stat(txtPath); err == nil {
		return txtPath, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("checking TXT file: %w", err)
	}

	// 5. Copy contents from CSV → TXT
	inFile, err := os.Open(csvPath)
	if err != nil {
		return "", fmt.Errorf("opening CSV for read: %w", err)
	}
	defer inFile.Close()

	outFile, err := os.Create(txtPath)
	if err != nil {
		return "", fmt.Errorf("creating TXT for write: %w", err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, inFile); err != nil {
		return "", fmt.Errorf("writing TXT file: %w", err)
	}

	return txtPath, nil
}

