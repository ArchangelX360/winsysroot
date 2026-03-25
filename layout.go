package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type ExtractLayout struct {
	Entries []ExtractEntry `json:"entries"`
}

type ExtractEntry struct {
	ArchivePath string `json:"archive_path"`
	OutputPath  string `json:"output_path"`
}

func loadExtractLayout(layoutPath string) (map[string]string, error) {
	raw, err := os.ReadFile(layoutPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read extract layout %s: %w", layoutPath, err)
	}

	var layout ExtractLayout
	if err := json.Unmarshal(raw, &layout); err != nil {
		return nil, fmt.Errorf("failed to parse extract layout %s: %w", layoutPath, err)
	}

	entriesByArchivePath := make(map[string]string, len(layout.Entries))
	for index, entry := range layout.Entries {
		if entry.ArchivePath == "" {
			return nil, fmt.Errorf("extract layout entry %d is missing archive_path", index)
		}
		if entry.OutputPath == "" {
			return nil, fmt.Errorf("extract layout entry %d is missing output_path", index)
		}
		if existing, ok := entriesByArchivePath[entry.ArchivePath]; ok && existing != entry.OutputPath {
			return nil, fmt.Errorf("extract layout contains conflicting output paths for %s", entry.ArchivePath)
		}
		entriesByArchivePath[entry.ArchivePath] = entry.OutputPath
	}

	return entriesByArchivePath, nil
}

func copyEntryMap(entries map[string]string) map[string]string {
	dup := make(map[string]string, len(entries))
	for archivePath, outputPath := range entries {
		dup[archivePath] = outputPath
	}
	return dup
}

func formatMissingEntries(entries map[string]string) string {
	if len(entries) == 0 {
		return ""
	}

	archivePaths := make([]string, 0, len(entries))
	for archivePath := range entries {
		archivePaths = append(archivePaths, archivePath)
	}
	sort.Strings(archivePaths)

	const maxEntries = 10
	if len(archivePaths) > maxEntries {
		return fmt.Sprintf("%s, ... (%d total)", stringsJoin(archivePaths[:maxEntries]), len(archivePaths))
	}
	return stringsJoin(archivePaths)
}

func stringsJoin(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	default:
		return values[0] + ", " + stringsJoin(values[1:])
	}
}
