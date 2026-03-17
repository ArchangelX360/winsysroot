package main

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

type TargetI interface {
	Create(path string, size int64, modTime time.Time) error
	io.WriteCloser
}

func main() {
	log.SetFlags(0)

	if len(os.Args) < 2 {
		printUsageAndExit()
	}

	switch os.Args[1] {
	case "plan":
		runPlan(os.Args[2:])
	case "assemble":
		runAssemble(os.Args[2:])
	case "list-win-sdk-versions":
		runListWinSDKVersions(os.Args[2:])
	case "help", "-h", "--help":
		printUsageAndExit()
	default:
		log.Fatalf("unknown subcommand %q", os.Args[1])
	}
}

func printUsageAndExit() {
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  winsysroot plan --installer-manifest <path> --winsdk-msi-dir <dir> --out-manifest <path> [options]\n")
	fmt.Fprintf(os.Stderr, "  winsysroot assemble --in-manifest <path> --winsdk-msi-dir <dir> --downloads-dir <dir> (--out-dir <dir> | --out-tar <path>) [options]\n")
	fmt.Fprintf(os.Stderr, "  winsysroot list-win-sdk-versions --installer-manifest <path>\n")
	os.Exit(2)
}

func runPlan(args []string) {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	installerManifestPath := fs.String("installer-manifest", "", "Path to the Visual Studio installer manifest JSON")
	winsdkMSIDir := fs.String("winsdk-msi-dir", "", "Directory that contains locally downloaded Windows SDK MSI payloads")
	outManifest := fs.String("out-manifest", "", "Write download plan JSON to this path")
	winSDKVersion := fs.String("win-sdk-version", "10.0.20348", "Version of the Windows SDK to use, without the patch version (e.g. 10.0.20348)")
	architectures := fs.String("architectures", "x64", "Comma-separated list of architectures to include in the sysroot. Supported are x86, x64, arm, arm64 and arm64ec.")
	slim := fs.Bool("slim", true, "Strip most excess files, ship only headers, libraries and object files. Also strips separate onecore, store and uwp libraries.")
	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}
	if *installerManifestPath == "" || *winsdkMSIDir == "" || *outManifest == "" {
		fs.Usage()
		log.Fatal("plan requires --installer-manifest, --winsdk-msi-dir, and --out-manifest")
	}

	installerManifest, err := loadInstallerManifest(*installerManifestPath)
	if err != nil {
		log.Fatal(err)
	}
	archList, err := splitArchitectures(*architectures)
	if err != nil {
		log.Fatal(err)
	}

	plan, err := buildDownloadPlan(installerManifest, *winsdkMSIDir, *winSDKVersion, archList, *slim)
	if err != nil {
		log.Fatal(err)
	}
	if err := writeJSONFile(*outManifest, plan); err != nil {
		log.Fatal(err)
	}
}

func runAssemble(args []string) {
	fs := flag.NewFlagSet("assemble", flag.ExitOnError)
	inManifest := fs.String("in-manifest", "", "Read download plan JSON from this path")
	winsdkMSIDir := fs.String("winsdk-msi-dir", "", "Directory that contains locally downloaded Windows SDK MSI payloads")
	downloadsDir := fs.String("downloads-dir", "", "Directory containing Bazel-downloaded CAB and VSIX artifacts")
	outDir := fs.String("out-dir", "", "Output sysroot under this directory. Exclusive with --out-tar.")
	outTar := fs.String("out-tar", "", "Output sysroot to a zstd-compressed tarball at the path given to this argument. Exclusive with --out-dir.")
	outMetadata := fs.String("out-metadata", "", "Optional path for extracted version metadata JSON")
	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}
	if *inManifest == "" || *winsdkMSIDir == "" || *downloadsDir == "" {
		fs.Usage()
		log.Fatal("assemble requires --in-manifest, --winsdk-msi-dir, and --downloads-dir")
	}

	plan, err := loadDownloadPlan(*inManifest)
	if err != nil {
		log.Fatal(err)
	}
	out, err := newOutputTarget(*outDir, *outTar)
	if err != nil {
		log.Fatal(err)
	}

	metadata, err := assembleFromPlan(plan, *winsdkMSIDir, *downloadsDir, out)
	closeErr := out.Close()
	if err != nil {
		log.Fatal(err)
	}
	if closeErr != nil {
		log.Fatalf("failed to finish writing output: %v", closeErr)
	}
	if *outMetadata != "" {
		if err := writeJSONFile(*outMetadata, metadata); err != nil {
			log.Fatal(err)
		}
	}
}

