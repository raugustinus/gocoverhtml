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
    <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500&family=Inter:wght@400;500;600&display=swap" rel="stylesheet">
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
    --bg-primary: #1e1e1e;
    --bg-secondary: #252526;
    --bg-tertiary: #2d2d2d;
    --bg-hover: #37373d;
    --bg-active: #094771;
    --text-primary: #cccccc;
    --text-secondary: #858585;
    --text-muted: #6e6e6e;
    --border: #3c3c3c;
    --accent: #0078d4;
    --covered: #2ea043;
    --covered-bg: rgba(46, 160, 67, 0.15);
    --not-covered: #f85149;
    --not-covered-bg: rgba(248, 81, 73, 0.15);
    --high: #2ea043;
    --medium: #d29922;
    --low: #f85149;
    --none: #6e6e6e;
}

body {
    font-family: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif;
    background: var(--bg-primary);
    color: var(--text-primary);
    height: 100vh;
    overflow: hidden;
}

.app {
    display: flex;
    height: 100vh;
}

.sidebar {
    width: 320px;
    background: var(--bg-secondary);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
}

.sidebar-header {
    padding: 16px;
    border-bottom: 1px solid var(--border);
}

.sidebar-header h1 {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-primary);
    margin-bottom: 8px;
}

.total-coverage {
    font-size: 28px;
    font-weight: 600;
    margin-bottom: 4px;
}

.total-coverage.high { color: var(--high); }
.total-coverage.medium { color: var(--medium); }
.total-coverage.low { color: var(--low); }
.total-coverage.none { color: var(--none); }

.stats {
    font-size: 12px;
    color: var(--text-secondary);
}

.search-container {
    position: relative;
    margin-top: 12px;
}

.search-input {
    width: 100%;
    padding: 8px 32px 8px 12px;
    font-size: 13px;
    font-family: inherit;
    background: var(--bg-primary);
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--text-primary);
    outline: none;
}

.search-input:focus {
    border-color: var(--accent);
}

.search-input::placeholder {
    color: var(--text-muted);
}

.search-icon {
    position: absolute;
    right: 10px;
    top: 50%;
    transform: translateY(-50%);
    font-size: 12px;
    pointer-events: none;
}

.search-clear {
    position: absolute;
    right: 10px;
    top: 50%;
    transform: translateY(-50%);
    font-size: 14px;
    cursor: pointer;
    color: var(--text-muted);
    display: none;
}

.search-clear:hover {
    color: var(--text-primary);
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
    padding: 8px 0;
}

.tree-dir, .tree-file {
    user-select: none;
}

.tree-dir-header, .tree-file {
    display: flex;
    align-items: center;
    padding: 4px 12px;
    cursor: pointer;
    font-size: 13px;
    gap: 6px;
}

.tree-dir-header:hover, .tree-file:hover {
    background: var(--bg-hover);
}

.tree-file.active {
    background: var(--bg-active);
}

.tree-icon {
    font-size: 14px;
    flex-shrink: 0;
}

.tree-name {
    flex: 1;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
}

.tree-percent {
    font-size: 11px;
    font-weight: 500;
    padding: 2px 6px;
    border-radius: 3px;
    flex-shrink: 0;
}

.tree-percent.high { background: rgba(46, 160, 67, 0.2); color: var(--high); }
.tree-percent.medium { background: rgba(210, 153, 34, 0.2); color: var(--medium); }
.tree-percent.low { background: rgba(248, 81, 73, 0.2); color: var(--low); }
.tree-percent.none { background: rgba(110, 110, 110, 0.2); color: var(--none); }

.tree-children {
    padding-left: 16px;
}

.tree-dir:not(.expanded) > .tree-children {
    display: none;
}

.content {
    flex: 1;
    overflow: hidden;
    display: flex;
    flex-direction: column;
}

.welcome {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    color: var(--text-secondary);
    gap: 16px;
}

.welcome-icon {
    font-size: 64px;
}

.welcome h2 {
    font-size: 20px;
    font-weight: 500;
    color: var(--text-primary);
}

.welcome p {
    font-size: 14px;
}

.legend {
    display: flex;
    gap: 24px;
    margin-top: 16px;
}

.legend-item {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
}

.legend-color {
    width: 16px;
    height: 16px;
    border-radius: 3px;
}

.legend-color.covered {
    background: var(--covered-bg);
    border-left: 3px solid var(--covered);
}

.legend-color.not-covered {
    background: var(--not-covered-bg);
    border-left: 3px solid var(--not-covered);
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
    padding: 12px 16px;
    background: var(--bg-secondary);
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
}

