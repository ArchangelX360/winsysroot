package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type DownloadManifest struct {
	SchemaVersion       int                     `json:"schema_version"`
	VSRelease           string                  `json:"vs_release"`
	WindowsSDKVersion   string                  `json:"windows_sdk_version"`
	Architectures       []string                `json:"architectures"`
	Slim                bool                    `json:"slim"`
	ChannelManifestID   string                  `json:"channel_manifest_id"`
	InstallerManifestID string                  `json:"installer_manifest_id"`
	Downloads           []DownloadManifestEntry `json:"downloads"`
}

type DownloadManifestEntry struct {
	Name            string   `json:"name"`
	Source          string   `json:"source"`
	PackageID       string   `json:"package_id"`
	PackageVersion  string   `json:"package_version"`
	PackageType     string   `json:"package_type"`
	PayloadFileName string   `json:"payload_file_name"`
	OutputPath      string   `json:"output_path"`
	URL             string   `json:"url"`
	URLs            []string `json:"urls"`
	Sha256          string   `json:"sha256,omitempty"`
	Integrity       string   `json:"integrity,omitempty"`
	Size            int      `json:"size,omitempty"`
}

type downloadManifestBuilder struct {
	entries    []DownloadManifestEntry
	nameCounts map[string]int
}

func newDownloadManifestBuilder() *downloadManifestBuilder {
	return &downloadManifestBuilder{
		nameCounts: map[string]int{},
	}
}

func (b *downloadManifestBuilder) add(source string, pkg Package, payloadFileName string, payloadURL string, payloadSha256 string, payloadSize int) {
	baseName := sanitizeDownloadName(fmt.Sprintf("%s_%s_%s", source, pkg.ID, payloadFileName))
	count := b.nameCounts[baseName]
	b.nameCounts[baseName] = count + 1
	entryName := baseName
	if count > 0 {
		entryName = fmt.Sprintf("%s_%d", baseName, count+1)
	}

	entry := DownloadManifestEntry{
		Name:            entryName,
		Source:          source,
		PackageID:       pkg.ID,
		PackageVersion:  pkg.Version,
		PackageType:     pkg.Type,
		PayloadFileName: payloadFileName,
		OutputPath:      path.Join(source, strings.TrimPrefix(strings.ReplaceAll(payloadFileName, "\\", "/"), "/")),
		URL:             payloadURL,
		URLs:            []string{payloadURL},
		Sha256:          payloadSha256,
		Size:            payloadSize,
	}
	if payloadSha256 != "" {
		if integrity, err := sha256HexToSRI(payloadSha256); err == nil {
			entry.Integrity = integrity
		}
	}
	b.entries = append(b.entries, entry)
}

func collectDownloadManifest(vsRelease string, winSDKVersion string, architectures []string, slim bool, channelID string, installerManifest InstallerManifest) (DownloadManifest, error) {
	manifestBuilder := newDownloadManifestBuilder()

	sdkPkg, err := findWinSDKPackage(installerManifest, winSDKVersion)
	if err != nil {
		return DownloadManifest{}, err
	}

	for _, payload := range sdkPkg.Payloads {
		if strings.HasSuffix(strings.ToLower(payload.FileName), ".msi") {
			manifestBuilder.add("winsdk", sdkPkg, payload.FileName, payload.URL, payload.Sha256, payload.Size)
		}
	}
	sdkCabMSIInfo, err := discoverWinSDKCabMSIInfo(sdkPkg)
	if err != nil {
		return DownloadManifest{}, err
	}
	for _, payload := range sdkPkg.Payloads {
		parts := strings.Split(payload.FileName, "\\")
		if len(parts) != 2 {
			continue
		}
		if sdkCabMSIInfo[strings.ToLower(parts[1])] != nil {
			manifestBuilder.add("winsdk", sdkPkg, payload.FileName, payload.URL, payload.Sha256, payload.Size)
		}
	}

	vcPkgs, err := selectVCToolPackages(installerManifest, architectures)
	if err != nil {
		return DownloadManifest{}, err
	}
	vcPackageIDs := make([]string, 0, len(vcPkgs))
	for pkgID := range vcPkgs {
		vcPackageIDs = append(vcPackageIDs, pkgID)
	}
	sort.Strings(vcPackageIDs)
	for _, pkgID := range vcPackageIDs {
		pkg := vcPkgs[pkgID]
		if !strings.EqualFold(pkg.Type, "vsix") {
			continue
		}
		if len(pkg.Payloads) == 0 {
			continue
		}
		payload := pkg.Payloads[0]
		manifestBuilder.add("vctools", pkg, payload.FileName, payload.URL, payload.Sha256, payload.Size)
	}

	return DownloadManifest{
		SchemaVersion:       1,
		VSRelease:           vsRelease,
		WindowsSDKVersion:   winSDKVersion,
		Architectures:       architectures,
		Slim:                slim,
		ChannelManifestID:   channelID,
		InstallerManifestID: installerManifest.Info.ID,
		Downloads:           manifestBuilder.entries,
	}, nil
}

func writeDownloadManifest(outputPath string, manifest DownloadManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode output manifest: %w", err)
	}
	parentDir := filepath.Dir(outputPath)
	if parentDir != "." {
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory for manifest %q: %w", outputPath, err)
		}
	}
	if err := os.WriteFile(outputPath, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("failed to write output manifest %q: %w", outputPath, err)
	}
	return nil
}

func sanitizeDownloadName(name string) string {
	var b strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '_' || c == '-' || c == '.' {
			b.WriteRune(c)
			continue
		}
		b.WriteByte('_')
	}
	return b.String()
}

func sha256HexToSRI(sha256Hex string) (string, error) {
	digest, err := hex.DecodeString(sha256Hex)
	if err != nil {
		return "", fmt.Errorf("invalid sha256 digest %q: %w", sha256Hex, err)
	}
	return "sha256-" + base64.StdEncoding.EncodeToString(digest), nil
}
