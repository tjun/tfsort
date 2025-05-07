# tfsort

**An opinionated sorter for Terraform configuration files.**

`tfsort` is a command-line tool written in Go that enforces a predictable order in your Terraform `.tf` files. It sorts:

*   top-level blocks (e.g. `terraform`, `provider`, `variable`, `locals`, `data`, `resource`, `module`, `output`) based on a predefined order.
*   `resource` and `data` blocks first by **type** (e.g., `aws_iam_role` before `aws_s3_bucket`) and then by **name** lexicographically.
*   elements within list attributes lexicographically.

This keeps your configuration organised, reduces noisy diffs, and makes code reviews easier.

---

## Features

*   Sorts top-level blocks according to a standard convention (`terraform`, `provider`, `variable`, etc.).
*   Sorts `resource` and `data` blocks by **type** then by **name**.
*   Sorts elements within list attributes lexicographically.
*   Optional in-place editing of files or writing to stdout.
*   Ignore sorting for specific list attributes using a `// tfsort:ignore` comment.
*   Recursive directory traversal.
*   Reads from stdin if no files are provided.
*   Zero external dependencies – a single static binary per platform.

---

## Installation

### Using pre-built binaries (recommended)

```bash
# macOS (Apple Silicon)
curl -L https://github.com/yourorg/tfsort/releases/latest/download/tfsort_darwin_arm64.tar.gz | tar -xz
sudo mv tfsort /usr/local/bin/
```

