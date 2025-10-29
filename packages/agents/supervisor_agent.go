package agents

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"devflow-agent/packages/config"
	repoActions "devflow-agent/packages/repository"

	"github.com/google/go-github/github"
	"github.com/swinton/go-probot/probot"
)

// SupervisorAgent orchestrates the workflow between File Analyzer and Code Generator
type SupervisorAgent struct {
	ctx         *probot.Context
	repoPath    string
	repoName    string
	issueNumber int
	issueTitle  string
	issueBody   string
	branchName  string
	labels      []string
}

// SupervisorResult contains the final output from the supervisor
type SupervisorResult struct {
	Success               bool
	ModifiedFiles         []string
	CommitMessage         string
	PRNumber              int
	ChangesSummary        string
	ImplementationDetails string
	TestingNotes          string
	Error                 error
}

// NewSupervisorAgent creates a new supervisor agent instance
func NewSupervisorAgent(
	ctx *probot.Context,
	repoPath string,
	repoName string,
	issueNumber int,
	issueTitle string,
	issueBody string,
	branchName string,
	labels []string,
) *SupervisorAgent {
	return &SupervisorAgent{
		ctx:         ctx,
		repoPath:    repoPath,
		repoName:    repoName,
		issueNumber: issueNumber,
		issueTitle:  issueTitle,
		issueBody:   issueBody,
		branchName:  branchName,
		labels:      labels,
	}
}

// Execute runs the complete multi-agent workflow
func (s *SupervisorAgent) Execute() (*SupervisorResult, error) {
	slog.Info("Supervisor: Starting multi-agent workflow", "issue", s.issueNumber)

	// Step 1: Invoke File Analyzer Agent
	filePaths, err := s.invokeFileAnalyzer()
	if err != nil {
		return &SupervisorResult{Success: false, Error: err}, err
	}

	slog.Info("Supervisor: File Analyzer identified files", "count", len(filePaths), "files", filePaths)

	// Step 2: Create code-files.md with consolidated content
	codeFilesPath, err := s.createCodeFilesDocument(filePaths)
	if err != nil {
		return &SupervisorResult{Success: false, Error: err}, err
	}

	slog.Info("Supervisor: Created code-files.md", "path", codeFilesPath)

	// Step 3: Invoke Code Generator Agent
	modifications, err := s.invokeCodeGenerator(codeFilesPath)
	if err != nil {
		return &SupervisorResult{Success: false, Error: err}, err
	}

	slog.Info("Supervisor: Code Generator completed", "modifiedFiles", len(modifications))

	// Step 4: Apply modifications to actual files
	modifiedFiles, err := s.applyModifications(modifications)
	if err != nil {
		return &SupervisorResult{Success: false, Error: err}, err
	}

	// Step 5: Create implementation summary
	changesSummary, implementationDetails, testingNotes := s.generateSummary(modifications)

	// Step 6: Create branch and commit changes
	err = s.createBranchAndCommit(modifiedFiles, changesSummary)
	if err != nil {
		return &SupervisorResult{Success: false, Error: err}, err
	}

	// Step 7: Create Pull Request
	pr, err := s.createPullRequest(changesSummary, implementationDetails, testingNotes)
	if err != nil {
		return &SupervisorResult{Success: false, Error: err}, err
	}

	slog.Info("Supervisor: Workflow completed successfully", "prNumber", pr.GetNumber())

	return &SupervisorResult{
		Success:               true,
		ModifiedFiles:         modifiedFiles,
		CommitMessage:         fmt.Sprintf("Resolve issue #%d: %s", s.issueNumber, s.issueTitle),
		PRNumber:              pr.GetNumber(),
		ChangesSummary:        changesSummary,
		ImplementationDetails: implementationDetails,
		TestingNotes:          testingNotes,
	}, nil
}

// invokeFileAnalyzer calls the File Analyzer Agent
func (s *SupervisorAgent) invokeFileAnalyzer() ([]string, error) {
	slog.Info("Supervisor: Invoking File Analyzer Agent")

	// Create File Analyzer Agent
	fileAnalyzer := NewFileAnalyzerAgent(s.repoPath, s.issueTitle, s.issueBody, s.labels)

	// Execute analysis
	result, err := fileAnalyzer.Analyze()
	if err != nil {
		return nil, fmt.Errorf("file analyzer failed: %w", err)
	}

	return result.FilePaths, nil
}

// createCodeFilesDocument consolidates all relevant file contents into code-files.md
func (s *SupervisorAgent) createCodeFilesDocument(filePaths []string) (string, error) {
	slog.Info("Supervisor: Creating code-files.md")

	cfg := config.GetConfig()
	codeFilesPath := filepath.Join(s.repoPath, cfg.Repository.DevflowDirectory, "code-files.md")

	var content strings.Builder
	content.WriteString("# Code Files for Issue Resolution\n\n")
	content.WriteString(fmt.Sprintf("**Issue:** #%d - %s\n\n", s.issueNumber, s.issueTitle))
	content.WriteString(fmt.Sprintf("**Description:**\n%s\n\n", s.issueBody))
	content.WriteString("---\n\n")

	for _, filePath := range filePaths {
		fullPath := filepath.Join(s.repoPath, filePath)

		// Check if file exists
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			slog.Warn("Supervisor: File does not exist", "file", filePath)
			continue
		}

		// Read file content
		fileContent, err := os.ReadFile(fullPath)
		if err != nil {
			slog.Warn("Supervisor: Could not read file", "file", filePath, "error", err)
			continue
		}

		// Get language for syntax highlighting
		ext := filepath.Ext(filePath)
		lang := getLanguageForExtension(ext)

		content.WriteString(fmt.Sprintf("## File: %s\n\n", filePath))
		content.WriteString(fmt.Sprintf("```%s\n", lang))
		content.WriteString(string(fileContent))
		if !strings.HasSuffix(string(fileContent), "\n") {
			content.WriteString("\n")
		}
		content.WriteString("```\n\n")
	}

	// Write to file
	err := os.WriteFile(codeFilesPath, []byte(content.String()), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write code-files.md: %w", err)
	}

	return codeFilesPath, nil
}

