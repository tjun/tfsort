# tfsort

**A sorter for Terraform configuration files.**

`tfsort` is a command-line tool written in Go that enforces a predictable order in your Terraform `.tf` files. It sorts

* top-level blocks (e.g. `resource`, `data`, `module`, `variable`, `output`, …)
* the **type** (`aws_instance`, `google_compute_instance`, …) and **name** inside `resource` / `data` blocks
* attributes within blocks (lists, maps, nested blocks) in a stable, lexicographical order

This keeps your configuration organised, reduces noisy diffs, and makes code reviews easier.

---

## Features

* Sort `resource` and `data` blocks by **type** then **name**
* Sort maps, lists, and nested blocks inside any block

* Optional in-place editing or safe write to stdout
* Ignore individual blocks via `// tfsort:ignore` leading comments
* Zero external dependencies – a single static binary per platform

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

If no files are given, `tfsort` reads from **stdin** and writes to **stdout**.

### Common flags

| Short | Long flag            | Default | Description                                                             |
|-------|----------------------|---------|-------------------------------------------------------------------------|
| `-r`  | `--recursive`        | false   | Walk directories recursively and process all `*.tf` files               |
| `-i`  | `--in-place`         | false   | Overwrite files in place instead of printing to stdout                  |
|       | `--sort-blocks`      | on      | Enable sorting of top-level blocks                                      |
|       | `--sort-attrs`       | on      | Enable sorting of attributes inside blocks (lists, maps, nested blocks) |
|       | `--sort-type-name`   | on      | Enable sorting of `resource`/`data` **type** and **name**               |
|       | `--diff`             | false   | Show a colourised diff instead of modifying output                      |
|       | `--dry-run`          | false   | Exit with non-zero status if changes would be made                      |
| `-h`  | `--help`             |         | Print help                                                              |
| `-v`  | `--version`          |         | Print version                                                           |

### Ignoring blocks

Place `// tfsort:ignore` on the **line before** a block to prevent any modifications:

```hcl
// tfsort:ignore
resource "aws_instance" "legacy" {
  # kept exactly as is
  user_data = file("userdata.sh")
  ami       = "ami-123456"
}
```

### Examples

Sort a single file and print result to stdout:

```bash
tfsort main.tf
```

Sort all `.tf` files recursively and overwrite them in place:

```bash
tfsort -r -i ./
```

Show what **would** change and return a non-zero exit code if formatting is needed:

```bash
tfsort --dry-run modules/vpc
```

Produce a colourised diff:

```bash
tfsort --diff prod/network.tf
```

---

## Exit codes

| Code | Meaning                           |
|------|-----------------------------------|
| 0    | Success, no changes needed        |
| 1    | Changes were made or would be made|
| 2    | Error during processing           |

---

## Contributing

Bug reports and pull requests are welcome! Please open an issue first if you plan a large change.

1. Fork the repo and create your branch: `git checkout -b feature/xyz`
2. Run `make test` and ensure the linter passes
3. Submit a PR describing your changes

---

## License

`tfsort` is released under the [MIT License](LICENSE).