Binaries for Linux, Windows and other architectures are available on the [releases page](https://github.com/tjun/tfsort/releases).

### Using `go install`

```bash
go install github.com/yourorg/tfsort@latest
```

> Go ≥ 1.22 are supported.

---

## Usage

```text
tfsort [flags] <file1.tf> [file2.tf] [...]
```

If no files are given, `tfsort` reads from **stdin** and writes the sorted output to **stdout**.

### Common flags

| Short | Long flag            | Default | Description                                                                                                                                                                                             |
|-------|----------------------|---------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `-r`  | `--recursive`        | false   | Walk directories recursively and process all `*.tf` files.                                                                                                                                              |
| `-i`  | `--in-place`         | false   | Overwrite files in place. For file inputs, files are only overwritten if changes are made. If no changes are necessary, the file is not touched. If input is from stdin, a warning is logged and output is written to stdout. |
|       | `--sort-blocks`      | true    | Enable sorting of top-level blocks (default: enabled).                                                                                                                                                   |
|       | `--sort-type-name`   | true    | Enable sorting of `resource`/`data` blocks by **type** and **name** (default: enabled).                                                                                                                  |
|       | `--dry-run`          | false   | Exit with status code 1 if any files would be changed, 0 otherwise. No files are written.                                                                                                               |
| `-h`  | `--help`             |         | Print help.                                                                                                                                                                                             |
| `-v`  | `--version`          |         | Print version.                                                                                                                                                                                          |

---

## Detailed Sorting Rules

`tfsort` applies the following sorting logic to your Terraform files:

### 1. Top-Level Block Sorting

Top-level blocks (those not nested within other blocks) are sorted according to a predefined conventional order. This helps in organizing the overall structure of a `.tf` file. The standard order is:

1.  `terraform`
2.  `provider`
3.  `variable`
4.  `locals`
5.  `data`
6.  `module`
7.  `resource`
8.  `output`

Blocks of the same type maintain their relative order unless further sorting rules (like resource type/name sorting) apply. Unknown block types are typically sorted after all known types. This sorting can be disabled using the `--sort-blocks=false` flag (though it is enabled by default).

### 2. Resource and Data Block Sorting

Within the `resource` and `data` block types, blocks are further sorted:

*   **Primary Sort: By Type:** Blocks are first grouped and sorted alphabetically by their type label (e.g., `aws_iam_role` comes before `aws_s3_bucket`).
*   **Secondary Sort: By Name:** Within each type group, blocks are then sorted alphabetically by their name label (e.g., for `aws_s3_bucket` type, `alpha_bucket` comes before `zeta_bucket`).

This ensures a consistent and predictable ordering for all your `resource` and `data` declarations. This sorting can be disabled using the `--sort-type-name=false` flag (though it is enabled by default).

**Example:**

```hcl
// Before:
resource "aws_s3_bucket" "config_storage" {}
resource "aws_iam_role" "app_role" {}
data "aws_caller_identity" "current" {}
resource "aws_s3_bucket" "asset_storage" {}

// After tfsort:
data "aws_caller_identity" "current" {} // Data blocks sorted with resources by type/name
resource "aws_iam_role" "app_role" {}
resource "aws_s3_bucket" "asset_storage" {}
resource "aws_s3_bucket" "config_storage" {}
```

### 3. List Attribute Sorting

Elements within list attributes are sorted lexicographically based on their HCL string representation.

*   **Simple Types:** For lists containing simple types like strings or numbers, the sorting is straightforward.
    *   Numbers are compared based on their numerical value (e.g., `22` comes before `80`, `443`).
    *   Strings are compared alphabetically (e.g., `"alpha"` comes before `"beta"`).
*   **Mixed Types:** When a list contains a mix of types (numbers, strings, objects/blocks):
    *   Numbers are generally sorted before strings.
    *   Strings are generally sorted before objects/blocks (which start with `{`).
    *   The exact order depends on the HCL string representation of each element as generated by the underlying HCL library, compared lexicographically (with numbers being a special case for numerical comparison).
*   **Ignoring List Sorting:** To prevent a specific list from being sorted, place a `// tfsort:ignore` comment immediately after the list's opening square bracket `[`, either on the same line or the next. (Refer to the "Ignoring List Sorting" section for examples).

**Example (Mixed List):**

```hcl
// Before:
security_group_rules = [
  { type = "ingress", from_port = 22, protocol = "tcp" },
  "http-80-tcp",
  1024,
  "ssh-22-tcp",
]

// After tfsort:
security_group_rules = [
  1024,
  "http-80-tcp",
  "ssh-22-tcp",
  { type = "ingress", from_port = 22, protocol = "tcp" },
]
```

*(Note: Attributes that are not lists (e.g., maps, simple string/number attributes) are not reordered based on their keys by `tfsort`. Their formatting might be normalized by the HCL writing library, but their relative order within a block is preserved.)*

---

### Ignoring List Sorting

To prevent a specific list attribute from being sorted, place a `// tfsort:ignore` comment immediately after the opening square bracket `[` of the list. The comment can be on the same line as the bracket or on the immediately following line, before any list elements.

**Examples of ignoring a list:**

1.  **Ignore on the same line as the opening bracket:**
    ```hcl
    resource "example" "demo" {
      mixed_list = [ // tfsort:ignore
        "omega",
        100,
        "alpha",
        20
      ]
      another_attr = "sorted normally" // Note: only list sorting is affected by ignore
    }
    ```
    In this case, the `mixed_list` will not have its elements sorted. Other blocks and list attributes (without the ignore comment) will be sorted as usual.

2.  **Ignore on the line after the opening bracket:**
    ```hcl
    resource "example" "demo" {
      another_list = [
        // tfsort:ignore
        "zulu",
        "yankee",
        "xray"
      ]
    }
    ```
    Similarly, `another_list` will remain unsorted.

Lists that do not have this specific comment pattern will be sorted.

### Examples of Sorting

#### Basic Usage

Sort a single file and print the result to stdout:
```bash
tfsort main.tf
```

Sort all `.tf` files recursively in the current directory and its subdirectories, overwriting them in place if changes are needed:
```bash
tfsort -r -i ./
```

Check if any files in the `modules/vpc` directory would be changed by sorting, and exit with status 1 if so (otherwise 0):
```bash
tfsort --dry-run modules/vpc
```

#### Sorting Top-Level Blocks

`tfsort` orders top-level blocks like `provider`, `variable`, `locals`, `data`, `resource`, and `output` according to a standard convention.

**Before:**
```hcl
resource "aws_instance" "web" {
  ami           = "ami-0c55b31ad29f50665"
  instance_type = "t2.micro"
}

variable "region" {
  description = "AWS region"
  type        = string
  default     = "us-west-2"
}

provider "aws" {
  region = var.region
}

output "instance_ip" {
  value = aws_instance.web.public_ip
}
```

**After `tfsort`:**
```hcl
provider "aws" {
  region = var.region
}

variable "region" {
  description = "AWS region"
  type        = string
  default     = "us-west-2"
}

resource "aws_instance" "web" {
  ami           = "ami-0c55b31ad29f50665"
  instance_type = "t2.micro"
}

output "instance_ip" {
  value = aws_instance.web.public_ip
}
```

#### Sorting Resource and Data Blocks by Type and Name

`resource` and `data` blocks are sorted first by their type (e.g., `aws_iam_role` before `aws_s3_bucket`) and then by their name lexicographically.

**Before:**
```hcl
resource "aws_s3_bucket" "config_storage" {
  bucket = "my-config-storage-bucket"
}

resource "aws_iam_role" "app_role" {
  name = "my_application_role"
  assume_role_policy = jsonencode({
    Version   = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
    }]
  })
}

data "aws_caller_identity" "current" {}

resource "aws_s3_bucket" "asset_storage" {
  bucket = "my-asset-storage-bucket"
}
```

**After `
