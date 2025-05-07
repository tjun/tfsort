package parser

import (
	"testing"
)

func TestParseHCL(t *testing.T) {
	testCases := []struct {
		name        string
		content     string
		filename    string
		wantErr     bool
		wantBlocks  int
		skipContent bool // Skip content check for error cases
	}{
		{
			name:       "valid simple resource",
			content:    `resource "test_resource" "example" { name = "hello" }`,
			filename:   "valid.tf",
			wantErr:    false,
			wantBlocks: 1,
		},
		{
			name: "valid multiple blocks",
			content: `
variable "input" {}

resource "test_resource" "a" {
  count = 1
}

data "source" "b" {
  filter = true
}
`,
			filename:   "multiple.tf",
			wantErr:    false,
			wantBlocks: 3,
		},
		{
			name: "valid with comments",
			content: `
# This is a comment
resource "test_resource" "commented" { // Inline comment
  attr = "value" # Trailing comment
}
/* Multi-line
   comment */
`,
			filename:   "comments.tf",
			wantErr:    false,
			wantBlocks: 1,
		},
		{
			name:        "invalid HCL syntax (missing brace)",
			content:     `resource "test_resource" "bad" { name = "oops" `,
			filename:    "invalid.tf",
			wantErr:     true,
			skipContent: true,
		},
		{
			name:        "invalid HCL syntax (unexpected token)",
			content:     `resource = "foo"`, // Treat as a top-level attribute by hclwrite
			filename:    "invalid2.tf",
			wantErr:     false, // hclwrite doesn't see this as an error, just an attribute
			wantBlocks:  0,     // It's parsed as an attribute, not a block
			skipContent: false, // Allow block count check (wantBlocks: 0)
		},
		{
			name:       "empty content",
			content:    "",
			filename:   "empty.tf",
			wantErr:    false, // Empty file is valid HCL
			wantBlocks: 0,
		},
		{
			name:       "only comments",
			content:    "# Just a comment\n// Another one",
			filename:   "only_comments.tf",
			wantErr:    false,
			wantBlocks: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotFile, err := ParseHCL([]byte(tc.content), tc.filename)

			if (err != nil) != tc.wantErr {
				t.Errorf("ParseHCL() error = %v, wantErr %v", err, tc.wantErr)
				return
			}

			if !tc.wantErr {
				if gotFile == nil {
					t.Errorf("ParseHCL() returned nil file, expected non-nil")
					return
				}
				if !tc.skipContent {
					gotBlocks := len(gotFile.Body().Blocks())
					if gotBlocks != tc.wantBlocks {
						t.Errorf("ParseHCL() got %d top-level blocks, want %d", gotBlocks, tc.wantBlocks)
					}
				}
			}
		})
	}
}
