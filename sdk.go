package main

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"git.dolansoft.org/lorenz/winsysroot/cab"
	"git.dolansoft.org/lorenz/winsysroot/msi"
)

var includeRegexp = regexp.MustCompile(`^Windows Kits/[^/]+/Include/[0-9\.]+/.*\.h(pp)?$`)
var libRegexp = regexp.MustCompile(`^Windows Kits/[^/]+/Lib/[0-9\.]+/.*\.[Ll][Ii][Bb]`)

func collectWinSDKPlan(manifest InstallerManifest, winsdkMSIDir, version string, architectures []string, slim bool, downloadsByName map[string]DownloadEntry) (WinSDKPlan, error) {
	sdkPkg, err := findWinSDKPackage(manifest, version)
	if err != nil {
		return WinSDKPlan{}, err
	}

	hasArch := make(map[string]bool)
	for _, arch := range architectures {
		hasArch[arch] = true
	}

	cabPayloads := make(map[string]int)
	for index, payload := range sdkPkg.Payloads {
		payloadPath := normalizeManifestPath(payload.FileName)
		if strings.EqualFold(path.Ext(payloadPath), ".cab") {
			cabPayloads[strings.ToLower(path.Base(payloadPath))] = index
		}
	}

	selectedDownloads := make(map[string]bool)
	var cabDownloads []WinSDKCABDownload
	for _, payload := range sdkPkg.Payloads {
		payloadPath := normalizeManifestPath(payload.FileName)
		if !strings.EqualFold(path.Ext(payloadPath), ".msi") {
			continue
		}
		msiPath := manifestLocalPath(winsdkMSIDir, payload.FileName)
		msiData, err := parseMSIFile(msiPath)
		if err != nil {
			return WinSDKPlan{}, fmt.Errorf("failed to parse MSI %s: %w", msiPath, err)
		}
		if !msiContainsRelevantFiles(msiData, hasArch, slim) {
			continue
		}
		for _, cabName := range msiData.CABFiles {
			payloadIndex, ok := cabPayloads[strings.ToLower(cabName)]
			if !ok {
				return WinSDKPlan{}, fmt.Errorf("failed to locate CAB payload %s in Windows SDK package %s", cabName, sdkPkg.ID)
			}
			cabPayload := sdkPkg.Payloads[payloadIndex]
			downloadPath := normalizedPayloadDownloadPath("winsdk", cabPayload.FileName)
			if err := addPlannedDownload(downloadsByName, payloadDownloadEntry(downloadPath, cabPayload.URL, cabPayload.Sha256)); err != nil {
				return WinSDKPlan{}, err
			}
			if selectedDownloads[downloadPath] {
				continue
			}
			selectedDownloads[downloadPath] = true
			cabDownloads = append(cabDownloads, WinSDKCABDownload{
				PayloadFileName: cabPayload.FileName,
				Download:        downloadPath,
			})
		}
	}

	sort.Slice(cabDownloads, func(i, j int) bool {
		return cabDownloads[i].Download < cabDownloads[j].Download
	})

	return WinSDKPlan{
		PackageID:    sdkPkg.ID,
		CABDownloads: cabDownloads,
	}, nil
}

