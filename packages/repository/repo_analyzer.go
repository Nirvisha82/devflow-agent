package repository

import (
	"bufio"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type FileInfo struct {
	Path         string
	RelativePath string
	Size         int64
	GitChanges   int
	Content      []byte
	Language     string
}

type RepoAnalyzer struct {
	RepoURL           string
	LocalPath         string
	OutputFile        string
	Files             []FileInfo
	gitignorePatterns []string
}

func (r *RepoAnalyzer) Generate() error {
	// fmt.Println("Cloning repository...")
	// if err := r.cloneRepo(); err != nil {
	// 	return fmt.Errorf("failed to clone repo: %w", err)
	// }
	// defer r.cleanup()

	fmt.Println("Analyzing files...")
	if err := r.analyzeFiles(); err != nil {
		return fmt.Errorf("failed to analyze files: %w", err)
	}

	fmt.Println("Generating markdown...")
	if err := r.generateMarkdown(); err != nil {
		return fmt.Errorf("failed to generate markdown: %w", err)
	}

	return nil
}

func (r *RepoAnalyzer) cloneRepo() error {
	_, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("git is not installed or not in PATH: %w", err)
	}

	tempDir := fmt.Sprintf("temp_repo_%s", strings.ReplaceAll(filepath.Base(r.RepoURL), ".", "_"))
	r.LocalPath = tempDir

	if _, err := os.Stat(tempDir); err == nil {
		os.RemoveAll(tempDir)
	}

	cmd := exec.Command("git", "clone", r.RepoURL, tempDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

func (r *RepoAnalyzer) analyzeFiles() error {
	r.parseGitignore()

	gitChanges, err := r.getGitChangeCounts()
	if err != nil {
		log.Printf("Warning: Could not get Git change counts: %v", err)
		gitChanges = make(map[string]int)
	}

	err = filepath.WalkDir(r.LocalPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if r.shouldIgnoreDirectory(path, d.Name()) {
				return fs.SkipDir
			}
			return nil
		}

		// Skip if file should be ignored
		if r.shouldIgnoreFile(path, d.Name()) {
			return nil
		}

		relPath, _ := filepath.Rel(r.LocalPath, path)
		if relPath == "." {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Error reading file %s: %v", relPath, err)
			return nil
		}

		// Skip binary files
		if r.isBinary(content) {
			return nil
		}

		file := FileInfo{
			Path:         path,
			RelativePath: relPath,
			Size:         int64(len(content)),
			GitChanges:   gitChanges[relPath],
			Content:      content,
			Language:     r.getLanguage(filepath.Ext(d.Name())),
		}

		r.Files = append(r.Files, file)
		return nil
	})

	if err != nil {
		return err
	}

	// Sort by Git change count (files with MORE changes at the BOTTOM - repomix behavior)
	sort.Slice(r.Files, func(i, j int) bool {
		return r.Files[i].GitChanges < r.Files[j].GitChanges
	})

	fmt.Printf("Found %d files after filtering\n", len(r.Files))
	return nil
}

func (r *RepoAnalyzer) parseGitignore() {
	gitignorePath := filepath.Join(r.LocalPath, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		r.gitignorePatterns = []string{}
		return
	}

	r.gitignorePatterns = []string{}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			r.gitignorePatterns = append(r.gitignorePatterns, line)
		}
	}
}

func (r *RepoAnalyzer) shouldIgnoreDirectory(path, name string) bool {
	relPath, _ := filepath.Rel(r.LocalPath, path)
	// Normalize path separators
	relPath = strings.ReplaceAll(relPath, "\\", "/")

	// Debug logging to see what's being checked
	// fmt.Printf("DEBUG: Checking directory: %s (name: %s)\n", relPath, name)

	// Check .gitignore patterns for directories
	for _, pattern := range r.gitignorePatterns {
		if matched, _ := filepath.Match(pattern, relPath); matched {
			// fmt.Printf("DEBUG: Directory %s ignored by gitignore pattern: %s\n", relPath, pattern)
			return true
		}
		if matched, _ := filepath.Match(pattern, name); matched {
			// fmt.Printf("DEBUG: Directory %s ignored by gitignore pattern: %s\n", relPath, pattern)
			return true
		}
		if strings.HasSuffix(pattern, "/") {
			dirPattern := strings.TrimSuffix(pattern, "/")
			if strings.HasPrefix(relPath, dirPattern+"/") || relPath == dirPattern {
				// fmt.Printf("DEBUG: Directory %s ignored by gitignore pattern: %s\n", relPath, pattern)
				return true
			}
		}
	}

	// Repomix's default ignore patterns for directories
	defaultIgnoreDirs := []string{
		"node_modules", ".git", ".svn", ".hg",
		"dist", "build", ".next", ".nuxt", "out",
		"coverage", ".nyc_output", ".coverage",
		"__pycache__", ".pytest_cache",
		".vscode", ".idea", ".venv", "venv", "env",
		"target", "bin", "obj", ".gradle", ".mvn",
		".DS_Store", "Thumbs.db",
		".turbo", ".vercel", ".netlify",
	}

	lowerName := strings.ToLower(name)
	lowerPath := strings.ToLower(relPath)

	for _, pattern := range defaultIgnoreDirs {
		// Be more specific - only ignore exact matches or paths that contain the pattern as a complete directory
		if lowerName == strings.ToLower(pattern) {
			// fmt.Printf("DEBUG: Directory %s ignored by default pattern: %s\n", relPath, pattern)
			return true
		}
		// Only ignore if the pattern appears as a complete directory name in the path
		if strings.Contains(lowerPath, "/"+strings.ToLower(pattern)+"/") ||
			strings.HasPrefix(lowerPath, strings.ToLower(pattern)+"/") {
			// fmt.Printf("DEBUG: Directory %s ignored by default pattern: %s\n", relPath, pattern)
			return true
		}
	}

	return false
}

