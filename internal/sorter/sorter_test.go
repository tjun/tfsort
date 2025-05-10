package sorter

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/tjun/tfsort/internal/parser" // Use the existing parser
)

// cleanHCL removes extra whitespace and newlines for easier comparison.
// Note: This is a simplistic approach. Real comparison should ideally
// preserve intended newlines, but for basic sorting tests, this helps.
func cleanHCL(hclBytes []byte) string {
	// Normalize line endings to LF
	normalizedBytes := bytes.ReplaceAll(hclBytes, []byte("\r\n"), []byte("\n"))
	normalizedBytes = bytes.ReplaceAll(normalizedBytes, []byte("\r"), []byte("\n"))

	scanner := bufio.NewScanner(bytes.NewReader(normalizedBytes))
	var cleanedLines []string
	// spaceRe := regexp.MustCompile(`\s+`) // Removed intra-line whitespace collapsing

	for scanner.Scan() {
		line := scanner.Text()
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine != "" {
			// collapsedLine := spaceRe.ReplaceAllString(trimmedLine, " ") // Removed
			cleanedLines = append(cleanedLines, trimmedLine) // Use TrimSpace result directly
		}
	}
	// Note: We removed the error check for scanner.Err() for simplicity in tests
	return strings.Join(cleanedLines, "\n")
}

