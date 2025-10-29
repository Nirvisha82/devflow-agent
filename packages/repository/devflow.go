package repository

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"devflow-agent/packages/ai"
	"devflow-agent/packages/config"
)

// DevflowFileInfo represents a file with enhanced metadata for Devflow analysis
type DevflowFileInfo struct {
	Path         string
	RelativePath string
	Size         int64
	Language     string
	Functions    []FunctionInfo
	Classes      []ClassInfo
	Imports      []string
	Exports      []string
	Purpose      string
	Role         string
}

// FunctionInfo represents a function within a file
type FunctionInfo struct {
	Name       string
	Signature  string
	Purpose    string
	Parameters []string
	ReturnType string
	LineNumber int
}

// ClassInfo represents a class within a file
type ClassInfo struct {
	Name       string
	Purpose    string
	Methods    []FunctionInfo
	Properties []string
	LineNumber int
}

// DependencyNode represents a node in the dependency graph
type DependencyNode struct {
	File         string   `json:"file"`
	Language     string   `json:"language"`
	Dependencies []string `json:"dependencies"`
	Exports      []string `json:"exports"`
	Imports      []string `json:"imports"`
}

// DependencyGraph represents the complete dependency graph
type DependencyGraph struct {
	Nodes       []DependencyNode `json:"nodes"`
	GeneratedAt time.Time        `json:"generated_at"`
	RepoURL     string           `json:"repo_url"`
}

// CreateDirectory creates a directory if it doesn't exist
func CreateDirectory(path string) error {
	return os.MkdirAll(path, 0755)
}

// GenerateRepoStructure creates a clean repository structure markdown file
func GenerateRepoStructure(repoPath, repoURL, outputFile string) error {
	slog.Info("Generating repository structure", "output", outputFile)

	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create structure file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	// Write header
	repoName := filepath.Base(strings.TrimSuffix(repoURL, ".git"))
	header := fmt.Sprintf(`# Repository Structure: %s

This document provides a comprehensive overview of the repository structure and organization.

**Repository URL:** %s  
**Generated:** %s  
**Purpose:** This file serves as a quick reference for understanding the codebase layout and organization.

## Directory Structure

`, repoName, repoURL, time.Now().Format("2006-01-02 15:04:05"))

	writer.WriteString(header)

	// Build directory structure
	allPaths := make(map[string]bool)

	// Walk through all files and directories
	err = filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		relPath, _ := filepath.Rel(repoPath, path)
		if relPath == "." {
			return nil
		}

		// Normalize path separators
		relPath = strings.ReplaceAll(relPath, "\\", "/")

		// Skip .devflow directory and other ignored patterns
		if shouldIgnoreForStructure(relPath, d.Name()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		// Add to paths
		allPaths[relPath] = !d.IsDir()
		return nil
	})

	if err != nil {
		return err
	}

	// Convert to sorted slice
	var paths []string
	for path := range allPaths {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	// Write directory structure
	writer.WriteString("```\n")
	for _, path := range paths {
		isFile := allPaths[path]
		depth := strings.Count(path, "/")
		indent := strings.Repeat("  ", depth)
		name := filepath.Base(path)

		if isFile {
			writer.WriteString(fmt.Sprintf("%s%s\n", indent, name))
		} else {
			writer.WriteString(fmt.Sprintf("%s%s/\n", indent, name))
		}
	}
	writer.WriteString("```\n\n")

	// Add file statistics
	fileCount := 0
	dirCount := 0
	for _, isFile := range allPaths {
		if isFile {
			fileCount++
		} else {
			dirCount++
		}
	}

	stats := fmt.Sprintf(`## Statistics

- **Total Directories:** %d
- **Total Files:** %d
- **Repository Size:** %s

## Key Directories

`, dirCount, fileCount, getRepoSize(repoPath))

	writer.WriteString(stats)

	// Identify key directories
	keyDirs := identifyKeyDirectories(allPaths)
	for _, dir := range keyDirs {
		writer.WriteString(fmt.Sprintf("- **%s/**: %s\n", dir.Name, dir.Description))
	}

	return nil
}

