package ai

import (
	"context"
	"devflow-agent/packages/config"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type IssueAnalysis struct {
	IssueTitle       string
	IssueDescription string
	Labels           []string
	RepoStructFile   string
}

type AnalysisResult struct {
	MarkdownContent string
	Error           error
}

// RepoAnalysis represents the input for repository analysis
type RepoAnalysis struct {
	RepoURL string
	Files   []DevflowFileInfo
}

// RepoAnalysisFromStructure represents analysis input using repo structure content
type RepoAnalysisFromStructure struct {
	RepoURL          string
	StructureContent string
}

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

func AnalyzeIssueWithAI(analysis *IssueAnalysis) (*AnalysisResult, error) {
	// Get API key from environment
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set in environment")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		slog.Error("Failed to create Gemini client", "error", err)
		return nil, err
	}
	defer client.Close()

	// Use configured model
	cfg := config.GetConfig()
	model := client.GenerativeModel(cfg.AI.Model)

	// Configure model settings
	model.SetTemperature(cfg.AI.Temperature)
	model.SetTopK(cfg.AI.TopK)
	model.SetTopP(cfg.AI.TopP)
	model.SetMaxOutputTokens(cfg.AI.MaxOutputTokens)

	// Read repository structure file
	repoContent, err := os.ReadFile(analysis.RepoStructFile)
	if err != nil {
		slog.Error("Failed to read repo structure file", "error", err)
		return nil, err
	}

	// Build the prompt
	prompt := buildAnalysisPrompt(analysis, string(repoContent))

	slog.Info("Sending request to Gemini API", "issueTitle", analysis.IssueTitle)

	// Generate content
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		slog.Error("Failed to generate content", "error", err)
		return nil, err
	}

	// Extract response
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no content generated")
	}

	markdownContent := fmt.Sprintf("%v", resp.Candidates[0].Content.Parts[0])

	slog.Info("Successfully generated analysis", "contentLength", len(markdownContent))

	return &AnalysisResult{
		MarkdownContent: markdownContent,
		Error:           nil,
	}, nil
}

func buildAnalysisPrompt(analysis *IssueAnalysis, repoContent string) string {
	labelsStr := ""
	for _, label := range analysis.Labels {
		labelsStr += fmt.Sprintf("- %s\n", label)
	}

	prompt := fmt.Sprintf(`You are an expert code analyst. Analyze the following GitHub issue and repository structure to provide detailed insights.

# Issue Information
**Title:** %s

**Description:**
%s

**Labels:**
%s

# Repository Structure and Code
%s

# Your Task
Provide a comprehensive analysis in markdown format that includes:

1. **Issue Summary**: Brief overview of what the issue is requesting
2. **Root Cause Analysis**: If it's a bug, identify potential root causes based on the codebase
3. **Affected Components**: List all files/modules that are likely affected
4. **Implementation Approach**: For new features or fixes, suggest implementation strategy
5. **Code Locations**: Highlight specific files and approximate line ranges where changes are needed
6. **Potential Risks**: Identify any side effects or related areas that might break
7. **Testing Recommendations**: Suggest what should be tested
8. **Additional Notes**: Any other relevant observations

Be specific with file paths and code references. Use the repository structure provided to give accurate locations.

Format your response in clean markdown with appropriate headers and code blocks.`,
		analysis.IssueTitle,
		analysis.IssueDescription,
		labelsStr,
		repoContent,
	)

	return prompt
}

// AnalyzeRepositoryWithAI generates comprehensive analysis of repository files
func AnalyzeRepositoryWithAI(analysis *RepoAnalysis) (*AnalysisResult, error) {
	// Get API key from environment
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set in environment")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		slog.Error("Failed to create Gemini client", "error", err)
		return nil, err
	}
	defer client.Close()

	// Use configured model
	cfg := config.GetConfig()
	model := client.GenerativeModel(cfg.AI.Model)

	// Configure model settings for repository analysis
	model.SetTemperature(cfg.AI.RepoAnalysisTemperature) // Lower temperature for more consistent analysis
	model.SetTopK(cfg.AI.TopK)
	model.SetTopP(cfg.AI.TopP)
	model.SetMaxOutputTokens(cfg.AI.MaxOutputTokens)

	// Build the prompt
	prompt := BuildRepoAnalysisPrompt(analysis)

	slog.Info("Sending repository analysis request to Gemini API", "repoURL", analysis.RepoURL, "fileCount", len(analysis.Files))

	// Generate content
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		slog.Error("Failed to generate repository analysis", "error", err)
		return nil, err
	}

	// Extract response
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no content generated")
	}

	markdownContent := fmt.Sprintf("%v", resp.Candidates[0].Content.Parts[0])

	slog.Info("Successfully generated repository analysis", "contentLength", len(markdownContent))

	return &AnalysisResult{
		MarkdownContent: markdownContent,
		Error:           nil,
	}, nil
}

