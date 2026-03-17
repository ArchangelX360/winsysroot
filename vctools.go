package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

var archTools = map[string]string{
	"arm":     "Microsoft.VisualStudio.Component.VC.Tools.ARM",
	"arm64":   "Microsoft.VisualStudio.Component.VC.Tools.ARM64",
	"arm64ec": "Microsoft.VisualStudio.Component.VC.Tools.ARM64EC",
	"x64":     "Microsoft.VisualStudio.Component.VC.Tools.x86.x64",
	"x86":     "Microsoft.VisualStudio.Component.VC.Tools.x86.x64",
}

func collectVCToolsPlan(manifest InstallerManifest, architectures []string, downloadsByName map[string]DownloadEntry) (VCToolsPlan, error) {
	pkgs, err := collectVCToolsPackages(manifest, architectures)
	if err != nil {
		return VCToolsPlan{}, err
	}

	packageIDs := make([]string, 0, len(pkgs))
	for pkgID := range pkgs {
		packageIDs = append(packageIDs, pkgID)
	}
	sort.Strings(packageIDs)

	var vsixDownloads []VCToolsVSIXDownload
	for _, pkgID := range packageIDs {
		pkg := pkgs[pkgID]
		if !strings.EqualFold(pkg.Type, "vsix") {
			continue
		}
		if len(pkg.Payloads) == 0 {
			return VCToolsPlan{}, fmt.Errorf("package %s has no payloads", pkg.ID)
		}
		payload := pkg.Payloads[0]
		downloadPath := path.Join("vctools", sanitizePathComponent(pkg.ID), normalizeManifestPath(payload.FileName))
		if err := addPlannedDownload(downloadsByName, payloadDownloadEntry(downloadPath, payload.URL, payload.Sha256)); err != nil {
			return VCToolsPlan{}, err
		}
		vsixDownloads = append(vsixDownloads, VCToolsVSIXDownload{
			PackageID:      pkg.ID,
			PackageVersion: pkg.Version,
			Download:       downloadPath,
		})
	}

	return VCToolsPlan{VSIXDownloads: vsixDownloads}, nil
}

func collectVCToolsPackages(manifest InstallerManifest, architectures []string) (map[string]Package, error) {
	pkgsByID := make(map[string]Package)
	for _, pkg := range manifest.Packages {
		pkgsByID[pkg.ID] = pkg
	}

	var pending []string
	for _, arch := range architectures {
		component := archTools[arch]
		if component == "" {
			return nil, fmt.Errorf("unknown architecture %q, don't know the correct tools package", arch)
		}
		if _, ok := pkgsByID[component]; !ok {
			return nil, fmt.Errorf("failed to find Visual Studio package %s in installer manifest", component)
		}
		pending = append(pending, component)
	}

	selected := make(map[string]Package)
	seen := make(map[string]bool)
	for len(pending) > 0 {
		pkgID := pending[0]
		pending = pending[1:]
		if seen[pkgID] {
			continue
		}
		seen[pkgID] = true
		pkg, ok := pkgsByID[pkgID]
		if !ok {
			continue
		}
		selected[pkgID] = pkg
		var dependencyIDs []string
		for dependencyID := range pkg.Dependencies {
			dependencyIDs = append(dependencyIDs, dependencyID)
		}
		sort.Strings(dependencyIDs)
		pending = append(pending, dependencyIDs...)
	}

	return selected, nil
}

func assembleVCTools(plan *DownloadPlan, downloadsDir string, out TargetI) ([]string, error) {
	hasArch := make(map[string]bool)
	for _, arch := range plan.Request.Architectures {
		hasArch[arch] = true
	}

	msvcVersions := map[string]bool{}

	for _, plannedVSIX := range plan.VCTools.VSIXDownloads {
		vsixPath := filepath.Join(downloadsDir, filepath.FromSlash(plannedVSIX.Download))
		file, err := os.Open(vsixPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open VSIX %s: %w", vsixPath, err)
		}
		fileInfo, err := file.Stat()
		if err != nil {
			file.Close()
			return nil, fmt.Errorf("failed to stat VSIX %s: %w", vsixPath, err)
		}
		archive, err := zip.NewReader(file, fileInfo.Size())
		if err != nil {
			file.Close()
			return nil, fmt.Errorf("failed to parse VSIX %s: %w", vsixPath, err)
		}
		for _, archiveFile := range archive.File {
			targetPath, version, keep := vctoolsOutputPath(archiveFile.Name, hasArch)
			if !keep {
				continue
			}
			msvcVersions[version] = true
			if err := out.Create(targetPath, archiveFile.FileInfo().Size(), archiveFile.FileInfo().ModTime()); err != nil {
				file.Close()
				return nil, fmt.Errorf("failed to create output file %s: %w", targetPath, err)
			}
			reader, err := archiveFile.Open()
			if err != nil {
				file.Close()
				return nil, fmt.Errorf("failed to open %s inside %s: %w", archiveFile.Name, vsixPath, err)
			}
			if _, err := io.Copy(out, reader); err != nil {
				reader.Close()
				file.Close()
				return nil, fmt.Errorf("failed to extract %s from %s: %w", archiveFile.Name, vsixPath, err)
			}
			if err := reader.Close(); err != nil {
				file.Close()
				return nil, fmt.Errorf("failed to close %s inside %s: %w", archiveFile.Name, vsixPath, err)
			}
		}
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("failed to close VSIX %s: %w", vsixPath, err)
		}
	}

	return sortedSetValues(msvcVersions), nil
}

func vctoolsOutputPath(archivePath string, hasArch map[string]bool) (string, string, bool) {
	if !strings.HasPrefix(archivePath, "Contents/VC/Tools/MSVC/") {
		return "", "", false
	}
	parts := strings.Split(archivePath, "/")
	if len(parts) < 6 {
		return "", "", false
	}
	typeDir := strings.ToLower(parts[5])
	if typeDir != "include" && typeDir != "lib" {
		return "", "", false
	}
	if typeDir == "lib" {
		if len(parts) < 7 || !hasArch[strings.ToLower(parts[6])] {
			return "", "", false
		}
	}
	return strings.TrimPrefix(archivePath, "Contents/"), parts[4], true
}

func assembleFromPlan(plan *DownloadPlan, winsdkMSIDir, downloadsDir string, out TargetI) (*AssemblyMetadata, error) {
	includeVersions, libVersions, err := assembleWinSDK(plan, winsdkMSIDir, downloadsDir, out)
	if err != nil {
		return nil, err
	}
	msvcVersions, err := assembleVCTools(plan, downloadsDir, out)
	if err != nil {
		return nil, err
	}
	return &AssemblyMetadata{
		MSVCVersions:              msvcVersions,
		WindowsSDKIncludeVersions: includeVersions,
		WindowsSDKLibVersions:     libVersions,
	}, nil
}