// GenerateRepoAnalysis creates an LLM-generated analysis of the repository
func GenerateRepoAnalysis(repoPath, repoURL, outputFile string) error {
	slog.Info("Generating repository analysis", "output", outputFile)

	// First, analyze all files to extract metadata
	files, err := analyzeFilesForDevflow(repoPath)
	if err != nil {
		return fmt.Errorf("failed to analyze files: %w", err)
	}

	// Convert to AI package types
	aiFiles := make([]ai.DevflowFileInfo, len(files))
	for i, file := range files {
		aiFiles[i] = ai.DevflowFileInfo{
			Path:         file.Path,
			RelativePath: file.RelativePath,
			Size:         file.Size,
			Language:     file.Language,
			Functions:    convertFunctions(file.Functions),
			Classes:      convertClasses(file.Classes),
			Imports:      file.Imports,
			Exports:      file.Exports,
			Purpose:      file.Purpose,
			Role:         file.Role,
		}
	}

	// Generate AI analysis
	analysis := &ai.RepoAnalysis{
		RepoURL: repoURL,
		Files:   aiFiles,
	}

	result, err := ai.AnalyzeRepositoryWithAI(analysis)
	if err != nil {
		return fmt.Errorf("failed to generate AI analysis: %w", err)
	}

	// Save the analysis
	return os.WriteFile(outputFile, []byte(result.MarkdownContent), 0644)
}

// GenerateDependencyGraph creates a dependency graph for the repository
func GenerateDependencyGraph(repoPath, outputFile string) error {
	slog.Info("Generating dependency graph", "output", outputFile)

	nodes, err := buildDependencyGraph(repoPath)
	if err != nil {
		return fmt.Errorf("failed to build dependency graph: %w", err)
	}

	graph := DependencyGraph{
		Nodes:       nodes,
		GeneratedAt: time.Now(),
		RepoURL:     "", // Will be set by caller if needed
	}

	jsonData, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal dependency graph: %w", err)
	}

	return os.WriteFile(outputFile, jsonData, 0644)
}

// SaveFileMetadata saves the extracted file metadata as JSON
func SaveFileMetadata(repoPath, outputFile string) error {
	slog.Info("Saving file metadata", "output", outputFile)

	files, err := analyzeFilesForDevflow(repoPath)
	if err != nil {
		return fmt.Errorf("failed to analyze files for metadata: %w", err)
	}

	jsonData, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal file metadata: %w", err)
	}

	return os.WriteFile(outputFile, jsonData, 0644)
}

// SaveAnalysisPrompt saves the prompt that would be sent to the LLM (simplified approach)
func SaveAnalysisPrompt(repoPath, repoURL, structureFile, outputFile string) error {
	slog.Info("Saving analysis prompt", "output", outputFile)

	// Read the repo-structure.md file (created by RepoAnalyzer)
	structureContent, err := os.ReadFile(structureFile)
	if err != nil {
		return fmt.Errorf("failed to read repo structure file: %w", err)
	}

	// Create the LLM prompt using the repo structure content
	prompt := fmt.Sprintf(`You are an expert code analyst. Analyze the following repository and provide comprehensive insights about the codebase.

# Repository Information
**Repository URL:** %s

# Repository Structure and Code Analysis
%s

# Your Task
Provide a comprehensive analysis in markdown format that includes:

## Repository Overview
1. **Project Type**: What kind of project is this? (web app, CLI tool, library, etc.)
2. **Architecture**: Describe the overall architecture and structure
3. **Technology Stack**: Identify the main technologies and frameworks used
4. **Entry Points**: Identify the main entry points and how the application starts

## File Analysis
For each important file, provide:
1. **Purpose**: What is this file's primary purpose?
2. **Role**: How does it fit into the larger system?
3. **Key Functions/Classes**: Brief description of main functions/classes and their logic
4. **Dependencies**: What other files/modules does it depend on?
5. **Business Logic**: What business rules or logic does it implement?

## System Relationships
1. **Data Flow**: How does data flow through the system?
2. **Key Components**: What are the most important components?
3. **Integration Points**: Where do different parts of the system connect?
4. **API/Interface Design**: How do components communicate?

## Development Insights
1. **Code Quality**: Overall assessment of code organization and patterns
2. **Design Patterns**: What design patterns are used?
3. **Potential Issues**: Any obvious problems or areas for improvement?
4. **Scalability**: How well would this scale?
5. **Maintainability**: How easy would this be to maintain and extend?

Format your response in clean markdown with appropriate headers and code blocks. Be specific and detailed in your analysis, referencing actual code when relevant.`, repoURL, string(structureContent))

	// Add header to make it clear this is the LLM input
	promptWithHeader := fmt.Sprintf(`# LLM Analysis Prompt

This file contains the exact prompt that would be sent to the LLM for repository analysis.

---

%s`, prompt)

	return os.WriteFile(outputFile, []byte(promptWithHeader), 0644)
}

