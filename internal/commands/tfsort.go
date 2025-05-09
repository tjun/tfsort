package commands

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/tjun/tfsort/internal/parser"
	"github.com/tjun/tfsort/internal/sorter"
	"github.com/urfave/cli/v3"
)

// InputSource represents a single source of HCL content (file or stdin)
type InputSource struct {
	Path    string // File path or "<stdin>"
	Content []byte
}

// flags defines the CLI flags for the tfsort command.
var flags = []cli.Flag{
	&cli.BoolFlag{
		Name:    "recursive",
		Aliases: []string{"r"},
		Usage:   "Walk directories recursively and process all `*.tf` files",
	},
	&cli.BoolFlag{
		Name:    "in-place",
		Aliases: []string{"i"},
		Usage:   "Overwrite files in place instead of printing to stdout",
	},
	&cli.BoolFlag{
		Name:  "sort-blocks",
		Value: true,
		Usage: "Enable sorting of top-level blocks",
	},
	&cli.BoolFlag{
		Name:  "sort-type-name",
		Value: true,
		Usage: "Enable sorting of `resource`/`data` **type** and **name**",
	},
	&cli.BoolFlag{
		Name:  "dry-run",
		Usage: "Exit with non-zero status if changes would be made",
	},
}

// GetFlags returns the flags for the tfsort command.
func GetFlags() []cli.Flag {
	return flags
}

// TfsortAction defines the core action for the tfsort command.
// v3 signature: func(context.Context, *cli.Command) error
func TfsortAction(ctx context.Context, cmd *cli.Command) error {
	// Extract arguments and flags from the context
	args := cmd.Args().Slice() // Get Args from cmd

	// Check if any positional argument looks like a flag
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			// Return a specific error message guiding the user
			return cli.Exit(fmt.Sprintf("Error: Flag '%s' found after file arguments. Please place flags before file arguments.", arg), 1)
		}
	}

	recursive := cmd.Bool("recursive")

	// Call the processing function with extracted values
	sources, err := processInputs(args, recursive) // Renamed function
	if err != nil {
		return fmt.Errorf("failed to process inputs: %w", err)
	}

	if len(sources) == 0 {
		if len(args) > 0 { // Check if args were originally provided
			log.Println("No input files found.")
		} else if !isInputFromPipe() {
			log.Println("No input files specified and no data piped from stdin.")
		}
		return nil
	}

	hasErrors := false
	changedInDryRun := false

	inPlace := cmd.Bool("in-place")
	dryRun := cmd.Bool("dry-run")

	sortOpts := sorter.SortOptions{
		SortBlocks:   cmd.Bool("sort-blocks"),
		SortTypeName: cmd.Bool("sort-type-name"),
	}

	for _, source := range sources {
		log.Printf("Processing: %s", source.Path)
		hclFile, parseDiags := parser.ParseHCL(source.Content, source.Path)

		if parseDiags.HasErrors() {
			log.Printf("Error parsing %s: %v", source.Path, parseDiags)
			hasErrors = true
			continue
		}
		if hclFile == nil { // Should not happen if no errors, but good to check
			log.Printf("Error parsing %s: parsed file is nil despite no errors", source.Path)
			hasErrors = true
			continue
		}

		originalBytes := make([]byte, len(source.Content))
		copy(originalBytes, source.Content)

		// Call the modified Sort function which returns a new file object
		sortedFile, err := sorter.Sort(hclFile, sortOpts)
		if err != nil {
			log.Printf("Error sorting %s: %v", source.Path, err)
			hasErrors = true
			continue
		}

		// If the returned file pointer is the same as the original, no changes were made.
		// Otherwise, compare bytes for robustness (though pointer check should suffice if Sort guarantees it).
		sortedBytes := sortedFile.Bytes()
		changed := !bytes.Equal(originalBytes, sortedBytes)
		// Alternative change detection: changed := sortedFile != hclFile (if Sort guarantees returning original on no change)

		if dryRun {
			if changed {
				changedInDryRun = true
				log.Printf("File %s would be changed.", source.Path)
			}
		} else if inPlace {
			if source.Path == "<stdin>" {
				log.Println("Warning: cannot write in-place for stdin input. Writing to stdout instead.")
				_, err := os.Stdout.Write(sortedBytes)
				if err != nil {
					log.Printf("Error writing to stdout for stdin (in-place mode): %v", err)
					hasErrors = true
				}
			} else if changed { // Only write if content actually changed
				if err := os.WriteFile(source.Path, sortedBytes, 0644); err != nil {
					log.Printf("Error writing file %s: %v", source.Path, err)
					hasErrors = true
				} else {
					log.Printf("Formatted %s", source.Path)
				}
			} else {
				log.Printf("No changes for %s", source.Path)
			}
		} else { // Default: print to stdout
			_, err := os.Stdout.Write(sortedBytes)
			if err != nil {
				log.Printf("Error writing to stdout for %s: %v", source.Path, err)
				hasErrors = true
			}
		}
	}

	if hasErrors {
		return cli.Exit("Encountered errors during processing.", 2)
	}

	if dryRun && changedInDryRun {
		return cli.Exit("Changes would be made.", 1)
	}

	return nil
}

// processInputs determines the target HCL sources based on arguments and flags.
func processInputs(args []string, recursive bool) ([]InputSource, error) {
	var sources []InputSource

	if len(args) == 0 && isInputFromPipe() {
		fmt.Println("Reading from stdin...")
		content, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read from stdin: %w", err)
		}
		if len(content) > 0 {
			sources = append(sources, InputSource{Path: "<stdin>", Content: content})
		}
	} else if len(args) > 0 {
		var filePaths []string
		for _, arg := range args {
			info, err := os.Stat(arg)
			if err != nil {
				log.Printf("Warning: could not stat %q: %v", arg, err)
				continue
			}

			if info.IsDir() {
				if recursive {
					err := filepath.WalkDir(arg, func(path string, d os.DirEntry, err error) error {
						if err != nil {
							log.Printf("Warning: error accessing path %q: %v", path, err)
							return nil
						}
						if !d.IsDir() && strings.HasSuffix(d.Name(), ".tf") {
							filePaths = append(filePaths, path)
						}
						return nil
					})
					if err != nil {
						log.Printf("Warning: error walking directory %q: %v", arg, err)
					}
				} else {
					log.Printf("Warning: skipping directory %q (use -r to process recursively)", arg)
				}
			} else if strings.HasSuffix(info.Name(), ".tf") {
				filePaths = append(filePaths, arg)
			} else {
				log.Printf("Warning: skipping non-.tf file %q", arg)
			}
		}

		seen := make(map[string]bool)
		uniquePaths := []string{}
		for _, p := range filePaths {
			absPath, err := filepath.Abs(p)
			if err != nil {
				log.Printf("Warning: could not get absolute path for %q: %v", p, err)
				absPath = p
			}
			if !seen[absPath] {
				seen[absPath] = true
				uniquePaths = append(uniquePaths, p)
			}
		}

		for _, path := range uniquePaths {
			content, err := os.ReadFile(path)
			if err != nil {
				log.Printf("Warning: failed to read file %q: %v", path, err)
				continue
			}
			if len(content) > 0 {
				sources = append(sources, InputSource{Path: path, Content: content})
			} else {
				log.Printf("Warning: skipping empty file %q", path)
			}
		}
	}

	return sources, nil
}

// isInputFromPipe checks if the program is receiving input from a pipe.
var isInputFromPipe = func() bool {
	fileInfo, _ := os.Stdin.Stat()
	return fileInfo != nil && (fileInfo.Mode()&os.ModeCharDevice) == 0
}
