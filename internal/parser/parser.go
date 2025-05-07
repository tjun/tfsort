package parser

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// ParseHCL parses the given HCL content using hclwrite to preserve formatting.
// filename is used for context in error messages.
func ParseHCL(content []byte, filename string) (*hclwrite.File, hcl.Diagnostics) {
	file, diags := hclwrite.ParseConfig(content, filename, hcl.InitialPos)

	// If diags contains errors, we still return the partially parsed file
	// along with the diagnostics. The caller can decide how to handle it.
	// The file might be nil if parsing failed very early.
	return file, diags // Return file and diags directly
}
