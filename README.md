# tfsort

**An opinionated sorter for Terraform configuration files.**

> **Warning:** This tool is currently under development and is **not yet recommended for production use**. Use at your own risk, and always review changes before applying them, especially when using the `--in-place` flag.

`tfsort` is a command-line tool written in Go that enforces a predictable order in your Terraform `.tf` files. It sorts:

- top-level blocks (e.g. `terraform`, `provider`, `variable`, `locals`, `data`, `resource`, `module`, `output`) based on a predefined order.
- `resource` and `data` blocks first by **type** (e.g., `aws_iam_role` before `aws_s3_bucket`) and then by **name** lexicographically.
- elements within list attributes lexicographically, with handling of comments and mixed data types.

This keeps your configuration organised, reduces noisy diffs, and makes code reviews easier.

---

## Features

- Sorts top-level blocks according to a standard convention (`terraform`, `provider`, `variable`, etc.).
- Sorts `resource` and `data` blocks by **type** then by **name**.
- Sorts elements within list attributes lexicographically with mixed-type handling.
- Comment preservation – comments stick with their associated list elements during sorting.
- Ignore sorting for specific list attributes using `// tfsort:ignore` or `# tfsort:ignore` comments.
- Zero external dependencies – a single static binary per platform.

---

## Installation

### Using pre-built binaries (recommended)

