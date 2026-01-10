// gocoverhtml generates beautiful HTML coverage reports from Go coverage profiles.
//
// Usage:
//
//	go test -coverprofile=coverage.out ./...
//	gocoverhtml -i coverage.out -o coverage/index.html
package main

import (
	"bufio"
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func main() {
	var (
		input   string
		output  string
		title   string
		theme   string
		srcRoot string
	)

	flag.StringVar(&input, "i", "coverage.out", "Input coverage profile")
	flag.StringVar(&output, "o", "coverage/index.html", "Output HTML file")
	flag.StringVar(&title, "title", "Coverage Report", "Report title")
	flag.StringVar(&theme, "theme", "dark", "Theme (light, dark)")
	flag.StringVar(&srcRoot, "src", ".", "Source root directory")
	flag.Parse()

	// Parse coverage profile
	profile, err := parseCoverageProfile(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing coverage profile: %v\n", err)
		os.Exit(1)
	}

	// Build file coverage map
	files := buildFileCoverage(profile)

	// Build tree structure
	tree := buildTree(files)

	// Read source files and annotate with coverage
	for _, fc := range files {
		fc.Lines = readAndAnnotateFile(srcRoot, fc)
	}

	// Generate HTML
	htmlContent := generateHTML(tree, files, title, theme)

	// Ensure output directory exists
	if dir := filepath.Dir(output); dir != "" {
		os.MkdirAll(dir, 0755)
	}

	// Write output
	if err := os.WriteFile(output, []byte(htmlContent), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Coverage report generated: %s\n", output)
}

// CoverageBlock represents a single coverage block from the profile
type CoverageBlock struct {
	StartLine  int
	StartCol   int
	EndLine    int
	EndCol     int
	Statements int
	Count      int
}

// LineCoverage represents coverage status for a single line
type LineCoverage struct {
	LineNum  int
	Content  string
	Covered  bool
	NotCovered bool
	Partial  bool
}

// FileCoverage represents coverage for a single file
type FileCoverage struct {
	Path       string // Full path like insights/internal/capture/packet.go
	Name       string // Just the filename
	Blocks     []CoverageBlock
	Lines      []LineCoverage
	Statements int
	Covered    int
	Percent    float64
}

// TreeNode represents a node in the file tree
type TreeNode struct {
	Name       string
	Path       string
	IsFile     bool
	Children   []*TreeNode
	File       *FileCoverage
	Statements int
	Covered    int
	Percent    float64
}

func parseCoverageProfile(filename string) (map[string][]CoverageBlock, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	blocks := make(map[string][]CoverageBlock)
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()

		// Skip mode line
		if strings.HasPrefix(line, "mode:") {
			continue
		}

		// Parse: file:startLine.startCol,endLine.endCol statements count
		parts := strings.Fields(line)
		if len(parts) != 3 {
			continue
		}

		// Parse file:position
		colonIdx := strings.LastIndex(parts[0], ":")
		if colonIdx == -1 {
			continue
		}
		file := parts[0][:colonIdx]
		positions := parts[0][colonIdx+1:]

		// Parse positions: startLine.startCol,endLine.endCol
		var startLine, startCol, endLine, endCol int
		fmt.Sscanf(positions, "%d.%d,%d.%d", &startLine, &startCol, &endLine, &endCol)

		statements, _ := strconv.Atoi(parts[1])
		count, _ := strconv.Atoi(parts[2])

		blocks[file] = append(blocks[file], CoverageBlock{
			StartLine:  startLine,
			StartCol:   startCol,
			EndLine:    endLine,
			EndCol:     endCol,
			Statements: statements,
			Count:      count,
		})
	}

	return blocks, scanner.Err()
}

func buildFileCoverage(blocks map[string][]CoverageBlock) map[string]*FileCoverage {
	files := make(map[string]*FileCoverage)

	for path, fileBlocks := range blocks {
		fc := &FileCoverage{
			Path:   path,
			Name:   filepath.Base(path),
			Blocks: fileBlocks,
		}

		for _, b := range fileBlocks {
			fc.Statements += b.Statements
			if b.Count > 0 {
				fc.Covered += b.Statements
			}
		}

		if fc.Statements > 0 {
			fc.Percent = float64(fc.Covered) / float64(fc.Statements) * 100
		}

		files[path] = fc
	}

	return files
}

func buildTree(files map[string]*FileCoverage) *TreeNode {
	root := &TreeNode{Name: "root", Path: ""}

	for path, fc := range files {
		// Remove module prefix for cleaner paths
		cleanPath := strings.TrimPrefix(path, "insights/")
		parts := strings.Split(cleanPath, "/")

		current := root
		currentPath := ""

		for i, part := range parts {
			if currentPath == "" {
				currentPath = part
			} else {
				currentPath = currentPath + "/" + part
			}

			isFile := i == len(parts)-1

			// Find or create child
			var child *TreeNode
			for _, c := range current.Children {
				if c.Name == part {
					child = c
					break
				}
			}

			if child == nil {
				child = &TreeNode{
					Name:   part,
					Path:   currentPath,
					IsFile: isFile,
				}
				if isFile {
					child.File = fc
				}
				current.Children = append(current.Children, child)
			}

			current = child
		}
	}

	// Calculate coverage for each node (bottom-up)
	calculateTreeCoverage(root)

	// Sort children at each level
	sortTree(root)

	return root
}

func calculateTreeCoverage(node *TreeNode) (statements, covered int) {
	if node.IsFile && node.File != nil {
		node.Statements = node.File.Statements
		node.Covered = node.File.Covered
		node.Percent = node.File.Percent
		return node.Statements, node.Covered
	}

	for _, child := range node.Children {
		s, c := calculateTreeCoverage(child)
		statements += s
		covered += c
	}

	node.Statements = statements
	node.Covered = covered
	if statements > 0 {
		node.Percent = float64(covered) / float64(statements) * 100
	}

	return statements, covered
}

func sortTree(node *TreeNode) {
	// Sort: directories first, then files, alphabetically
	sort.Slice(node.Children, func(i, j int) bool {
		if node.Children[i].IsFile != node.Children[j].IsFile {
			return !node.Children[i].IsFile // directories first
		}
		return node.Children[i].Name < node.Children[j].Name
	})

	for _, child := range node.Children {
		sortTree(child)
	}
}

func readAndAnnotateFile(srcRoot string, fc *FileCoverage) []LineCoverage {
	// Try to find the source file
	cleanPath := strings.TrimPrefix(fc.Path, "insights/")
	filePath := filepath.Join(srcRoot, cleanPath)

	content, err := os.ReadFile(filePath)
	if err != nil {
		// File not found, return empty
		return nil
	}

	lines := strings.Split(string(content), "\n")
	result := make([]LineCoverage, len(lines))

	for i, line := range lines {
		result[i] = LineCoverage{
			LineNum: i + 1,
			Content: line,
		}
	}

	// Mark covered/not covered lines based on blocks
	for _, block := range fc.Blocks {
		for lineNum := block.StartLine; lineNum <= block.EndLine; lineNum++ {
			if lineNum > 0 && lineNum <= len(result) {
				if block.Count > 0 {
					result[lineNum-1].Covered = true
				} else {
					result[lineNum-1].NotCovered = true
				}
			}
		}
	}

	return result
}

func generateHTML(tree *TreeNode, files map[string]*FileCoverage, title, theme string) string {
	var b strings.Builder

	// Calculate totals
	totalStatements := tree.Statements
	totalCovered := tree.Covered
	totalPercent := tree.Percent

	b.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>` + html.EscapeString(title) + `</title>
    <link href="https://fonts.googleapis.com/css2?family=Roboto:wght@300;400;500;700&family=Roboto+Mono:wght@400;500&display=swap" rel="stylesheet">
    <style>
`)
	b.WriteString(getCSS(theme))
	b.WriteString(`
    </style>
</head>
<body>
    <div class="app">
        <aside class="sidebar">
            <div class="sidebar-header">
                <h1>` + html.EscapeString(title) + `</h1>
                <div class="total-coverage ` + getCoverageClass(totalPercent) + `">
                    ` + fmt.Sprintf("%.1f%%", totalPercent) + `
                </div>
                <div class="stats">` + fmt.Sprintf("%d / %d statements", totalCovered, totalStatements) + `</div>
                <div class="search-container">
                    <input type="text" id="search-input" class="search-input" placeholder="Search files (regex)..." />
                    <span class="search-icon">🔍</span>
                    <span class="search-clear" id="search-clear" onclick="clearSearch()">✕</span>
                </div>
            </div>
            <div class="tree" id="tree">
`)

	// Generate tree HTML
	for _, child := range tree.Children {
		generateTreeNode(&b, child, 0)
	}

	b.WriteString(`
            </div>
        </aside>
        <main class="content">
            <div class="welcome" id="welcome">
                <div class="welcome-icon">📊</div>
                <h2>Coverage Report</h2>
                <p>Select a file from the tree to view its coverage details.</p>
                <div class="legend">
                    <div class="legend-item"><span class="legend-color covered"></span> Covered</div>
                    <div class="legend-item"><span class="legend-color not-covered"></span> Not Covered</div>
                </div>
            </div>
`)

	// Generate file content sections (hidden by default)
	for path, fc := range files {
		cleanPath := strings.TrimPrefix(path, "insights/")
		fileID := pathToID(cleanPath)

		b.WriteString(`
            <div class="file-view" id="file-` + fileID + `" style="display: none;">
                <div class="file-header">
                    <div class="file-path">` + html.EscapeString(cleanPath) + `</div>
                    <div class="file-stats">
                        <span class="coverage-badge ` + getCoverageClass(fc.Percent) + `">` + fmt.Sprintf("%.1f%%", fc.Percent) + `</span>
                        <span class="stmt-count">` + fmt.Sprintf("%d / %d statements", fc.Covered, fc.Statements) + `</span>
                    </div>
                </div>
                <div class="code-container">
                    <table class="code">
                        <tbody>
`)

		for _, line := range fc.Lines {
			lineClass := ""
			if line.Covered {
				lineClass = "covered"
			} else if line.NotCovered {
				lineClass = "not-covered"
			}

			b.WriteString(`                            <tr class="` + lineClass + `">
                                <td class="line-num">` + strconv.Itoa(line.LineNum) + `</td>
                                <td class="line-content"><pre>` + html.EscapeString(line.Content) + `</pre></td>
                            </tr>
`)
		}

		b.WriteString(`
                        </tbody>
                    </table>
                </div>
            </div>
`)
	}

	b.WriteString(`
        </main>
    </div>
    <script>
`)
	b.WriteString(getJS())
	b.WriteString(`
    </script>
</body>
</html>
`)

	return b.String()
}

func generateTreeNode(b *strings.Builder, node *TreeNode, depth int) {
	indent := strings.Repeat("    ", depth)
	coverClass := getCoverageClass(node.Percent)

	if node.IsFile {
		fileID := pathToID(node.Path)
		b.WriteString(indent + `<div class="tree-file" data-file="` + fileID + `" onclick="showFile('` + fileID + `')">
` + indent + `    <span class="tree-icon">📄</span>
` + indent + `    <span class="tree-name">` + html.EscapeString(node.Name) + `</span>
` + indent + `    <span class="tree-percent ` + coverClass + `">` + fmt.Sprintf("%.0f%%", node.Percent) + `</span>
` + indent + `</div>
`)
	} else {
		// Directory node
		hasChildren := len(node.Children) > 0
		expandClass := ""
		if hasChildren {
			expandClass = "expandable" // collapsed by default
		}

		b.WriteString(indent + `<div class="tree-dir ` + expandClass + `">
` + indent + `    <div class="tree-dir-header" onclick="toggleDir(this)">
` + indent + `        <span class="tree-icon">` + getDirIcon(hasChildren) + `</span>
` + indent + `        <span class="tree-name">` + html.EscapeString(node.Name) + `</span>
` + indent + `        <span class="tree-percent ` + coverClass + `">` + fmt.Sprintf("%.0f%%", node.Percent) + `</span>
` + indent + `    </div>
`)

		if hasChildren {
			b.WriteString(indent + `    <div class="tree-children">
`)
			for _, child := range node.Children {
				generateTreeNode(b, child, depth+2)
			}
			b.WriteString(indent + `    </div>
`)
		}

		b.WriteString(indent + `</div>
`)
	}
}

func pathToID(path string) string {
	// Convert path to valid HTML ID
	return strings.ReplaceAll(strings.ReplaceAll(path, "/", "-"), ".", "-")
}

func getDirIcon(hasChildren bool) string {
	// Return closed folder icon (collapsed by default)
	return "📁"
}

func getCoverageClass(percent float64) string {
	switch {
	case percent >= 80:
		return "high"
	case percent >= 50:
		return "medium"
	case percent > 0:
		return "low"
	default:
		return "none"
	}
}

func getCSS(theme string) string {
	if theme == "light" {
		return cssLight
	}
	return cssDark
}

func getJS() string {
	return `
let activeFile = null;

function showFile(fileId) {
    // Hide welcome message
    document.getElementById('welcome').style.display = 'none';

    // Hide current file if any
    if (activeFile) {
        document.getElementById('file-' + activeFile).style.display = 'none';
        document.querySelector('[data-file="' + activeFile + '"]')?.classList.remove('active');
    }

    // Show new file
    const fileView = document.getElementById('file-' + fileId);
    if (fileView) {
        fileView.style.display = 'flex';
        document.querySelector('[data-file="' + fileId + '"]')?.classList.add('active');
        activeFile = fileId;
    }
}

function toggleDir(header) {
    const dir = header.parentElement;
    if (!dir.classList.contains('expandable')) return;

    const isExpanded = dir.classList.contains('expanded');
    dir.classList.toggle('expanded');

    const icon = header.querySelector('.tree-icon');
    icon.textContent = isExpanded ? '📁' : '📂';
}

// Search functionality
const searchInput = document.getElementById('search-input');
const searchContainer = searchInput?.parentElement;

function performSearch(query) {
    const tree = document.getElementById('tree');
    const files = tree.querySelectorAll('.tree-file');
    const dirs = tree.querySelectorAll('.tree-dir');

    // Reset all
    files.forEach(f => {
        f.classList.remove('search-hidden', 'search-match');
    });
    dirs.forEach(d => {
        d.classList.remove('search-hidden');
    });

    if (!query) {
        searchContainer?.classList.remove('has-value');
        return;
    }

    searchContainer?.classList.add('has-value');

    // Try to compile as regex, fall back to substring match
    let regex;
    try {
        regex = new RegExp(query, 'i');
    } catch (e) {
        regex = new RegExp(query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'i');
    }

    // Find matching files
    const matchingFiles = new Set();
    files.forEach(file => {
        const name = file.querySelector('.tree-name')?.textContent || '';
        const fileId = file.getAttribute('data-file') || '';
        // Match against filename or full path (fileId contains path with dashes)
        const path = fileId.replace(/-/g, '/');
        if (regex.test(name) || regex.test(path)) {
            matchingFiles.add(file);
            file.classList.add('search-match');
        } else {
            file.classList.add('search-hidden');
        }
    });

    // Show parent directories of matching files, hide others
    dirs.forEach(dir => {
        const hasVisibleContent = hasVisibleDescendant(dir, matchingFiles);
        if (!hasVisibleContent) {
            dir.classList.add('search-hidden');
        } else {
            // Expand directories with matches
            if (dir.classList.contains('expandable') && !dir.classList.contains('expanded')) {
                dir.classList.add('expanded');
                const icon = dir.querySelector('.tree-dir-header .tree-icon');
                if (icon) icon.textContent = '📂';
            }
        }
    });
}

function hasVisibleDescendant(dir, matchingFiles) {
    const children = dir.querySelector('.tree-children');
    if (!children) return false;

    // Check direct file children
    const files = children.querySelectorAll(':scope > .tree-file');
    for (const f of files) {
        if (matchingFiles.has(f)) return true;
    }

    // Check directory children recursively
    const subDirs = children.querySelectorAll(':scope > .tree-dir');
    for (const d of subDirs) {
        if (hasVisibleDescendant(d, matchingFiles)) return true;
    }

    return false;
}

function clearSearch() {
    if (searchInput) {
        searchInput.value = '';
        performSearch('');
        searchInput.focus();
    }
}

// Setup search event listeners
if (searchInput) {
    searchInput.addEventListener('input', (e) => {
        performSearch(e.target.value);
    });

    searchInput.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') {
            if (searchInput.value) {
                clearSearch();
                e.stopPropagation();
            }
        }
    });
}

