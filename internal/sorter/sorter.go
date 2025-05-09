package sorter

import (
	"bytes"

	"github.com/hashicorp/hcl/v2/hclwrite"
)

// SortOptions defines the sorting behavior.
type SortOptions struct {
	SortBlocks   bool
	SortTypeName bool
	// List sorting is implicitly always on
}

// Sort parses the input file, sorts it according to options, and returns a new sorted file object.
// If no changes are made, the original file object might be returned (or a new identical one).
func Sort(file *hclwrite.File, options SortOptions) (*hclwrite.File, error) {
	if file == nil || file.Body() == nil {
		return file, nil // Return original if input is invalid or empty
	}

	// Create a new file to build the sorted result
	newFile := hclwrite.NewEmptyFile()
	newBody := newFile.Body()

	// --- Step 1: Copy Attributes (Block sorting shouldn't affect them) ---
	for name, attr := range file.Body().Attributes() {
		// Use SetAttributeRaw to preserve the original expression structure as much as possible
		newBody.SetAttributeRaw(name, attr.Expr().BuildTokens(nil))
	}

	// --- Step 2: Sort Blocks and add to newBody ---
	if options.SortBlocks {
		sortAndAddBlocksToBody(file.Body(), newBody, options) // Call the new function
	} else {
		// If block sorting is disabled, copy blocks in their original order
		blocks := file.Body().Blocks()
		for i, block := range blocks {
			newBody.AppendBlock(block)
			if i < len(blocks)-1 {
				newBody.AppendNewline()
			}
		}
	}

	// --- Step 3: Sort Lists within the new body ---
	SortListValuesInBody(newBody) // Call the new function from list_sorter.go

	// Check if anything actually changed compared to original file bytes
	originalBytes := file.Bytes()
	newBytes := newFile.Bytes()
	if bytes.Equal(originalBytes, newBytes) {
		// Return the original file object pointer if no changes were detected
		// This helps the caller potentially avoid unnecessary writes.
		return file, nil
	}

	return newFile, nil // Return the new, potentially modified file object
}
