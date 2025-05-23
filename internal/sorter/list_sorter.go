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

type listElement struct {
	leadingComments hclwrite.Tokens // Comments and whitespace preceding the element
	tokens          hclwrite.Tokens // The actual content tokens of the element (may include same-line comments)
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

	// Determine if special comment handling is needed
	hasComment := false
	for _, elem := range elementsCopy {
		for _, t := range elem.tokens { // MODIFIED TO USE elem.tokens
			if t.Type == hclsyntax.TokenComment {
				hasComment = true
				break
			}
		}
		if hasComment {
			break
		}
	}
	if hasComment { // This is the main logic block for commented lists
		// This is the start of the 'if hasComment' block REPLACEMENT
		rebuiltTokens := hclwrite.Tokens{}
		rebuiltTokens = append(rebuiltTokens, tokens[0]) // Append the opening bracket '['

		if len(elementsCopy) > 0 {
			// Add a newline after '[' to signal a multi-line list.
			rebuiltTokens = append(rebuiltTokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
		}

		for i, elem := range elementsCopy {
			rebuiltTokens = append(rebuiltTokens, elem.leadingComments...)

			// Separate value tokens and comment tokens from elem.tokens.
			var valueTokens hclwrite.Tokens
			var commentTokens hclwrite.Tokens
			for _, t := range elem.tokens { // elem.tokens is from the original parseSingleElement
				if t.Type == hclsyntax.TokenComment {
					commentTokens = append(commentTokens, t)
				} else {
					valueTokens = append(valueTokens, t)
				}
			}

			rebuiltTokens = append(rebuiltTokens, valueTokens...)

			// Conditionally add structural comma if not the last element
			// AND valueTokens doesn't already end with a comma.
			valueEndsWithComma := false
			if len(valueTokens) > 0 {
				if valueTokens[len(valueTokens)-1].Type == hclsyntax.TokenComma {
					valueEndsWithComma = true
				}
			}
			if !valueEndsWithComma && i < len(elementsCopy)-1 {
				rebuiltTokens = append(rebuiltTokens, &hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")})
			}

			// Append comment tokens (if any)
			if len(commentTokens) > 0 {
				rebuiltTokens = append(rebuiltTokens, commentTokens...)
			}

			// Conditionally add a newline if the element's content doesn't already end with one.
			elementContentEndsWithNewline := false
			// Check based on the last *actually appended* tokens for this element's line.
			// Start checking from commentTokens, then valueTokens if no comments.
			if len(commentTokens) > 0 {
				lastToken := commentTokens[len(commentTokens)-1]
				if lastToken.Type == hclsyntax.TokenNewline ||
					(lastToken.Type == hclsyntax.TokenComment && bytes.HasSuffix(lastToken.Bytes, []byte("\n"))) {
					elementContentEndsWithNewline = true
				}
			} else if len(valueTokens) > 0 {
				// No line comment tokens, check the last of the value tokens.
				// This also implicitly checks the comma if it was part of valueTokens.
				lastToken := valueTokens[len(valueTokens)-1]
				if lastToken.Type == hclsyntax.TokenNewline ||
					(lastToken.Type == hclsyntax.TokenComment && bytes.HasSuffix(lastToken.Bytes, []byte("\n"))) {
					// This condition is less likely for typical valueTokens if parseSingleElement trims them,
					// but included for robustness.
					elementContentEndsWithNewline = true
				}
			} else if len(elem.leadingComments) > 0 {
				// Element has no value or line comments, only leading comments (e.g. an empty line, or a commented-out item)
				// Check if the leading comments themselves ended with a newline.
				lastToken := elem.leadingComments[len(elem.leadingComments)-1]
				if lastToken.Type == hclsyntax.TokenNewline ||
					(lastToken.Type == hclsyntax.TokenComment && bytes.HasSuffix(lastToken.Bytes, []byte("\n"))) {
					elementContentEndsWithNewline = true
				}
			}
			// If elementContentEndsWithNewline is still false (e.g. simple value, no comment, no trailing newline in value),
			// then we need to add an explicit newline.

			if !elementContentEndsWithNewline {
				rebuiltTokens = append(rebuiltTokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
			}
		}

		// After the loop, if elements were processed, ensure the entire list content
		// ends with a newline before the closing bracket.
		if len(elementsCopy) > 0 {
			alreadyEndsInEffectiveNewline := false
			if rl := len(rebuiltTokens); rl > 0 {
				// Check the very last token added to rebuiltTokens.
				// This could be from the last element's own content (value/comment) or an explicit newline.
				lastBuilderToken := rebuiltTokens[rl-1]
				if lastBuilderToken.Type == hclsyntax.TokenNewline ||
					(lastBuilderToken.Type == hclsyntax.TokenComment && bytes.HasSuffix(lastBuilderToken.Bytes, []byte("\n"))) {
					alreadyEndsInEffectiveNewline = true
				}
			}
			if !alreadyEndsInEffectiveNewline {
				rebuiltTokens = append(rebuiltTokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
			}
		} else if len(innerTokens) > 0 { // Empty list, but was originally multi-line.
			isOriginalMultilineEmpty := false
			for _, tok := range innerTokens {
				if tok.Type == hclsyntax.TokenNewline {
					isOriginalMultilineEmpty = true
					break
				}
			}
			if isOriginalMultilineEmpty && len(rebuiltTokens) == 1 && rebuiltTokens[0].Type == hclsyntax.TokenOBrack {
				rebuiltTokens = append(rebuiltTokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
			}
		}

		rebuiltTokens = append(rebuiltTokens, tokens[len(tokens)-1]) // Append the closing bracket ']'
		return rebuiltTokens, true
		// This is the end of the 'if hasComment' block replacement
	} // End of 'if hasComment' logic block

	// Default rebuild for lists without comments (or if hasComment is false)
	return rebuildListTokensFromElements(elementsCopy, innerTokens, tokens[0], tokens[len(tokens)-1]), true
} // End of trySortSimpleListTokens function

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
		newInnerTokens = append(newInnerTokens, elem.tokens...) // MODIFIED to use elem.tokens

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
		leadingComments: leadingCommentsAccumulator,
		tokens:          finalContentTokens, // Store the processed content tokens
		key:             sortKeyBytes,
		ctyValue:        ctyVal,
		isNumber:        isNum,
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
