package main

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

func outputFilePath(rootDir, relPath string) (string, error) {
	normalized := strings.ReplaceAll(relPath, "\\", "/")
	cleanPath := path.Clean(normalized)
	if cleanPath == "." || cleanPath == "" {
		return "", fmt.Errorf("invalid output path %q", relPath)
	}
	if path.IsAbs(cleanPath) || cleanPath == ".." || strings.HasPrefix(cleanPath, "../") {
		return "", fmt.Errorf("output path %q escapes the output root", relPath)
	}
	return filepath.Join(rootDir, filepath.FromSlash(cleanPath)), nil
}

func writeOutputFile(rootDir, relPath string, modTime time.Time, reader io.Reader) error {
	targetPath, err := outputFilePath(rootDir, relPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directories for %s: %w", targetPath, err)
	}

	file, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", targetPath, err)
	}
	if _, err := io.Copy(file, reader); err != nil {
		file.Close()
		return fmt.Errorf("failed to write %s: %w", targetPath, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close %s: %w", targetPath, err)
	}
	if err := os.Chtimes(targetPath, modTime, modTime); err != nil {
		return fmt.Errorf("failed to update timestamps for %s: %w", targetPath, err)
	}
	return nil
}