Binaries for Mac, Linux, and Windows are available on the [releases page](https://github.com/tjun/tfsort/releases).

### Using `go install`

```bash
go install github.com/tjun/tfsort/cmd/tfsort@latest
```

> Go ≥ 1.24 is required.

---

## Usage

```text
tfsort [flags] <file1.tf> [file2.tf] [...]
```

If no files are given, `tfsort` reads from **stdin** and writes the sorted output to **stdout**.

### Common flags

| Short | Long flag             | Default | Description                                                                                                                                                                                                                   |
| ----- | --------------------- | ------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `-r`  | `--recursive`         | false   | Walk directories recursively and process all `*.tf` files.                                                                                                                                                                    |
| `-i`  | `--in-place`          | false   | Overwrite files in place. For file inputs, files are only overwritten if changes are made. If no changes are necessary, the file is not touched. If input is from stdin, a warning is logged and output is written to stdout. |
|       | `--no-sort-blocks`    | false   | Disable sorting of top-level blocks (default: enabled).                                                                                                                                                                       |
|       | `--no-sort-type-name` | false   | Disable sorting of `resource`/`data` blocks by **type** and **name** (default: enabled).                                                                                                                                      |
|       | `--no-sort-list`      | false   | Disable sorting of list attribute values (default: enabled).                                                                                                                                                                  |
|       | `--dry-run`           | false   | Exit with status code 1 if any files would be changed, 0 otherwise. No files are written.                                                                                                                                     |
| `-h`  | `--help`              |         | Print help.                                                                                                                                                                                                                   |
| `-v`  | `--version`           |         | Print version.                                                                                                                                                                                                                |

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

Blocks of the same type maintain their relative order unless further sorting rules (like resource type/name sorting) apply. Unknown block types are typically sorted after all known types. This sorting can be disabled using the `--no-sort-blocks` flag (sorting is enabled by default).

### 2. Resource and Data Block Sorting

Within the `resource` and `data` block types, blocks are further sorted:

- **Primary Sort: By Type:** Blocks are first grouped and sorted alphabetically by their type label (e.g., `aws_iam_role` comes before `aws_s3_bucket`).
- **Secondary Sort: By Name:** Within each type group, blocks are then sorted alphabetically by their name label (e.g., for `aws_s3_bucket` type, `alpha_bucket` comes before `zeta_bucket`).

This ensures a consistent and predictable ordering for all your `resource` and `data` declarations. This sorting can be disabled using the `--no-sort-type-name` flag (sorting is enabled by default).

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

- **Simple Types:** For lists containing simple types like strings or numbers, the sorting is straightforward.
  - Numbers are compared based on their numerical value (e.g., `22` comes before `80`, `443`).
  - Strings are compared alphabetically (e.g., `"alpha"` comes before `"beta"`).
- **Mixed Types:** When a list contains a mix of types (numbers, strings, objects/blocks):
  - Numbers are generally sorted before strings.
  - Strings are generally sorted before objects/blocks (which start with `{`).
  - The exact order depends on the HCL string representation of each element as generated by the underlying HCL library, compared lexicographically (with numbers being a special case for numerical comparison).
- **Ignoring List Sorting:** To prevent a specific list from being sorted, place a `// tfsort:ignore` or `# tfsort:ignore` comment immediately after the list's opening square bracket `[`, either on the same line or the next. (Refer to the "Ignoring List Sorting" section for examples).

### Advanced List Features

`tfsort` handles various complex list scenarios:

**Mixed Type Lists with Comments:**

```hcl
// Before:
security_group_rules = [
  { type = "ingress", from_port = 22, protocol = "tcp" },
  "http-80-tcp", # HTTP traffic
  1024,          # Custom port
  "ssh-22-tcp",  # SSH access
]

// After tfsort:
security_group_rules = [
  1024,          # Custom port
  "http-80-tcp", # HTTP traffic
  "ssh-22-tcp",  # SSH access
  { type = "ingress", from_port = 22, protocol = "tcp" },
]
```

**Function Calls with Lists:**

```hcl
// Before:
allowed_ports = toset([
  "https-443-tcp",
  "http-80-tcp",
  "ssh-22-tcp",
])

// After tfsort:
allowed_ports = toset([
  "http-80-tcp",
  "https-443-tcp",
  "ssh-22-tcp",
])
```

_(Note: Attributes that are not lists (e.g., maps, simple string/number attributes) are not reordered based on their keys by `tfsort`. Their formatting might be normalized by the HCL writing library, but their relative order within a block is preserved.)_

### Enhanced Comment Handling

`tfsort` provides comment preservation that ensures comments stay with their associated elements during sorting. This feature has been significantly improved to handle complex commenting scenarios:

- **Leading Comments Stick to Elements:** Comments that appear immediately before an element are treated as "leading comments" associated with that element. When the list is sorted, these comments **move together with their associated element**.
- **Inline Comments Preserved:** Comments that appear on the same line as list elements (e.g., `"item", # comment`) are perfectly preserved and move with their element.
- **Proper Multi-line Formatting:** Both single-line and multi-line lists maintain proper formatting with appropriate trailing commas and newlines.
- **Mixed Comment Styles:** Both `//` and `#` comment styles are fully supported throughout.
- **Visual Grouping Not Preserved:** Blank lines or comments intended to visually separate groups of elements within a single list **are not treated as sorting boundaries**. The entire list's elements are sorted together based on the element content.

**Example:**

Consider this input list structured with comments and blank lines:

```hcl
# Before tfsort:
example_list = [
  # Group A
  "charlie",
  "alpha",

  # Group B
  "bravo",
]
```

After running `tfsort`, the elements will be sorted alphabetically, with comments moving with the element they precede, and the blank line likely removed:

```hcl
# After tfsort:
example_list = [
  # Group A
  "alpha",

  # Group B
  "bravo",
  # Group A
  "charlie",
]
```

Notice that `# Group B` moved with `"bravo"`, and the original grouping is lost due to the unified sort.

**Workarounds for Preserving Groups/Order:**

If you need to maintain specific groups of elements or a precise manual order within a list:

1.  **Use `# tfsort:ignore`:** Add the `# tfsort:ignore` comment immediately after the opening bracket `[` to disable sorting for the entire list.
2.  **Split the List:** Define multiple separate lists, potentially using local variables, and manage their contents independently. For example:

    ```hcl
    locals {
      group_a = [
        "charlie",
        "alpha",
      ]
      group_b = [
        "bravo",
      ]
    }

    resource "something" "example" {
      # Use concat or other functions as needed
      combined_list = concat(local.group_a, local.group_b)
      # Or use the groups separately
      list_a = local.group_a
      list_b = local.group_b
    }
    ```

    In this case, `tfsort` would sort the elements within `local.group_a` and `local.group_b` individually, but the `concat` function would preserve the group order in `combined_list`.

---

## Ignoring List Sorting

To prevent a specific list attribute from being sorted, place a `// tfsort:ignore` or `# tfsort:ignore` comment immediately after the opening square bracket `[` of the list. The comment can be on the same line as the bracket or on the immediately following line, before any list elements.

**Examples of ignoring a list:**

1.  **Ignore on the same line as the opening bracket:**

    ```hcl
    resource "example" "demo" {
      mixed_list = [ # tfsort:ignore
        "omega",
        100,
        "alpha",
        20
      ]
      another_attr = "sorted normally" // Note: only list sorting is affected by ignore
    }
    ```

2.  **Ignore on the line after the opening bracket:**

    ```hcl
    resource "example" "demo" {
      another_list = [
        # tfsort:ignore
        "zulu",
        "yankee",
        "xray"
      ]
    }
    ```

3.  **Using hash-style comments:**

    ```hcl
    variable "custom_order_ports" {
      type = list(number)
      default = [ # tfsort:ignore
        443,  # HTTPS first - high priority
        80,   # HTTP second
        22,   # SSH last - restricted access
      ]
    }
    ```

4.  **Complex lists with ignore directive:**
    ```hcl
    locals {
      deployment_order = [
        # tfsort:ignore - preserve deployment sequence
        { name = "database", priority = 1 },
        { name = "backend",  priority = 2 },
        { name = "frontend", priority = 3 },
      ]
    }
    ```

Lists that do not have this specific comment pattern will be sorted according to the standard lexicographic rules. The ignore directive only affects the specific list where it appears – other lists and block sorting continue normally.

---

## Command Examples

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

Sort from stdin and write to stdout:

```bash
cat main.tf | tfsort
```

---

## Exit codes

| Code | Meaning                                                                                                                                                     |
| ---- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 0    | Success. Files were already sorted, or `--in-place` was used and changes were successfully applied.                                                         |
| 1    | Changes were detected or applied. This includes `--dry-run` detecting changes, or if the default behavior (writing to stdout) resulted in modified content. |
| 2    | Error during processing (e.g., parsing error, file I/O error).                                                                                              |

---

## Contributing

Bug reports and pull requests are welcome! Please open an issue first if you plan a large change.

1.  Fork the repo and create your branch: `git checkout -b feature/xyz`
2.  Run `make test` and ensure the linter passes (`make lint`)
3.  Submit a PR describing your changes

---

## License

`tfsort` is released under the [MIT License](LICENSE).