func (r *RepoAnalyzer) shouldIgnoreFile(path, name string) bool {
	relPath, _ := filepath.Rel(r.LocalPath, path)
	// Normalize path separators
	relPath = strings.ReplaceAll(relPath, "\\", "/")

	// Check .gitignore patterns for files
	for _, pattern := range r.gitignorePatterns {
		if matched, _ := filepath.Match(pattern, relPath); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
	}

	// Repomix's default ignore patterns for files
	defaultIgnoreFiles := []string{
		"package-lock.json", "yarn.lock", "pnpm-lock.yaml", "bun.lockb",
		"go.sum", "Pipfile.lock", "poetry.lock", "Gemfile.lock",
		"composer.lock", "mix.lock", "pubspec.lock",
		".env", ".env.local", ".env.production", ".env.development",
	}

	// File extensions to ignore (binary/media files)
	ignoreExtensions := []string{
		// Images
		".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp", ".bmp", ".tiff",
		// Videos
		".mp4", ".avi", ".mov", ".mkv", ".wmv", ".flv", ".webm",
		// Audio
		".mp3", ".wav", ".flac", ".aac", ".ogg", ".wma",
		// Documents
		".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
		// Archives
		".zip", ".rar", ".7z", ".tar", ".gz", ".bz2", ".xz",
		// Executables
		".exe", ".dll", ".so", ".dylib", ".app", ".deb", ".rpm",
		// Fonts
		".ttf", ".otf", ".woff", ".woff2", ".eot",
		// Other binary
		".bin", ".dat", ".db", ".sqlite", ".sqlite3",
	}

	lowerName := strings.ToLower(name)

	// Check exact file names
	for _, pattern := range defaultIgnoreFiles {
		if lowerName == strings.ToLower(pattern) {
			return true
		}
	}

	// Check file extensions
	for _, ext := range ignoreExtensions {
		if strings.HasSuffix(lowerName, ext) {
			return true
		}
	}

	// Additional patterns - be more selective with hidden files
	if strings.HasPrefix(lowerName, ".") && len(name) > 1 {
		// Allow common config files but ignore most hidden files
		allowedHidden := []string{
			".gitignore", ".gitattributes", ".editorconfig",
			".eslintrc", ".prettierrc", ".babelrc",
			".dockerignore", ".env.example", ".nvmrc",
		}

		found := false
		for _, allowed := range allowedHidden {
			if lowerName == strings.ToLower(allowed) || strings.HasPrefix(lowerName, strings.ToLower(allowed)) {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}

	return false
}

func (r *RepoAnalyzer) getGitChangeCounts() (map[string]int, error) {
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)

	if err := os.Chdir(r.LocalPath); err != nil {
		return nil, err
	}

	cmd := exec.Command("git", "log", "--name-only", "--pretty=format:", "--all")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	changes := make(map[string]int)
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			changes[line]++
		}
	}

	return changes, nil
}