func runListWinSDKVersions(args []string) {
	fs := flag.NewFlagSet("list-win-sdk-versions", flag.ExitOnError)
	installerManifestPath := fs.String("installer-manifest", "", "Path to the Visual Studio installer manifest JSON")
	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}
	if *installerManifestPath == "" {
		fs.Usage()
		log.Fatal("list-win-sdk-versions requires --installer-manifest")
	}

	installerManifest, err := loadInstallerManifest(*installerManifestPath)
	if err != nil {
		log.Fatal(err)
	}

	seen := map[string]bool{}
	packageRegexp := regexp.MustCompile(`^Win.*SDK_([0-9.]+)$`)
	var versions []string
	for _, pkg := range installerManifest.Packages {
		match := packageRegexp.FindStringSubmatch(pkg.ID)
		if len(match) < 2 || seen[match[1]] {
			continue
		}
		seen[match[1]] = true
		versions = append(versions, match[1])
	}
	sort.Strings(versions)
	for _, version := range versions {
		fmt.Printf("%s\n", version)
	}
}

func splitArchitectures(value string) ([]string, error) {
	var architectures []string
	seen := map[string]bool{}
	for _, arch := range strings.Split(value, ",") {
		arch = strings.TrimSpace(arch)
		if arch == "" || seen[arch] {
			continue
		}
		seen[arch] = true
		architectures = append(architectures, arch)
	}
	if len(architectures) == 0 {
		return nil, errors.New("no architectures specified")
	}
	return architectures, nil
}

func loadInstallerManifest(manifestPath string) (InstallerManifest, error) {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return InstallerManifest{}, fmt.Errorf("failed to read installer manifest %s: %w", manifestPath, err)
	}
	var manifest InstallerManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return InstallerManifest{}, fmt.Errorf("failed to parse installer manifest %s: %w", manifestPath, err)
	}
	return manifest, nil
}

func loadDownloadPlan(planPath string) (*DownloadPlan, error) {
	raw, err := os.ReadFile(planPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read download plan %s: %w", planPath, err)
	}
	var plan DownloadPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return nil, fmt.Errorf("failed to parse download plan %s: %w", planPath, err)
	}
	if plan.SchemaVersion < 2 {
		return nil, fmt.Errorf("unsupported download plan schema_version %d", plan.SchemaVersion)
	}
	return &plan, nil
}

func writeJSONFile(outputPath string, value interface{}) error {
	raw, err := json.MarshalIndent(value, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to encode JSON for %s: %w", outputPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directories for %s: %w", outputPath, err)
	}
	if err := os.WriteFile(outputPath, append(raw, '\n'), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", outputPath, err)
	}
	return nil
}

