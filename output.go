package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
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

func runMakeSpacelessAliases(args []string) {
	fs := flag.NewFlagSet("make-spaceless-aliases", flag.ExitOnError)
	rootDir := fs.String("root-dir", "", "Directory rooted at the extracted sysroot")
	parseFlagsOrExit(fs, args)

	if *rootDir == "" {
		fs.Usage()
		log.Fatal("make-spaceless-aliases requires --root-dir")
	}
	if err := createSpacelessAliases(*rootDir); err != nil {
		log.Fatal(err)
	}
}

type pathAlias struct {
	sourcePath string
	aliasPath  string
}

func createSpacelessAliases(rootDir string) error {
	var dirAliases []pathAlias
	var fileAliases []pathAlias
	err := filepath.WalkDir(rootDir, func(sourcePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if sourcePath == rootDir {
			return nil
		}

		aliasPath, err := spacelessAliasPath(rootDir, sourcePath, entry.IsDir())
		if err != nil {
			return err
		}
		if aliasPath == "" {
			return nil
		}

		alias := pathAlias{
			sourcePath: sourcePath,
			aliasPath:  aliasPath,
		}
		if entry.IsDir() {
			dirAliases = append(dirAliases, alias)
		} else {
			fileAliases = append(fileAliases, alias)
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, alias := range dirAliases {
		if err := createAliasDirectory(alias.sourcePath, alias.aliasPath); err != nil {
			return err
		}
	}
	for _, alias := range fileAliases {
		if err := createAliasFile(alias.sourcePath, alias.aliasPath); err != nil {
			return err
		}
	}
	return nil
}

func spacelessAliasPath(rootDir, sourcePath string, isDir bool) (string, error) {
	relPath, err := filepath.Rel(rootDir, sourcePath)
	if err != nil {
		return "", fmt.Errorf("failed to compute relative path for %s: %w", sourcePath, err)
	}

	parts := strings.Split(filepath.ToSlash(relPath), "/")
	limit := len(parts)
	if !isDir {
		limit--
	}
	changed := false
	for i := 0; i < limit; i++ {
		aliasPart := strings.ReplaceAll(parts[i], " ", "")
		if aliasPart != parts[i] {
			changed = true
			parts[i] = aliasPart
		}
	}
	if !changed {
		return "", nil
	}
	return filepath.Join(append([]string{rootDir}, parts...)...), nil
}

func createAliasDirectory(sourcePath, aliasPath string) error {
	if err := ensureAliasDoesNotExist(sourcePath, aliasPath); err != nil {
		return err
	}
	if err := os.MkdirAll(aliasPath, 0755); err != nil {
		return fmt.Errorf("failed to create alias directory %s for %s: %w", aliasPath, sourcePath, err)
	}
	return nil
}

func createAliasFile(sourcePath, aliasPath string) error {
	if err := ensureAliasDoesNotExist(sourcePath, aliasPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(aliasPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory for alias %s: %w", aliasPath, err)
	}
	if err := os.Link(sourcePath, aliasPath); err == nil {
		return nil
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open %s for alias copy: %w", sourcePath, err)
	}
	defer sourceFile.Close()

	aliasFile, err := os.Create(aliasPath)
	if err != nil {
		return fmt.Errorf("failed to create alias file %s: %w", aliasPath, err)
	}
	defer aliasFile.Close()

	if _, err := io.Copy(aliasFile, sourceFile); err != nil {
		return fmt.Errorf("failed to copy %s to %s: %w", sourcePath, aliasPath, err)
	}
	return nil
}

func ensureAliasDoesNotExist(sourcePath, aliasPath string) error {
	if _, err := os.Lstat(aliasPath); err == nil {
		return fmt.Errorf("alias path %s for %s already exists", aliasPath, sourcePath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat alias path %s: %w", aliasPath, err)
	}
	return nil
}