// Keyboard navigation
document.addEventListener('keydown', (e) => {
    // Focus search on Ctrl/Cmd + F
    if ((e.ctrlKey || e.metaKey) && e.key === 'f') {
        e.preventDefault();
        searchInput?.focus();
        searchInput?.select();
        return;
    }

    if (e.key === 'Escape') {
        if (activeFile) {
            document.getElementById('file-' + activeFile).style.display = 'none';
            document.querySelector('[data-file="' + activeFile + '"]')?.classList.remove('active');
            document.getElementById('welcome').style.display = 'flex';
            activeFile = null;
        }
    }
});
`
}

const cssDark = `
* {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
}

:root {
    --md-primary-fg-color: #4051b5;
    --md-primary-bg-color: #fff;
    --md-accent-fg-color: #526cfe;
    --bg-primary: #1a1a2e;
    --bg-secondary: #16213e;
    --bg-tertiary: #1f2940;
    --bg-hover: #253550;
    --bg-active: #4051b5;
    --text-primary: #e8eaed;
    --text-secondary: #9aa0a6;
    --text-muted: #6b7280;
    --border: #2d3748;
    --accent: #526cfe;
    --covered: #00c853;
    --covered-bg: rgba(0, 200, 83, 0.12);
    --not-covered: #ff5252;
    --not-covered-bg: rgba(255, 82, 82, 0.12);
    --high: #00c853;
    --medium: #ffab00;
    --low: #ff5252;
    --none: #6b7280;
    --shadow: 0 2px 4px rgba(0,0,0,0.2), 0 4px 8px rgba(0,0,0,0.1);
    --shadow-sm: 0 1px 2px rgba(0,0,0,0.2);
}