func assembleWinSDK(plan *DownloadPlan, winsdkMSIDir, downloadsDir string, out TargetI) ([]string, []string, error) {
	hasArch := make(map[string]bool)
	for _, arch := range plan.Request.Architectures {
		hasArch[arch] = true
	}

	msiInfos, err := collectRelevantMSIInfos(winsdkMSIDir, hasArch, plan.Request.Slim)
	if err != nil {
		return nil, nil, err
	}

	includeVersions := map[string]bool{}
	libVersions := map[string]bool{}

	for _, cabDownload := range plan.WindowsSDK.CABDownloads {
		cabKey := strings.ToLower(path.Base(normalizeManifestPath(cabDownload.PayloadFileName)))
		msiInfo := msiInfos[cabKey]
		if msiInfo == nil {
			return nil, nil, fmt.Errorf("no MSI metadata found for CAB %s", cabDownload.PayloadFileName)
		}
		cabPath := filepath.Join(downloadsDir, filepath.FromSlash(cabDownload.Download))
		cabFile, err := os.Open(cabPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open CAB %s: %w", cabPath, err)
		}
		cabF, err := cab.New(cabFile)
		if err != nil {
			cabFile.Close()
			return nil, nil, fmt.Errorf("failed to parse CAB %s: %w", cabPath, err)
		}
		for {
			hdr, err := cabF.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				cabFile.Close()
				return nil, nil, fmt.Errorf("failed to read CAB %s: %w", cabPath, err)
			}
			outPath := msiInfo.FileMap[hdr.Name]
			if outPath == "" || !shouldKeepWinSDKPath(outPath, hasArch, plan.Request.Slim) {
				continue
			}
			recordWinSDKVersion(outPath, includeVersions, libVersions)
			if err := out.Create(outPath, int64(hdr.Size), hdr.CreateTime); err != nil {
				cabFile.Close()
				return nil, nil, fmt.Errorf("failed to create output file %s: %w", outPath, err)
			}
			if _, err := io.Copy(out, cabF); err != nil {
				cabFile.Close()
				return nil, nil, fmt.Errorf("failed to extract %s from %s: %w", hdr.Name, cabPath, err)
			}
		}
		if err := cabFile.Close(); err != nil {
			return nil, nil, fmt.Errorf("failed to close CAB %s: %w", cabPath, err)
		}
	}

	return sortedSetValues(includeVersions), sortedSetValues(libVersions), nil
}

func findWinSDKPackage(manifest InstallerManifest, version string) (Package, error) {
	packageRegexp := regexp.MustCompile(`^Win.*SDK_` + regexp.QuoteMeta(version) + "$")
	for _, pkg := range manifest.Packages {
		if packageRegexp.MatchString(pkg.ID) {
			return pkg, nil
		}
	}
	return Package{}, fmt.Errorf("failed to find Windows SDK package for version %s", version)
}

func parseMSIFile(msiPath string) (*msi.MSI, error) {
	file, err := os.Open(msiPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return msi.Parse(file)
}

func msiContainsRelevantFiles(msiData *msi.MSI, hasArch map[string]bool, slim bool) bool {
	for _, targetFile := range msiData.FileMap {
		if shouldKeepWinSDKPath(targetFile, hasArch, slim) {
			return true
		}
	}
	return false
}

func collectRelevantMSIInfos(winsdkMSIDir string, hasArch map[string]bool, slim bool) (map[string]*msi.MSI, error) {
	cabs := make(map[string]*msi.MSI)
	err := filepath.WalkDir(winsdkMSIDir, func(currentPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".msi") {
			return nil
		}
		msiData, err := parseMSIFile(currentPath)
		if err != nil {
			return fmt.Errorf("failed to parse MSI %s: %w", currentPath, err)
		}
		if !msiContainsRelevantFiles(msiData, hasArch, slim) {
			return nil
		}
		for _, cabName := range msiData.CABFiles {
			cabs[strings.ToLower(cabName)] = msiData
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return cabs, nil
}

func shouldKeepWinSDKPath(outPath string, hasArch map[string]bool, slim bool) bool {
	if !(includeRegexp.MatchString(outPath) || libRegexp.MatchString(outPath)) {
		return false
	}
	parts := strings.Split(outPath, "/")
	if len(parts) < 4 {
		return false
	}
	typeDir := strings.ToLower(parts[2])
	switch typeDir {
	case "include":
		if slim {
			ext := strings.ToLower(path.Ext(outPath))
			return ext == "" || ext == ".h" || ext == ".hpp" || ext == ".c" || ext == ".cpp"
		}
		return true
	case "lib":
		if len(parts) < 6 {
			return false
		}
		archDir := strings.ToLower(parts[5])
		if !hasArch[archDir] {
			return false
		}
		if slim {
			ext := strings.ToLower(path.Ext(outPath))
			return ext == ".lib" || ext == ".obj"
		}
		return true
	default:
		return false
	}
}

func recordWinSDKVersion(outPath string, includeVersions, libVersions map[string]bool) {
	parts := strings.Split(outPath, "/")
	if len(parts) < 4 {
		return
	}
	switch strings.ToLower(parts[2]) {
	case "include":
		includeVersions[parts[3]] = true
	case "lib":
		libVersions[parts[3]] = true
	}
}

func sortedSetValues(values map[string]bool) []string {
	ordered := make([]string, 0, len(values))
	for value := range values {
		ordered = append(ordered, value)
	}
	sort.Strings(ordered)
	return ordered
}
