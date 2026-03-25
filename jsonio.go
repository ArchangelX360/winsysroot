package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func printJSON(value interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "\t")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("failed to encode JSON to stdout: %w", err)
	}
	return nil
}

func writeJSONFile(outputPath string, value interface{}) error {
	raw, err := json.MarshalIndent(value, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to encode JSON for %s: %w", outputPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directories for %s: %w", outputPath, err)
	}
	if err := os.WriteFile(outputPath, append(raw, '\n'), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", outputPath, err)
	}
	return nil
}
