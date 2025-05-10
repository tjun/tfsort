package sorter

import (
   "testing"

   "github.com/tjun/tfsort/internal/parser"
)


func TestBlockSort(t *testing.T) {
   testCases := []struct {
       name        string
       inputHCL    string
       wantHCL     string
       sortOptions SortOptions
       wantErr     bool
       skipClean   bool
   }{
       // --- Basic Block Sorting ---
       {
           name: "sort basic blocks",
           inputHCL: `
resource "b" "r2" {}
variable "a" {}
data "c" "d1" {}
`,
           wantHCL: `
variable "a" {}
data "c" "d1" {}
resource "b" "r2" {}
`,
           sortOptions: SortOptions{SortBlocks: true, SortTypeName: true, SortList: true},
       },
       {
           name: "sort blocks with standard order",
           inputHCL: `
output "z" {}
resource "aws_instance" "web" {}
module "vpc" {}
locals {}
data "aws_ami" "ubuntu" {}
variable "region" {}
provider "aws" {}
terraform {}
`,
           wantHCL: `
terraform {}
provider "aws" {}
variable "region" {}
locals {}
data "aws_ami" "ubuntu" {}
module "vpc" {}
resource "aws_instance" "web" {}
output "z" {}
`,
           sortOptions: SortOptions{SortBlocks: true, SortTypeName: true, SortList: true},
       },
       {
           name: "disable block sort",
           inputHCL: `
resource "b" "r2" {}
local "a" {}
`,
           wantHCL: `
resource "b" "r2" {}
local "a" {}
`,
           sortOptions: SortOptions{SortBlocks: false, SortTypeName: true, SortList: true},
       },
       // --- Resource/Data Type & Name Sorting ---
       {
           name: "sort resource by type then name",
           inputHCL: `
resource "aws_s3_bucket" "main" {}
resource "aws_instance" "web" {}
resource "aws_s3_bucket" "logs" {}
resource "aws_instance" "db" {}
`,
           wantHCL: `
resource "aws_instance" "db" {}
resource "aws_instance" "web" {}
resource "aws_s3_bucket" "logs" {}
resource "aws_s3_bucket" "main" {}
`,
           sortOptions: SortOptions{SortBlocks: true, SortTypeName: true, SortList: true},
       },
       {
           name: "sort data by type then name",
           inputHCL: `
data "aws_ami" "ubuntu" {}
data "aws_caller_identity" "current" {}
data "aws_ami" "amazon_linux" {}
`,
           wantHCL: `
data "aws_ami" "amazon_linux" {}
data "aws_ami" "ubuntu" {}
data "aws_caller_identity" "current" {}
`,
           sortOptions: SortOptions{SortBlocks: true, SortTypeName: true, SortList: true},
       },
       {
           name: "disable type/name sort",
           inputHCL: `
resource "aws_s3_bucket" "main" {}
resource "aws_instance" "web" {}
resource "aws_s3_bucket" "logs" {}
`,
           wantHCL: `
resource "aws_s3_bucket" "main" {}
resource "aws_instance" "web" {}
resource "aws_s3_bucket" "logs" {}
`,
           sortOptions: SortOptions{SortBlocks: true, SortTypeName: false, SortList: true},
       },
       // --- Comments ---
       {
           name: "preserve_comments_during_block_sort",
           inputHCL: `
# Resource B comment
resource "b" "r2" {}

// Variable A comment
variable "a" {}

locals {
  a = "a"
}
`,
           wantHCL: `
// Variable A comment
variable "a" {}

locals {
  a = "a"
}

# Resource B comment
resource "b" "r2" {}
`,
           sortOptions: SortOptions{SortBlocks: true, SortTypeName: true, SortList: true},
       },
       // --- Ignore Directive ---
       {
           name: "ignore_block_with_directive",
           inputHCL: `
resource "a" "r1" { // Should not be sorted first & attrs inside untouched
  zone = "z"
  ami  = "a"
  list = [ // tfsort:ignore
    "z", "a"
  ]
}
resource "c" "r3" {} // Should be sorted after 'b'
resource "b" "r2" {} // Should be sorted after 'a'
`,
           wantHCL: `
resource "a" "r1" { // Should not be sorted first & attrs inside untouched
  zone = "z"
  ami  = "a"
  list = [ // tfsort:ignore
    "z", "a"
  ]
}

resource "b" "r2" {} // Should be sorted after 'a'

resource "c" "r3" {} // Should be sorted after 'b'
`,
           sortOptions: SortOptions{SortBlocks: true, SortTypeName: true, SortList: true},
           skipClean:   false,
       },
       {
           name: "ignore_directive_only_affects_its_block_(other_block_lists_sorted)",
           inputHCL: `
variable "z" {} // sort me

resource "a" "r1" {
  c = 3
  b = 2
  list = [ // tfsort:ignore
    "z", "a"
  ]
}

resource "b" "r2" {
  c = 3
  b = 2

  list = [
    // tfsort:ignore
    "z",
    "a",
  ]
}

resource "c" "r3" { // sort me, sort my list, but not my other attrs
  y    = 9
  list = ["a", "z"] // This list IS sorted
  x    = 8
}
`,
           wantHCL: `
variable "z" {} // sort me

resource "a" "r1" {
  c = 3
  b = 2
  list = [ // tfsort:ignore
    "z", "a"
  ]
}

resource "b" "r2" {
  c = 3
  b = 2

  list = [
    // tfsort:ignore
    "z",
    "a",
  ]
}

resource "c" "r3" { // sort me, sort my list, but not my other attrs
  y    = 9
  list = ["a", "z"] // This list IS sorted
  x    = 8
}
`,
           sortOptions: SortOptions{SortBlocks: true, SortTypeName: true, SortList: true},
           skipClean:   false,
       },
       // --- Empty/No-op ---
       {
           name:     "empty input",
           inputHCL: "",
           wantHCL:  "",
           sortOptions: SortOptions{SortBlocks: true, SortTypeName: true, SortList: true},
       },
       {
           name:     "already sorted input",
           inputHCL: "variable \"a\" {}\nresource \"b\" \"c\" { list = [\"a\", \"b\"] }",
           wantHCL:  "variable \"a\" {}\nresource \"b\" \"c\" { list = [\"a\", \"b\"] }",
           sortOptions: SortOptions{SortBlocks: true, SortTypeName: true, SortList: true},
       },
       {
           name:        "expect parsing error",
           inputHCL:    `
resource "a" "b" {
  malformed = 
}
`,
           wantHCL:     "",
           sortOptions: SortOptions{SortBlocks: true},
           wantErr:     true,
       },
   }

   for _, tc := range testCases {
       t.Run(tc.name, func(t *testing.T) {
           // Parse the input HCL string
           hclFile, parseDiags := parser.ParseHCL([]byte(tc.inputHCL), "test.tf")

           // Check for parsing errors
           if parseDiags.HasErrors() {
               if !tc.wantErr {
                   t.Fatalf("Failed to parse input HCL: %v", parseDiags)
               }
               return
           }

           if hclFile == nil {
               t.Fatal("Parsed HCL file is nil without parsing errors")
           }

           // Call the Sort function
           sortedFile, sortErr := Sort(hclFile, tc.sortOptions)
           if sortErr != nil && !tc.wantErr {
               t.Errorf("Sort() unexpected error = %v", sortErr)
               return
           }
           if sortErr == nil && tc.wantErr {
               t.Errorf("Sort() error = nil, wantErr %v (expected a sort error, but got none)", tc.wantErr)
               return
           }
           if tc.wantErr {
               return
           }

           if sortedFile == nil {
               t.Fatal("Sort() returned nil file without error")
           }

           gotHCLBytes := sortedFile.Bytes()
           var gotCleanedHCL, wantCleanedHCL string

           if tc.skipClean {
               gotCleanedHCL = string(gotHCLBytes)
               wantCleanedHCL = tc.wantHCL
           } else {
               gotCleanedHCL = cleanHCL(gotHCLBytes)
               wantCleanedHCL = cleanHCL([]byte(tc.wantHCL))
           }

           if gotCleanedHCL != wantCleanedHCL {
               t.Errorf("Sort() output mismatch for test case: %s\nGot:\n%s\n\nWant:\n%s", tc.name, gotCleanedHCL, wantCleanedHCL)
               // Raw output can be helpful for subtle diffs not caught by cleanHCL
               if string(gotHCLBytes) != gotCleanedHCL || tc.wantHCL != wantCleanedHCL {
                   t.Logf("Raw Got:\n%s", string(gotHCLBytes))
                   t.Logf("Raw Want (as provided in test case):\n%s", tc.wantHCL)
               }
           }
       })
   }
}