// GenerateRepoAnalysisWithLLM generates AI analysis using the repo structure content
func GenerateRepoAnalysisWithLLM(repoPath, repoURL, structureFile, outputFile string) error {
	slog.Info("Generating LLM analysis", "output", outputFile)

	// Read the repo-structure.md file (created by RepoAnalyzer)
	structureContent, err := os.ReadFile(structureFile)
	if err != nil {
		return fmt.Errorf("failed to read repo structure file: %w", err)
	}

	// Create the analysis request
	analysis := &ai.RepoAnalysisFromStructure{
		RepoURL:          repoURL,
		StructureContent: string(structureContent),
	}

	// Generate AI analysis
	result, err := ai.AnalyzeRepositoryFromStructure(analysis)
	if err != nil {
		return fmt.Errorf("failed to generate AI analysis: %w", err)
	}

	return os.WriteFile(outputFile, []byte(result.MarkdownContent), 0644)
}

// CreateDevflowReadme creates a README file for the .devflow directory
func CreateDevflowReadme(outputFile, repoName string) error {
	slog.Info("Creating Devflow README", "output", outputFile)

	readme := fmt.Sprintf(`# Devflow Knowledge Base

This directory contains the Devflow knowledge base for **%s**.

## Files

- **repo-structure.md**: Flattened repository structure with complete file contents and AST analysis
- **file-metadata.json**: Extracted metadata (functions, classes, imports) from all source files
- **repo-analysis-prompt.md**: The exact prompt that would be sent to the LLM for analysis
- **dependency-graph.json**: Dependency relationships between files
- **repo-analysis.md**: AI-generated analysis (created when LLM analysis is enabled)
- **README.md**: This file

## Purpose

The Devflow knowledge base provides a comprehensive understanding of the repository that can be efficiently queried without re-analyzing the entire codebase each time. This enables faster and more accurate issue-to-PR workflows.

## Usage

These files are automatically generated and maintained by the Devflow agent. They should not be manually edited as they will be regenerated during repository updates.

## Generated

%s

---

*This knowledge base was generated by Devflow Agent*
`, repoName, time.Now().Format("2006-01-02 15:04:05"))

	return os.WriteFile(outputFile, []byte(readme), 0644)
}

// Helper functions

func shouldIgnoreForStructure(relPath, name string) bool {
	cfg := config.GetConfig()
	// Ignore .devflow directory
	if strings.HasPrefix(relPath, cfg.Repository.DevflowDirectory+"/") {
		return true
	}

	// Use existing ignore logic
	analyzer := &RepoAnalyzer{}
	return analyzer.shouldIgnoreDirectory(relPath, name) || analyzer.shouldIgnoreFile(relPath, name)
}

func getRepoSize(repoPath string) string {
	var size int64
	filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				size += info.Size()
			}
		}
		return nil
	})

	// Convert to human readable format
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

type KeyDirectory struct {
	Name        string
	Description string
}

func identifyKeyDirectories(allPaths map[string]bool) []KeyDirectory {
	keyDirs := []KeyDirectory{}

	// Common patterns
	patterns := map[string]string{
		"src":         "Source code directory",
		"lib":         "Library code",
		"app":         "Application code",
		"components":  "UI components",
		"pages":       "Application pages",
		"utils":       "Utility functions",
		"config":      "Configuration files",
		"docs":        "Documentation",
		"tests":       "Test files",
		"test":        "Test files",
		"__tests__":   "Test files",
		"packages":    "Package/module code",
		"handlers":    "Request handlers",
		"types":       "Type definitions",
		"models":      "Data models",
		"services":    "Service layer",
		"controllers": "Controllers",
		"middleware":  "Middleware functions",
		"routes":      "Route definitions",
		"public":      "Public assets",
		"static":      "Static files",
		"assets":      "Asset files",
		"build":       "Build output",
		"dist":        "Distribution files",
		"out":         "Output files",
	}

	// Find directories that match patterns
	for path, isFile := range allPaths {
		if !isFile {
			dirName := filepath.Base(path)
			if desc, exists := patterns[dirName]; exists {
				keyDirs = append(keyDirs, KeyDirectory{
					Name:        dirName,
					Description: desc,
				})
			}
		}
	}

	return keyDirs
}

func analyzeFilesForDevflow(repoPath string) ([]DevflowFileInfo, error) {
	var files []DevflowFileInfo

	err := filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if shouldIgnoreForStructure(path, d.Name()) {
				return fs.SkipDir
			}
			return nil
		}

		relPath, _ := filepath.Rel(repoPath, path)
		if relPath == "." {
			return nil
		}

		relPath = strings.ReplaceAll(relPath, "\\", "/")

		if shouldIgnoreForStructure(relPath, d.Name()) {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		// Skip binary files
		if isBinary(content) {
			return nil
		}

		ext := filepath.Ext(d.Name())
		language := getLanguage(ext)

		fileInfo := DevflowFileInfo{
			Path:         path,
			RelativePath: relPath,
			Size:         int64(len(content)),
			Language:     language,
		}

		// Analyze file content based on language
		switch language {
		case "go":
			analyzeGoFile(content, &fileInfo)
		case "javascript", "typescript":
			analyzeJSFile(content, &fileInfo)
		case "python":
			analyzePythonFile(content, &fileInfo)
		}

		files = append(files, fileInfo)
		return nil
	})

	return files, err
}