body {
    font-family: 'Roboto', -apple-system, BlinkMacSystemFont, sans-serif;
    background: var(--bg-primary);
    color: var(--text-primary);
    height: 100vh;
    overflow: hidden;
    font-size: 14px;
    line-height: 1.6;
    -webkit-font-smoothing: antialiased;
}

.app {
    display: flex;
    height: 100vh;
}

.sidebar {
    width: 300px;
    background: var(--bg-secondary);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
    box-shadow: var(--shadow);
    z-index: 10;
}

.sidebar-header {
    padding: 20px;
    border-bottom: 1px solid var(--border);
    background: linear-gradient(135deg, var(--md-primary-fg-color), var(--accent));
}

.sidebar-header h1 {
    font-size: 16px;
    font-weight: 500;
    color: #fff;
    margin-bottom: 12px;
    letter-spacing: 0.02em;
}

.total-coverage {
    font-size: 36px;
    font-weight: 700;
    margin-bottom: 4px;
}

.total-coverage.high { color: #69f0ae; }
.total-coverage.medium { color: #ffd740; }
.total-coverage.low { color: #ff8a80; }
.total-coverage.none { color: rgba(255,255,255,0.5); }

.stats {
    font-size: 12px;
    color: rgba(255,255,255,0.8);
    font-weight: 400;
}

.search-container {
    position: relative;
    margin-top: 16px;
}

.search-input {
    width: 100%;
    padding: 10px 36px 10px 14px;
    font-size: 14px;
    font-family: inherit;
    background: rgba(255,255,255,0.1);
    border: none;
    border-radius: 8px;
    color: #fff;
    outline: none;
    transition: background 0.2s ease;
}

.search-input:focus {
    background: rgba(255,255,255,0.15);
}

.search-input::placeholder {
    color: rgba(255,255,255,0.5);
}

.search-icon {
    position: absolute;
    right: 12px;
    top: 50%;
    transform: translateY(-50%);
    font-size: 14px;
    pointer-events: none;
    opacity: 0.6;
}

.search-clear {
    position: absolute;
    right: 12px;
    top: 50%;
    transform: translateY(-50%);
    font-size: 14px;
    cursor: pointer;
    color: rgba(255,255,255,0.6);
    display: none;
}

.search-clear:hover {
    color: #fff;
}

.search-container.has-value .search-icon {
    display: none;
}

.search-container.has-value .search-clear {
    display: block;
}

.tree-file.search-hidden,
.tree-dir.search-hidden {
    display: none;
}

.tree-file.search-match .tree-name {
    color: var(--accent);
    font-weight: 500;
}

.tree {
    flex: 1;
    overflow-y: auto;
    padding: 12px 0;
}

.tree-dir, .tree-file {
    user-select: none;
}

.tree-dir-header, .tree-file {
    display: flex;
    align-items: center;
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
    gap: 8px;
    transition: background 0.15s ease;
    border-radius: 0;
    margin: 0 8px;
    border-radius: 4px;
}

.tree-dir-header:hover, .tree-file:hover {
    background: var(--bg-hover);
}

.tree-file.active {
    background: var(--bg-active);
    color: #fff;
}

.tree-icon {
    font-size: 16px;
    flex-shrink: 0;
    opacity: 0.8;
}

.tree-name {
    flex: 1;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-weight: 400;
}

.tree-percent {
    font-size: 11px;
    font-weight: 500;
    padding: 3px 8px;
    border-radius: 12px;
    flex-shrink: 0;
}

.tree-percent.high { background: rgba(0, 200, 83, 0.15); color: var(--high); }
.tree-percent.medium { background: rgba(255, 171, 0, 0.15); color: var(--medium); }
.tree-percent.low { background: rgba(255, 82, 82, 0.15); color: var(--low); }
.tree-percent.none { background: rgba(107, 114, 128, 0.15); color: var(--none); }

.tree-children {
    padding-left: 20px;
}

.tree-dir:not(.expanded) > .tree-children {
    display: none;
}

.content {
    flex: 1;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    background: var(--bg-primary);
}

.welcome {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    color: var(--text-secondary);
    gap: 20px;
}

.welcome-icon {
    font-size: 72px;
    opacity: 0.8;
}

.welcome h2 {
    font-size: 24px;
    font-weight: 500;
    color: var(--text-primary);
}

.welcome p {
    font-size: 14px;
    max-width: 300px;
    text-align: center;
}

.legend {
    display: flex;
    gap: 32px;
    margin-top: 24px;
}

.legend-item {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 13px;
}

.legend-color {
    width: 20px;
    height: 20px;
    border-radius: 4px;
}

.legend-color.covered {
    background: var(--covered-bg);
    border-left: 4px solid var(--covered);
}

.legend-color.not-covered {
    background: var(--not-covered-bg);
    border-left: 4px solid var(--not-covered);
}

.file-view {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
}

.file-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 24px;
    background: var(--bg-secondary);
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
}

.file-path {
    font-family: 'Roboto Mono', monospace;
    font-size: 13px;
    color: var(--text-primary);
    font-weight: 500;
}

.file-stats {
    display: flex;
    align-items: center;
    gap: 16px;
}

.coverage-badge {
    font-size: 12px;
    font-weight: 500;
    padding: 6px 14px;
    border-radius: 16px;
}

.coverage-badge.high { background: rgba(0, 200, 83, 0.15); color: var(--high); }
.coverage-badge.medium { background: rgba(255, 171, 0, 0.15); color: var(--medium); }
.coverage-badge.low { background: rgba(255, 82, 82, 0.15); color: var(--low); }
.coverage-badge.none { background: rgba(107, 114, 128, 0.15); color: var(--none); }

.stmt-count {
    font-size: 12px;
    color: var(--text-secondary);
}

.code-container {
    flex: 1;
    overflow: auto;
    background: var(--bg-tertiary);
}

.code {
    width: 100%;
    border-collapse: collapse;
    font-family: 'Roboto Mono', monospace;
    font-size: 13px;
    line-height: 1.6;
}

.code tr.covered {
    background: var(--covered-bg);
}

.code tr.not-covered {
    background: var(--not-covered-bg);
}

.code tr.covered td.line-num {
    border-left: 4px solid var(--covered);
}

.code tr.not-covered td.line-num {
    border-left: 4px solid var(--not-covered);
}

.code td {
    padding: 0;
    vertical-align: top;
}

.line-num {
    width: 60px;
    padding: 0 16px;
    text-align: right;
    color: var(--text-muted);
    background: var(--bg-secondary);
    user-select: none;
    position: sticky;
    left: 0;
    border-left: 4px solid transparent;
    font-size: 12px;
}

.line-content {
    padding: 0 20px;
    white-space: pre;
}

.line-content pre {
    margin: 0;
    font-family: inherit;
    font-size: inherit;
}

/* Scrollbar styling */
::-webkit-scrollbar {
    width: 8px;
    height: 8px;
}

::-webkit-scrollbar-track {
    background: var(--bg-secondary);
}

::-webkit-scrollbar-thumb {
    background: var(--bg-hover);
    border-radius: 4px;
}

::-webkit-scrollbar-thumb:hover {
    background: var(--text-muted);
}
`

const cssLight = `
* {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
}

:root {
    --md-primary-fg-color: #4051b5;
    --md-primary-bg-color: #fff;
    --md-accent-fg-color: #526cfe;
    --bg-primary: #ffffff;
    --bg-secondary: #f8f9fa;
    --bg-tertiary: #f1f3f4;
    --bg-hover: #e8eaed;
    --bg-active: #4051b5;
    --text-primary: #202124;
    --text-secondary: #5f6368;
    --text-muted: #9aa0a6;
    --border: #dadce0;
    --accent: #4051b5;
    --covered: #1e8e3e;
    --covered-bg: rgba(30, 142, 62, 0.08);
    --not-covered: #d93025;
    --not-covered-bg: rgba(217, 48, 37, 0.08);
    --high: #1e8e3e;
    --medium: #f9ab00;
    --low: #d93025;
    --none: #9aa0a6;
    --shadow: 0 1px 3px rgba(0,0,0,0.1), 0 2px 6px rgba(0,0,0,0.05);
    --shadow-sm: 0 1px 2px rgba(0,0,0,0.08);
}

body {
    font-family: 'Roboto', -apple-system, BlinkMacSystemFont, sans-serif;
    background: var(--bg-primary);
    color: var(--text-primary);
    height: 100vh;
    overflow: hidden;
    font-size: 14px;
    line-height: 1.6;
    -webkit-font-smoothing: antialiased;
}

.app {
    display: flex;
    height: 100vh;
}

.sidebar {
    width: 300px;
    background: var(--bg-secondary);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
    box-shadow: var(--shadow);
    z-index: 10;
}

.sidebar-header {
    padding: 20px;
    border-bottom: 1px solid var(--border);
    background: linear-gradient(135deg, var(--md-primary-fg-color), var(--md-accent-fg-color));
}

.sidebar-header h1 {
    font-size: 16px;
    font-weight: 500;
    color: #fff;
    margin-bottom: 12px;
    letter-spacing: 0.02em;
}

.total-coverage {
    font-size: 36px;
    font-weight: 700;
    margin-bottom: 4px;
}

.total-coverage.high { color: #81c995; }
.total-coverage.medium { color: #fdd663; }
.total-coverage.low { color: #f28b82; }
.total-coverage.none { color: rgba(255,255,255,0.5); }

.stats {
    font-size: 12px;
    color: rgba(255,255,255,0.85);
    font-weight: 400;
}

.search-container {
    position: relative;
    margin-top: 16px;
}

.search-input {
    width: 100%;
    padding: 10px 36px 10px 14px;
    font-size: 14px;
    font-family: inherit;
    background: rgba(255,255,255,0.2);
    border: none;
    border-radius: 8px;
    color: #fff;
    outline: none;
    transition: background 0.2s ease;
}

.search-input:focus {
    background: rgba(255,255,255,0.25);
}

.search-input::placeholder {
    color: rgba(255,255,255,0.6);
}

.search-icon {
    position: absolute;
    right: 12px;
    top: 50%;
    transform: translateY(-50%);
    font-size: 14px;
    pointer-events: none;
    opacity: 0.7;
}

.search-clear {
    position: absolute;
    right: 12px;
    top: 50%;
    transform: translateY(-50%);
    font-size: 14px;
    cursor: pointer;
    color: rgba(255,255,255,0.7);
    display: none;
}

.search-clear:hover {
    color: #fff;
}

.search-container.has-value .search-icon {
    display: none;
}

.search-container.has-value .search-clear {
    display: block;
}

.tree-file.search-hidden,
.tree-dir.search-hidden {
    display: none;
}

.tree-file.search-match .tree-name {
    color: var(--accent);
    font-weight: 500;
}

.tree {
    flex: 1;
    overflow-y: auto;
    padding: 12px 0;
}

.tree-dir, .tree-file {
    user-select: none;
}

.tree-dir-header, .tree-file {
    display: flex;
    align-items: center;
    padding: 8px 16px;
    cursor: pointer;
    font-size: 13px;
    gap: 8px;
    transition: background 0.15s ease;
    margin: 0 8px;
    border-radius: 4px;
}

.tree-dir-header:hover, .tree-file:hover {
    background: var(--bg-hover);
}

.tree-file.active {
    background: var(--bg-active);
    color: #fff;
}

.tree-icon {
    font-size: 16px;
    flex-shrink: 0;
    opacity: 0.8;
}

.tree-name {
    flex: 1;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    font-weight: 400;
}

.tree-percent {
    font-size: 11px;
    font-weight: 500;
    padding: 3px 8px;
    border-radius: 12px;
    flex-shrink: 0;
}

.tree-percent.high { background: rgba(30, 142, 62, 0.1); color: var(--high); }
.tree-percent.medium { background: rgba(249, 171, 0, 0.15); color: #b06000; }
.tree-percent.low { background: rgba(217, 48, 37, 0.1); color: var(--low); }
.tree-percent.none { background: rgba(154, 160, 166, 0.15); color: var(--none); }

.tree-children {
    padding-left: 20px;
}

.tree-dir:not(.expanded) > .tree-children {
    display: none;
}

.content {
    flex: 1;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    background: var(--bg-primary);
}

.welcome {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    color: var(--text-secondary);
    gap: 20px;
}

.welcome-icon {
    font-size: 72px;
    opacity: 0.8;
}

.welcome h2 {
    font-size: 24px;
    font-weight: 500;
    color: var(--text-primary);
}

.welcome p {
    font-size: 14px;
    max-width: 300px;
    text-align: center;
}

.legend {
    display: flex;
    gap: 32px;
    margin-top: 24px;
}

.legend-item {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 13px;
}

.legend-color {
    width: 20px;
    height: 20px;
    border-radius: 4px;
}

.legend-color.covered {
    background: var(--covered-bg);
    border-left: 4px solid var(--covered);
}

.legend-color.not-covered {
    background: var(--not-covered-bg);
    border-left: 4px solid var(--not-covered);
}

.file-view {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
}

.file-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 24px;
    background: var(--bg-secondary);
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
}

.file-path {
    font-family: 'Roboto Mono', monospace;
    font-size: 13px;
    color: var(--text-primary);
    font-weight: 500;
}

.file-stats {
    display: flex;
    align-items: center;
    gap: 16px;
}

.coverage-badge {
    font-size: 12px;
    font-weight: 500;
    padding: 6px 14px;
    border-radius: 16px;
}

.coverage-badge.high { background: rgba(30, 142, 62, 0.1); color: var(--high); }
.coverage-badge.medium { background: rgba(249, 171, 0, 0.15); color: #b06000; }
.coverage-badge.low { background: rgba(217, 48, 37, 0.1); color: var(--low); }
.coverage-badge.none { background: rgba(154, 160, 166, 0.15); color: var(--none); }

.stmt-count {
    font-size: 12px;
    color: var(--text-secondary);
}

.code-container {
    flex: 1;
    overflow: auto;
    background: var(--bg-tertiary);
}

.code {
    width: 100%;
    border-collapse: collapse;
    font-family: 'Roboto Mono', monospace;
    font-size: 13px;
    line-height: 1.6;
}

.code tr.covered {
    background: var(--covered-bg);
}

.code tr.not-covered {
    background: var(--not-covered-bg);
}

.code tr.covered td.line-num {
    border-left: 4px solid var(--covered);
}

.code tr.not-covered td.line-num {
    border-left: 4px solid var(--not-covered);
}

.code td {
    padding: 0;
    vertical-align: top;
}

.line-num {
    width: 60px;
    padding: 0 16px;
    text-align: right;
    color: var(--text-muted);
    background: var(--bg-secondary);
    user-select: none;
    position: sticky;
    left: 0;
    border-left: 4px solid transparent;
    font-size: 12px;
}

.line-content {
    padding: 0 20px;
    white-space: pre;
}

.line-content pre {
    margin: 0;
    font-family: inherit;
    font-size: inherit;
}

/* Scrollbar styling */
::-webkit-scrollbar {
    width: 8px;
    height: 8px;
}

::-webkit-scrollbar-track {
    background: var(--bg-secondary);
}

::-webkit-scrollbar-thumb {
    background: var(--border);
    border-radius: 4px;
}

::-webkit-scrollbar-thumb:hover {
    background: var(--text-muted);
}
`
