package sorter

import (
	"bytes"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

// SortOptions defines the sorting behavior.
type SortOptions struct {
	SortBlocks   bool
	SortTypeName bool
	// List sorting is implicitly always on
}

// Sort modifies the given hclwrite.File in place according to the options.
func Sort(file *hclwrite.File, options SortOptions) error {
	if file == nil || file.Body() == nil {
		return nil // Nothing to sort
	}

	if options.SortBlocks {
		sortTopLevelBlocks(file.Body(), options)
	}

	// Always sort list values recursively
	sortListValues(file.Body())

	return nil
}

// Standard Terraform block type order.
var blockOrder = map[string]int{
	"terraform": 1,
	"provider":  2,
	"variable":  3,
	"locals":    4,
	"data":      5,
	"module":    6, // Module before resource
	"resource":  7,
	"output":    8,
	// Add other known block types if necessary
}

// getBlockSortKey determines the primary sort key for a block based on its type.
func getBlockSortKey(block *hclwrite.Block) int {
	blockType := block.Type()
	if order, ok := blockOrder[blockType]; ok {
		return order
	}
	// Assign a high number to unknown block types to sort them last.
	return 99
}

// sortTopLevelBlocks sorts the blocks directly within the body.
func sortTopLevelBlocks(body *hclwrite.Body, options SortOptions) {
	blocks := body.Blocks() // Get a slice of the current blocks

	if len(blocks) <= 1 {
		return // No need to sort 0 or 1 blocks
	}

	// Create a copy to sort, keep original order for comparison
	originalBlocks := make([]*hclwrite.Block, len(blocks))
	copy(originalBlocks, blocks)
	blocksToSort := blocks // Use the original slice reference for sorting

	// Sort the blocks slice based on the defined order.
	sort.SliceStable(blocksToSort, func(i, j int) bool {
		keyI := getBlockSortKey(blocksToSort[i])
		keyJ := getBlockSortKey(blocksToSort[j])

		if keyI != keyJ {
			return keyI < keyJ
		}

		// Primary keys are the same, apply secondary sort if applicable.
		if options.SortTypeName && (blocksToSort[i].Type() == "resource" || blocksToSort[i].Type() == "data") {
			labelsI := blocksToSort[i].Labels()
			labelsJ := blocksToSort[j].Labels()

			if len(labelsI) > 0 && len(labelsJ) > 0 {
				if labelsI[0] != labelsJ[0] {
					return labelsI[0] < labelsJ[0]
				}
				if len(labelsI) > 1 && len(labelsJ) > 1 {
					if labelsI[1] != labelsJ[1] {
						return labelsI[1] < labelsJ[1]
					}
				}
				return false
			}
			return false
		}
		return false
	})

	// Check if the order actually changed before rewriting
	changed := false
	for i := range blocksToSort {
		if blocksToSort[i] != originalBlocks[i] { // Compare pointers
			changed = true
			break
		}
	}

	if !changed {
		return // Order hasn't changed, no need to rewrite blocks
	}

	// Remove all existing blocks from the body first.
	currentBlocks := body.Blocks() // Get snapshot before removal
	for _, block := range currentBlocks {
		body.RemoveBlock(block)
	}

	// Append the blocks back in the sorted order, ensuring one newline between them.
	for i, block := range blocksToSort { // Use the sorted slice
		body.AppendBlock(block)
		// Add exactly one newline after each block, except the last one.
		if i < len(blocksToSort)-1 {
			body.AppendNewline()
		}
	}
}

// --- List Sorting Implementation (String-based, Simple Literals/Idents Only) ---

// listElement stores the original tokens for a list element and its sortable key.
type listElement struct {
	tokens   hclwrite.Tokens
	key      []byte    // Primary sort key (raw HCL string representation of the element)
	ctyValue cty.Value // cty.Value for type-aware comparison (especially numbers)
	isNumber bool      // True if ctyValue is a known number type
}

// sortListValues recursively finds and sorts simple lists within a body.
func sortListValues(body *hclwrite.Body) {
	if body == nil {
		return
	}

	attrs := body.Attributes()
	attrNames := make([]string, 0, len(attrs))
	for name := range attrs {
		attrNames = append(attrNames, name)
	}
	sort.Strings(attrNames) // Process attributes in a consistent order

	for _, name := range attrNames {
		attr := attrs[name]
		attrExprTokens := attr.Expr().BuildTokens(nil)

		// Attempt to parse and sort if it looks like a simple list
		newTokens, wasSorted := trySortSimpleListTokens(attrExprTokens)
		if wasSorted {
			body.SetAttributeRaw(name, newTokens)
		}
	}

	// Recursively sort lists within nested blocks
	for _, block := range body.Blocks() {
		sortListValues(block.Body()) // Recurse
	}
}

// trySortSimpleListTokens attempts to sort tokens representing a simple list.
// It returns the new tokens and true if sorting was successful, otherwise nil and false.
func trySortSimpleListTokens(tokens hclwrite.Tokens) (hclwrite.Tokens, bool) {
	if len(tokens) < 2 || tokens[0].Type != hclsyntax.TokenOBrack || tokens[len(tokens)-1].Type != hclsyntax.TokenCBrack {
		return tokens, false // Not a list or malformed
	}

	// Check for "tfsort:ignore" comment immediately after the opening bracket '['
	// Allows for optional whitespace/newline between '[' and the comment.
	for i := 1; i < len(tokens)-1; i++ { // Iterate tokens after '[' and before ']'
		tok := tokens[i]
		if tok.Type == hclsyntax.TokenComment && bytes.Contains(tok.Bytes, []byte("tfsort:ignore")) {
			return tokens, false // Found ignore directive, do not sort
		}
		// If we find something other than whitespace or a newline before an ignore comment,
		// then the ignore directive is not in the valid position.
		if tok.Type != hclsyntax.TokenTabs && tok.Type != hclsyntax.TokenNewline {
			break
		}
	}

	innerTokens := tokens[1 : len(tokens)-1]
	if len(innerTokens) == 0 {
		return tokens, false // Empty list, no change
	}

	// extractSimpleListElements will also check for ignore comments within the elements.
	elementsCopy, ok := extractSimpleListElements(innerTokens)
	if !ok || len(elementsCopy) <= 1 { // If not parseable (e.g., due to ignore) or only one/zero elements
		return tokens, false
	}

	// Check if any actual change would occur before rebuilding tokens
	originalOrder := make([]listElement, len(elementsCopy))
	copy(originalOrder, elementsCopy)

	sort.SliceStable(elementsCopy, func(i, j int) bool {
		elemI := elementsCopy[i]
		elemJ := elementsCopy[j]

		// Type-aware comparison: numbers first, then by byte key
		if elemI.isNumber && elemJ.isNumber {
			// Compare numbers using their big.Float representation
			valI := elemI.ctyValue.AsBigFloat()
			valJ := elemJ.ctyValue.AsBigFloat()
			cmpResult := valI.Cmp(valJ)
			if cmpResult != 0 {
				return cmpResult < 0
			}
			// If numeric values are equal, fallback to key comparison
		}

		// If one is a number and the other isn't, numbers come first.
		if elemI.isNumber && !elemJ.isNumber {
			return true
		}
		if !elemI.isNumber && elemJ.isNumber {
			return false
		}

		// Default to byte comparison for non-numbers or as tie-breaker
		return bytes.Compare(elemI.key, elemJ.key) == -1
	})

	changed := false
	if len(elementsCopy) != len(originalOrder) { // Should not happen but as a safeguard
		changed = true
	} else {
		for i := 0; i < len(elementsCopy); i++ {
			if !bytes.Equal(originalOrder[i].key, elementsCopy[i].key) {
				changed = true
				break
			}
		}
	}

	if !changed {
		return tokens, false
	}

	// Rebuild the inner token list
	newInnerTokens := hclwrite.Tokens{}
	for i, elem := range elementsCopy {
		newInnerTokens = append(newInnerTokens, elem.tokens...)
		if i < len(elementsCopy)-1 {
			newInnerTokens = append(newInnerTokens, &hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")})
		}
	}

	// Construct the final token list
	finalTokens := hclwrite.Tokens{tokens[0]} // Start with '['
	finalTokens = append(finalTokens, newInnerTokens...)

	originallyMultiLine := false
	if len(innerTokens) > 0 {
		// Check if original innerTokens contained any newline, implying it was multi-line.
		// Also, ensure the last token of the original inner list was a newline.
		// This is a heuristic. A more robust check might involve analyzing line numbers.
		hasInternalNewline := false
		// Check all but the last token for a newline to determine if it was formatted over multiple lines internally.
		// If the list has only one element and it's on a newline, that also counts.
		if len(innerTokens) > 1 {
			for _, tok := range innerTokens[:len(innerTokens)-1] {
				if tok.Type == hclsyntax.TokenNewline {
					hasInternalNewline = true
					break
				}
			}
		}

		// A list is considered multi-line if its first element starts on a new line,
		// or if it has internal newlines, AND it ends with a newline before the closing bracket.
		if (innerTokens[0].Type == hclsyntax.TokenNewline || hasInternalNewline) && innerTokens[len(innerTokens)-1].Type == hclsyntax.TokenNewline {
			originallyMultiLine = true
		}
	}

	if len(newInnerTokens) > 0 { // If there are elements in the sorted list
		if originallyMultiLine {
			// If the original list was multi-line, ensure the sorted list also ends with a newline before the bracket
			lastGeneratedToken := newInnerTokens[len(newInnerTokens)-1]
			if lastGeneratedToken.Type != hclsyntax.TokenNewline {
				finalTokens = append(finalTokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
			}
		} // No specific action for single-line original if newInnerTokens ends with NL; it's preserved.
	} else if originallyMultiLine { // Original list was multi-line but new list is empty
		// Preserve the multi-line empty list format e.g. list = [
		// ]
		finalTokens = append(finalTokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
	}

	finalTokens = append(finalTokens, tokens[len(tokens)-1]) // Append ']'

	return finalTokens, true
}

// extractPrimaryTokenBytes extracts the sortable byte key and cty.Value from a slice of tokens.
func extractPrimaryTokenBytes(elementTokens hclwrite.Tokens) (key []byte, val cty.Value, isNum bool, success bool) {
	if len(elementTokens) == 0 {
		return nil, cty.NilVal, false, false
	}
	var keyBuffer bytes.Buffer
	for _, token := range elementTokens {
		keyBuffer.Write(token.Bytes)
	}
	key = keyBuffer.Bytes()

	// Attempt to parse the token bytes as a cty.Value to check for numbers
	if len(elementTokens) == 1 && elementTokens[0].Type == hclsyntax.TokenNumberLit {
		// Use ctyjson.Unmarshal to parse the number literal bytes directly
		parsedVal, err := ctyjson.Unmarshal(elementTokens[0].Bytes, cty.Number)
		if err == nil && parsedVal.Type() == cty.Number {
			return key, parsedVal, true, true
		}
	}

	// For other types or if parsing as number fails, return with isNum=false
	// The key is still valid for byte-wise sorting.
	// For non-number literals, we might not get a useful cty.Value directly from Value(nil) on arbitrary tokens,
	// so we'll rely on the byte key for sorting them.
	// A more sophisticated approach would involve a full expression parser.
	return key, cty.UnknownVal(cty.DynamicPseudoType), false, true // Success is true because we have a key
}

// extractSimpleListElements parses the inner tokens of a list (excluding brackets)
// and extracts each element as a listElement. It also handles commas and newlines.
// It returns a slice of listElements and a boolean indicating success.
func extractSimpleListElements(innerTokens hclwrite.Tokens) ([]listElement, bool) {
	var elements []listElement
	var currentElementTokens hclwrite.Tokens
	level := 0 // To handle nested structures like function calls or other lists/maps if we decide to support them later (currently not used for sorting key itself)

	for i := 0; i < len(innerTokens); i++ {
		token := innerTokens[i]

		// Basic brace/bracket/paren matching to identify element boundaries
		switch token.Type {
		case hclsyntax.TokenOBrace, hclsyntax.TokenOBrack, hclsyntax.TokenOParen:
			level++
		case hclsyntax.TokenCBrace, hclsyntax.TokenCBrack, hclsyntax.TokenCParen:
			level--
		}

		currentElementTokens = append(currentElementTokens, token)

		// Element ends at a comma (if not nested) or at the end of tokens
		if level == 0 && token.Type == hclsyntax.TokenComma || i == len(innerTokens)-1 {
			// If ended with a comma, remove the comma from currentElementTokens for the actual element
			elementTokensToProcess := currentElementTokens
			if token.Type == hclsyntax.TokenComma {
				elementTokensToProcess = currentElementTokens[:len(currentElementTokens)-1]
			}

			// Trim leading/trailing whitespace/newline tokens from the element itself for a cleaner key
			// but keep them in the listElement.tokens for reconstruction.
			startIndex, endIndex := 0, len(elementTokensToProcess)-1
			for startIndex < len(elementTokensToProcess) && (elementTokensToProcess[startIndex].Type == hclsyntax.TokenNewline || elementTokensToProcess[startIndex].Type == hclsyntax.TokenTabs) {
				startIndex++
			}
			for endIndex >= startIndex && (elementTokensToProcess[endIndex].Type == hclsyntax.TokenNewline || elementTokensToProcess[endIndex].Type == hclsyntax.TokenTabs) {
				endIndex--
			}

			if startIndex > endIndex { // Element was all whitespace/newlines
				// Skip elements that are purely whitespace.
				currentElementTokens = hclwrite.Tokens{}
				continue // Skip to the next token in innerTokens
			}

			contentTokensForKey := elementTokensToProcess[startIndex : endIndex+1]
			sortKeyBytes, ctyVal, isNum, success := extractPrimaryTokenBytes(contentTokensForKey)
			if !success {
				var tokensForLog []string
				for _, t := range contentTokensForKey {
					tokensForLog = append(tokensForLog, fmt.Sprintf("{Type: %s, Bytes: '%s'}", t.Type.GoString(), string(t.Bytes)))
				}
				log.Printf("Warning: Could not extract sort key from list element tokens: [%s], skipping sort for this list.", strings.Join(tokensForLog, ", "))
				return nil, false
			}
			elements = append(elements, listElement{
				tokens:   elementTokensToProcess, // Store original full tokens for the element
				key:      sortKeyBytes,           // Store key derived from content tokens
				ctyValue: ctyVal,
				isNumber: isNum,
			})
			currentElementTokens = hclwrite.Tokens{}
		}
	}

	// Final check for unbalanced structures at the end of all inner tokens
	if level != 0 {
		log.Println("Warning: Unbalanced parentheses at end of list, skipping sort.")
		return nil, false
	}
	if len(elements) == 0 && len(innerTokens) > 0 {
		hasNonWhitespaceOrComment := false
		for _, tok := range innerTokens {
			if tok.Type != hclsyntax.TokenNewline && tok.Type != hclsyntax.TokenComment && tok.Type != hclsyntax.TokenTabs {
				hasNonWhitespaceOrComment = true
				break
			}
		}
		if hasNonWhitespaceOrComment {
			// This case implies that innerTokens had content, but no elements were extracted.
			// This could happen with malformed lists or lists with only comments that aren't tfsort:ignore.
			// The tfsort:ignore directive at the list start is handled in trySortSimpleListTokens.
			log.Printf("Warning: Could not parse elements from non-empty list content that was not an ignore directive: %s. Skipping list sort.", string(innerTokens.Bytes()))
			return nil, false
		}
	}
	return elements, true
}
