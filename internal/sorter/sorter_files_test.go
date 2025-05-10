package sorter

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tjun/tfsort/internal/parser"
)

func TestFiles(t *testing.T) {
	testdataDir := "testdata"

	// Find all input files matching input_*.tf
	inputFiles, err := filepath.Glob(filepath.Join(testdataDir, "input_*.tf"))
	if err != nil {
		t.Fatalf("Failed to glob for input files in %s: %v", testdataDir, err)
	}
	if len(inputFiles) == 0 {
		t.Logf("No input files found matching input_*.tf in %s", testdataDir)
		return
	}

	for _, inputFile := range inputFiles {
		// Derive want file name from input file name
		baseName := filepath.Base(inputFile)
		testName := strings.TrimSuffix(strings.TrimPrefix(baseName, "input_"), ".tf")
		wantFile := filepath.Join(testdataDir, fmt.Sprintf("want_%s.tf", testName))

		// Run each input file as a subtest
		t.Run(testName, func(t *testing.T) {
			inputBytes, err := os.ReadFile(inputFile)
			if err != nil {
				t.Fatalf("Failed to read input file %s: %v", inputFile, err)
			}

			// --- Define Sort Options for Golden Tests ---
			// Consider making this configurable per test if needed later
			sortOptions := SortOptions{
				SortBlocks:   true,
				SortTypeName: true,
				SortList:     true,
			}

			// --- Parse and Sort ---
			hclFile, parseDiags := parser.ParseHCL(inputBytes, inputFile)
			if parseDiags.HasErrors() {
				t.Fatalf("Failed to parse input HCL %s:\n%v", inputFile, parseDiags)
			}
			if hclFile == nil {
				t.Fatalf("Parsed HCL file is nil for %s without errors", inputFile)
			}

			// Call Sort and receive both return values
			sortedFile, err := Sort(hclFile, sortOptions)
			if err != nil {
				t.Fatalf("Sort failed for %s: %v", inputFile, err)
			}
			if sortedFile == nil { // Defensive check
				t.Fatalf("Sort returned nil file for %s without error", inputFile)
			}

			gotBytes := sortedFile.Bytes() // Use the returned file object

			// --- Compare with Golden File ---
			wantBytes, err := os.ReadFile(wantFile)
			if err != nil {
				if os.IsNotExist(err) {
					t.Fatalf("Want file %s not found. Please create it with the expected output.", wantFile)
				}
				t.Fatalf("Failed to read want file %s: %v", wantFile, err)
			}

			// Normalize only line endings for comparison
			normalizedGotBytes := bytes.ReplaceAll(gotBytes, []byte("\r\n"), []byte("\n"))
			normalizedGotBytes = bytes.ReplaceAll(normalizedGotBytes, []byte("\r"), []byte("\n"))

			normalizedWantBytes := bytes.ReplaceAll(wantBytes, []byte("\r\n"), []byte("\n"))
			normalizedWantBytes = bytes.ReplaceAll(normalizedWantBytes, []byte("\r"), []byte("\n"))

			// Compare bytes after normalizing line endings
			if !bytes.Equal(normalizedGotBytes, normalizedWantBytes) {
				t.Errorf("Output for test case '%s' does not match want file %s.\n"+
					"Got:\n---\n%s\n---\nWant:\n---\n%s\n---",
					testName, wantFile, string(gotBytes), string(wantBytes))
				// Optional: Use go-cmp for a detailed diff on non-trimmed content if needed for debugging
				// import "github.com/google/go-cmp/cmp"
				// diff := cmp.Diff(string(wantBytes), string(gotBytes))
				// t.Errorf("Output mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