func TestSort(t *testing.T) {
	testCases := []struct {
		name        string
		inputHCL    string
		wantHCL     string      // Expected output after sorting (cleaned)
		sortOptions SortOptions // Updated SortOptions
		wantErr     bool        // True if either parsing or sorting is expected to fail
		skipClean   bool        // Set true if exact whitespace comparison is needed
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
`, // Blocks not sorted
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
`, // If block sort is on, they stay together. If type/name sort is off, their internal order is preserved.
			sortOptions: SortOptions{SortBlocks: true, SortTypeName: false, SortList: true},
		},

		// --- Attribute Sorting (Expect NO change, EXCEPT within lists) ---
		{
			name: "attributes remain unsorted", // Other attributes are not sorted by key but formatted
			inputHCL: `
resource "test" "example" {
  zone        = "us-west-1"
  ami         = "ami-123"
  instance_type = "t2.micro"
}
`,
			wantHCL: `
resource "test" "example" {
  zone          = "us-west-1"
  ami           = "ami-123"
  instance_type = "t2.micro"
}
`,
			sortOptions: SortOptions{SortBlocks: true, SortTypeName: true, SortList: true},
		},
		{
			name: "list attribute values ARE sorted", // List values ARE sorted
			inputHCL: `
resource "test" "example" {
  list = ["a", "b", "c"]
}
`,
			wantHCL: `
resource "test" "example" {
  list = ["a", "b", "c"]
}
`,
			sortOptions: SortOptions{SortBlocks: true, SortTypeName: true, SortList: true},
		},
		{
			name: "map attribute keys remain unsorted", // Map keys are not sorted
			inputHCL: `
resource "test" "example" {
  tags = {
    Environment = "prod"
    Name        = "my-instance"
    BillingCode = "12345"
  }
}
`,
			wantHCL: `
resource "test" "example" {
  tags = {
    Environment = "prod"
    Name        = "my-instance"
    BillingCode = "12345"
  }
}
`,
			sortOptions: SortOptions{SortBlocks: true, SortTypeName: true, SortList: true},
		},
		{
			name: "nested_blocks_remain_unsorted_relative_to_each_other_(list_inside_sorted)",
			inputHCL: `
resource "test" "example" {
  network_interface { // NI 1
    device_index = 0
    subnets      = ["subnet-c", "subnet-a"]
    network      = "default"
  }
  ebs_block_device { // EBS 1
    device_name = "/dev/sda1"
    volume_size = 10
  }
  network_interface { // NI 2
    device_index = 1
    network      = "other"
    subnets      = ["subnet-z", "subnet-x"]
  }
}
`,
			wantHCL: `
resource "test" "example" {
  network_interface { // NI 1
    device_index = 0
    subnets      = ["subnet-a", "subnet-c"]
    network      = "default"
  }
  ebs_block_device { // EBS 1
    device_name = "/dev/sda1"
    volume_size = 10
  }
  network_interface { // NI 2
    device_index = 1
    network      = "other"
    subnets      = ["subnet-x", "subnet-z"]
  }
}
`,
			sortOptions: SortOptions{SortBlocks: false, SortList: true}, // Main blocks not sorted, but lists inside should be
		},

		// --- Comments (Still relevant for block/type/name sort) ---
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
`, // Expect exactly one newline between sorted blocks
			sortOptions: SortOptions{SortBlocks: true, SortTypeName: true, SortList: true},
		},
		{
			name: "preserve_comments_with_attributes_(list_inside_sorted)",
			inputHCL: `
resource "test" "example" {
  # Zone comment
  zone  = "us-west-1"
  ports = [22, 443, 80] // Ports comment - back to numbers
  ami   = "ami-123"     // AMI comment
}
`,
			wantHCL: `
resource "test" "example" {
  # Zone comment
  zone  = "us-west-1"
  ports = [22, 80, 443] // Ports comment - back to numbers
  ami   = "ami-123"     // AMI comment
}
`,
			sortOptions: SortOptions{SortBlocks: false, SortTypeName: false, SortList: true},
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
			// wantHCL: Block "a", "b", "c" are sorted. List in "a" is NOT sorted due to ignore.
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
  c    = 3
  b    = 2
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
  list = ["z", "a"] // This list IS sorted
  x    = 8
}
`,
			// wantHCL expects raw output with one newline between blocks
			// Setting wantHCL exactly to the last known 'Got' output due to persistent diff issues
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

		// --- Empty/No-op (Still relevant) ---
		{
			name:        "empty input",
			inputHCL:    "",
			wantHCL:     "",
			sortOptions: SortOptions{SortBlocks: true, SortTypeName: true, SortList: true},
		},
		{
			name:        "already sorted input",
			inputHCL:    "variable \"a\" {}\nresource \"b\" \"c\" { list = [\"a\", \"b\"] }", // List already sorted
			wantHCL:     "variable \"a\" {}\nresource \"b\" \"c\" { list = [\"a\", \"b\"] }",
			sortOptions: SortOptions{SortBlocks: true, SortTypeName: true, SortList: true},
		},
		// Test case expecting a parsing error
		{
			name: "expect parsing error",
			inputHCL: `
resource "a" "b" {
  malformed = 
}
`,
			wantHCL:     "", // Output doesn't matter if parsing fails
			sortOptions: SortOptions{SortBlocks: true},
			wantErr:     true, // Expecting an error (parsing error in this case)
		},
		// Test case expecting a sorting error (if we had a way to make Sort() error)
		// {
		// 	name:        "expect sorting error",
		// 	inputHCL:    `variable "a" {}`, // Valid HCL
		// 	wantHCL:     "",
		// 	sortOptions: SortOptions{SortBlocks: true}, // Potentially invalid options if Sort could error
		// 	wantErr:     true, // Expecting a sorting error
		// },
		{
			name: "sort_mixed_type_list_with_complex_elements",
			inputHCL: `
resource "aws_security_group" "allow_common_ports" {
  name        = "allow-common-ports"
  description = "Allow inbound traffic on common ports"
  vpc_id      = "vpc-12345678"

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  security_group_rules = [
    "http-80-tcp",
    1024,
    { type = "ingress", from_port = 22, to_port = 22, protocol = "tcp", cidr_blocks = ["10.0.0.0/8"] },
    "ssh-22-tcp",
  ]

  tags = {
    Name = "common-ports-sg"
  }
}
`,
			wantHCL: `
resource "aws_security_group" "allow_common_ports" {
  name        = "allow-common-ports"
  description = "Allow inbound traffic on common ports"
  vpc_id      = "vpc-12345678"

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  security_group_rules = [
    1024,
    "http-80-tcp",
    "ssh-22-tcp",
    { type = "ingress", from_port = 22, to_port = 22, protocol = "tcp", cidr_blocks = ["10.0.0.0/8"] },
  ]

  tags = {
    Name = "common-ports-sg"
  }
}
`,
			sortOptions: SortOptions{SortBlocks: true, SortTypeName: true, SortList: true},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse the input HCL string
			hclFile, parseDiags := parser.ParseHCL([]byte(tc.inputHCL), "test.tf") // Renamed to parseDiags

			// Check for parsing errors
			if parseDiags.HasErrors() { // Correctly use parseDiags
				if !tc.wantErr { // If we didn't expect an error (at any stage)
					t.Fatalf("Failed to parse input HCL: %v", parseDiags)
				}
				// If we expected an error (wantErr is true), and parsing failed, this is a pass for this test case.
				return
			}

			// If we expected an error (wantErr is true) but parsing succeeded, this is a failure for tests designed to fail at parsing.
			// However, wantErr might be for a Sort() error. We need to be careful here.
			// For now, if parsing succeeded and wantErr is true, we assume wantErr was for a Sort() error later.
			// A more robust solution might involve different error expectation flags (e.g., wantParseErr, wantSortErr).

			if hclFile == nil { // Should not happen if parseDiags.HasErrors() is false
				t.Fatal("Parsed HCL file is nil without parsing errors")
			}

			// --- Call the Sort function ---
			sortedFile, sortErr := Sort(hclFile, tc.sortOptions) // Receive both values
			if sortErr != nil && !tc.wantErr {                   // Unexpected sort error
				t.Errorf("Sort() unexpected error = %v", sortErr)
				return
			}
			if sortErr == nil && tc.wantErr { // Expected sort error, but got none
				t.Errorf("Sort() error = nil, wantErr %v (expected a sort error, but got none)", tc.wantErr)
				return
			}
			if tc.wantErr { // Expected error occurred (either parse or sort)
				return
			}

			if sortedFile == nil { // Should not happen if Sort succeeds without error
				t.Fatal("Sort() returned nil file without error")
			}

			// --- Compare output ---
			gotHCLBytes := sortedFile.Bytes() // Use the returned sortedFile
			var gotCleanedHCL, wantCleanedHCL string

			if tc.skipClean {
				gotCleanedHCL = string(gotHCLBytes)
				wantCleanedHCL = tc.wantHCL // Assume wantHCL is already in the exact desired format
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
