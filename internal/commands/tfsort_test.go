package commands

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings" // For comparing output, if needed for more complex stdout checks
	"testing"

	// urfave/cli is needed to construct the app for testing TfsortAction
	"github.com/urfave/cli/v3"
)

// setupTestFiles creates temporary files and directories for testing.
// It returns the root temporary directory path and a cleanup function.
func setupTestFiles(t *testing.T, structure map[string]string) (string, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "tfsort-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	cleanup := func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Failed to remove temp dir %s: %v", tmpDir, err)
		}
	}

	for path, content := range structure {
		// Ensure relative paths are joined with tmpDir
		fullPath := filepath.Join(tmpDir, path)
		dir := filepath.Dir(fullPath)
		// Create parent directories if they don't exist
		if err := os.MkdirAll(dir, 0755); err != nil {
			cleanup()
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
		// Create the file, even if content is empty, if it's a .tf file or has content
		if content != "" || filepath.Ext(path) == ".tf" {
			if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
				cleanup()
				t.Fatalf("Failed to write file %s: %v", fullPath, err)
			}
		}
	}

	return tmpDir, cleanup
}

func TestProcessInputs(t *testing.T) {
	// --- Test Cases Definition ---
	testCases := []struct {
		name        string
		setup       map[string]string
		args        []string
		recursive   bool
		mockStdin   string
		wantSources []InputSource
		wantErr     bool
	}{
		{
			name:        "no args, no stdin pipe",
			args:        []string{},
			recursive:   false,
			wantSources: []InputSource{},
		},
		{
			name:        "no args, but stdin has content (mocked)",
			args:        []string{},
			recursive:   false,
			mockStdin:   "variable \"from_stdin\" {}",
			wantSources: []InputSource{{Path: "<stdin>", Content: []byte("variable \"from_stdin\" {}")}},
		},
		{
			name:        "single valid tf file",
			setup:       map[string]string{"main.tf": "resource \"null_resource\" \"a\" {}"},
			args:        []string{"main.tf"},
			recursive:   false,
			wantSources: []InputSource{{Path: "main.tf", Content: []byte("resource \"null_resource\" \"a\" {}")}},
		},
		{
			name:        "multiple valid tf files",
			setup:       map[string]string{"a.tf": "data {}", "b.tf": "resource {}"},
			args:        []string{"a.tf", "b.tf"},
			recursive:   false,
			wantSources: []InputSource{{Path: "a.tf", Content: []byte("data {}")}, {Path: "b.tf", Content: []byte("resource {}")}},
		},
		{
			name:        "directory without recursive",
			setup:       map[string]string{"subdir/main.tf": "resource {}"},
			args:        []string{"subdir"},
			recursive:   false,
			wantSources: []InputSource{}, // Skips directory
		},
		{
			name: "directory with recursive",
			setup: map[string]string{
				"subdir/main.tf":         "resource {}",
				"subdir/other.txt":       "hello",
				"subdir/nested/vars.tf":  "variable {}",
				"subdir/nested/.dotfile": "ignore me",
			},
			args:      []string{"subdir"},
			recursive: true,
			wantSources: []InputSource{
				{Path: filepath.Join("subdir", "main.tf"), Content: []byte("resource {}")},
				{Path: filepath.Join("subdir", "nested", "vars.tf"), Content: []byte("variable {}")},
			},
		},
		{
			name: "root dir recursive",
			setup: map[string]string{
				"root.tf":     "provider {}",
				"subdir/a.tf": "resource {}",
			},
			args:      []string{"."},
			recursive: true,
			wantSources: []InputSource{
				{Path: "root.tf", Content: []byte("provider {}")},
				{Path: filepath.Join("subdir", "a.tf"), Content: []byte("resource {}")},
			},
		},
		{
			name: "mix files and recursive directory",
			setup: map[string]string{
				"root.tf":        "provider {}",
				"subdir/main.tf": "resource {}",
			},
			args:      []string{"root.tf", "subdir"},
			recursive: true,
			wantSources: []InputSource{
				{Path: "root.tf", Content: []byte("provider {}")},
				{Path: filepath.Join("subdir", "main.tf"), Content: []byte("resource {}")},
			},
		},
		{
			name:        "non-existent file",
			setup:       map[string]string{"exists.tf": "content"},
			args:        []string{"nonexistent.tf", "exists.tf"},
			recursive:   false,
			wantSources: []InputSource{{Path: "exists.tf", Content: []byte("content")}},
		},
		{
			name:        "non-tf file",
			setup:       map[string]string{"main.tf": "resource {}", "notes.txt": "text"},
			args:        []string{"main.tf", "notes.txt"},
			recursive:   false,
			wantSources: []InputSource{{Path: "main.tf", Content: []byte("resource {}")}}, // Skips notes.txt
		},
		{
			name:        "empty tf file",
			setup:       map[string]string{"main.tf": "resource {}", "empty.tf": ""},
			args:        []string{"main.tf", "empty.tf"},
			recursive:   false,
			wantSources: []InputSource{{Path: "main.tf", Content: []byte("resource {}")}}, // Skips empty.tf
		},
		{
			name:        "duplicate file arguments",
			setup:       map[string]string{"main.tf": "resource {}"},
			args:        []string{"main.tf", "main.tf"},
			recursive:   false,
			wantSources: []InputSource{{Path: "main.tf", Content: []byte("resource {}")}}, // Deduplicated
		},
		{
			name: "recursive dir with explicit file inside",
			setup: map[string]string{
				"subdir/main.tf":  "resource {}",
				"subdir/other.tf": "data {}",
			},
			args:      []string{"subdir", filepath.Join("subdir", "main.tf")},
			recursive: true,
			wantSources: []InputSource{
				// Deduplication should keep only one entry per absolute path
				{Path: filepath.Join("subdir", "main.tf"), Content: []byte("resource {}")},
				{Path: filepath.Join("subdir", "other.tf"), Content: []byte("data {}")},
			},
		},
		{
			name:        "absolute path argument",
			setup:       map[string]string{"abs_test.tf": "absolute"},
			args:        []string{},
			recursive:   false,
			wantSources: []InputSource{{Path: "", Content: []byte("absolute")}},
		},
	}

	// --- Test Execution ---
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := "."
			// Initialize cleanup to a no-op function outside the if block
			cleanup := func() {}
			if len(tc.setup) > 0 || tc.name == "absolute path argument" { // Need tmpDir for absolute path test too
				var actualCleanup func()                            // Declare a temporary variable for the real cleanup func
				tmpDir, actualCleanup = setupTestFiles(t, tc.setup) // Assign to the temporary variable
				cleanup = actualCleanup                             // Assign the real cleanup func to the outer scope variable
			}
			defer cleanup() // Defer the cleanup function (either no-op or the real one)

			// Adjust args to be relative to tmpDir, or create absolute path for that test case
			adjustedArgs := make([]string, len(tc.args))
			isAbsTest := tc.name == "absolute path argument"
			if isAbsTest {
				absPath := filepath.Join(tmpDir, "abs_test.tf")
				adjustedArgs = []string{absPath}
				tc.wantSources[0].Path = absPath // Update expected path for absolute test
			} else {
				for i, arg := range tc.args {
					if !strings.HasPrefix(arg, "-") {
						adjustedArgs[i] = filepath.Join(tmpDir, arg)
					} else {
						adjustedArgs[i] = arg
					}
				}
			}

			// Mock Stdin if needed
			originalStdin := os.Stdin // Store original Stdin (type inferred)
			simulatedIsPipe := false  // Assume no pipe unless stdin is mocked

			if tc.mockStdin != "" {
				simulatedIsPipe = true // Mark that we intend to simulate a pipe
				content := []byte(tc.mockStdin)
				r, w, _ := os.Pipe()
				os.Stdin = r
				go func() {
					defer func() {
						if err := w.Close(); err != nil {
							t.Errorf("error closing write pipe in test setup: %v", err)
						}
					}()
					_, err := w.Write(content)
					if err != nil {
						t.Errorf("error writing to stdin pipe in test setup: %v", err)
					}
				}()
				defer func() {
					os.Stdin = originalStdin // Restore Stdin first
					if err := r.Close(); err != nil {
						t.Errorf("error closing read pipe in test setup: %v", err)
					}
				}() // Restore and close read pipe
			} else {
				simulatedIsPipe = false
			}

			// --- Override isInputFromPipe for predictable testing ---
			originalIsInputFromPipe := isInputFromPipe
			isInputFromPipe = func() bool { return simulatedIsPipe }     // Override with test value
			defer func() { isInputFromPipe = originalIsInputFromPipe }() // Restore original

			gotSources, err := processInputs(adjustedArgs, tc.recursive)

			if tc.wantErr {
				if err == nil {
					t.Errorf("processInputs() error = nil, wantErr %v", tc.wantErr)
				}
				return // Error expected and occurred (or not), end test case here
			}
			if err != nil {
				t.Fatalf("processInputs() unexpected error = %v", err)
			}

			// Normalize paths for comparison if needed, but current implementation should handle it.
			for i := range tc.wantSources {
				if tc.wantSources[i].Path != "<stdin>" && !filepath.IsAbs(tc.wantSources[i].Path) && tc.setup[tc.wantSources[i].Path] != "" {
					tc.wantSources[i].Path = filepath.Join(tmpDir, tc.wantSources[i].Path)
				}
			}

			if !equalInputSources(gotSources, tc.wantSources) {
				t.Errorf("processInputs() gotSources = %v, want %v", gotSources, tc.wantSources)
			}
		})
	}
}

