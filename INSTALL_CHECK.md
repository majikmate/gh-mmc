# Installing the Check Command

## Quick Install

If you already have gh-mmc installed, you can update to get the new `check` command:

```bash
cd ~/workspace/majikmate/gh-mmc
go build -o gh-mmc
gh extension upgrade mmc
```

## Manual Build and Install

### 1. Clone or Update the Repository

```bash
cd ~/workspace/majikmate/gh-mmc
git pull origin main
```

### 2. Build the Extension

```bash
go build -o gh-mmc
```

### 3. Install/Reinstall as GitHub CLI Extension

```bash
# If already installed, remove it first
gh extension remove mmc

# Install from local directory
gh extension install .
```

## Verify Installation

```bash
gh mmc --help
```

You should see `check` listed in the Available Commands.

## Test the Check Command

```bash
# Navigate to a classroom with assignments
cd /path/to/your/classroom

# Run the check command
gh mmc check -e .html
```

## Troubleshooting

### "check command not found"

- Rebuild the extension: `go build -o gh-mmc`
- Reinstall: `gh extension remove mmc && gh extension install .`

### Build errors

- Ensure Go is installed: `go version`
- Update dependencies: `go mod tidy`
- Check Go version compatibility (requires Go 1.19+)

### Permission denied

```bash
chmod +x gh-mmc
```

## What's New

The `check` command adds plagiarism detection to gh-mmc:

- **Compare student submissions** across assignments
- **Visual similarity matrix** with color-coded results
- **Configurable thresholds** for warnings
- **Multiple file type support** (.html, .css, .js, etc.)

## Next Steps

1. Read the full documentation: `CHECK_COMMAND.md`
2. Try it on an assignment: `gh mmc check -e .html`
3. Adjust threshold as needed: `gh mmc check -e .css -t 80`

## Files Added

```
pkg/similarity/
  └── similarity.go          # Similarity detection algorithms
cmd/check/
  └── check.go               # Check command implementation
cmd/root/
  └── root.go                # Updated to register check command
```

## Contributing

Found a bug or have a feature request? Please open an issue on GitHub.