// invokeCodeGenerator calls the Code Generator Agent
func (s *SupervisorAgent) invokeCodeGenerator(codeFilesPath string) (map[string]string, error) {
	slog.Info("Supervisor: Invoking Code Generator Agent")

	// Read code-files.md
	codeFilesContent, err := os.ReadFile(codeFilesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read code-files.md: %w", err)
	}

	// Create Code Generator Agent
	codeGenerator := NewCodeGeneratorAgent(
		string(codeFilesContent),
		s.issueTitle,
		s.issueBody,
		s.repoPath,
	)

	// Execute code generation
	result, err := codeGenerator.Generate()
	if err != nil {
		return nil, fmt.Errorf("code generator failed: %w", err)
	}

	return result.Modifications, nil
}

// applyModifications writes the generated code changes to actual files
func (s *SupervisorAgent) applyModifications(modifications map[string]string) ([]string, error) {
	slog.Info("Supervisor: Applying modifications to files", "count", len(modifications))

	var modifiedFiles []string

	for filePath, newContent := range modifications {
		fullPath := filepath.Join(s.repoPath, filePath)

		// Create directory if it doesn't exist
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		// Write modified content
		err := os.WriteFile(fullPath, []byte(newContent), 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to write file %s: %w", filePath, err)
		}

		modifiedFiles = append(modifiedFiles, fullPath)
		slog.Info("Supervisor: Modified file", "file", filePath)
	}

	return modifiedFiles, nil
}

// generateSummary creates documentation for the PR
func (s *SupervisorAgent) generateSummary(modifications map[string]string) (string, string, string) {
	var filesList []string
	for filePath := range modifications {
		filesList = append(filesList, fmt.Sprintf("- `%s`", filePath))
	}

	changesSummary := fmt.Sprintf("Modified %d file(s):\n%s", len(filesList), strings.Join(filesList, "\n"))

	implementationDetails := fmt.Sprintf(`This PR implements the changes requested in issue #%d.

The following approach was taken:
1. Analyzed the issue and identified relevant files using dependency graph
2. Generated code modifications using AI analysis
3. Applied changes to the codebase

### Files Modified:
%s

### Changes Made:
- Implemented the requested functionality
- Updated relevant code sections
- Ensured compatibility with existing codebase`, s.issueNumber, strings.Join(filesList, "\n"))

	testingNotes := `### Testing Recommendations:
- Verify that the implementation meets the requirements specified in the issue
- Test edge cases and error handling
- Run existing tests to ensure no regressions
- Review code quality and adherence to project standards`

	return changesSummary, implementationDetails, testingNotes
}

// createBranchAndCommit creates a new branch and commits the changes
func (s *SupervisorAgent) createBranchAndCommit(modifiedFiles []string, changesSummary string) error {
	slog.Info("Supervisor: Creating branch and committing changes", "branch", s.branchName)

	// Create branch
	err := repoActions.CreateBranch(s.ctx, s.repoName, s.branchName)
	if err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	// Commit message
	commitMessage := fmt.Sprintf("Resolve issue #%d: %s\n\n%s", s.issueNumber, s.issueTitle, changesSummary)

	// Commit all modified files
	err = repoActions.CommitMultipleFiles(s.ctx, s.repoName, s.branchName, commitMessage, modifiedFiles)
	if err != nil {
		return fmt.Errorf("failed to commit files: %w", err)
	}

	return nil
}

// createPullRequest creates a pull request for the changes
func (s *SupervisorAgent) createPullRequest(changesSummary, implementationDetails, testingNotes string) (*github.PullRequest, error) {
	slog.Info("Supervisor: Creating pull request")

	pr, err := repoActions.CreateIssueResolutionPR(
		s.ctx,
		s.repoName,
		s.branchName,
		s.issueNumber,
		s.issueTitle,
		changesSummary,
		implementationDetails,
		testingNotes,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create PR: %w", err)
	}

	return pr, nil
}

// Helper function to get language identifier for syntax highlighting
func getLanguageForExtension(ext string) string {
	languageMap := map[string]string{
		".go":    "go",
		".js":    "javascript",
		".jsx":   "jsx",
		".ts":    "typescript",
		".tsx":   "tsx",
		".py":    "python",
		".java":  "java",
		".cpp":   "cpp",
		".c":     "c",
		".cs":    "csharp",
		".rb":    "ruby",
		".php":   "php",
		".rs":    "rust",
		".kt":    "kotlin",
		".swift": "swift",
		".yaml":  "yaml",
		".yml":   "yaml",
		".json":  "json",
		".xml":   "xml",
		".html":  "html",
		".css":   "css",
		".scss":  "scss",
		".md":    "markdown",
		".sh":    "bash",
		".sql":   "sql",
		".toml":  "toml",
		".ini":   "ini",
		".conf":  "conf",
		".env":   "bash",
	}

	if lang, exists := languageMap[strings.ToLower(ext)]; exists {
		return lang
	}
	return ""
}
