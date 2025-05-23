package sorter

import (
	"bytes"
	// "fmt" // Keep commented out unless parseSingleElement's log line is restored
	// "log" // Keep commented out unless parseSingleElement's log line is restored
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

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

func trySortSimpleListTokens(tokens hclwrite.Tokens) (hclwrite.Tokens, bool) {
	if !isValidListStructure(tokens) {
		return tokens, false
	}

	innerTokens := tokens[1 : len(tokens)-1]
	if len(innerTokens) == 0 || checkIgnoreDirective(innerTokens) {
		return tokens, false
	}

	elements, ok := extractSimpleListElements(innerTokens)
	if !ok || len(elements) <= 1 {
		return tokens, false
	}

	sortedElements, hasChanged := sortListElements(elements)
	if !hasChanged {
		return tokens, false
	}

	if hasComments(sortedElements) {
		return rebuildCommentedListTokens(sortedElements, tokens[0], tokens[len(tokens)-1]), true
	}

	return rebuildListTokensFromElements(sortedElements, innerTokens, tokens[0], tokens[len(tokens)-1]), true
}

// isValidListStructure checks if tokens represent a valid list structure [...]
func isValidListStructure(tokens hclwrite.Tokens) bool {
	return len(tokens) >= 2 &&
		tokens[0].Type == hclsyntax.TokenOBrack &&
		tokens[len(tokens)-1].Type == hclsyntax.TokenCBrack
}

// sortListElements sorts the given list elements and returns the sorted list and whether any change occurred
func sortListElements(elements []listElement) ([]listElement, bool) {
	originalOrder := make([]listElement, len(elements))
	copy(originalOrder, elements)

	sort.SliceStable(elements, func(i, j int) bool {
		return compareListElements(elements[i], elements[j])
	})

	// Check if order changed
	for i := 0; i < len(elements); i++ {
		if !bytes.Equal(originalOrder[i].Key, elements[i].Key) {
			return elements, true
		}
	}
	return elements, false
}

// compareListElements provides the comparison logic for sorting list elements
func compareListElements(elemI, elemJ listElement) bool {
	if elemI.IsNumber && elemJ.IsNumber {
		valI := elemI.CtyValue.AsBigFloat()
		valJ := elemJ.CtyValue.AsBigFloat()
		cmpResult := valI.Cmp(valJ)
		if cmpResult != 0 {
			return cmpResult < 0
		}
		return bytes.Compare(elemI.Key, elemJ.Key) == -1
	}

	if elemI.IsNumber && !elemJ.IsNumber {
		return true
	}
	if !elemI.IsNumber && elemJ.IsNumber {
		return false
	}

	return bytes.Compare(elemI.Key, elemJ.Key) == -1
}

// hasComments checks if any element in the list contains comments
func hasComments(elements []listElement) bool {
	for _, elem := range elements {
		for _, t := range elem.Tokens {
			if t.Type == hclsyntax.TokenComment {
				return true
			}
		}
	}
	return false
}

// rebuildCommentedListTokens rebuilds tokens for lists that contain comments
func rebuildCommentedListTokens(elements []listElement, openBracket, closeBracket *hclwrite.Token) hclwrite.Tokens {
	rebuiltTokens := hclwrite.Tokens{openBracket}

	if len(elements) > 0 {
		rebuiltTokens = append(rebuiltTokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
	}

	for i, elem := range elements {
		rebuiltTokens = append(rebuiltTokens, processElementForCommentedList(elem, i, len(elements))...)
	}

	rebuiltTokens = append(rebuiltTokens, ensureTrailingNewline(rebuiltTokens)...)
	rebuiltTokens = append(rebuiltTokens, closeBracket)

	return rebuiltTokens
}

// processElementForCommentedList processes a single element for commented list rebuilding
func processElementForCommentedList(elem listElement, index, totalElements int) hclwrite.Tokens {
	var tokens hclwrite.Tokens

	// Clean leading comments for non-first elements to avoid double-spacing
	cleanedLeadingComments := elem.LeadingComments
	if index > 0 {
		for len(cleanedLeadingComments) > 0 && cleanedLeadingComments[0].Type == hclsyntax.TokenNewline {
			cleanedLeadingComments = cleanedLeadingComments[1:]
		}
	}
	tokens = append(tokens, cleanedLeadingComments...)

	// Separate value and comment tokens
	valueTokens, commentTokens := separateValueAndCommentTokens(elem.Tokens)
	tokens = append(tokens, valueTokens...)

	// Add comma if needed
	if !endsWithComma(valueTokens) && index < totalElements-1 {
		tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")})
	}

	// Add comment tokens
	tokens = append(tokens, commentTokens...)

	// Add newline if no comments (comments typically already have newlines)
	if len(commentTokens) == 0 {
		tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
	}

	return tokens
}

// separateValueAndCommentTokens separates an element's tokens into value and comment tokens
func separateValueAndCommentTokens(tokens hclwrite.Tokens) (valueTokens, commentTokens hclwrite.Tokens) {
	for _, t := range tokens {
		if t.Type == hclsyntax.TokenComment {
			commentTokens = append(commentTokens, t)
		} else {
			valueTokens = append(valueTokens, t)
		}
	}
	return
}

// endsWithComma checks if the token list ends with a comma
func endsWithComma(tokens hclwrite.Tokens) bool {
	return len(tokens) > 0 && tokens[len(tokens)-1].Type == hclsyntax.TokenComma
}

// ensureTrailingNewline ensures the token list ends with a newline if needed
func ensureTrailingNewline(tokens hclwrite.Tokens) hclwrite.Tokens {
	if len(tokens) == 0 {
		return hclwrite.Tokens{}
	}

	lastToken := tokens[len(tokens)-1]
	if lastToken.Type == hclsyntax.TokenNewline ||
		(lastToken.Type == hclsyntax.TokenComment && bytes.HasSuffix(lastToken.Bytes, []byte("\n"))) {
		return hclwrite.Tokens{}
	}

	return hclwrite.Tokens{&hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")}}
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
	willBeMultiLine := isMultiLineList(sortedElements)
	newInnerTokens := buildInnerTokensForUncommentedList(sortedElements, willBeMultiLine)
	isMultiLine := determineMultiLineStatus(newInnerTokens, originalInnerTokens)

	return constructFinalTokenList(newInnerTokens, openingBracket, closingBracket, isMultiLine)
}

// isMultiLineList checks if the list should be formatted as multi-line based on element content
func isMultiLineList(elements []listElement) bool {
	for _, elem := range elements {
		for _, tok := range append(elem.LeadingComments, elem.Tokens...) {
			if tok.Type == hclsyntax.TokenNewline {
				return true
			}
		}
	}
	return false
}

// buildInnerTokensForUncommentedList builds the inner token sequence for lists without comments
func buildInnerTokensForUncommentedList(elements []listElement, willBeMultiLine bool) hclwrite.Tokens {
	var tokens hclwrite.Tokens

	for i, elem := range elements {
		// Clean up leading comments for the first element to avoid extra newlines
		cleanedLeadingComments := elem.LeadingComments
		if i == 0 && willBeMultiLine {
			cleanedLeadingComments = removeLeadingNewlines(cleanedLeadingComments)
		}

		tokens = append(tokens, cleanedLeadingComments...)
		tokens = append(tokens, elem.Tokens...)

		// Add comma between elements (except after the last one)
		if i < len(elements)-1 {
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")})
		}
	}

	return tokens
}

// removeLeadingNewlines removes leading newline tokens from a token list
func removeLeadingNewlines(tokens hclwrite.Tokens) hclwrite.Tokens {
	for len(tokens) > 0 && tokens[0].Type == hclsyntax.TokenNewline {
		tokens = tokens[1:]
	}
	return tokens
}

// determineMultiLineStatus determines if the final list should be formatted as multi-line
func determineMultiLineStatus(newInnerTokens, originalInnerTokens hclwrite.Tokens) bool {
	// Check new tokens for newlines
	for _, tok := range newInnerTokens {
		if tok.Type == hclsyntax.TokenNewline {
			return true
		}
	}

	// If no new tokens but had original tokens, check if original was multi-line
	if len(newInnerTokens) == 0 && len(originalInnerTokens) > 0 {
		for _, tok := range originalInnerTokens {
			if tok.Type == hclsyntax.TokenNewline {
				return true
			}
		}
	}

	return false
}

// constructFinalTokenList builds the complete token list with proper formatting
func constructFinalTokenList(innerTokens hclwrite.Tokens, openBracket, closeBracket *hclwrite.Token, isMultiLine bool) hclwrite.Tokens {
	finalTokens := hclwrite.Tokens{openBracket}

	// Add opening newline for multi-line lists
	if isMultiLine && len(innerTokens) > 0 {
		finalTokens = append(finalTokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
	}

	finalTokens = append(finalTokens, innerTokens...)

	// Add trailing formatting for multi-line lists
	if isMultiLine {
		finalTokens = append(finalTokens, addTrailingFormatting(innerTokens)...)
	}

	finalTokens = append(finalTokens, closeBracket)
	return finalTokens
}

// addTrailingFormatting adds trailing comma and newline for multi-line lists
func addTrailingFormatting(innerTokens hclwrite.Tokens) hclwrite.Tokens {
	var trailing hclwrite.Tokens

	if len(innerTokens) > 0 {
		// Add trailing comma if needed
		if !endsWithMeaningfulComma(innerTokens) {
			trailing = append(trailing, &hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")})
		}

		// Add trailing newline if needed
		if !endsWithNewline(trailing, innerTokens) {
			trailing = append(trailing, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
		}
	}

	return trailing
}

// endsWithMeaningfulComma checks if the tokens end with a meaningful comma (ignoring whitespace)
func endsWithMeaningfulComma(tokens hclwrite.Tokens) bool {
	for i := len(tokens) - 1; i >= 0; i-- {
		tok := tokens[i]
		if tok.Type != hclsyntax.TokenNewline && tok.Type != hclsyntax.TokenTabs {
			return tok.Type == hclsyntax.TokenComma
		}
	}
	return false
}

// endsWithNewline checks if either the trailing tokens or the last inner token is a newline
func endsWithNewline(trailingTokens, innerTokens hclwrite.Tokens) bool {
	// Check trailing tokens first
	if len(trailingTokens) > 0 {
		return trailingTokens[len(trailingTokens)-1].Type == hclsyntax.TokenNewline
	}

	// Check inner tokens
	if len(innerTokens) > 0 {
		return innerTokens[len(innerTokens)-1].Type == hclsyntax.TokenNewline
	}

	return false
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

func parseSingleElement(rawElementTokens hclwrite.Tokens) (*listElement, bool, bool) {
	elementTokensToProcess := rawElementTokens

	// Strip the list-structural trailing comma if present at the end of rawElementTokens.
	if len(rawElementTokens) > 0 {
		lastSignificantTokenIdx := len(rawElementTokens) - 1
		for lastSignificantTokenIdx >= 0 {
			tok := rawElementTokens[lastSignificantTokenIdx]
			if tok.Type == hclsyntax.TokenNewline || tok.Type == hclsyntax.TokenTabs {
				lastSignificantTokenIdx--
			} else {
				break
			}
		}
		if lastSignificantTokenIdx >= 0 && rawElementTokens[lastSignificantTokenIdx].Type == hclsyntax.TokenComma {
			elementTokensToProcess = rawElementTokens[:lastSignificantTokenIdx]
		}
	}

	leadingCommentsAccumulator := hclwrite.Tokens{}
	contentStartIndex := 0
	for contentStartIndex < len(elementTokensToProcess) {
		tok := elementTokensToProcess[contentStartIndex]
		if tok.Type == hclsyntax.TokenNewline || tok.Type == hclsyntax.TokenTabs || tok.Type == hclsyntax.TokenComment {
			leadingCommentsAccumulator = append(leadingCommentsAccumulator, tok)
			contentStartIndex++
		} else {
			break
		}
	}

	elementContentTokens := elementTokensToProcess[contentStartIndex:]

	// Trim trailing newlines/tabs from elementContentTokens to get the final tokens for the element.
	// These tokens will be used for key generation and stored in listElement.tokens.
	contentKeyEndIndex := len(elementContentTokens) - 1
	for contentKeyEndIndex >= 0 {
		tok := elementContentTokens[contentKeyEndIndex]
		if tok.Type == hclsyntax.TokenNewline || tok.Type == hclsyntax.TokenTabs {
			contentKeyEndIndex--
		} else {
			break
		}
	}
	var finalContentTokens hclwrite.Tokens
	if contentKeyEndIndex < 0 && len(elementContentTokens) > 0 { // all were whitespace/newlines
		finalContentTokens = hclwrite.Tokens{}
	} else if contentKeyEndIndex >= 0 {
		finalContentTokens = elementContentTokens[:contentKeyEndIndex+1]
	} else { // elementContentTokens was empty
		finalContentTokens = hclwrite.Tokens{}
	}

	// If after all stripping, there are no content tokens and no leading comments, it's truly an empty spot.
	if len(finalContentTokens) == 0 && len(leadingCommentsAccumulator) == 0 {
		return nil, true, true
	}

	sortKeyBytes, ctyVal, isNum, _ := extractPrimaryTokenBytes(finalContentTokens) // Use _ for success flag

	elem := &listElement{
		LeadingComments: leadingCommentsAccumulator,
		Tokens:          finalContentTokens, // Store the processed content tokens
		Key:             sortKeyBytes,
		CtyValue:        ctyVal,
		IsNumber:        isNum,
	}
	isEmpty := len(finalContentTokens) == 0
	return elem, isEmpty, true
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
// extractSimpleListElements parses the inner tokens of a simple list (excluding outer brackets)
// and extracts each element as a listElement, preserving leading comments and trailing comments.
// It supports both multi-line lists (grouped by line) and inline lists (grouped by commas).
// extractSimpleListElements parses the inner tokens of a list (excluding brackets)
// splitting elements on top-level commas, preserving nested structures and comments.
func extractSimpleListElements(innerTokens hclwrite.Tokens) ([]listElement, bool) {
	var elements []listElement
	var current hclwrite.Tokens
	level := 0
	for i := 0; i < len(innerTokens); i++ {
		tok := innerTokens[i]
		// Track nested braces, brackets, parentheses
		switch tok.Type {
		case hclsyntax.TokenOBrace, hclsyntax.TokenOBrack, hclsyntax.TokenOParen:
			level++
		case hclsyntax.TokenCBrace, hclsyntax.TokenCBrack, hclsyntax.TokenCParen:
			level--
		}
		current = append(current, tok)
		// At top-level comma, finish this element
		if level == 0 && tok.Type == hclsyntax.TokenComma {
			// Include any trailing whitespace/comments immediately after comma
			for i+1 < len(innerTokens) {
				next := innerTokens[i+1]
				if next.Type == hclsyntax.TokenTabs || next.Type == hclsyntax.TokenComment {
					current = append(current, next)
					i++
					continue
				}
				break
			}
			// Parse this element slice
			elem, isEmpty, ok := parseSingleElement(current)
			if !ok {
				return nil, false
			}
			if !isEmpty {
				elements = append(elements, *elem)
			}
			current = hclwrite.Tokens{}
		} else if i == len(innerTokens)-1 {
			// Last element without trailing comma
			elem, isEmpty, ok := parseSingleElement(current)
			if !ok {
				return nil, false
			}
			if !isEmpty {
				elements = append(elements, *elem)
			}
			current = hclwrite.Tokens{}
		}
	}
	return elements, true
}

// --- Helper functions to add ---

// checkSimpleListLiteral checks if tokens represent a simple list literal [...]
// checkSimpleListLiteral checks if tokens represent a simple list literal [...]
// It skips leading/trailing whitespace and newline tokens when detecting the list brackets.
func checkSimpleListLiteral(tokens hclwrite.Tokens) (hclwrite.Tokens, bool) {
	// Skip leading whitespace/newline tokens
	start := 0
	for start < len(tokens) && (tokens[start].Type == hclsyntax.TokenNewline || tokens[start].Type == hclsyntax.TokenTabs) {
		start++
	}
	// Skip trailing whitespace/newline tokens
	end := len(tokens) - 1
	for end >= 0 && (tokens[end].Type == hclsyntax.TokenNewline || tokens[end].Type == hclsyntax.TokenTabs) {
		end--
	}
	// Check for enclosing brackets
	if end > start && tokens[start].Type == hclsyntax.TokenOBrack && tokens[end].Type == hclsyntax.TokenCBrack {
		return tokens[start : end+1], true
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
