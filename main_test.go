package main

import (
	"path/filepath"
	"testing"
)

func TestSpacelessAliasPath(t *testing.T) {
	rootDir := filepath.Join(string(filepath.Separator), "tmp", "winsysroot")

	tests := []struct {
		name       string
		sourcePath string
		isDir      bool
		want       string
	}{
		{
			name:       "directory without spaces is ignored",
			sourcePath: filepath.Join(rootDir, "VC", "Tools", "MSVC"),
			isDir:      true,
			want:       "",
		},
		{
			name:       "directory aliases strip spaces from all components",
			sourcePath: filepath.Join(rootDir, "Windows Kits", "10", "Include", "Some Dir"),
			isDir:      true,
			want:       filepath.Join(rootDir, "WindowsKits", "10", "Include", "SomeDir"),
		},
		{
			name:       "file aliases strip spaces from directory components only",
			sourcePath: filepath.Join(rootDir, "Windows Kits", "10", "Lib", "Some Dir", "My Lib.lib"),
			isDir:      false,
			want:       filepath.Join(rootDir, "WindowsKits", "10", "Lib", "SomeDir", "My Lib.lib"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := spacelessAliasPath(rootDir, test.sourcePath, test.isDir)
			if err != nil {
				t.Fatalf("spacelessAliasPath returned error: %v", err)
			}
			if got != test.want {
				t.Fatalf("spacelessAliasPath(%q) = %q, want %q", test.sourcePath, got, test.want)
			}
		})
	}
}
