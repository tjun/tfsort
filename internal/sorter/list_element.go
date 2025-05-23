package sorter

import (
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

type listElement struct {
	LeadingComments  hclwrite.Tokens
	Tokens           hclwrite.Tokens
	TrailingComments hclwrite.Tokens
	Key              []byte
	CtyValue         cty.Value
	IsNumber         bool
}

func (e listElement) FullTokens() hclwrite.Tokens {
	var full hclwrite.Tokens
	full = append(full, e.LeadingComments...)
	full = append(full, e.Tokens...)
	full = append(full, e.TrailingComments...)
	return full
}