.file-path {
    font-family: 'JetBrains Mono', monospace;
    font-size: 13px;
    color: var(--text-primary);
}

.file-stats {
    display: flex;
    align-items: center;
    gap: 12px;
}

.coverage-badge {
    font-size: 12px;
    font-weight: 600;
    padding: 4px 10px;
    border-radius: 4px;
}

.coverage-badge.high { background: rgba(46, 160, 67, 0.2); color: var(--high); }
.coverage-badge.medium { background: rgba(210, 153, 34, 0.2); color: var(--medium); }
.coverage-badge.low { background: rgba(248, 81, 73, 0.2); color: var(--low); }
.coverage-badge.none { background: rgba(110, 110, 110, 0.2); color: var(--none); }

.stmt-count {
    font-size: 12px;
    color: var(--text-secondary);
}

.code-container {
    flex: 1;
    overflow: auto;
    background: var(--bg-primary);
}

.code {
    width: 100%;
    border-collapse: collapse;
    font-family: 'JetBrains Mono', monospace;
    font-size: 13px;
    line-height: 1.5;
}

.code tr.covered {
    background: var(--covered-bg);
}

.code tr.not-covered {
    background: var(--not-covered-bg);
}

.code tr.covered td.line-num {
    border-left: 3px solid var(--covered);
}

.code tr.not-covered td.line-num {
    border-left: 3px solid var(--not-covered);
}

.code td {
    padding: 0;
    vertical-align: top;
}

.line-num {
    width: 50px;
    padding: 0 12px;
    text-align: right;
    color: var(--text-muted);
    background: var(--bg-secondary);
    user-select: none;
    position: sticky;
    left: 0;
    border-left: 3px solid transparent;
}

.line-content {
    padding: 0 16px;
    white-space: pre;
}

.line-content pre {
    margin: 0;
    font-family: inherit;
    font-size: inherit;
}

/* Scrollbar styling */
::-webkit-scrollbar {
    width: 10px;
    height: 10px;
}

::-webkit-scrollbar-track {
    background: var(--bg-primary);
}

::-webkit-scrollbar-thumb {
    background: var(--bg-tertiary);
    border-radius: 5px;
}

::-webkit-scrollbar-thumb:hover {
    background: var(--bg-hover);
}
`

const cssLight = `
* {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
}

:root {
    --bg-primary: #ffffff;
    --bg-secondary: #f5f5f5;
    --bg-tertiary: #ebebeb;
    --bg-hover: #e8e8e8;
    --bg-active: #cce5ff;
    --text-primary: #24292f;
    --text-secondary: #57606a;
    --text-muted: #8b949e;
    --border: #d8dee4;
    --accent: #0969da;
    --covered: #1a7f37;
    --covered-bg: rgba(26, 127, 55, 0.1);
    --not-covered: #cf222e;
    --not-covered-bg: rgba(207, 34, 46, 0.1);
    --high: #1a7f37;
    --medium: #9a6700;
    --low: #cf222e;
    --none: #8b949e;
}

body {
    font-family: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif;
    background: var(--bg-primary);
    color: var(--text-primary);
    height: 100vh;
    overflow: hidden;
}

.app {
    display: flex;
    height: 100vh;
}

.sidebar {
    width: 320px;
    background: var(--bg-secondary);
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
}

.sidebar-header {
    padding: 16px;
    border-bottom: 1px solid var(--border);
}

.sidebar-header h1 {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-primary);
    margin-bottom: 8px;
}

.total-coverage {
    font-size: 28px;
    font-weight: 600;
    margin-bottom: 4px;
}

.total-coverage.high { color: var(--high); }
.total-coverage.medium { color: var(--medium); }
.total-coverage.low { color: var(--low); }
.total-coverage.none { color: var(--none); }

.stats {
    font-size: 12px;
    color: var(--text-secondary);
}

.search-container {
    position: relative;
    margin-top: 12px;
}

.search-input {
    width: 100%;
    padding: 8px 32px 8px 12px;
    font-size: 13px;
    font-family: inherit;
    background: var(--bg-primary);
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--text-primary);
    outline: none;
}

.search-input:focus {
    border-color: var(--accent);
}

.search-input::placeholder {
    color: var(--text-muted);
}

.search-icon {
    position: absolute;
    right: 10px;
    top: 50%;
    transform: translateY(-50%);
    font-size: 12px;
    pointer-events: none;
}