func BuildRepoAnalysisPrompt(analysis *RepoAnalysis) string {
	// Build file summaries
	fileSummaries := ""
	for _, file := range analysis.Files {
		fileSummaries += fmt.Sprintf("## File: %s\n", file.RelativePath)
		fileSummaries += fmt.Sprintf("- **Language:** %s\n", file.Language)
		fileSummaries += fmt.Sprintf("- **Size:** %d bytes\n", file.Size)

		if len(file.Functions) > 0 {
			fileSummaries += "- **Functions:**\n"
			for _, fn := range file.Functions {
				fileSummaries += fmt.Sprintf("  - `%s` (line %d)\n", fn.Name, fn.LineNumber)
			}
		}

		if len(file.Classes) > 0 {
			fileSummaries += "- **Classes:**\n"
			for _, cls := range file.Classes {
				fileSummaries += fmt.Sprintf("  - `%s` (line %d)\n", cls.Name, cls.LineNumber)
			}
		}

		if len(file.Imports) > 0 {
			fileSummaries += "- **Imports:**\n"
			for _, imp := range file.Imports {
				fileSummaries += fmt.Sprintf("  - `%s`\n", imp)
			}
		}

		fileSummaries += "\n"
	}

	prompt := fmt.Sprintf(`You are an expert code analyst. Analyze the following repository structure and provide comprehensive insights about each file's purpose and role.

# Repository Information
**Repository URL:** %s
**Total Files Analyzed:** %d

# File Analysis Data
%s

# Your Task
Provide a comprehensive analysis in markdown format that includes:

## Repository Overview
1. **Project Type**: What kind of project is this? (web app, CLI tool, library, etc.)
2. **Architecture**: Describe the overall architecture and structure
3. **Technology Stack**: Identify the main technologies and frameworks used
4. **Entry Points**: Identify the main entry points and how the application starts

## File Analysis
For each file, provide:
1. **Purpose**: What is this file's primary purpose?
2. **Role**: How does it fit into the larger system?
3. **Key Functions/Classes**: Brief description of main functions/classes
4. **Dependencies**: What other files/modules does it depend on?
5. **Dependents**: What other files/modules depend on this file?

## System Relationships
1. **Data Flow**: How does data flow through the system?
2. **Key Components**: What are the most important components?
3. **Integration Points**: Where do different parts of the system connect?

## Development Insights
1. **Code Quality**: Overall assessment of code organization
2. **Patterns**: What design patterns are used?
3. **Potential Issues**: Any obvious problems or areas for improvement?

Format your response in clean markdown with appropriate headers and code blocks. Be specific and detailed in your analysis.`,
		analysis.RepoURL,
		len(analysis.Files),
		fileSummaries,
	)

	return prompt
}

// AnalyzeRepositoryFromStructure generates comprehensive analysis using repo structure content
func AnalyzeRepositoryFromStructure(analysis *RepoAnalysisFromStructure) (*AnalysisResult, error) {
	// Get API key from environment
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set in environment")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		slog.Error("Failed to create Gemini client", "error", err)
		return nil, err
	}
	defer client.Close()

	// Use configured model
	cfg := config.GetConfig()
	model := client.GenerativeModel(cfg.AI.Model)

	// Configure model settings for repository analysis
	model.SetTemperature(cfg.AI.RepoAnalysisTemperature)
	model.SetTopK(cfg.AI.TopK)
	model.SetTopP(cfg.AI.TopP)
	model.SetMaxOutputTokens(cfg.AI.MaxOutputTokens)

	// Build the prompt using repo structure content
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

Format your response in clean markdown with appropriate headers and code blocks. Be specific and detailed in your analysis, referencing actual code when relevant.`, analysis.RepoURL, analysis.StructureContent)

	slog.Info("Sending repository analysis request to Gemini API", "repoURL", analysis.RepoURL)

	// Generate content
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		slog.Error("Failed to generate repository analysis", "error", err)
		return nil, err
	}

	// Extract response
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no content generated")
	}

	markdownContent := fmt.Sprintf("%v", resp.Candidates[0].Content.Parts[0])

	slog.Info("Successfully generated repository analysis", "contentLength", len(markdownContent))

	return &AnalysisResult{
		MarkdownContent: markdownContent,
		Error:           nil,
	}, nil
}
