package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
)

func runWriteVFS(args []string) {
	fs := flag.NewFlagSet("write-vfs", flag.ExitOnError)
	rootDir := fs.String("root-dir", "", "Directory to index")
	virtualRoot := fs.String("virtual-root", "", "Path exposed at the VFS root. Defaults to --root-dir.")
	outputPath := fs.String("out", "", "Path to write vfsoverlay JSON. Defaults to <root-dir>/vfsoverlay.yaml.")
	parseFlagsOrExit(fs, args)

	if *rootDir == "" {
		fs.Usage()
		log.Fatal("write-vfs requires --root-dir")
	}

	finalVirtualRoot := *virtualRoot
	if finalVirtualRoot == "" {
		finalVirtualRoot = *rootDir
	}
	finalOutputPath := *outputPath
	if finalOutputPath == "" {
		finalOutputPath = filepath.Join(*rootDir, "vfsoverlay.yaml")
	}

	if err := writeVFSOverlay(*rootDir, finalVirtualRoot, finalOutputPath); err != nil {
		log.Fatal(err)
	}
}

func writeVFSOverlay(rootDir, virtualRoot, outputPath string) error {
	rootDirAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return fmt.Errorf("failed to resolve %s: %w", rootDir, err)
	}
	outputPathAbs, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("failed to resolve %s: %w", outputPath, err)
	}

	trueValue := true
	falseValue := false
	root := &Inode{
		Type: "directory",
		Name: filepath.ToSlash(virtualRoot),
	}
	vfs := VFS{
		Version:         0,
		CaseSensitive:   &falseValue,
		OverlayRelative: &trueValue,
		RedirectingWith: RedirectingWithFallthrough,
		Roots:           []*Inode{root},
	}

	err = filepath.WalkDir(rootDirAbs, func(currentPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if currentPath == rootDirAbs || entry.IsDir() {
			return nil
		}

		currentPathAbs, err := filepath.Abs(currentPath)
		if err != nil {
			return err
		}
		if currentPathAbs == outputPathAbs {
			return nil
		}

		relPath, err := filepath.Rel(rootDirAbs, currentPathAbs)
		if err != nil {
			return fmt.Errorf("failed to compute relative path for %s: %w", currentPath, err)
		}
		relPath = filepath.ToSlash(relPath)
		if err := root.Place(path.Dir(relPath), true, &Inode{
			Type:             "file",
			Name:             path.Base(relPath),
			ExternalContents: relPath,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	return writeJSONFile(outputPath, vfs)
}
