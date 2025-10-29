package ai

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// AgentA represents the File Selector/Planner agent
type AgentA struct {
	IssueTitle       string
	IssueDescription string
	Labels           []string
	RepoAnalysisFile string
}

// AgentAResult represents the output from Agent A
type AgentAResult struct {
	RelevantFiles   []string
	Plan            string
	Context         string
	Priority        string
	EstimatedEffort string
}

// AnalyzeIssueWithAgentA analyzes the issue and determines which files are relevant
func AnalyzeIssueWithAgentA(agentA *AgentA) (*AgentAResult, error) {
	// Read the repository analysis file
	repoAnalysis, err := os.ReadFile(agentA.RepoAnalysisFile)
	if err != nil {
		slog.Error("Failed to read repo analysis file", "error", err)
		return nil, err
	}

	// Build the prompt for Agent A
	prompt := buildAgentAPrompt(agentA, string(repoAnalysis))

	// Use Gemini to analyze
	result, err := generateWithGemini(prompt, "agent-a-file-selector")
	if err != nil {
		return nil, err
	}

	// Parse the result
	return parseAgentAResult(result)
}

func buildAgentAPrompt(agentA *AgentA, repoAnalysis string) string {
	labelsStr := ""
	for _, label := range agentA.Labels {
		labelsStr += fmt.Sprintf("- %s\n", label)
	}

	prompt := fmt.Sprintf(`You are Agent A - the File Selector and Planner in the Devflow system.

# Your Role
You analyze GitHub issues and determine which files are relevant for implementing the requested changes. You create a high-level plan and identify the specific files that need to be modified.

# Issue Information
**Title:** %s

**Description:**
%s

**Labels:**
%s

# Repository Analysis
%s

# Your Task
Analyze this issue and provide:

1. **Relevant Files**: List the specific files that need to be modified (provide exact file paths)
2. **Implementation Plan**: High-level step-by-step plan for implementing the changes
3. **Context**: Additional context about the issue and its impact
4. **Priority**: Assessment of priority (low, medium, high, critical)
5. **Estimated Effort**: Rough estimate of implementation complexity (simple, moderate, complex, very complex)

# Output Format
Respond in the following JSON format:
{
  "relevant_files": ["path/to/file1.go", "path/to/file2.js"],
  "plan": "Step-by-step implementation plan",
  "context": "Additional context about the issue",
  "priority": "high",
  "estimated_effort": "moderate"
}

Be specific with file paths and provide actionable, detailed plans.`,
		agentA.IssueTitle,
		agentA.IssueDescription,
		labelsStr,
		repoAnalysis,
	)

	return prompt
}

func parseAgentAResult(result string) (*AgentAResult, error) {
	// Simple parsing - in a real implementation, you'd use proper JSON parsing
	// For now, we'll extract the information using string manipulation

	lines := strings.Split(result, "\n")
	var relevantFiles []string
	var plan, context, priority, estimatedEffort string

	inRelevantFiles := false
	inPlan := false
	inContext := false
	inPriority := false
	inEstimatedEffort := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "relevant_files") {
			inRelevantFiles = true
			continue
		}
		if strings.Contains(line, "plan") {
			inRelevantFiles = false
			inPlan = true
			continue
		}
		if strings.Contains(line, "context") {
			inPlan = false
			inContext = true
			continue
		}
		if strings.Contains(line, "priority") {
			inContext = false
			inPriority = true
			continue
		}
		if strings.Contains(line, "estimated_effort") {
			inPriority = false
			inEstimatedEffort = true
			continue
		}

		if inRelevantFiles && strings.Contains(line, "\"") {
			// Extract file path
			start := strings.Index(line, "\"")
			end := strings.LastIndex(line, "\"")
			if start != -1 && end != -1 && end > start {
				filePath := line[start+1 : end]
				relevantFiles = append(relevantFiles, filePath)
			}
		}

		if inPlan && line != "" && !strings.Contains(line, "{") && !strings.Contains(line, "}") {
			plan += line + "\n"
		}

		if inContext && line != "" && !strings.Contains(line, "{") && !strings.Contains(line, "}") {
			context += line + "\n"
		}

		if inPriority && strings.Contains(line, "\"") {
			start := strings.Index(line, "\"")
			end := strings.LastIndex(line, "\"")
			if start != -1 && end != -1 && end > start {
				priority = line[start+1 : end]
			}
		}

		if inEstimatedEffort && strings.Contains(line, "\"") {
			start := strings.Index(line, "\"")
			end := strings.LastIndex(line, "\"")
			if start != -1 && end != -1 && end > start {
				estimatedEffort = line[start+1 : end]
			}
		}
	}

	// If parsing failed, provide defaults
	if len(relevantFiles) == 0 {
		relevantFiles = []string{"main.go", "handlers/issues.go"} // Default files
	}
	if plan == "" {
		plan = "Analyze the issue and implement the requested changes"
	}
	if context == "" {
		context = "Standard issue implementation"
	}
	if priority == "" {
		priority = "medium"
	}
	if estimatedEffort == "" {
		estimatedEffort = "moderate"
	}

	return &AgentAResult{
		RelevantFiles:   relevantFiles,
		Plan:            strings.TrimSpace(plan),
		Context:         strings.TrimSpace(context),
		Priority:        priority,
		EstimatedEffort: estimatedEffort,
	}, nil
}

// generateWithGemini is a helper function to generate content using Gemini
func generateWithGemini(prompt, agentType string) (string, error) {
	// This would use the same Gemini client setup as the other functions
	// For now, we'll return a placeholder
	slog.Info("Generating content with Gemini", "agent", agentType)

	return `{
  "relevant_files": ["main.go", "packages/handlers/issues.go", "packages/ai/ai.go"],
  "plan": "1. Analyze the issue requirements\n2. Identify the specific changes needed\n3. Implement the changes in the relevant files\n4. Test the implementation\n5. Create a pull request",
  "context": "This is a standard issue that requires code changes across multiple files",
  "priority": "high",
  "estimated_effort": "moderate"
}`, nil
}
