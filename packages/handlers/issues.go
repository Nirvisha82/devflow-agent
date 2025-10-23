package handlers

import (
	"context"
	"devflow-agent/packages/ai"
	"devflow-agent/packages/config"
	repoActions "devflow-agent/packages/repository"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/google/go-github/github"
	"github.com/swinton/go-probot/probot"
)

func HandleIssues(ctx *probot.Context) error {
	// Your existing issue handling logic
	event := ctx.Payload.(*github.IssuesEvent)

	// Extract key information
	issueTitle := event.Issue.GetTitle()
	issueNumber := event.Issue.GetNumber()
	repoName := event.Repo.GetFullName()
	action := event.GetAction()

	slog.Info(" Issue Action:", "action", action)
	slog.Info(" Issue", "issueNumber", issueNumber, "issueTitle", issueTitle)
	slog.Info(" Repository:", "repoName", repoName)

	// Process different actions using switch case
	switch action {
	case "opened":
		slog.Info("Issue opened - will process when labeled", "issueNumber", issueNumber)
		return nil
		// return handleIssueOpened(ctx, event, repoName, issueNumber, issueTitle)
	case "labeled":
		return handleIssueLabeled(ctx, event, repoName, issueNumber, issueTitle)
	default:
		slog.Info("Skipping action", "action", action)
		return nil
	}
}

func handleIssueOpened(ctx *probot.Context, event *github.IssuesEvent, repoName string, issueNumber int, issueTitle string) error {
	cfg := config.GetConfig()
	// Check if issue already has required labels
	if hasRequiredLabels(event.Issue.Labels) {
		branchName := fmt.Sprintf("%s%d-%s", cfg.Issues.BranchPrefix, issueNumber, repoActions.SanitizeBranchName(issueTitle))
		if branchExists(ctx, repoName, branchName) {
			slog.Info("Issue already processed - branch exists", "issueNumber", issueNumber, "branch", branchName)
			return nil
		}

		slog.Info("Issue opened with required labels - proceeding with workflow", "issueNumber", issueNumber)
		return processIssue(ctx, repoName, issueNumber, issueTitle)
	}

	slog.Info(" Issue opened without required labels - waiting for labels", "issueNumber", issueNumber)
	return nil
}

func handleIssueLabeled(ctx *probot.Context, event *github.IssuesEvent, repoName string, issueNumber int, issueTitle string) error {
	cfg := config.GetConfig()
	// Check if the newly labeled issue now has required labels
	if !hasRequiredLabels(event.Issue.Labels) {
		slog.Info("Issue labeled but still missing required labels", "issueNumber", issueNumber)
		return nil
	}

	// Check if we've already processed this issue (deduplication)
	branchName := fmt.Sprintf("%s%d-%s", cfg.Issues.BranchPrefix, issueNumber, repoActions.SanitizeBranchName(issueTitle))
	if branchExists(ctx, repoName, branchName) {
		slog.Info(" Issue already processed - branch exists", "issueNumber", issueNumber, "branch", branchName)
		return nil
	}

	slog.Info("Issue labeled with required labels - proceeding with workflow", "issueNumber", issueNumber)
	return processIssue(ctx, repoName, issueNumber, issueTitle)
}