.search-clear {
    position: absolute;
    right: 10px;
    top: 50%;
    transform: translateY(-50%);
    font-size: 14px;
    cursor: pointer;
    color: var(--text-muted);
    display: none;
}

.search-clear:hover {
    color: var(--text-primary);
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
    padding: 8px 0;
}

.tree-dir, .tree-file {
    user-select: none;
}

.tree-dir-header, .tree-file {
    display: flex;
    align-items: center;
    padding: 4px 12px;
    cursor: pointer;
    font-size: 13px;
    gap: 6px;
}

.tree-dir-header:hover, .tree-file:hover {
    background: var(--bg-hover);
}

.tree-file.active {
    background: var(--bg-active);
}

.tree-icon {
    font-size: 14px;
    flex-shrink: 0;
}

.tree-name {
    flex: 1;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
}

.tree-percent {
    font-size: 11px;
    font-weight: 500;
    padding: 2px 6px;
    border-radius: 3px;
    flex-shrink: 0;
}

.tree-percent.high { background: rgba(26, 127, 55, 0.15); color: var(--high); }
.tree-percent.medium { background: rgba(154, 103, 0, 0.15); color: var(--medium); }
.tree-percent.low { background: rgba(207, 34, 46, 0.15); color: var(--low); }
.tree-percent.none { background: rgba(139, 148, 158, 0.15); color: var(--none); }

.tree-children {
    padding-left: 16px;
}

.tree-dir:not(.expanded) > .tree-children {
    display: none;
}

.content {
    flex: 1;
    overflow: hidden;
    display: flex;
    flex-direction: column;
}

.welcome {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    color: var(--text-secondary);
    gap: 16px;
}

.welcome-icon {
    font-size: 64px;
}

.welcome h2 {
    font-size: 20px;
    font-weight: 500;
    color: var(--text-primary);
}

.welcome p {
    font-size: 14px;
}

.legend {
    display: flex;
    gap: 24px;
    margin-top: 16px;
}

.legend-item {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
}

.legend-color {
    width: 16px;
    height: 16px;
    border-radius: 3px;
}

.legend-color.covered {
    background: var(--covered-bg);
    border-left: 3px solid var(--covered);
}

.legend-color.not-covered {
    background: var(--not-covered-bg);
    border-left: 3px solid var(--not-covered);
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
    padding: 12px 16px;
    background: var(--bg-secondary);
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
}

.file-path {
    font-family: 'JetBrains Mono', monospace;
    font-size: 13px;
    color: var(--text-primary);
}

.file-stats {
    display: flex;
    align-items: center;
    gap: 12px;
}

.coverage-badge {
    font-size: 12px;
    font-weight: 600;
    padding: 4px 10px;
    border-radius: 4px;
}

.coverage-badge.high { background: rgba(26, 127, 55, 0.15); color: var(--high); }
.coverage-badge.medium { background: rgba(154, 103, 0, 0.15); color: var(--medium); }
.coverage-badge.low { background: rgba(207, 34, 46, 0.15); color: var(--low); }
.coverage-badge.none { background: rgba(139, 148, 158, 0.15); color: var(--none); }

.stmt-count {
    font-size: 12px;
    color: var(--text-secondary);
}

.code-container {
    flex: 1;
    overflow: auto;
    background: var(--bg-primary);
}

.code {
    width: 100%;
    border-collapse: collapse;
    font-family: 'JetBrains Mono', monospace;
    font-size: 13px;
    line-height: 1.5;
}

.code tr.covered {
    background: var(--covered-bg);
}

.code tr.not-covered {
    background: var(--not-covered-bg);
}

.code tr.covered td.line-num {
    border-left: 3px solid var(--covered);
}

.code tr.not-covered td.line-num {
    border-left: 3px solid var(--not-covered);
}

.code td {
    padding: 0;
    vertical-align: top;
}

.line-num {
    width: 50px;
    padding: 0 12px;
    text-align: right;
    color: var(--text-muted);
    background: var(--bg-secondary);
    user-select: none;
    position: sticky;
    left: 0;
    border-left: 3px solid transparent;
}

.line-content {
    padding: 0 16px;
    white-space: pre;
}

.line-content pre {
    margin: 0;
    font-family: inherit;
    font-size: inherit;
}

/* Scrollbar styling */
::-webkit-scrollbar {
    width: 10px;
    height: 10px;
}

::-webkit-scrollbar-track {
    background: var(--bg-secondary);
}

::-webkit-scrollbar-thumb {
    background: var(--bg-tertiary);
    border-radius: 5px;
}

::-webkit-scrollbar-thumb:hover {
    background: var(--bg-hover);
}
`
