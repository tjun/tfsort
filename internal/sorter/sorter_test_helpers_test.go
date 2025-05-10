package sorter

import (
   "bufio"
   "bytes"
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
   // spaceRe := regexp.MustCompile(`\s+`) // Removed intra-line whitespace collapsing

   for scanner.Scan() {
       line := scanner.Text()
       trimmedLine := strings.TrimSpace(line)
       if trimmedLine != "" {
           // collapsedLine := spaceRe.ReplaceAllString(trimmedLine, " ") // Removed
           cleanedLines = append(cleanedLines, trimmedLine) // Use TrimSpace result directly
       }
   }
   // Note: We removed the error check for scanner.Err() for simplicity in tests
   return strings.Join(cleanedLines, "\n")
}