func buildDependencyGraph(repoPath string) ([]DependencyNode, error) {
	var nodes []DependencyNode

	err := filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if shouldIgnoreForStructure(path, d.Name()) {
				return fs.SkipDir
			}
			return nil
		}

		relPath, _ := filepath.Rel(repoPath, path)
		if relPath == "." {
			return nil
		}

		relPath = strings.ReplaceAll(relPath, "\\", "/")

		if shouldIgnoreForStructure(relPath, d.Name()) {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		if isBinary(content) {
			return nil
		}

		ext := filepath.Ext(d.Name())
		language := getLanguage(ext)

		node := DependencyNode{
			File:         relPath,
			Language:     language,
			Dependencies: []string{},
			Exports:      []string{},
			Imports:      []string{},
		}

		// Extract dependencies based on language
		switch language {
		case "go":
			extractGoDependencies(content, &node)
		case "javascript", "typescript":
			extractJSDependencies(content, &node)
		case "python":
			extractPythonDependencies(content, &node)
		}

		nodes = append(nodes, node)
		return nil
	})

	return nodes, err
}

// Language-specific analysis functions

func analyzeGoFile(content []byte, fileInfo *DevflowFileInfo) {
	lines := strings.Split(string(content), "\n")

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// Extract function definitions
		if strings.HasPrefix(line, "func ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				funcName := strings.Split(parts[1], "(")[0]
				fileInfo.Functions = append(fileInfo.Functions, FunctionInfo{
					Name:       funcName,
					Signature:  line,
					LineNumber: i + 1,
				})
			}
		}

		// Extract imports
		if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "\"") {
			if strings.Contains(line, "\"") {
				start := strings.Index(line, "\"")
				end := strings.LastIndex(line, "\"")
				if start != -1 && end != -1 && end > start {
					importPath := line[start+1 : end]
					fileInfo.Imports = append(fileInfo.Imports, importPath)
				}
			}
		}
	}
}

func analyzeJSFile(content []byte, fileInfo *DevflowFileInfo) {
	lines := strings.Split(string(content), "\n")

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// Extract function definitions
		if strings.Contains(line, "function ") || strings.Contains(line, "=>") {
			// Simple function detection
			if strings.Contains(line, "function") {
				parts := strings.Fields(line)
				for j, part := range parts {
					if part == "function" && j+1 < len(parts) {
						funcName := strings.Split(parts[j+1], "(")[0]
						fileInfo.Functions = append(fileInfo.Functions, FunctionInfo{
							Name:       funcName,
							Signature:  line,
							LineNumber: i + 1,
						})
						break
					}
				}
			}
		}

		// Extract imports/exports
		if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "export ") {
			if strings.Contains(line, "from ") {
				parts := strings.Split(line, "from ")
				if len(parts) >= 2 {
					importPath := strings.Trim(strings.Trim(parts[1], ";"), "\"'")
					fileInfo.Imports = append(fileInfo.Imports, importPath)
				}
			}
		}
	}
}

func analyzePythonFile(content []byte, fileInfo *DevflowFileInfo) {
	lines := strings.Split(string(content), "\n")

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// Extract function definitions
		if strings.HasPrefix(line, "def ") {
			funcName := strings.Split(line, "(")[0]
			funcName = strings.TrimPrefix(funcName, "def ")
			fileInfo.Functions = append(fileInfo.Functions, FunctionInfo{
				Name:       funcName,
				Signature:  line,
				LineNumber: i + 1,
			})
		}

		// Extract class definitions
		if strings.HasPrefix(line, "class ") {
			className := strings.Split(line, "(")[0]
			className = strings.TrimPrefix(className, "class ")
			fileInfo.Classes = append(fileInfo.Classes, ClassInfo{
				Name:       className,
				LineNumber: i + 1,
			})
		}

		// Extract imports
		if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "from ") {
			if strings.Contains(line, "import ") {
				parts := strings.Split(line, "import ")
				if len(parts) >= 2 {
					importPath := strings.Split(parts[1], " ")[0]
					fileInfo.Imports = append(fileInfo.Imports, importPath)
				}
			}
		}
	}
}

