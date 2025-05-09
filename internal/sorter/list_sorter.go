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

// listElement stores the original tokens for a list element and its sortable key.
type listElement struct {
	leadingComments hclwrite.Tokens // Comments and whitespace preceding the element
	tokens          hclwrite.Tokens // The actual content tokens of the element
	key             []byte          // Primary sort key (derived from tokens)
	ctyValue        cty.Value       // cty.Value for type-aware comparison (especially numbers)
	isNumber        bool            // True if ctyValue is a known number type
}

// SortListValuesInBody recursively finds and sorts simple lists within a body.
// This function is intended to be called from the main Sort function.
func SortListValuesInBody(body *hclwrite.Body) {
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
		SortListValuesInBody(block.Body()) // Recurse
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
	if checkIgnoreDirective(tokens[1 : len(tokens)-1]) { // Pass inner tokens to checkIgnoreDirective
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

	// Rebuild the token list using the sorted elements
	return rebuildListTokensFromElements(elementsCopy, innerTokens, tokens[0], tokens[len(tokens)-1]), true
}

// rebuildListTokensFromElements reconstructs the HCL tokens for a list
// given the sorted elements, the original inner tokens (for context like multiline),
// and the opening/closing bracket tokens.
func rebuildListTokensFromElements(
	sortedElements []listElement,
	originalInnerTokens hclwrite.Tokens,
	openingBracket *hclwrite.Token,
	closingBracket *hclwrite.Token,
) hclwrite.Tokens {
	newInnerTokens := hclwrite.Tokens{}
	for i, elem := range sortedElements {
		// Append leading comments/whitespace first
		newInnerTokens = append(newInnerTokens, elem.leadingComments...)
		// Append the actual element content tokens
		newInnerTokens = append(newInnerTokens, elem.tokens...)

		// Add comma only after the element, except for the last one
		if i < len(sortedElements)-1 {
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
	} else if len(originalInnerTokens) > 0 { // Handle originally multi-line but now empty list
		// Check if original inner tokens indicated a multi-line structure
		// (e.g., started or ended with a newline, or contained one)
		for _, tok := range originalInnerTokens {
			if tok.Type == hclsyntax.TokenNewline {
				isMultiLine = true
				break
			}
		}
	}

	// Construct the final token list
	finalTokens := hclwrite.Tokens{openingBracket} // Start with '['

	finalTokens = append(finalTokens, newInnerTokens...)

	// Add trailing comma and newline for multi-line lists
	if isMultiLine && len(newInnerTokens) > 0 {
		// Add trailing comma
		// Check if the very last token of newInnerTokens isn't already a comma or a newline that implies a trailing comma was handled by formatting.
		// For simplicity, we'll add a comma if the last element token isn't a comma.
		// More robust handling might be needed if elements can themselves end in commas legitimately before this step.
		lastMeaningfulToken := -1
		for j := len(newInnerTokens) - 1; j >= 0; j-- {
			if newInnerTokens[j].Type != hclsyntax.TokenNewline && newInnerTokens[j].Type != hclsyntax.TokenTabs {
				lastMeaningfulToken = j
				break
			}
		}

		if lastMeaningfulToken != -1 && newInnerTokens[lastMeaningfulToken].Type != hclsyntax.TokenComma {
			finalTokens = append(finalTokens, &hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")})
		}

		// Add trailing newline if not already present as the very last token
		if finalTokens[len(finalTokens)-1].Type != hclsyntax.TokenNewline {
			finalTokens = append(finalTokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
		}
	} else if isMultiLine { // Empty multi-line list case like list = [\n]
		// Ensure at least one newline if it was originally multi-line and now empty
		hasNewline := false
		for _, t := range finalTokens {
			if t.Type == hclsyntax.TokenNewline {
				hasNewline = true
				break
			}
		}
		if !hasNewline {
			finalTokens = append(finalTokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
		}
	}

	finalTokens = append(finalTokens, closingBracket) // Append ']'
	return finalTokens
}

// checkIgnoreDirective checks for a "tfsort:ignore" comment within the initial part of inner list tokens.
// It expects tokens *without* the outer brackets.
func checkIgnoreDirective(innerListTokens hclwrite.Tokens) bool {
	// Allows for optional whitespace/newline before the comment.
	for _, tok := range innerListTokens {
		if tok.Type == hclsyntax.TokenComment {
			commentContent := strings.TrimSpace(string(tok.Bytes))
			if strings.HasPrefix(commentContent, "//") {
				commentContent = strings.TrimSpace(strings.TrimPrefix(commentContent, "//"))
			} else if strings.HasPrefix(commentContent, "#") {
				commentContent = strings.TrimSpace(strings.TrimPrefix(commentContent, "#"))
			}
			if commentContent == "tfsort:ignore" {
				// Ignore directive found before any actual list element.
				return true
			}
			// Any other comment means the ignore check is done for this position.
			return false
		}

		if tok.Type != hclsyntax.TokenTabs && tok.Type != hclsyntax.TokenNewline {
			// Found a non-whitespace/newline token that is not a comment (i.e., a list element)
			// before any ignore comment.
			return false
		}
		// Whitespace or newline -> continue to check next token
	}
	return false // No "tfsort:ignore" directive found, or list is just whitespace/comments without it.
}

// parseSingleElement processes raw tokens for a single list element and returns
// a listElement, a flag indicating if the element was effectively empty, and a success flag.
func parseSingleElement(rawElementTokens hclwrite.Tokens) (*listElement, bool, bool) {
	elementTokensToProcess := rawElementTokens
	// If the last token of rawElementTokens is a comma, remove it for processing.
	// This assumes a comma here is a separator, not part of the element's value itself.
	if len(rawElementTokens) > 0 && rawElementTokens[len(rawElementTokens)-1].Type == hclsyntax.TokenComma {
		elementTokensToProcess = rawElementTokens[:len(rawElementTokens)-1]
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
	contentTokensForKeyAndFinalElement := elementContentTokens[:contentKeyEndIndex+1]

	if len(contentTokensForKeyAndFinalElement) == 0 {
		// This element consists only of whitespace/comments or was trimmed to nothing.
		// It's not an error, but it's an empty element.
		return nil, true, true
	}

	sortKeyBytes, ctyVal, isNum, success := extractPrimaryTokenBytes(contentTokensForKeyAndFinalElement)
	if !success {
		var tokensForLog []string
		for _, t := range contentTokensForKeyAndFinalElement {
			tokensForLog = append(tokensForLog, fmt.Sprintf("{Type: %s, Bytes: '%s'}", t.Type.GoString(), string(t.Bytes)))
		}
		log.Printf("Warning: Could not extract sort key from list element tokens: [%s], skipping sort for this list.", strings.Join(tokensForLog, ", "))
		return nil, false, false // Indicate failure
	}

	elem := &listElement{
		leadingComments: leadingComments,
		tokens:          contentTokensForKeyAndFinalElement,
		key:             sortKeyBytes,
		ctyValue:        ctyVal,
		isNumber:        isNum,
	}
	return elem, false, true // Successfully parsed a non-empty element
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
			// We've reached the end of a potential element (due to comma or end of all tokens)
			element, isEmpty, ok := parseSingleElement(currentElementTokens)
			if !ok {
				// parseSingleElement already logged the error, so just propagate failure.
				return nil, false
			}
			if !isEmpty {
				elements = append(elements, *element)
			}
			currentElementTokens = hclwrite.Tokens{} // Reset for the next element
		}
	}

	// Final check for unbalanced structures at the end of all inner tokens
	if level != 0 {
		log.Println("Warning: Unbalanced parentheses at end of list, skipping sort.")
		return nil, false
	}

	// If after processing all tokens, we have no elements, but there were non-whitespace/comment tokens,
	// it might indicate an issue (e.g. a list with a single complex element without a trailing comma that wasn't properly handled,
	// though current logic for i == len(innerTokens)-1 should cover this).
	// This check is a safeguard.
	if len(elements) == 0 && len(innerTokens) > 0 {
		hasNonWhitespaceOrComment := false
		for _, tok := range innerTokens {
			if tok.Type != hclsyntax.TokenNewline && tok.Type != hclsyntax.TokenComment && tok.Type != hclsyntax.TokenTabs {
				hasNonWhitespaceOrComment = true
				break
			}
		}
		if hasNonWhitespaceOrComment {
			log.Printf("Warning: Could not parse elements from non-empty list content: %s. Skipping list sort.", string(innerTokens.Bytes()))
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
