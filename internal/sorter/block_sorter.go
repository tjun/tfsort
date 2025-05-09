package sorter

import (
	"sort"

	"github.com/hashicorp/hcl/v2/hclwrite"
)

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

// sortAndAddBlocksToBody sorts blocks from originalBody according to options
// and appends them to targetBody.
func sortAndAddBlocksToBody(originalBody *hclwrite.Body, targetBody *hclwrite.Body, options SortOptions) {
	blocks := originalBody.Blocks()
	if len(blocks) == 0 {
		return
	}

	blocksToSort := make([]*hclwrite.Block, len(blocks))
	copy(blocksToSort, blocks)

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
		return false // Maintain original order for same-keyed items or if type/name sort is off
	})

	// Add sorted blocks to the new body
	for i, block := range blocksToSort {
		targetBody.AppendBlock(block)
		if i < len(blocksToSort)-1 {
			targetBody.AppendNewline() // Ensure newline between blocks
		}
	}
}