// Dependency extraction functions

func extractGoDependencies(content []byte, node *DependencyNode) {
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "\"") {
			if strings.Contains(line, "\"") {
				start := strings.Index(line, "\"")
				end := strings.LastIndex(line, "\"")
				if start != -1 && end != -1 && end > start {
					importPath := line[start+1 : end]
					node.Imports = append(node.Imports, importPath)
				}
			}
		}
	}
}

func extractJSDependencies(content []byte, node *DependencyNode) {
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "import ") {
			if strings.Contains(line, "from ") {
				parts := strings.Split(line, "from ")
				if len(parts) >= 2 {
					importPath := strings.Trim(strings.Trim(parts[1], ";"), "\"'")
					node.Imports = append(node.Imports, importPath)
				}
			}
		}
	}
}

func extractPythonDependencies(content []byte, node *DependencyNode) {
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "from ") {
			if strings.Contains(line, "import ") {
				parts := strings.Split(line, "import ")
				if len(parts) >= 2 {
					importPath := strings.Split(parts[1], " ")[0]
					node.Imports = append(node.Imports, importPath)
				}
			}
		}
	}
}

// Helper functions from existing code

func isBinary(content []byte) bool {
	checkSize := 8192
	if len(content) < checkSize {
		checkSize = len(content)
	}

	for i := 0; i < checkSize; i++ {
		if content[i] == 0 {
			return true
		}
	}

	nonPrintable := 0
	for i := 0; i < checkSize; i++ {
		if content[i] < 32 && content[i] != '\n' && content[i] != '\r' && content[i] != '\t' {
			nonPrintable++
		}
	}

	return float64(nonPrintable)/float64(checkSize) > 0.30
}

func getLanguage(ext string) string {
	languageMap := map[string]string{
		".go":            "go",
		".js":            "javascript",
		".jsx":           "jsx",
		".ts":            "typescript",
		".tsx":           "tsx",
		".py":            "python",
		".java":          "java",
		".cpp":           "cpp",
		".cc":            "cpp",
		".cxx":           "cpp",
		".c":             "c",
		".cs":            "csharp",
		".html":          "html",
		".htm":           "html",
		".css":           "css",
		".scss":          "scss",
		".sass":          "sass",
		".less":          "less",
		".json":          "json",
		".xml":           "xml",
		".yaml":          "yaml",
		".yml":           "yaml",
		".md":            "markdown",
		".markdown":      "markdown",
		".sh":            "bash",
		".bash":          "bash",
		".zsh":           "zsh",
		".fish":          "fish",
		".sql":           "sql",
		".rb":            "ruby",
		".php":           "php",
		".rs":            "rust",
		".kt":            "kotlin",
		".swift":         "swift",
		".dart":          "dart",
		".vue":           "vue",
		".svelte":        "svelte",
		".r":             "r",
		".R":             "r",
		".scala":         "scala",
		".clj":           "clojure",
		".hs":            "haskell",
		".elm":           "elm",
		".ex":            "elixir",
		".exs":           "elixir",
		".pl":            "perl",
		".lua":           "lua",
		".vim":           "vim",
		".dockerfile":    "dockerfile",
		".toml":          "toml",
		".ini":           "ini",
		".cfg":           "ini",
		".conf":          "conf",
		".env":           "bash",
		".gitignore":     "",
		".gitattributes": "",
		".editorconfig":  "ini",
		".eslintrc":      "json",
		".prettierrc":    "json",
		".babelrc":       "json",
	}

	if lang, exists := languageMap[strings.ToLower(ext)]; exists {
		return lang
	}

	return ""
}

// Conversion functions to convert between local and AI package types
func convertFunctions(functions []FunctionInfo) []ai.FunctionInfo {
	aiFunctions := make([]ai.FunctionInfo, len(functions))
	for i, fn := range functions {
		aiFunctions[i] = ai.FunctionInfo{
			Name:       fn.Name,
			Signature:  fn.Signature,
			Purpose:    fn.Purpose,
			Parameters: fn.Parameters,
			ReturnType: fn.ReturnType,
			LineNumber: fn.LineNumber,
		}
	}
	return aiFunctions
}

func convertClasses(classes []ClassInfo) []ai.ClassInfo {
	aiClasses := make([]ai.ClassInfo, len(classes))
	for i, cls := range classes {
		aiClasses[i] = ai.ClassInfo{
			Name:       cls.Name,
			Purpose:    cls.Purpose,
			Methods:    convertFunctions(cls.Methods),
			Properties: cls.Properties,
			LineNumber: cls.LineNumber,
		}
	}
	return aiClasses
}
