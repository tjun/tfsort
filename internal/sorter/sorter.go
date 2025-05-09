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
	blocks := file.Body().Blocks()
	if len(blocks) > 0 {
		originalBlocks := make([]*hclwrite.Block, len(blocks))
		copy(originalBlocks, blocks)
		blocksToSort := blocks // Sort the original slice

		sort.SliceStable(blocksToSort, func(i, j int) bool {
			keyI := getBlockSortKey(blocksToSort[i])
			keyJ := getBlockSortKey(blocksToSort[j])
			if keyI != keyJ {
				return keyI < keyJ
			}
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
				}
			}
			return false
		})

		// Add sorted blocks to the new body
		for i, block := range blocksToSort {
			newBody.AppendBlock(block) // Add block to the new body
			if i < len(blocksToSort)-1 {
				newBody.AppendNewline() // Ensure newline between blocks
			}
		}
	} // else: no blocks to sort

	// --- Step 3: Sort Lists within the new body ---
	sortListValues(newBody) // Apply list sorting to the newly constructed body

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

// --- List Sorting Implementation (String-based, Simple Literals/Idents Only) ---

// listElement stores the original tokens for a list element and its sortable key.
type listElement struct {
	leadingComments hclwrite.Tokens // Comments and whitespace preceding the element
	tokens          hclwrite.Tokens // The actual content tokens of the element
	key             []byte          // Primary sort key (derived from tokens)
	ctyValue        cty.Value       // cty.Value for type-aware comparison (especially numbers)
	isNumber        bool            // True if ctyValue is a known number type
}

// sortListValues recursively finds and sorts simple lists within a body or nested expressions.
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
		originalExprTokens := attr.Expr().BuildTokens(nil)

		// Recursively find and sort lists within the expression tokens
		newExprTokens, wasModified := sortListsInTokens(originalExprTokens)

		if wasModified { // Check if any modification happened
			body.SetAttributeRaw(name, newExprTokens)
		}
	}

	// Recursively sort lists within nested blocks
	for _, block := range body.Blocks() {
		sortListValues(block.Body()) // Recurse
	}
}

// --- New recursive function ---
// sortListsInTokens recursively finds list literals [...] and other structures within a token sequence and sorts them.
// Returns the potentially modified tokens and a boolean indicating if any modification occurred.
func sortListsInTokens(tokens hclwrite.Tokens) (hclwrite.Tokens, bool) {
	// 1. Check if it's a simple list literal
	listTokens, isListLiteral := checkSimpleListLiteral(tokens)
	if isListLiteral {
		return trySortSimpleListTokens(listTokens)
	}

	// 2. Check if it's a toset([...]) call
	listTokensInsideToset, isTosetList := checkTosetListCall(tokens)
	if isTosetList {
		sortedInnerListTokens, listWasSorted := trySortSimpleListTokens(listTokensInsideToset)
		if listWasSorted {
			return buildTosetCallTokens(sortedInnerListTokens), true
		}
		return tokens, false // List inside toset was not sorted
	}

	return tokens, false // Not a recognized list structure to sort
}

