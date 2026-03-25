package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	log.SetFlags(0)

	if len(os.Args) < 2 {
		printUsageAndExit()
	}

	switch os.Args[1] {
	case "msi-info":
		runMSIInfo(os.Args[2:])
	case "cab-extract":
		runCABExtract(os.Args[2:])
	case "zip-list":
		runZIPList(os.Args[2:])
	case "zip-extract":
		runZIPExtract(os.Args[2:])
	case "write-vfs":
		runWriteVFS(os.Args[2:])
	case "help", "-h", "--help":
		printUsageAndExit()
	default:
		log.Fatalf("unknown subcommand %q", os.Args[1])
	}
}

func printUsageAndExit() {
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  winsysroot msi-info --input <path> [--out <path>]\n")
	fmt.Fprintf(os.Stderr, "  winsysroot cab-extract --layout <path> --out-dir <dir> --cab <path> [--cab <path> ...]\n")
	fmt.Fprintf(os.Stderr, "  winsysroot zip-list --input <path> [--out <path>]\n")
	fmt.Fprintf(os.Stderr, "  winsysroot zip-extract --input <path> --layout <path> --out-dir <dir>\n")
	fmt.Fprintf(os.Stderr, "  winsysroot write-vfs --root-dir <dir> [--virtual-root <path>] [--out <path>]\n")
	os.Exit(2)
}

func parseFlagsOrExit(fs *flag.FlagSet, args []string) {
	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}
}