func (r *RepoAnalyzer) generateMarkdown() error {
	file, err := os.Create(r.OutputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	r.writeHeader(writer)
	r.writeDirectoryStructure(writer)
	r.writeFileContents(writer)

	return nil
}

func (r *RepoAnalyzer) writeHeader(writer *bufio.Writer) {
	repoName := filepath.Base(strings.TrimSuffix(r.RepoURL, ".git"))

	header := fmt.Sprintf(`This file is a merged representation of the entire codebase, combined into a single document.
The content has been processed for AI analysis and code review purposes.

# File Summary

## Purpose
This file contains a packed representation of the entire repository's contents.
It is designed to be easily consumable by AI systems for analysis, code review,
or other automated processes.

## File Format
The content is organized as follows:
1. This summary section
2. Repository information
3. Directory structure
4. Repository files (if enabled)
5. Multiple file entries, each consisting of:
  a. A header with the file path (## File: path/to/file)
  b. The full contents of the file in a code block

## Usage Guidelines
- This file should be treated as read-only. Any changes should be made to the
  original repository files, not this packed version.
- When processing this file, use the file path to distinguish
  between different files in the repository.
- Be aware that this file may contain sensitive information. Handle it with
  the same level of security as you would the original repository.

## Notes
- Some files may have been excluded based on .gitignore rules and default ignore patterns
- Binary files are not included in this packed representation. Please refer to the Repository Structure section for a complete list of file paths, including binary files
- Files matching patterns in .gitignore are excluded
- Files matching default ignore patterns are excluded
- Files are sorted by Git change count (files with more changes are at the bottom)

# Repository Information
- **Repository URL:** %s
- **Repository Name:** %s
- **Total Files Analyzed:** %d
- **Generated:** %s

`, r.RepoURL, repoName, len(r.Files), time.Now().Format("2006-01-02 15:04:05"))

	writer.WriteString(header)
}

func (r *RepoAnalyzer) writeDirectoryStructure(writer *bufio.Writer) {
	writer.WriteString("# Directory Structure\n```\n")

	// Build directory structure - include ALL files and directories
	allPaths := make(map[string]bool)

	// Add all file paths and their parent directories
	for _, file := range r.Files {
		// Normalize path separators to forward slashes
		normalizedPath := strings.ReplaceAll(file.RelativePath, "\\", "/")
		parts := strings.Split(normalizedPath, "/")
		currentPath := ""

		// Add all parent directories
		for i, part := range parts {
			if i == 0 {
				currentPath = part
			} else {
				currentPath = currentPath + "/" + part // Use forward slash
			}

			// Mark as directory or file
			isFile := i == len(parts)-1
			allPaths[currentPath] = isFile
		}
	}

	// Also walk the actual directory to catch empty directories
	filepath.WalkDir(r.LocalPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		relPath, _ := filepath.Rel(r.LocalPath, path)
		if relPath == "." {
			return nil
		}

		// Normalize path separators
		relPath = strings.ReplaceAll(relPath, "\\", "/")

		// Skip ignored directories but show them in structure if they contain files
		if d.IsDir() && r.shouldIgnoreDirectory(path, d.Name()) {
			return fs.SkipDir
		}

		// Add to paths if not already present
		if _, exists := allPaths[relPath]; !exists {
			allPaths[relPath] = !d.IsDir()
		}

		return nil
	})

	// Convert to sorted slice
	var paths []string
	for path := range allPaths {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	// Write directory structure
	for _, path := range paths {
		isFile := allPaths[path]
		depth := strings.Count(path, "/") // Count forward slashes
		indent := strings.Repeat("  ", depth)
		name := filepath.Base(path)

		if isFile {
			writer.WriteString(fmt.Sprintf("%s%s\n", indent, name))
		} else {
			writer.WriteString(fmt.Sprintf("%s%s/\n", indent, name))
		}
	}

	writer.WriteString("```\n\n")
}

func (r *RepoAnalyzer) writeFileContents(writer *bufio.Writer) {
	writer.WriteString("# Files\n\n")

	for i, file := range r.Files {
		fmt.Printf("File %d/%d: %s (changes: %d)\n", i+1, len(r.Files), file.RelativePath, file.GitChanges)

		// Normalize path separators to forward slashes (like repomix)
		normalizedPath := strings.ReplaceAll(file.RelativePath, "\\", "/")

		writer.WriteString(fmt.Sprintf("## File: %s\n", normalizedPath))
		writer.WriteString(fmt.Sprintf("````%s\n", file.Language))
		writer.WriteString(string(file.Content))

		if !strings.HasSuffix(string(file.Content), "\n") {
			writer.WriteString("\n")
		}

		writer.WriteString("````\n\n")
	}
}

func (r *RepoAnalyzer) cleanup() {
	if r.LocalPath != "" {
		fmt.Printf("Cleaning up: %s\n", r.LocalPath)
		os.RemoveAll(r.LocalPath)
	}
}

func (r *RepoAnalyzer) isBinary(content []byte) bool {
	// Check first 8192 bytes for null bytes (more comprehensive than original)
	checkSize := 8192
	if len(content) < checkSize {
		checkSize = len(content)
	}

	for i := 0; i < checkSize; i++ {
		if content[i] == 0 {
			return true
		}
	}

	// Additional heuristic: if more than 30% of characters are non-printable
	nonPrintable := 0
	for i := 0; i < checkSize; i++ {
		if content[i] < 32 && content[i] != '\n' && content[i] != '\r' && content[i] != '\t' {
			nonPrintable++
		}
	}

	return float64(nonPrintable)/float64(checkSize) > 0.30
}

func (r *RepoAnalyzer) getLanguage(ext string) string {
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
