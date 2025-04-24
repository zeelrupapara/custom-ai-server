package utils

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConvertCSVToTxt checks if a .txt version exists. If yes, returns its path.
// Otherwise, converts the .csv file to .txt and returns the new path.
func ConvertCSVToTxt(csvPath string) (string, error) {
	// Derive the .txt path from the .csv path
	txtPath := strings.TrimSuffix(csvPath, filepath.Ext(csvPath)) + ".txt"

	// Check if the .txt file already exists
	if _, err := os.Stat(txtPath); err == nil {
		// File exists, return its path
		return txtPath, nil
	} else if !os.IsNotExist(err) {
		// Other error while checking
		return "", fmt.Errorf("error checking txt file existence: %w", err)
	}

	// Open the CSV file
	file, err := os.Open(csvPath)
	if err != nil {
		return "", fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	// Parse the CSV
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return "", fmt.Errorf("failed to read CSV data: %w", err)
	}

	// Create the .txt file
	txtFile, err := os.Create(txtPath)
	if err != nil {
		return "", fmt.Errorf("failed to create TXT file: %w", err)
	}
	defer txtFile.Close()

	// Write CSV rows as tab-separated lines
	for _, record := range records {
		line := strings.Join(record, "\t") + "\n"
		if _, err := txtFile.WriteString(line); err != nil {
			return "", fmt.Errorf("failed to write to TXT file: %w", err)
		}
	}

	return txtPath, nil
}
