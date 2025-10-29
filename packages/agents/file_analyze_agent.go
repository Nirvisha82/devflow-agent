package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"devflow-agent/packages/config"

	"google.golang.org/genai"
)

// FileAnalyzerAgent analyzes the issue and determines which files need modification
type FileAnalyzerAgent struct {
	repoPath   string
	issueTitle string
	issueBody  string
	labels     []string
}

// FileAnalyzerResult contains the output from File Analyzer
type FileAnalyzerResult struct {
	FilePaths []string
	Reasoning string
}

// DependencyGraph represents the dependency graph structure
type DependencyGraph struct {
	Nodes []DependencyNode `json:"nodes"`
}

// DependencyNode represents a node in the dependency graph
type DependencyNode struct {
	File         string   `json:"file"`
	Language     string   `json:"language"`
	Dependencies []string `json:"dependencies"`
	Exports      []string `json:"exports"`
	Imports      []string `json:"imports"`
}

// NewFileAnalyzerAgent creates a new file analyzer agent
func NewFileAnalyzerAgent(repoPath, issueTitle, issueBody string, labels []string) *FileAnalyzerAgent {
	return &FileAnalyzerAgent{
		repoPath:   repoPath,
		issueTitle: issueTitle,
		issueBody:  issueBody,
		labels:     labels,
	}
}

// Analyze performs the file analysis
func (f *FileAnalyzerAgent) Analyze() (*FileAnalyzerResult, error) {
	slog.Info("FileAnalyzer: Starting analysis", "issue", f.issueTitle)

	cfg := config.GetConfig()

	// Read repo-analysis.md
	repoAnalysisPath := cfg.GetDevflowPath(f.repoPath, cfg.Files.AnalysisFile)
	repoAnalysis, err := os.ReadFile(repoAnalysisPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read repo analysis: %w", err)
	}

	// Read dependency-graph.json
	dependencyGraphPath := cfg.GetDevflowPath(f.repoPath, cfg.Files.DependencyFile)
	dependencyData, err := os.ReadFile(dependencyGraphPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read dependency graph: %w", err)
	}

	var depGraph DependencyGraph
	err = json.Unmarshal(dependencyData, &depGraph)
	if err != nil {
		return nil, fmt.Errorf("failed to parse dependency graph: %w", err)
	}

	// Analyze using AI (Gemini)
	filePaths, reasoning, err := f.analyzeWithAI(string(repoAnalysis), depGraph)
	if err != nil {
		return nil, fmt.Errorf("AI analysis failed: %w", err)
	}

	// Validate and expand file paths using dependency graph
	expandedPaths := f.expandWithDependencies(filePaths, depGraph)

	slog.Info("FileAnalyzer: Analysis complete", "files", len(expandedPaths))

	return &FileAnalyzerResult{
		FilePaths: expandedPaths,
		Reasoning: reasoning,
	}, nil
}

// analyzeWithAI uses Gemini to identify relevant files
func (f *FileAnalyzerAgent) analyzeWithAI(repoAnalysis string, depGraph DependencyGraph) ([]string, string, error) {
	slog.Info("FileAnalyzer: Analyzing with AI")

	// Build context about available files
	availableFiles := make([]string, len(depGraph.Nodes))
	for i, node := range depGraph.Nodes {
		availableFiles[i] = node.File
	}

	prompt := fmt.Sprintf(`You are a File Analyzer Agent in the Devflow system. Your task is to identify which files need to be modified to resolve the given issue.

# Issue Information
**Title:** %s

**Description:**
%s

**Labels:** %s

# Repository Analysis
%s

# Available Files
%s

# Your Task
Analyze this issue and identify the specific files that need to be modified. Consider:
1. The core functionality mentioned in the issue
2. Related files that might be affected
3. Test files that should be updated
4. Configuration files if relevant

Respond in JSON format:
{
  "files": ["path/to/file1.go", "path/to/file2.go"],
  "reasoning": "Explanation of why these files were selected"
}

Be specific with file paths. Only include files that actually need modification.
Do NOT use markdown formatting in file paths. Return ONLY JSON with no code blocks or backticks.`,
		f.issueTitle,
		f.issueBody,
		strings.Join(f.labels, ", "),
		repoAnalysis,
		strings.Join(availableFiles, "\n"),
	)

	// Call Gemini API
	result, err := f.generateWithGemini(prompt)
	if err != nil {
		return nil, "", err
	}

	// Parse JSON response
	var response struct {
		Files     []string `json:"files"`
		Reasoning string   `json:"reasoning"`
	}

	// Try to parse as JSON
	err = json.Unmarshal([]byte(result), &response)
	if err != nil {
		// Fallback: try to extract files manually
		slog.Warn("FileAnalyzer: Failed to parse JSON response, extracting files manually")
		files := extractFilesFromText(result)
		return files, "AI analysis completed (manual extraction)", nil
	}

	return response.Files, response.Reasoning, nil
}

