# gocoverhtml

A beautiful HTML coverage report generator for Go.

## Installation

```bash
go install github.com/raugustinus/gocoverhtml@latest
```

## Usage

```bash
# Generate coverage profile
go test -coverprofile=coverage.out ./...

# Generate HTML report
gocoverhtml -i coverage.out -o coverage/index.html
```

## Options

- `-i` - Input coverage profile (default: `coverage.out`)
- `-o` - Output HTML file (default: `coverage/index.html`)
- `-title` - Report title (default: `Coverage Report`)
- `-theme` - Theme: `light` or `dark` (default: `dark`)
- `-src` - Source root directory (default: `.`)

## Features

- Beautiful dark and light themes
- File tree navigation with coverage percentages
- Line-by-line coverage highlighting
- Regex-powered file search
- Keyboard navigation (Ctrl/Cmd+F to search, Escape to close)
- Self-contained single HTML file output
