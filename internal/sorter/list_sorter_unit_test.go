package sorter

import (
	"testing"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

func TestIsValidListStructure(t *testing.T) {
	tests := []struct {
		name     string
		tokens   hclwrite.Tokens
		expected bool
	}{
		{
			name: "valid list structure",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
				{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")},
			},
			expected: true,
		},
		{
			name:     "empty tokens",
			tokens:   hclwrite.Tokens{},
			expected: false,
		},
		{
			name: "single token",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
			},
			expected: false,
		},
		{
			name: "wrong token types",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenOParen, Bytes: []byte("(")},
				{Type: hclsyntax.TokenCParen, Bytes: []byte(")")},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidListStructure(tt.tokens)
			if result != tt.expected {
				t.Errorf("isValidListStructure() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCompareListElements(t *testing.T) {
	tests := []struct {
		name     string
		elemI    listElement
		elemJ    listElement
		expected bool
	}{
		{
			name: "string comparison - i less than j",
			elemI: listElement{
				Key:      []byte("apple"),
				IsNumber: false,
			},
			elemJ: listElement{
				Key:      []byte("banana"),
				IsNumber: false,
			},
			expected: true,
		},
		{
			name: "string comparison - i greater than j",
			elemI: listElement{
				Key:      []byte("banana"),
				IsNumber: false,
			},
			elemJ: listElement{
				Key:      []byte("apple"),
				IsNumber: false,
			},
			expected: false,
		},
		{
			name: "number comparison - smaller number first",
			elemI: listElement{
				Key:      []byte("10"),
				CtyValue: cty.NumberIntVal(10),
				IsNumber: true,
			},
			elemJ: listElement{
				Key:      []byte("20"),
				CtyValue: cty.NumberIntVal(20),
				IsNumber: true,
			},
			expected: true,
		},
		{
			name: "number vs string - number comes first",
			elemI: listElement{
				Key:      []byte("10"),
				CtyValue: cty.NumberIntVal(10),
				IsNumber: true,
			},
			elemJ: listElement{
				Key:      []byte("apple"),
				IsNumber: false,
			},
			expected: true,
		},
		{
			name: "string vs number - number comes first",
			elemI: listElement{
				Key:      []byte("apple"),
				IsNumber: false,
			},
			elemJ: listElement{
				Key:      []byte("10"),
				CtyValue: cty.NumberIntVal(10),
				IsNumber: true,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareListElements(tt.elemI, tt.elemJ)
			if result != tt.expected {
				t.Errorf("compareListElements() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestHasComments(t *testing.T) {
	tests := []struct {
		name     string
		elements []listElement
		expected bool
	}{
		{
			name: "no comments",
			elements: []listElement{
				{
					Tokens: hclwrite.Tokens{
						{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
						{Type: hclsyntax.TokenQuotedLit, Bytes: []byte("test")},
						{Type: hclsyntax.TokenCQuote, Bytes: []byte("\"")},
					},
				},
			},
			expected: false,
		},
		{
			name: "has comments",
			elements: []listElement{
				{
					Tokens: hclwrite.Tokens{
						{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
						{Type: hclsyntax.TokenQuotedLit, Bytes: []byte("test")},
						{Type: hclsyntax.TokenCQuote, Bytes: []byte("\"")},
						{Type: hclsyntax.TokenComment, Bytes: []byte("# comment")},
					},
				},
			},
			expected: true,
		},
		{
			name:     "empty elements",
			elements: []listElement{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasComments(tt.elements)
			if result != tt.expected {
				t.Errorf("hasComments() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSeparateValueAndCommentTokens(t *testing.T) {
	tests := []struct {
		name            string
		tokens          hclwrite.Tokens
		expectedValue   int // count of value tokens
		expectedComment int // count of comment tokens
	}{
		{
			name: "mixed tokens",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenQuotedLit, Bytes: []byte("test")},
				{Type: hclsyntax.TokenCQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenComment, Bytes: []byte("# comment")},
			},
			expectedValue:   3,
			expectedComment: 1,
		},
		{
			name: "only value tokens",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenQuotedLit, Bytes: []byte("test")},
				{Type: hclsyntax.TokenCQuote, Bytes: []byte("\"")},
			},
			expectedValue:   3,
			expectedComment: 0,
		},
		{
			name: "only comment tokens",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenComment, Bytes: []byte("# comment1")},
				{Type: hclsyntax.TokenComment, Bytes: []byte("# comment2")},
			},
			expectedValue:   0,
			expectedComment: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valueTokens, commentTokens := separateValueAndCommentTokens(tt.tokens)
			if len(valueTokens) != tt.expectedValue {
				t.Errorf("separateValueAndCommentTokens() valueTokens count = %v, want %v", len(valueTokens), tt.expectedValue)
			}
			if len(commentTokens) != tt.expectedComment {
				t.Errorf("separateValueAndCommentTokens() commentTokens count = %v, want %v", len(commentTokens), tt.expectedComment)
			}
		})
	}
}

func TestEndsWithComma(t *testing.T) {
	tests := []struct {
		name     string
		tokens   hclwrite.Tokens
		expected bool
	}{
		{
			name: "ends with comma",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenQuotedLit, Bytes: []byte("test")},
				{Type: hclsyntax.TokenCQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenComma, Bytes: []byte(",")},
			},
			expected: true,
		},
		{
			name: "does not end with comma",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenQuotedLit, Bytes: []byte("test")},
				{Type: hclsyntax.TokenCQuote, Bytes: []byte("\"")},
			},
			expected: false,
		},
		{
			name:     "empty tokens",
			tokens:   hclwrite.Tokens{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := endsWithComma(tt.tokens)
			if result != tt.expected {
				t.Errorf("endsWithComma() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEndsWithMeaningfulComma(t *testing.T) {
	tests := []struct {
		name     string
		tokens   hclwrite.Tokens
		expected bool
	}{
		{
			name: "ends with comma before whitespace",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenQuotedLit, Bytes: []byte("test")},
				{Type: hclsyntax.TokenCQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenComma, Bytes: []byte(",")},
				{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
				{Type: hclsyntax.TokenTabs, Bytes: []byte("  ")},
			},
			expected: true,
		},
		{
			name: "does not end with meaningful comma",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenQuotedLit, Bytes: []byte("test")},
				{Type: hclsyntax.TokenCQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
			},
			expected: false,
		},
		{
			name:     "empty tokens",
			tokens:   hclwrite.Tokens{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := endsWithMeaningfulComma(tt.tokens)
			if result != tt.expected {
				t.Errorf("endsWithMeaningfulComma() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRemoveLeadingNewlines(t *testing.T) {
	tests := []struct {
		name     string
		tokens   hclwrite.Tokens
		expected int // expected length after removal
	}{
		{
			name: "remove leading newlines",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
				{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
				{Type: hclsyntax.TokenTabs, Bytes: []byte("  ")},
				{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
			},
			expected: 2, // Should remove 2 newlines, keep tabs and quote
		},
		{
			name: "no leading newlines",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenTabs, Bytes: []byte("  ")},
				{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
			},
			expected: 2, // Should keep all tokens
		},
		{
			name: "all newlines",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
				{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
			},
			expected: 0, // Should remove all tokens
		},
		{
			name:     "empty tokens",
			tokens:   hclwrite.Tokens{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeLeadingNewlines(tt.tokens)
			if len(result) != tt.expected {
				t.Errorf("removeLeadingNewlines() length = %v, want %v", len(result), tt.expected)
			}
		})
	}
}

func TestIsMultiLineList(t *testing.T) {
	tests := []struct {
		name     string
		elements []listElement
		expected bool
	}{
		{
			name: "contains newlines in leading comments",
			elements: []listElement{
				{
					LeadingComments: hclwrite.Tokens{
						{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
						{Type: hclsyntax.TokenTabs, Bytes: []byte("  ")},
					},
					Tokens: hclwrite.Tokens{
						{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
					},
				},
			},
			expected: true,
		},
		{
			name: "contains newlines in element tokens",
			elements: []listElement{
				{
					LeadingComments: hclwrite.Tokens{},
					Tokens: hclwrite.Tokens{
						{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
						{Type: hclsyntax.TokenQuotedLit, Bytes: []byte("test")},
						{Type: hclsyntax.TokenCQuote, Bytes: []byte("\"")},
						{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
					},
				},
			},
			expected: true,
		},
		{
			name: "no newlines",
			elements: []listElement{
				{
					LeadingComments: hclwrite.Tokens{},
					Tokens: hclwrite.Tokens{
						{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
						{Type: hclsyntax.TokenQuotedLit, Bytes: []byte("test")},
						{Type: hclsyntax.TokenCQuote, Bytes: []byte("\"")},
					},
				},
			},
			expected: false,
		},
		{
			name:     "empty elements",
			elements: []listElement{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMultiLineList(tt.elements)
			if result != tt.expected {
				t.Errorf("isMultiLineList() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSortListElements(t *testing.T) {
	tests := []struct {
		name            string
		elements        []listElement
		expectedChanged bool
		expectedOrder   []string // keys in expected order
	}{
		{
			name: "needs sorting",
			elements: []listElement{
				{Key: []byte("banana"), IsNumber: false},
				{Key: []byte("apple"), IsNumber: false},
				{Key: []byte("cherry"), IsNumber: false},
			},
			expectedChanged: true,
			expectedOrder:   []string{"apple", "banana", "cherry"},
		},
		{
			name: "already sorted",
			elements: []listElement{
				{Key: []byte("apple"), IsNumber: false},
				{Key: []byte("banana"), IsNumber: false},
				{Key: []byte("cherry"), IsNumber: false},
			},
			expectedChanged: false,
			expectedOrder:   []string{"apple", "banana", "cherry"},
		},
		{
			name: "numbers and strings",
			elements: []listElement{
				{Key: []byte("banana"), IsNumber: false},
				{Key: []byte("10"), CtyValue: cty.NumberIntVal(10), IsNumber: true},
				{Key: []byte("apple"), IsNumber: false},
				{Key: []byte("5"), CtyValue: cty.NumberIntVal(5), IsNumber: true},
			},
			expectedChanged: true,
			expectedOrder:   []string{"5", "10", "apple", "banana"},
		},
		{
			name:            "empty list",
			elements:        []listElement{},
			expectedChanged: false,
			expectedOrder:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, changed := sortListElements(tt.elements)
			if changed != tt.expectedChanged {
				t.Errorf("sortListElements() changed = %v, want %v", changed, tt.expectedChanged)
			}
			if len(result) != len(tt.expectedOrder) {
				t.Errorf("sortListElements() length = %v, want %v", len(result), len(tt.expectedOrder))
				return
			}
			for i, expectedKey := range tt.expectedOrder {
				if string(result[i].Key) != expectedKey {
					t.Errorf("sortListElements() result[%d].Key = %v, want %v", i, string(result[i].Key), expectedKey)
				}
			}
		})
	}
}