func processIssue(ctx *probot.Context, repoName string, issueNumber int, issueTitle string) error {
	cfg := config.GetConfig()
	event := ctx.Payload.(*github.IssuesEvent)
	branchName := fmt.Sprintf("%s%d-%s", cfg.Issues.BranchPrefix, issueNumber, repoActions.SanitizeBranchName(issueTitle))

	slog.Info("Starting three-agent workflow", "issueNumber", issueNumber, "branch", branchName)

	// Clone repository temporarily
	repoPath, _, err := repoActions.CloneRepository(repoName)
	if err != nil {
		slog.Error("Failed to clone repository", "error", err)
		return err
	}
	// defer func() {
	// 	if cleanupErr := repoActions.CleanupRepo(repoPath); cleanupErr != nil {
	// 		slog.Error("Failed to cleanup", "repoPath", repoPath, "error", cleanupErr)
	// 	}
	// }()

	// Check if Devflow knowledge base exists
	repoStructureFile := cfg.GetDevflowPath(repoPath, cfg.Files.StructureFile)
	_dependecygraph := cfg.GetDevflowPath(repoPath, cfg.Files.DependencyFile)
	slog.Debug(_dependecygraph)

	// If knowledge base doesn't exist, create it first
	if _, err := os.Stat(repoStructureFile); os.IsNotExist(err) {
		slog.Info("Devflow knowledge base not found, creating it first")
		if err := initializeDevflowKnowledgeBaseFromIssues(ctx, repoName); err != nil {
			slog.Error("Failed to initialize knowledge base", "error", err)
			return err
		}
		// Re-clone to get the updated knowledge base
		if cleanupErr := repoActions.CleanupRepo(repoPath); cleanupErr != nil {
			slog.Error("Failed to cleanup after knowledge base creation", "error", cleanupErr)
		}
		_, _, err = repoActions.CloneRepository(repoName)
		if err != nil {
			slog.Error("Failed to re-clone repository", "error", err)
			return err
		}
	}

	// Agent A: File Selector/Planner
	slog.Info("Running Agent A: File Selector/Planner")
	agentA := &ai.AgentA{
		IssueTitle:       issueTitle,
		IssueDescription: event.Issue.GetBody(),
		Labels:           getIssueLabelNames(event.Issue.Labels),
		RepoAnalysisFile: repoStructureFile,
	}

	agentAResult, err := ai.AnalyzeIssueWithAgentA(agentA)
	if err != nil {
		slog.Error("Agent A failed", "error", err)
		return err
	}

	slog.Info("Agent A completed", "relevantFiles", agentAResult.RelevantFiles, "plan", agentAResult.Plan)

	// // Agent B: Code Analyzer/Suggester
	// slog.Info("Running Agent B: Code Analyzer/Suggester")
	// agentB := &ai.AgentB{
	// 	AgentAResult:     agentAResult,
	// 	IssueTitle:       issueTitle,
	// 	IssueDescription: event.Issue.GetBody(),
	// 	RepoPath:         repoPath,
	// 	DependencyGraph:  dependencyGraphFile,
	// }

	// agentBResult, err := ai.AnalyzeWithAgentB(agentB)
	// if err != nil {
	// 	slog.Error("Agent B failed", "error", err)
	// 	return err
	// }

	// slog.Info("Agent B completed", "suggestions", len(agentBResult.CodeSuggestions))

	// // Agent C: Code Generator/Implementer
	// slog.Info("Running Agent C: Code Generator/Implementer")
	// agentC := &ai.AgentC{
	// 	AgentBResult:     agentBResult,
	// 	IssueTitle:       issueTitle,
	// 	IssueDescription: event.Issue.GetBody(),
	// 	RepoPath:         repoPath,
	// 	BranchName:       branchName,
	// }

	// agentCResult, err := ai.ImplementWithAgentC(agentC)
	// if err != nil {
	// 	slog.Error("Agent C failed", "error", err)
	// 	return err
	// }

	// if !agentCResult.Success {
	// 	slog.Error("Agent C implementation failed", "error", agentCResult.Error)
	// 	return fmt.Errorf("implementation failed: %s", agentCResult.Error)
	// }

	// slog.Info("Agent C completed", "modifiedFiles", agentCResult.ModifiedFiles)

	// Create branch and commit changes
	// err = repoActions.CreateBranch(ctx, repoName, branchName)
	// if err != nil {
	// 	slog.Error("Failed to create branch", "error", err)
	// 	return err
	// }

	// // Commit all modified files
	// for _, file := range agentCResult.ModifiedFiles {
	// 	fullPath := repoPath + "/" + file
	// 	err = repoActions.CommitFile(ctx, repoName, branchName, agentCResult.CommitMessage, fullPath)
	// 	if err != nil {
	// 		slog.Error("Failed to commit file", "file", file, "error", err)
	// 		return err
	// 	}
	// }

	// // Create summary document
	// summaryFile := repoPath + "/devflow-implementation-summary.md"
	// err = repoActions.SaveAnalysisToFile(agentCResult.Summary, summaryFile)
	// if err != nil {
	// 	slog.Error("Failed to save implementation summary", "error", err)
	// 	return err
	// }

	// // Commit summary
	// err = repoActions.CommitFile(ctx, repoName, branchName, "Add implementation summary", summaryFile)
	// if err != nil {
	// 	slog.Error("Failed to commit summary", "error", err)
	// 	return err
	// }

	// TODO: Create Pull Request
	// This would require additional GitHub API calls to create a PR
	// slog.Info("Three-agent workflow completed successfully",
	// 	"issueNumber", issueNumber,
	// 	"branch", branchName,
	// 	"modifiedFiles", len(agentCResult.ModifiedFiles))

	return nil
}

