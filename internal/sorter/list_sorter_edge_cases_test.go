package sorter

import (
	"testing"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/tjun/tfsort/internal/parser"
)

func TestCheckIgnoreDirective(t *testing.T) {
	tests := []struct {
		name     string
		tokens   hclwrite.Tokens
		expected bool
	}{
		{
			name: "has ignore directive",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
				{Type: hclsyntax.TokenTabs, Bytes: []byte("  ")},
				{Type: hclsyntax.TokenComment, Bytes: []byte("// tfsort:ignore")},
			},
			expected: true,
		},
		{
			name: "has ignore directive with hash comment",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenComment, Bytes: []byte("# tfsort:ignore")},
			},
			expected: true,
		},
		{
			name: "has other comment",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenComment, Bytes: []byte("// some other comment")},
			},
			expected: false,
		},
		{
			name: "no comments",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
				{Type: hclsyntax.TokenTabs, Bytes: []byte("  ")},
				{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
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
			result := checkIgnoreDirective(tt.tokens)
			if result != tt.expected {
				t.Errorf("checkIgnoreDirective() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestListSortingIgnoreDirective(t *testing.T) {
	tests := []struct {
		name        string
		inputHCL    string
		wantHCL     string
		description string
	}{
		{
			name: "ignore directive prevents sorting",
			inputHCL: `
resource "test" "example" {
  list = [
    // tfsort:ignore
    "c",
    "a",
    "b"
  ]
}`,
			wantHCL: `
resource "test" "example" {
  list = [
    // tfsort:ignore
    "c",
    "a",
    "b"
  ]
}`,
			description: "List with ignore directive should remain unsorted",
		},
		{
			name: "ignore directive with hash comment",
			inputHCL: `
resource "test" "example" {
  list = [
    # tfsort:ignore
    "c",
    "a",
    "b"
  ]
}`,
			wantHCL: `
resource "test" "example" {
  list = [
    # tfsort:ignore
    "c",
    "a",
    "b"
  ]
}`,
			description: "List with hash-style ignore directive should remain unsorted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hclFile, diags := parser.ParseHCL([]byte(tt.inputHCL), "test.tf")
			if diags.HasErrors() {
				t.Fatalf("Failed to parse input HCL: %v", diags)
			}

			sortedFile, err := Sort(hclFile, SortOptions{SortBlocks: true, SortTypeName: true, SortList: true})
			if err != nil {
				t.Errorf("Sort() error = %v", err)
				return
			}

			got := cleanHCL(sortedFile.Bytes())
			want := cleanHCL([]byte(tt.wantHCL))

			if got != want {
				t.Errorf("Sort() output mismatch for %s\nGot:\n%s\nWant:\n%s", tt.description, got, want)
			}
		})
	}
}

func TestComplexCommentScenarios(t *testing.T) {
	tests := []struct {
		name        string
		inputHCL    string
		wantHCL     string
		description string
	}{
		{
			name: "multiple inline comments",
			inputHCL: `
resource "test" "example" {
  list = [
    "zebra", # last animal
    "apple", # first fruit
    "banana" # second fruit
  ]
}`,
			wantHCL: `
resource "test" "example" {
  list = [
    "apple",  # first fruit
    "banana", # second fruit
    "zebra",  # last animal
  ]
}`,
			description: "Elements with inline comments should sort by value, preserving comments",
		},
		{
			name: "leading comments on elements",
			inputHCL: `
resource "test" "example" {
  list = [
    # Comment for zebra
    "zebra",
    # Comment for apple 
    "apple",
    # Comment for banana
    "banana"
  ]
}`,
			wantHCL: `
resource "test" "example" {
  list = [
    # Comment for apple
    "apple",
    # Comment for banana
    "banana",
    # Comment for zebra
    "zebra",
  ]
}`,
			description: "Elements with leading comments should sort together with their comments",
		},
		{
			name: "mixed comment styles",
			inputHCL: `
resource "test" "example" {
  list = [
    "zebra", // trailing comment
    # leading comment
    "apple",
    "banana" # inline comment
  ]
}`,
			wantHCL: `
resource "test" "example" {
  list = [
    "apple",
    "banana", # inline comment
    "zebra",  // trailing comment
    # leading comment
  ]
}`,
			description: "Mixed comment styles should be preserved during sorting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hclFile, diags := parser.ParseHCL([]byte(tt.inputHCL), "test.tf")
			if diags.HasErrors() {
				t.Fatalf("Failed to parse input HCL: %v", diags)
			}

			sortedFile, err := Sort(hclFile, SortOptions{SortBlocks: true, SortTypeName: true, SortList: true})
			if err != nil {
				t.Errorf("Sort() error = %v", err)
				return
			}

			got := cleanHCL(sortedFile.Bytes())
			want := cleanHCL([]byte(tt.wantHCL))

			if got != want {
				t.Errorf("Sort() output mismatch for %s\nGot:\n%s\nWant:\n%s", tt.description, got, want)
			}
		})
	}
}

func TestEdgeCaseListStructures(t *testing.T) {
	tests := []struct {
		name        string
		inputHCL    string
		wantHCL     string
		description string
	}{
		{
			name: "empty list",
			inputHCL: `
resource "test" "example" {
  list = []
}`,
			wantHCL: `
resource "test" "example" {
  list = []
}`,
			description: "Empty list should remain unchanged",
		},
		{
			name: "single element list",
			inputHCL: `
resource "test" "example" {
  list = ["only"]
}`,
			wantHCL: `
resource "test" "example" {
  list = ["only"]
}`,
			description: "Single element list should remain unchanged",
		},
		{
			name: "list with complex expressions",
			inputHCL: `
resource "test" "example" {
  list = [
    var.zebra,
    var.apple,
    "literal_zebra",
    "literal_apple"
  ]
}`,
			wantHCL: `
resource "test" "example" {
  list = [
    "literal_apple",
    "literal_zebra",
    var.apple,
    var.zebra,
  ]
}`,
			description: "List with mixed literal and variable expressions should sort correctly",
		},
		{
			name: "nested list in toset",
			inputHCL: `
resource "test" "example" {
  list = toset([
    "zebra", 
    "apple", 
    "banana" 
  ])
}`,
			wantHCL: `
resource "test" "example" {
  list = toset([
    "apple",
    "banana",
    "zebra",
  ])
}`,
			description: "List inside toset() should be sorted",
		},
		{
			name: "numbers and strings mixed",
			inputHCL: `
resource "test" "example" {
  list = [
    "zebra",
    100,
    "apple",
    50,
    "banana",
    25
  ]
}`,
			wantHCL: `
resource "test" "example" {
  list = [
    25,
    50,
    100,
    "apple",
    "banana",
    "zebra",
  ]
}`,
			description: "Numbers should sort before strings, with proper numeric ordering",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hclFile, diags := parser.ParseHCL([]byte(tt.inputHCL), "test.tf")
			if diags.HasErrors() {
				t.Fatalf("Failed to parse input HCL: %v", diags)
			}

			sortedFile, err := Sort(hclFile, SortOptions{SortBlocks: true, SortTypeName: true, SortList: true})
			if err != nil {
				t.Errorf("Sort() error = %v", err)
				return
			}

			got := cleanHCL(sortedFile.Bytes())
			want := cleanHCL([]byte(tt.wantHCL))

			if got != want {
				t.Errorf("Sort() output mismatch for %s\nGot:\n%s\nWant:\n%s", tt.description, got, want)
			}
		})
	}
}

