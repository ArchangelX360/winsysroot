package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"git.dolansoft.org/lorenz/winsysroot/cab"
)

func runCABExtract(args []string) {
	fs := flag.NewFlagSet("cab-extract", flag.ExitOnError)
	layoutPath := fs.String("layout", "", "Path to an extract layout JSON file")
	outDir := fs.String("out-dir", "", "Directory to extract files into")
	var cabPaths stringListFlag
	fs.Var(&cabPaths, "cab", "Path to a CAB file. May be repeated.")
	parseFlagsOrExit(fs, args)

	if *layoutPath == "" || *outDir == "" || len(cabPaths) == 0 {
		fs.Usage()
		log.Fatal("cab-extract requires --layout, --out-dir, and at least one --cab")
	}

	layout, err := loadExtractLayout(*layoutPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := extractCABFiles(cabPaths, layout, *outDir); err != nil {
		log.Fatal(err)
	}
}

func extractCABFiles(cabPaths []string, layout map[string]string, outDir string) error {
	remaining := copyEntryMap(layout)

	for _, cabPath := range cabPaths {
		cabFile, err := os.Open(cabPath)
		if err != nil {
			return fmt.Errorf("failed to open CAB %s: %w", cabPath, err)
		}

		cabReader, err := cab.New(cabFile)
		if err != nil {
			cabFile.Close()
			return fmt.Errorf("failed to parse CAB %s: %w", cabPath, err)
		}

		for {
			header, err := cabReader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				cabFile.Close()
				return fmt.Errorf("failed to read CAB %s: %w", cabPath, err)
			}

			outputPath, ok := remaining[header.Name]
			if !ok {
				continue
			}
			if err := writeOutputFile(outDir, outputPath, header.CreateTime, cabReader); err != nil {
				cabFile.Close()
				return fmt.Errorf("failed to extract %s from %s: %w", header.Name, cabPath, err)
			}
			delete(remaining, header.Name)
		}

		if err := cabFile.Close(); err != nil {
			return fmt.Errorf("failed to close CAB %s: %w", cabPath, err)
		}
	}

	if len(remaining) > 0 {
		return fmt.Errorf("failed to find %d requested CAB entries: %s", len(remaining), formatMissingEntries(remaining))
	}
	return nil
}