func branchExists(ctx *probot.Context, repoName, branchName string) bool {
	parts := strings.Split(repoName, "/")
	if len(parts) != 2 {
		slog.Error("Invalid repo name format", "repoName", repoName)
		return false
	}

	owner := parts[0]
	repo := parts[1]

	_, _, err := ctx.GitHub.Git.GetRef(context.Background(), owner, repo, "refs/heads/"+branchName)
	return err == nil // If no error, branch exists
}

// hasRequiredLabels checks if the issue has any of the required labels
func hasRequiredLabels(labels []github.Label) bool {
	cfg := config.GetConfig()
	requiredLabels := cfg.Issues.RequiredLabels

	// Convert issue labels to a map for faster lookup
	issueLabelMap := make(map[string]bool)
	for _, label := range labels {
		if label.Name != nil {
			// Convert to lowercase for case-insensitive comparison
			issueLabelMap[strings.ToLower(*label.Name)] = true
		}
	}

	// Check if any required label exists
	for _, requiredLabel := range requiredLabels {
		if issueLabelMap[strings.ToLower(requiredLabel)] {
			slog.Info(" Found required label:", "reqLabel", requiredLabel)
			return true
		}
	}

	slog.Info(" Required labels not found. Issue has labels", "labels", getIssueLabelNames(labels))
	return false
}

// Helper function to get label names for logging
func getIssueLabelNames(labels []github.Label) []string {
	var labelNames []string
	for _, label := range labels {
		if label.Name != nil {
			labelNames = append(labelNames, *label.Name)
		}
	}
	return labelNames
}