// trySortSimpleListTokens attempts to sort tokens representing a simple list.
// It returns the new tokens and true if sorting was successful, otherwise nil and false.
func trySortSimpleListTokens(tokens hclwrite.Tokens) (hclwrite.Tokens, bool) {
	if len(tokens) < 2 || tokens[0].Type != hclsyntax.TokenOBrack || tokens[len(tokens)-1].Type != hclsyntax.TokenCBrack {
		return tokens, false // Not a list or malformed
	}

	// Check for "tfsort:ignore" comment immediately after the opening bracket '['
	// Allows for optional whitespace/newline between '[' and the comment.
	ignoreThisList := false
	for i := 1; i < len(tokens)-1; i++ {
		tok := tokens[i]

		if tok.Type == hclsyntax.TokenComment {
			commentContent := strings.TrimSpace(string(tok.Bytes))
			if strings.HasPrefix(commentContent, "//") {
				commentContent = strings.TrimSpace(strings.TrimPrefix(commentContent, "//"))
			} else if strings.HasPrefix(commentContent, "#") {
				commentContent = strings.TrimSpace(strings.TrimPrefix(commentContent, "#"))
			}
			if commentContent == "tfsort:ignore" {
				// Ignore directive found! Valid only if no element appeared before it.
				ignoreThisList = true
			}
			// If a comment is found (ignore directive or not),
			// the ignore check at this position is complete (any ignore directive further right is invalid).
			break
		}

		if tok.Type != hclsyntax.TokenTabs && tok.Type != hclsyntax.TokenNewline {
			// Found a non-whitespace/newline/comment token (i.e., a list element) before any ignore comment.
			// Exit the loop, leaving ignoreThisList as false.
			break
		}
		// Whitespace or newline -> skip and check the next token
	}

	if ignoreThisList {
		return tokens, false // Ignore!
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
		var result bool
		if elemI.isNumber && elemJ.isNumber {
			valI := elemI.ctyValue.AsBigFloat()
			valJ := elemJ.ctyValue.AsBigFloat()
			cmpResult := valI.Cmp(valJ)
			if cmpResult != 0 {
				result = cmpResult < 0
			} else {
				result = bytes.Compare(elemI.key, elemJ.key) == -1
			}
		} else if elemI.isNumber && !elemJ.isNumber {
			result = true
		} else if !elemI.isNumber && elemJ.isNumber {
			result = false
		} else {
			result = bytes.Compare(elemI.key, elemJ.key) == -1
		}
		return result
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

	// Rebuild the inner token list, adding comma only after the element, except for the last one
	newInnerTokens := hclwrite.Tokens{}
	for i, elem := range elementsCopy {
		// Append leading comments/whitespace first
		newInnerTokens = append(newInnerTokens, elem.leadingComments...)
		// Append the actual element content tokens
		newInnerTokens = append(newInnerTokens, elem.tokens...)

		// Add comma only after the element, except for the last one
		if i < len(elementsCopy)-1 {
			newInnerTokens = append(newInnerTokens, &hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")})
		}
	}

	// Determine if the result should be treated as multi-line
	isMultiLine := false
	if len(newInnerTokens) > 0 {
		for _, tok := range newInnerTokens {
			if tok.Type == hclsyntax.TokenNewline {
				isMultiLine = true
				break
			}
		}
	} else if len(innerTokens) > 0 { // Handle originally multi-line but now empty list
		if innerTokens[0].Type == hclsyntax.TokenNewline || innerTokens[len(innerTokens)-1].Type == hclsyntax.TokenNewline {
			isMultiLine = true // Simplified check for empty multi-line
		}
	}

	// Construct the final token list
	finalTokens := hclwrite.Tokens{tokens[0]} // Start with '['

	// Add leading newline for multi-line non-empty lists if appropriate (optional, based on original)
	// For simplicity, let's rely on leadingComments of the first element for now.

	finalTokens = append(finalTokens, newInnerTokens...)

	// Add trailing comma and newline for multi-line lists
	if isMultiLine && len(newInnerTokens) > 0 {
		// Add trailing comma
		// Check if the very last token isn't already a comma (unlikely with current loop)
		if newInnerTokens[len(newInnerTokens)-1].Type != hclsyntax.TokenComma {
			finalTokens = append(finalTokens, &hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")})
		}
		// Add trailing newline if not already present
		if newInnerTokens[len(newInnerTokens)-1].Type != hclsyntax.TokenNewline {
			finalTokens = append(finalTokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
		}
	} else if isMultiLine { // Empty multi-line list case like list = [\n]
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
	if len(elementTokens) == 1 && elementTokens[0].Type == hclsyntax.TokenNumberLit {
		parsedVal, err := ctyjson.Unmarshal(elementTokens[0].Bytes, cty.Number)
		if err == nil && parsedVal.Type() == cty.Number {
			return key, parsedVal, true, true
		}
	}
	return key, cty.UnknownVal(cty.DynamicPseudoType), false, true
}

// extractSimpleListElements parses the inner tokens of a list (excluding brackets)
// and extracts each element as a listElement including its leading comments/whitespace.
func extractSimpleListElements(innerTokens hclwrite.Tokens) ([]listElement, bool) {
	var elements []listElement
	var currentElementTokens hclwrite.Tokens
	level := 0

	for i := 0; i < len(innerTokens); i++ {
		token := innerTokens[i]
		switch token.Type {
		case hclsyntax.TokenOBrace, hclsyntax.TokenOBrack, hclsyntax.TokenOParen:
			level++
		case hclsyntax.TokenCBrace, hclsyntax.TokenCBrack, hclsyntax.TokenCParen:
			level--
		}

		currentElementTokens = append(currentElementTokens, token)

		if level == 0 && token.Type == hclsyntax.TokenComma || i == len(innerTokens)-1 {
			elementTokensToProcess := currentElementTokens
			if token.Type == hclsyntax.TokenComma {
				elementTokensToProcess = currentElementTokens[:len(currentElementTokens)-1]
			}

			// Separate leading comments/whitespace from the actual content tokens
			leadingComments := hclwrite.Tokens{}
			contentStartIndex := 0
			for contentStartIndex < len(elementTokensToProcess) {
				tok := elementTokensToProcess[contentStartIndex]
				if tok.Type == hclsyntax.TokenNewline || tok.Type == hclsyntax.TokenTabs || tok.Type == hclsyntax.TokenComment {
					leadingComments = append(leadingComments, tok)
					contentStartIndex++
				} else {
					break // Found the start of the content
				}
			}
			elementContentTokens := elementTokensToProcess[contentStartIndex:]

			// Trim trailing whitespace/newline from elementContentTokens for the key generation
			contentKeyEndIndex := len(elementContentTokens) - 1
			for contentKeyEndIndex >= 0 {
				tok := elementContentTokens[contentKeyEndIndex]
				if tok.Type == hclsyntax.TokenNewline || tok.Type == hclsyntax.TokenTabs {
					contentKeyEndIndex--
				} else {
					break
				}
			}

			// Use content without trailing whitespace/NL for key generation AND for stored tokens
			contentTokensForKeyAndFinalElement := elementContentTokens[:contentKeyEndIndex+1]

			if len(contentTokensForKeyAndFinalElement) == 0 {
				currentElementTokens = hclwrite.Tokens{}
				continue
			}
			sortKeyBytes, ctyVal, isNum, success := extractPrimaryTokenBytes(contentTokensForKeyAndFinalElement)
			if !success {
				var tokensForLog []string
				for _, t := range contentTokensForKeyAndFinalElement {
					tokensForLog = append(tokensForLog, fmt.Sprintf("{Type: %s, Bytes: '%s'}", t.Type.GoString(), string(t.Bytes)))
				}
				log.Printf("Warning: Could not extract sort key from list element tokens: [%s], skipping sort for this list.", strings.Join(tokensForLog, ", "))
				return nil, false
			}
			elements = append(elements, listElement{
				leadingComments: leadingComments,
				tokens:          contentTokensForKeyAndFinalElement,
				key:             sortKeyBytes,
				ctyValue:        ctyVal,
				isNumber:        isNum,
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
			log.Printf("Warning: Could not parse elements from non-empty list content that was not an ignore directive: %s. Skipping list sort.", string(innerTokens.Bytes()))
			return nil, false
		}
	}
	return elements, true
}

// --- Helper functions to add ---

// checkSimpleListLiteral checks if tokens represent a simple list literal [...]
func checkSimpleListLiteral(tokens hclwrite.Tokens) (hclwrite.Tokens, bool) {
	if len(tokens) >= 2 && tokens[0].Type == hclsyntax.TokenOBrack && tokens[len(tokens)-1].Type == hclsyntax.TokenCBrack {
		return tokens, true
	}
	return nil, false
}

// checkTosetListCall checks if tokens represent a toset([...]) call
// and returns the inner list tokens [...] if it does.
func checkTosetListCall(tokens hclwrite.Tokens) (innerListTokens hclwrite.Tokens, isTosetList bool) {
	// Expected structure: TokenIdent(toset), TokenOParen, TokenOBrack, ..., TokenCBrack, TokenCParen
	if len(tokens) >= 5 &&
		tokens[0].Type == hclsyntax.TokenIdent && string(tokens[0].Bytes) == "toset" &&
		tokens[1].Type == hclsyntax.TokenOParen &&
		tokens[2].Type == hclsyntax.TokenOBrack &&
		tokens[len(tokens)-2].Type == hclsyntax.TokenCBrack &&
		tokens[len(tokens)-1].Type == hclsyntax.TokenCParen {

		// Extract the tokens between OBrack and CBrack
		return tokens[2 : len(tokens)-1], true // Return the [...] part including brackets
	}
	return nil, false
}

// buildTosetCallTokens rebuilds the toset([...]) expression tokens from sorted inner list tokens.
func buildTosetCallTokens(sortedListTokens hclwrite.Tokens) hclwrite.Tokens {
	// Build: TokenIdent(toset), TokenOParen, sortedListTokens..., TokenCParen
	finalTokens := hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: []byte("toset")},
		{Type: hclsyntax.TokenOParen, Bytes: []byte("(")},
	}
	finalTokens = append(finalTokens, sortedListTokens...)
	finalTokens = append(finalTokens, &hclwrite.Token{Type: hclsyntax.TokenCParen, Bytes: []byte(")")})
	return finalTokens
}
