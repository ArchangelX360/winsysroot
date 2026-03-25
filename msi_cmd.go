package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"git.dolansoft.org/lorenz/winsysroot/msi"
)

type MSIInfo struct {
	CABFiles []string          `json:"cab_files"`
	FileMap  map[string]string `json:"file_map"`
}

func runMSIInfo(args []string) {
	fs := flag.NewFlagSet("msi-info", flag.ExitOnError)
	inputPath := fs.String("input", "", "Path to an MSI file")
	outputPath := fs.String("out", "", "Optional JSON output path. Prints to stdout when omitted.")
	parseFlagsOrExit(fs, args)

	if *inputPath == "" {
		fs.Usage()
		log.Fatal("msi-info requires --input")
	}

	info, err := loadMSIInfo(*inputPath)
	if err != nil {
		log.Fatal(err)
	}
	if *outputPath != "" {
		if err := writeJSONFile(*outputPath, info); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := printJSON(info); err != nil {
		log.Fatal(err)
	}
}

func loadMSIInfo(msiPath string) (*MSIInfo, error) {
	file, err := os.Open(msiPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open MSI %s: %w", msiPath, err)
	}
	defer file.Close()

	parsed, err := msi.Parse(file)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MSI %s: %w", msiPath, err)
	}

	fileMap := make(map[string]string, len(parsed.FileMap))
	for archivePath, outputPath := range parsed.FileMap {
		fileMap[archivePath] = outputPath
	}

	return &MSIInfo{
		CABFiles: append([]string(nil), parsed.CABFiles...),
		FileMap:  fileMap,
	}, nil
}
