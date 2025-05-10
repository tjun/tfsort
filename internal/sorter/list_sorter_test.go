package sorter

import (
   "testing"

   "github.com/tjun/tfsort/internal/parser"
)


func TestListSort(t *testing.T) {
   testCases := []struct {
       name        string
       inputHCL    string
       wantHCL     string
       sortOptions SortOptions
       wantErr     bool
       skipClean   bool
   }{
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
       // --- List Sorting Complex Elements ---
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
       // --- Test: List sorting can be disabled ---
       {
           name: "disable list sort",
           inputHCL: `
resource "test" "example" {
  list = ["b", "a", "c"]
}
`,
           wantHCL: `
resource "test" "example" {
  list = ["b", "a", "c"]
}
`,
           sortOptions: SortOptions{SortBlocks: true, SortTypeName: true, SortList: false},
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
               if string(gotHCLBytes) != gotCleanedHCL || tc.wantHCL != wantCleanedHCL {
                   t.Logf("Raw Got:\n%s", string(gotHCLBytes))
                   t.Logf("Raw Want (as provided in test case):\n%s", tc.wantHCL)
               }
           }
       })
   }
}