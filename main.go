// gocoverhtml generates beautiful HTML coverage reports from Go coverage profiles.
//
// Usage:
//
//	go test -coverprofile=coverage.out ./...
//	gocoverhtml -i coverage.out -o coverage/index.html
package main

import (
	"bufio"
	"bytes"
	"embed"
	"flag"
	"fmt"
	"html"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

//go:embed templates/*
var templatesFS embed.FS

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
	htmlContent, err := generateHTML(tree, files, title, theme)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating HTML: %v\n", err)
		os.Exit(1)
	}

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
	LineNum    int
	Content    string
	Covered    bool
	NotCovered bool
	Partial    bool
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

// Template data structures
type TemplateData struct {
	Title              string
	CSS                template.CSS
	JS                 template.JS
	TotalPercent       float64
	TotalCovered       int
	TotalStatements    int
	TotalCoverageClass string
	Tree               *TemplateTreeNode
	Files              map[string]*TemplateFile
}

type TemplateTreeNode struct {
	Name          string
	Path          string
	ID            string
	IsFile        bool
	HasChildren   bool
	Children      []*TemplateTreeNode
	Percent       float64
	CoverageClass string
}

type TemplateFile struct {
	ID            string
	CleanPath     string
	Percent       float64
	Statements    int
	Covered       int
	CoverageClass string
	Lines         []TemplateLine
}

type TemplateLine struct {
	LineNum int
	Content string
	Class   string
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

func generateHTML(tree *TreeNode, files map[string]*FileCoverage, title, theme string) (string, error) {
	// Load templates
	tmpl, err := template.ParseFS(templatesFS, "templates/index.html.tmpl")
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	// Load CSS
	cssFile := "templates/dark.css"
	if theme == "light" {
		cssFile = "templates/light.css"
	}
	cssBytes, err := templatesFS.ReadFile(cssFile)
	if err != nil {
		return "", fmt.Errorf("reading CSS: %w", err)
	}

	// Load JS
	jsBytes, err := templatesFS.ReadFile("templates/app.js")
	if err != nil {
		return "", fmt.Errorf("reading JS: %w", err)
	}

	// Convert tree to template tree
	templateTree := convertTreeNode(tree)

	// Convert files to template files
	templateFiles := make(map[string]*TemplateFile)
	for path, fc := range files {
		cleanPath := strings.TrimPrefix(path, "insights/")
		fileID := pathToID(cleanPath)

		lines := make([]TemplateLine, len(fc.Lines))
		for i, line := range fc.Lines {
			class := ""
			if line.Covered {
				class = "covered"
			} else if line.NotCovered {
				class = "not-covered"
			}
			lines[i] = TemplateLine{
				LineNum: line.LineNum,
				Content: html.EscapeString(line.Content),
				Class:   class,
			}
		}

		templateFiles[path] = &TemplateFile{
			ID:            fileID,
			CleanPath:     cleanPath,
			Percent:       fc.Percent,
			Statements:    fc.Statements,
			Covered:       fc.Covered,
			CoverageClass: getCoverageClass(fc.Percent),
			Lines:         lines,
		}
	}

	// Prepare template data
	data := TemplateData{
		Title:              title,
		CSS:                template.CSS(cssBytes),
		JS:                 template.JS(jsBytes),
		TotalPercent:       tree.Percent,
		TotalCovered:       tree.Covered,
		TotalStatements:    tree.Statements,
		TotalCoverageClass: getCoverageClass(tree.Percent),
		Tree:               templateTree,
		Files:              templateFiles,
	}

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

func convertTreeNode(node *TreeNode) *TemplateTreeNode {
	templateNode := &TemplateTreeNode{
		Name:          node.Name,
		Path:          node.Path,
		ID:            pathToID(node.Path),
		IsFile:        node.IsFile,
		HasChildren:   len(node.Children) > 0,
		Percent:       node.Percent,
		CoverageClass: getCoverageClass(node.Percent),
	}

	for _, child := range node.Children {
		templateNode.Children = append(templateNode.Children, convertTreeNode(child))
	}

	return templateNode
}

func pathToID(path string) string {
	// Convert path to valid HTML ID
	return strings.ReplaceAll(strings.ReplaceAll(path, "/", "-"), ".", "-")
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