// equalInputSources compares two slices of InputSource for equality.
// It sorts them by Path first for stable comparison.
func equalInputSources(a, b []InputSource) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Slice(a, func(i, j int) bool { return a[i].Path < a[j].Path })
	sort.Slice(b, func(i, j int) bool { return b[i].Path < b[j].Path })

	for i := range a {
		if a[i].Path != b[i].Path || !bytes.Equal(a[i].Content, b[i].Content) {
			return false
		}
	}
	return true
}

// captureOutput helper function (as previously defined, but without testify)
func captureOutput(t *testing.T, actionFunc func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Errorf("os.Pipe failed: %v", err)
	}
	originalStdout := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = originalStdout
	}()

	actionFunc() // Execute the function whose output we want to capture

	if err := w.Close(); err != nil {
		t.Logf("Warning: failed to close writer pipe: %v", err)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Logf("Warning: failed to copy from reader pipe to buffer: %v", err)
	}
	return buf.String()
}

func TestTfsortActionOutputModes(t *testing.T) {
	// Assuming setupTestFiles is available from the same package
	// If not, it needs to be defined or imported.

	originalIsInputFromPipe := isInputFromPipe // Store original for restoration
	defer func() { isInputFromPipe = originalIsInputFromPipe }()

	testCases := []struct {
		name                string
		setup               map[string]string // Files to create map[relPath]content
		args                []string          // CLI args (e.g., ["-i", "file.tf"] or ["--dry-run", "file.tf"])
		mockStdin           string            // For stdin input
		wantStdout          string            // Expected content of stdout (if applicable)
		wantFileContent     map[string]string // Expected content of files after run (for -i) map[relPath]content
		wantExitCode        int               // Expected exit code (for --dry-run or errors)
		wantErrMsgSubstring string            // Expected substring in error message from TfsortAction (if any)
	}{
		{
			name:         "default output to stdout",
			setup:        map[string]string{"unsorted.tf": "resource \"b\" \"b\" {}\nvariable \"a\" \"a\" {}\n"},
			args:         []string{"unsorted.tf"}, // This path will be made absolute to tmpDir
			wantStdout:   "variable \"a\" \"a\" {}\n\nresource \"b\" \"b\" {}\n",
			wantExitCode: 0,
		},
		{
			name:  "in-place sorting changes file",
			setup: map[string]string{"unsorted_for_inplace.tf": "resource \"z\" \"z\" {}\nmodule \"a\" \"a\" {}\n"},
			args:  []string{"-i", "unsorted_for_inplace.tf"},
			wantFileContent: map[string]string{
				"unsorted_for_inplace.tf": "module \"a\" \"a\" {}\n\nresource \"z\" \"z\" {}\n",
			},
			wantExitCode: 0,
		},
		{
			name:  "in-place sorting no changes",
			setup: map[string]string{"sorted_for_inplace.tf": "module \"a\" \"a\" {}\n\nresource \"z\" \"z\" {}\n"},
			args:  []string{"-i", "sorted_for_inplace.tf"},
			wantFileContent: map[string]string{
				"sorted_for_inplace.tf": "module \"a\" \"a\" {}\n\nresource \"z\" \"z\" {}\n",
			},
			wantExitCode: 0,
		},
		{
			name:         "dry-run changes detected",
			setup:        map[string]string{"unsorted_for_dryrun.tf": "resource \"b\" \"b\" {}\nvariable \"a\" \"a\" {}\n"},
			args:         []string{"--dry-run", "unsorted_for_dryrun.tf"},
			wantExitCode: 1,
			wantFileContent: map[string]string{ // File should NOT be changed
				"unsorted_for_dryrun.tf": "resource \"b\" \"b\" {}\nvariable \"a\" \"a\" {}\n"},
		},
		{
			name:         "dry-run no changes",
			setup:        map[string]string{"sorted_for_dryrun.tf": "variable \"a\" \"a\" {}\n\nresource \"b\" \"b\" {}\n"},
			args:         []string{"--dry-run", "sorted_for_dryrun.tf"},
			wantExitCode: 0,
			wantFileContent: map[string]string{
				"sorted_for_dryrun.tf": "variable \"a\" \"a\" {}\n\nresource \"b\" \"b\" {}\n",
			},
		},
		{
			name:         "stdin to stdout",
			args:         []string{}, // No file args, implies stdin
			mockStdin:    "resource \"b\" \"b\" {}\nvariable \"a\" \"a\" {}\n",
			wantStdout:   "Reading from stdin...\nvariable \"a\" \"a\" {}\n\nresource \"b\" \"b\" {}\n",
			wantExitCode: 0,
		},
		{
			name:         "stdin with in-place flag (should warn and write to stdout)",
			args:         []string{"-i"},
			mockStdin:    "resource \"b\" \"b\" {}\nvariable \"a\" \"a\" {}\n",
			wantStdout:   "Reading from stdin...\nvariable \"a\" \"a\" {}\n\nresource \"b\" \"b\" {}\n", // Also check log for warning later
			wantExitCode: 0,                                                                             // Expect 0 as it falls back to stdout
		},
		{
			name:         "stdin with dry-run changes detected",
			args:         []string{"--dry-run"},
			mockStdin:    "resource \"b\" \"b\" {}\nvariable \"a\" \"a\" {}\n",
			wantExitCode: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, cleanup := setupTestFiles(t, tc.setup)
			defer cleanup()

			adjustedArgs := make([]string, 0, len(tc.args))

			// If args contains file paths, make them absolute.
			// Flags like -i, --dry-run should be preserved as is.
			for _, arg := range tc.args {
				isFlag := strings.HasPrefix(arg, "-")
				_, isSetupFile := tc.setup[arg]
				if !isFlag && isSetupFile {
					adjustedArgs = append(adjustedArgs, filepath.Join(tmpDir, arg))
				} else {
					adjustedArgs = append(adjustedArgs, arg)
				}
			}

			currentWantFileContent := make(map[string]string)
			if len(tc.wantFileContent) > 0 {
				for relPathSetup, content := range tc.wantFileContent {
					currentWantFileContent[filepath.Join(tmpDir, relPathSetup)] = content
				}
			}

			// Mock Stdin if needed
			var originalStdin *os.File // Keep var declaration here if assigned in if block
			simulatedIsPipe := false

			if tc.mockStdin != "" {
				simulatedIsPipe = true
				originalStdin = os.Stdin // Assign here
				r, w, _ := os.Pipe()
				os.Stdin = r
				go func() {
					defer func() {
						if err := w.Close(); err != nil {
							t.Errorf("error closing write pipe in test setup: %v", err)
						}
					}()
					_, err := w.Write([]byte(tc.mockStdin))
					if err != nil {
						t.Errorf("error writing to stdin pipe in test setup: %v", err)
					}
				}()
				defer func() {
					os.Stdin = originalStdin // Restore Stdin first
					if err := r.Close(); err != nil {
						t.Errorf("error closing read pipe in test setup: %v", err)
					}
				}()
			} else {
				simulatedIsPipe = false
			}
			isInputFromPipe = func() bool { return simulatedIsPipe }

			var actualExitCode int // Initialize to 0, will be set by actionErr handling
			var actionErr error
			var errOutput bytes.Buffer // Buffer for error output

			// Create a new app instance for each test run to avoid state pollution
			app := &cli.Command{
				Name:      "tfsort-test-app",
				Flags:     GetFlags(),
				Action:    TfsortAction,
				ErrWriter: &errOutput, // Set ErrWriter
				ExitErrHandler: func(ctx context.Context, cmd *cli.Command, err error) {
					// Prevent os.Exit during tests
					// Errors will be returned by cmd.Run
				},
			}

			stdoutOutput := captureOutput(t, func() {
				runArgs := append([]string{app.Name}, adjustedArgs...)

				errReturnedByRun := app.Run(context.Background(), runArgs)
				actionErr = errReturnedByRun
			})

			// Determine actualExitCode based on actionErr
			if actionErr == nil {
				actualExitCode = 0
			} else {
				if exitCoder, ok := actionErr.(cli.ExitCoder); ok {
					actualExitCode = exitCoder.ExitCode()
				} else {
					// If TfsortAction returns a non-ExitCoder error, this implies an
					// internal application error. Per our convention (README exit codes),
					// this should correspond to exit code 2.
					actualExitCode = 2
				}
			}

			// Assertions
			if tc.wantStdout != "" {
				// Normalize newlines in wantStdout and actual stdout before comparing
				normalizedWantStdout := strings.ReplaceAll(tc.wantStdout, "\r\n", "\n")
				normalizedActualStdout := strings.ReplaceAll(stdoutOutput, "\r\n", "\n")
				if normalizedWantStdout != normalizedActualStdout {
					t.Errorf("stdout content mismatch for test '%s':\nWant:\n%s\nGot:\n%s", tc.name, normalizedWantStdout, normalizedActualStdout)
				}
			}

			if tc.wantExitCode != actualExitCode {
				t.Errorf("exit code mismatch for test '%s': want %d, got %d. Action error: %v", tc.name, tc.wantExitCode, actualExitCode, actionErr)
			}

			if tc.wantErrMsgSubstring != "" {
				if actionErr == nil {
					t.Errorf("expected error containing '%s' but got nil error for test '%s'", tc.wantErrMsgSubstring, tc.name)
				} else if !strings.Contains(actionErr.Error(), tc.wantErrMsgSubstring) {
					t.Errorf("error message mismatch for test '%s': want substring '%s', got '%s'", tc.name, tc.wantErrMsgSubstring, actionErr.Error())
				}
			} else {
				// If no specific error message is expected, but we got an error that wasn't an ExitCoder(0) or ExitCoder(1)
				if actionErr != nil {
					if exitErr, ok := actionErr.(cli.ExitCoder); ok {
						if exitErr.ExitCode() != 0 && exitErr.ExitCode() != 1 { // 0 and 1 are "normal" exit codes for this app
							t.Errorf("unexpected error for test '%s': %v", tc.name, actionErr)
						}
					} else { // Not an ExitCoder error
						t.Errorf("unexpected non-ExitCoder error for test '%s': %v", tc.name, actionErr)
					}
				}
			}

			if len(tc.wantFileContent) > 0 {
				// t.Logf("Checking file contents for test: %s", tc.name)
				for absPathToCheck := range currentWantFileContent { // Iterate over keys to check existence
					_, err := os.Stat(absPathToCheck)
					if err != nil {
						t.Logf("  os.Stat error for %s: %v", absPathToCheck, err)
					}
				}
			}
		})
	}
}
