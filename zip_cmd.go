package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
)

type ZIPListing struct {
	Entries []string `json:"entries"`
}

func runZIPList(args []string) {
	fs := flag.NewFlagSet("zip-list", flag.ExitOnError)
	inputPath := fs.String("input", "", "Path to a ZIP or VSIX archive")
	outputPath := fs.String("out", "", "Optional JSON output path. Prints to stdout when omitted.")
	parseFlagsOrExit(fs, args)

	if *inputPath == "" {
		fs.Usage()
		log.Fatal("zip-list requires --input")
	}

	entries, err := listZIPEntries(*inputPath)
	if err != nil {
		log.Fatal(err)
	}
	listing := ZIPListing{Entries: entries}
	if *outputPath != "" {
		if err := writeJSONFile(*outputPath, listing); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := printJSON(listing); err != nil {
		log.Fatal(err)
	}
}

func runZIPExtract(args []string) {
	fs := flag.NewFlagSet("zip-extract", flag.ExitOnError)
	inputPath := fs.String("input", "", "Path to a ZIP or VSIX archive")
	layoutPath := fs.String("layout", "", "Path to an extract layout JSON file")
	outDir := fs.String("out-dir", "", "Directory to extract files into")
	parseFlagsOrExit(fs, args)

	if *inputPath == "" || *layoutPath == "" || *outDir == "" {
		fs.Usage()
		log.Fatal("zip-extract requires --input, --layout, and --out-dir")
	}

	layout, err := loadExtractLayout(*layoutPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := extractZIPFiles(*inputPath, layout, *outDir); err != nil {
		log.Fatal(err)
	}
}

func listZIPEntries(zipPath string) ([]string, error) {
	file, err := os.Open(zipPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open ZIP %s: %w", zipPath, err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat ZIP %s: %w", zipPath, err)
	}
	archive, err := zip.NewReader(file, fileInfo.Size())
	if err != nil {
		return nil, fmt.Errorf("failed to parse ZIP %s: %w", zipPath, err)
	}

	entries := make([]string, 0, len(archive.File))
	for _, archiveFile := range archive.File {
		if archiveFile.FileInfo().IsDir() {
			continue
		}
		entries = append(entries, archiveFile.Name)
	}
	sort.Strings(entries)
	return entries, nil
}

func extractZIPFiles(zipPath string, layout map[string]string, outDir string) error {
	file, err := os.Open(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open ZIP %s: %w", zipPath, err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat ZIP %s: %w", zipPath, err)
	}
	archive, err := zip.NewReader(file, fileInfo.Size())
	if err != nil {
		return fmt.Errorf("failed to parse ZIP %s: %w", zipPath, err)
	}

	remaining := copyEntryMap(layout)
	for _, archiveFile := range archive.File {
		outputPath, ok := remaining[archiveFile.Name]
		if !ok || archiveFile.FileInfo().IsDir() {
			continue
		}

		reader, err := archiveFile.Open()
		if err != nil {
			return fmt.Errorf("failed to open %s inside %s: %w", archiveFile.Name, zipPath, err)
		}
		if err := writeOutputFile(outDir, outputPath, archiveFile.FileInfo().ModTime(), reader); err != nil {
			reader.Close()
			return fmt.Errorf("failed to extract %s from %s: %w", archiveFile.Name, zipPath, err)
		}
		if err := reader.Close(); err != nil {
			return fmt.Errorf("failed to close %s inside %s: %w", archiveFile.Name, zipPath, err)
		}
		delete(remaining, archiveFile.Name)
	}

	if len(remaining) > 0 {
		return fmt.Errorf("failed to find %d requested ZIP entries: %s", len(remaining), formatMissingEntries(remaining))
	}
	return nil
}
