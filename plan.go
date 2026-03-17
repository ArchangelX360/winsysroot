package main

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

const planSchemaVersion = 2

type DownloadPlan struct {
	SchemaVersion int             `json:"schema_version"`
	Request       PlanRequest     `json:"request"`
	Downloads     []DownloadEntry `json:"downloads"`
	WindowsSDK    WinSDKPlan      `json:"windows_sdk"`
	VCTools       VCToolsPlan     `json:"vc_tools"`
}

type PlanRequest struct {
	WindowsSDKVersion string   `json:"windows_sdk_version"`
	Architectures     []string `json:"architectures"`
	Slim              bool     `json:"slim"`
}

type DownloadEntry struct {
	Name      string   `json:"name"`
	URL       string   `json:"url,omitempty"`
	URLs      []string `json:"urls,omitempty"`
	Sha256    string   `json:"sha256,omitempty"`
	Integrity string   `json:"integrity,omitempty"`
}

type WinSDKPlan struct {
	PackageID    string              `json:"package_id"`
	CABDownloads []WinSDKCABDownload `json:"cab_downloads"`
}

type WinSDKCABDownload struct {
	PayloadFileName string `json:"payload_file_name"`
	Download        string `json:"download"`
}

type VCToolsPlan struct {
	VSIXDownloads []VCToolsVSIXDownload `json:"vsix_downloads"`
}

type VCToolsVSIXDownload struct {
	PackageID      string `json:"package_id"`
	PackageVersion string `json:"package_version"`
	Download       string `json:"download"`
}

type AssemblyMetadata struct {
	MSVCVersions              []string `json:"msvc_versions"`
	WindowsSDKIncludeVersions []string `json:"windows_sdk_include_versions"`
	WindowsSDKLibVersions     []string `json:"windows_sdk_lib_versions"`
}

func buildDownloadPlan(installerManifest InstallerManifest, winsdkMSIDir, winSDKVersion string, architectures []string, slim bool) (*DownloadPlan, error) {
	downloadsByName := make(map[string]DownloadEntry)

	windowsSDKPlan, err := collectWinSDKPlan(installerManifest, winsdkMSIDir, winSDKVersion, architectures, slim, downloadsByName)
	if err != nil {
		return nil, err
	}
	vcToolsPlan, err := collectVCToolsPlan(installerManifest, architectures, downloadsByName)
	if err != nil {
		return nil, err
	}

	downloads := make([]DownloadEntry, 0, len(downloadsByName))
	for _, download := range downloadsByName {
		downloads = append(downloads, download)
	}
	sort.Slice(downloads, func(i, j int) bool {
		return downloads[i].Name < downloads[j].Name
	})

	return &DownloadPlan{
		SchemaVersion: planSchemaVersion,
		Request: PlanRequest{
			WindowsSDKVersion: winSDKVersion,
			Architectures:     append([]string(nil), architectures...),
			Slim:              slim,
		},
		Downloads:  downloads,
		WindowsSDK: windowsSDKPlan,
		VCTools:    vcToolsPlan,
	}, nil
}

func addPlannedDownload(downloadsByName map[string]DownloadEntry, download DownloadEntry) error {
	existing, ok := downloadsByName[download.Name]
	if !ok {
		downloadsByName[download.Name] = download
		return nil
	}
	if existing.URL != download.URL || existing.Sha256 != download.Sha256 || existing.Integrity != download.Integrity || !stringSlicesEqual(existing.URLs, download.URLs) {
		return fmt.Errorf("conflicting download entries for %s", download.Name)
	}
	return nil
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func payloadDownloadEntry(name, payloadURL, sha256 string) DownloadEntry {
	return DownloadEntry{
		Name:   name,
		URL:    payloadURL,
		Sha256: sha256,
	}
}

func normalizedPayloadDownloadPath(prefix, payloadPath string) string {
	return path.Join(prefix, normalizeManifestPath(payloadPath))
}

func sanitizePathComponent(value string) string {
	replacer := strings.NewReplacer("\\", "_", "/", "_", ":", "_")
	return replacer.Replace(value)
}
