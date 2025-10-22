package ai

import (
	"context"
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

	// Use Gemini 1.5 Flash (faster, still 1M tokens) or Pro (2M tokens)
	model := client.GenerativeModel("gemini-2.5-flash")

	// Configure model settings
	model.SetTemperature(0.7)
	model.SetTopK(40)
	model.SetTopP(0.95)
	model.SetMaxOutputTokens(8192)

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