func normalizeManifestPath(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

func manifestLocalPath(rootDir, manifestPath string) string {
	return filepath.Join(rootDir, filepath.FromSlash(normalizeManifestPath(manifestPath)))
}

func newOutputTarget(outDir, outTar string) (TargetI, error) {
	if outDir != "" && outTar != "" {
		return nil, errors.New("--out-dir and --out-tar are mutually exclusive")
	}
	if outDir != "" {
		return newVFSTargetLayer(&directoryTarget{rootDir: outDir}, outDir), nil
	}
	if outTar != "" {
		outInner, err := newArchiveTarget(outTar)
		if err != nil {
			return nil, fmt.Errorf("failed to create output tar archive: %w", err)
		}
		return newVFSTargetLayer(outInner, "/winsysroot"), nil
	}
	return nil, errors.New("please pass either --out-dir or --out-tar")
}

type vfsTargetLayer struct {
	t TargetI
	i *Inode
	v VFS
}

func newVFSTargetLayer(t TargetI, sysrootPath string) *vfsTargetLayer {
	var vfs VFS
	vfs.Version = 0
	vfs.RedirectingWith = RedirectingWithFallthrough
	True := true
	False := false
	vfs.CaseSensitive = &False
	vfs.OverlayRelative = &True

	winsysRoot := Inode{
		Type: "directory",
		Name: sysrootPath,
	}
	vfs.Roots = append(vfs.Roots, &winsysRoot)
	return &vfsTargetLayer{
		t: t,
		i: &winsysRoot,
		v: vfs,
	}
}

func (v *vfsTargetLayer) Create(p string, size int64, modTime time.Time) error {
	if err := v.i.Place(path.Dir(p), true, &Inode{
		Type:             "file",
		Name:             path.Base(p),
		ExternalContents: p,
	}); err != nil {
		return err
	}
	return v.t.Create(p, size, modTime)
}

func (v *vfsTargetLayer) Write(b []byte) (int, error) {
	return v.t.Write(b)
}

func (v *vfsTargetLayer) Close() error {
	vfsRaw, err := json.MarshalIndent(v.v, "", "\t")
	if err != nil {
		return fmt.Errorf("failed to encode VFS overlay metadata: %w", err)
	}
	if err := v.t.Create("vfsoverlay.yaml", int64(len(vfsRaw)), time.Now()); err != nil {
		return fmt.Errorf("failed to create VFS overlay output: %w", err)
	}
	if _, err := v.t.Write(vfsRaw); err != nil {
		return fmt.Errorf("failed to write VFS overlay: %w", err)
	}
	return v.t.Close()
}

type archiveTarget struct {
	outFile *os.File
	outComp *zstd.Encoder
	out     *tar.Writer
}

func newArchiveTarget(name string) (*archiveTarget, error) {
	outFile, err := os.Create(name)
	if err != nil {
		return nil, fmt.Errorf("failed to create output archive: %w", err)
	}
	outComp, err := zstd.NewWriter(outFile)
	if err != nil {
		outFile.Close()
		return nil, fmt.Errorf("failed to initialize zstd compressor: %w", err)
	}
	out := tar.NewWriter(outComp)
	return &archiveTarget{
		outFile: outFile,
		outComp: outComp,
		out:     out,
	}, nil
}

func (a *archiveTarget) Close() error {
	if err := a.out.Close(); err != nil {
		return err
	}
	if err := a.outComp.Close(); err != nil {
		return err
	}
	return a.outFile.Close()
}

func (a *archiveTarget) Create(path string, size int64, modTime time.Time) error {
	return a.out.WriteHeader(&tar.Header{
		Name:    path,
		ModTime: modTime,
		Size:    size,
		Mode:    0644,
	})
}

func (a *archiveTarget) Write(b []byte) (int, error) {
	return a.out.Write(b)
}

type directoryTarget struct {
	rootDir  string
	currFile *os.File
}

func (d *directoryTarget) Create(path string, size int64, modTime time.Time) error {
	if d.currFile != nil {
		if err := d.currFile.Close(); err != nil {
			return err
		}
	}
	targetPath := filepath.Join(d.rootDir, filepath.FromSlash(path))
	f, err := os.Create(targetPath)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}
		f, err = os.Create(targetPath)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	d.currFile = f
	return nil
}

func (d *directoryTarget) Write(b []byte) (int, error) {
	return d.currFile.Write(b)
}

func (d *directoryTarget) Close() error {
	if d.currFile != nil {
		return d.currFile.Close()
	}
	return nil
}