func TestCheckSimpleListLiteral(t *testing.T) {
	tests := []struct {
		name           string
		tokens         hclwrite.Tokens
		expectedResult bool
		expectedLength int // length of returned tokens if valid
	}{
		{
			name: "valid simple list",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
				{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenQuotedLit, Bytes: []byte("test")},
				{Type: hclsyntax.TokenCQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")},
			},
			expectedResult: true,
			expectedLength: 5,
		},
		{
			name: "valid list with leading/trailing whitespace",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
				{Type: hclsyntax.TokenTabs, Bytes: []byte("  ")},
				{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
				{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenQuotedLit, Bytes: []byte("test")},
				{Type: hclsyntax.TokenCQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")},
				{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
			},
			expectedResult: true,
			expectedLength: 5, // Should return just the bracket-enclosed portion
		},
		{
			name: "not a list - parentheses",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenOParen, Bytes: []byte("(")},
				{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenQuotedLit, Bytes: []byte("test")},
				{Type: hclsyntax.TokenCQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenCParen, Bytes: []byte(")")},
			},
			expectedResult: false,
			expectedLength: 0,
		},
		{
			name: "not a list - missing closing bracket",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
				{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenQuotedLit, Bytes: []byte("test")},
				{Type: hclsyntax.TokenCQuote, Bytes: []byte("\"")},
			},
			expectedResult: false,
			expectedLength: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, isValid := checkSimpleListLiteral(tt.tokens)
			if isValid != tt.expectedResult {
				t.Errorf("checkSimpleListLiteral() valid = %v, want %v", isValid, tt.expectedResult)
			}
			if isValid && len(result) != tt.expectedLength {
				t.Errorf("checkSimpleListLiteral() result length = %v, want %v", len(result), tt.expectedLength)
			}
		})
	}
}

func TestCheckTosetListCall(t *testing.T) {
	tests := []struct {
		name           string
		tokens         hclwrite.Tokens
		expectedResult bool
		expectedLength int // length of returned inner list tokens if valid
	}{
		{
			name: "valid toset call",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenIdent, Bytes: []byte("toset")},
				{Type: hclsyntax.TokenOParen, Bytes: []byte("(")},
				{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
				{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenQuotedLit, Bytes: []byte("test")},
				{Type: hclsyntax.TokenCQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")},
				{Type: hclsyntax.TokenCParen, Bytes: []byte(")")},
			},
			expectedResult: true,
			expectedLength: 5, // From [ to ] inclusive
		},
		{
			name: "not a toset call - wrong function name",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenIdent, Bytes: []byte("tolist")},
				{Type: hclsyntax.TokenOParen, Bytes: []byte("(")},
				{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
				{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenQuotedLit, Bytes: []byte("test")},
				{Type: hclsyntax.TokenCQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")},
				{Type: hclsyntax.TokenCParen, Bytes: []byte(")")},
			},
			expectedResult: false,
			expectedLength: 0,
		},
		{
			name: "not a toset call - missing brackets",
			tokens: hclwrite.Tokens{
				{Type: hclsyntax.TokenIdent, Bytes: []byte("toset")},
				{Type: hclsyntax.TokenOParen, Bytes: []byte("(")},
				{Type: hclsyntax.TokenOQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenQuotedLit, Bytes: []byte("test")},
				{Type: hclsyntax.TokenCQuote, Bytes: []byte("\"")},
				{Type: hclsyntax.TokenCParen, Bytes: []byte(")")},
			},
			expectedResult: false,
			expectedLength: 0,
		},
		{
			name:           "empty tokens",
			tokens:         hclwrite.Tokens{},
			expectedResult: false,
			expectedLength: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, isValid := checkTosetListCall(tt.tokens)
			if isValid != tt.expectedResult {
				t.Errorf("checkTosetListCall() valid = %v, want %v", isValid, tt.expectedResult)
			}
			if isValid && len(result) != tt.expectedLength {
				t.Errorf("checkTosetListCall() result length = %v, want %v", len(result), tt.expectedLength)
			}
		})
	}
}