// generateWithGemini calls the new Gemini API
func (f *FileAnalyzerAgent) generateWithGemini(prompt string) (string, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY not set in environment")
	}

	ctx := context.Background()

	// Create client using new SDK
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create Gemini client: %w", err)
	}
	// Note: No Close() method in new SDK - client manages lifecycle automatically

	cfg := config.GetConfig()

	// Create generation config - use float64 and int types directly
	temperature := float32(cfg.AI.Temperature)
	topK := float32(cfg.AI.TopK)
	topP := float32(cfg.AI.TopP)
	maxTokens := int32(cfg.AI.MaxOutputTokens)

	genConfig := &genai.GenerateContentConfig{
		Temperature:     &temperature,
		TopK:            &topK,
		TopP:            &topP,
		MaxOutputTokens: maxTokens,
	}

	// Generate content using new API
	result, err := client.Models.GenerateContent(
		ctx,
		cfg.AI.Model,
		genai.Text(prompt),
		genConfig,
	)
	if err != nil {
		return "", fmt.Errorf("gemini API call failed: %w", err)
	}

	// Extract text from response
	if result == nil || result.Text() == "" {
		return "", fmt.Errorf("no content generated by Gemini")
	}

	return result.Text(), nil
}

// expandWithDependencies expands the file list with dependencies
func (f *FileAnalyzerAgent) expandWithDependencies(filePaths []string, depGraph DependencyGraph) []string {
	fileSet := make(map[string]bool)

	// Add initial files
	for _, path := range filePaths {
		fileSet[path] = true
	}

	// Add direct dependencies
	for _, path := range filePaths {
		for _, node := range depGraph.Nodes {
			if node.File == path {
				for _, dep := range node.Dependencies {
					// Only add local dependencies (not external packages)
					if !strings.Contains(dep, "/") || strings.HasPrefix(dep, ".") {
						fileSet[dep] = true
					}
				}
			}
		}
	}

	// Convert back to slice
	result := make([]string, 0, len(fileSet))
	for file := range fileSet {
		result = append(result, file)
	}

	return result
}

// Helper function to extract file paths from text
func extractFilesFromText(text string) []string {
	fileSet := make(map[string]bool)

	lines := strings.Split(text, "\n")

	for _, line := range lines {
		if !strings.Contains(line, "/") || !strings.Contains(line, ".") {
			continue
		}

		// Remove markdown formatting first
		line = strings.ReplaceAll(line, "**", "")
		line = strings.ReplaceAll(line, "__", "")
		line = strings.ReplaceAll(line, "*", "")
		line = strings.ReplaceAll(line, "_", "")

		parts := strings.Fields(line)
		for _, part := range parts {
			// Clean up the part
			part = strings.Trim(part, `"',.;:[]{}()*`+"`")
			part = strings.TrimPrefix(part, "->")
			part = strings.TrimPrefix(part, "=>")

			// Check if it looks like a valid file path
			if strings.Contains(part, "/") && strings.Contains(part, ".") &&
				!strings.ContainsAny(part, "*`_[]{}()\"'") {
				fileSet[part] = true
			}
		}
	}

	// Convert back to slice
	result := make([]string, 0, len(fileSet))
	for file := range fileSet {
		result = append(result, file)
	}

	if len(result) == 0 {
		slog.Warn("FileAnalyzer: No files extracted from AI response, using defaults")
		result = []string{"main.go", "packages/handlers/issues.go"}
	}

	return result
}