// initializeDevflowKnowledgeBaseFromIssues creates the Devflow knowledge base from the issues handler
func initializeDevflowKnowledgeBaseFromIssues(ctx *probot.Context, repoName string) error {
	slog.Info("Initializing Devflow knowledge base from issues handler", "repo", repoName)

	// Clone repository temporarily
	repoPath, repoURL, err := repoActions.CloneRepository(repoName)
	if err != nil {
		slog.Error("Failed to clone repository for knowledge base initialization", "error", err)
		return err
	}
	defer func() {
		if cleanupErr := repoActions.CleanupRepo(repoPath); cleanupErr != nil {
			slog.Error("Failed to cleanup repository", "repoPath", repoPath, "error", cleanupErr)
		}
	}()

	// Create .devflow directory
	cfg := config.GetConfig()
	devflowDir := cfg.GetDevflowDir(repoPath)
	if err := repoActions.CreateDirectory(devflowDir); err != nil {
		slog.Error("Failed to create .devflow directory", "error", err)
		return err
	}

	// Step 1: Generate repo-structure.md using RepoAnalyzer (flattened structure)
	structureFile := cfg.GetDevflowPath(repoPath, cfg.Files.StructureFile)
	if err := repoActions.AnalyzeRepo(ctx, structureFile, repoPath, repoURL); err != nil {
		slog.Error("Failed to generate repo structure", "error", err)
		return err
	}

	// Step 2: Save debug files (only if debug mode is enabled)
	var metadataFile, promptFile string
	if cfg.Debug.CreateDebugFiles {
		// Save file metadata as JSON
		metadataFile = cfg.GetDevflowPath(repoPath, cfg.Files.MetadataFile)
		if err := repoActions.SaveFileMetadata(repoPath, metadataFile); err != nil {
			slog.Error("Failed to save file metadata", "error", err)
			return err
		}

		// Save analysis prompt (using repo structure content)
		promptFile = cfg.GetDevflowPath(repoPath, cfg.Files.AnalysisPromptFile)
		if err := repoActions.SaveAnalysisPrompt(repoPath, repoURL, structureFile, promptFile); err != nil {
			slog.Error("Failed to save analysis prompt", "error", err)
			return err
		}
		slog.Info("Debug files created", "metadata", metadataFile, "prompt", promptFile)
	}

	// Step 4: Generate LLM analysis
	analysisFile := cfg.GetDevflowPath(repoPath, cfg.Files.AnalysisFile)
	if err := repoActions.GenerateRepoAnalysisWithLLM(repoPath, repoURL, structureFile, analysisFile); err != nil {
		slog.Error("Failed to generate LLM analysis", "error", err)
		return err
	}

	// Step 5: Build dependency graph
	dependencyFile := cfg.GetDevflowPath(repoPath, cfg.Files.DependencyFile)
	if err := repoActions.GenerateDependencyGraph(repoPath, dependencyFile); err != nil {
		slog.Error("Failed to generate dependency graph", "error", err)
		return err
	}

	// Step 6: Create .devflow/README.md
	readmeFile := cfg.GetDevflowPath(repoPath, cfg.Files.ReadmeFile)
	if err := repoActions.CreateDevflowReadme(readmeFile, repoName); err != nil {
		slog.Error("Failed to create Devflow README", "error", err)
		return err
	}

	// Step 5: Commit all files to the repository
	branchName := cfg.Installations.KnowledgeBaseBranch
	if err := repoActions.CreateBranch(ctx, repoName, branchName); err != nil {
		slog.Error("Failed to create knowledge base branch", "error", err)
		return err
	}

	// Prepare files to commit (core files always, debug files conditionally)
	devflowFiles := []string{
		structureFile,
		analysisFile,
		dependencyFile,
		readmeFile,
	}

	// Add debug files if they were created
	if cfg.Debug.CreateDebugFiles {
		devflowFiles = append(devflowFiles, metadataFile, promptFile)
	}

	// Commit all files in a single commit
	if err := repoActions.CommitMultipleFiles(ctx, repoName, branchName, cfg.Installations.KnowledgeBaseCommit, devflowFiles); err != nil {
		slog.Error("Failed to commit Devflow files", "error", err)
		return err
	}

	// Create pull request for knowledge base initialization (temporary - will be replaced with actual issue resolution)
	pr, err := repoActions.CreateInstallationPR(ctx, repoName, branchName)
	if err != nil {
		slog.Error("Failed to create pull request", "error", err)
		return err
	}

	// Cleanup temporary repository (if enabled)
	if cfg.Repository.CleanupTempRepos {
		if cleanupErr := repoActions.CleanupRepo(repoPath); cleanupErr != nil {
			slog.Error("Failed to cleanup temporary repository", "repoPath", repoPath, "error", cleanupErr)
		} else {
			slog.Info("Temporary repository cleaned up", "repoPath", repoPath)
		}
	} else {
		slog.Info("Temporary repository preserved for debugging", "repoPath", repoPath)
	}

	slog.Info("Devflow knowledge base initialized successfully",
		"repo", repoName,
		"branch", branchName,
		"prNumber", pr.GetNumber(),
		"prURL", pr.GetHTMLURL())
	return nil
}
