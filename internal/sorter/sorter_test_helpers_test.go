package sorter

import (
	"bufio"
	"bytes"
	"log"
	"strings"
)

// cleanHCL removes extra whitespace and newlines for easier comparison.
// Note: This is a simplistic approach. Real comparison should ideally
// preserve intended newlines, but for basic sorting tests, this helps.
func cleanHCL(hclBytes []byte) string {
	// Normalize line endings to LF
	normalizedBytes := bytes.ReplaceAll(hclBytes, []byte("\r\n"), []byte("\n"))
	normalizedBytes = bytes.ReplaceAll(normalizedBytes, []byte("\r"), []byte("\n"))

	scanner := bufio.NewScanner(bytes.NewReader(normalizedBytes))
	var cleanedLines []string

	for scanner.Scan() {
		line := scanner.Text()
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine != "" {
			cleanedLines = append(cleanedLines, trimmedLine) // Use TrimSpace result directly
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Error while scanning: %v", err)
	}
	return strings.Join(cleanedLines, "\n")
}
