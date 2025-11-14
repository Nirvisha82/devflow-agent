package handlers

import (
	"context"
	"devflow-agent/packages/ai"
	"devflow-agent/packages/config"
	repoActions "devflow-agent/packages/repository"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/github"
	"github.com/swinton/go-probot/probot"
)

// ensureClosingLink prepends "Closes #<n>" unless a closing keyword is already present.
func ensureClosingLink(prBody string, issueNumber int) string {
	linkLine := fmt.Sprintf("Closes #%d", issueNumber)
	low := strings.ToLower(prBody)
	if strings.Contains(low, "closes #") || strings.Contains(low, "fixes #") || strings.Contains(low, "resolves #") {
		return prBody
	}
	if prBody == "" {
		return linkLine
	}
	return linkLine + "\n\n" + prBody
}

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

	slog.Info("Starting Python Strands agent workflow", "issueNumber", issueNumber, "branch", branchName)

	// Clone repository
	repoPath, _, err := repoActions.CloneRepository(repoName)
	if err != nil {
		slog.Error("Failed to clone repository", "error", err)
		return err
	}

	// --- Ensure .devflow reflects latest origin/main BEFORE invoking Python agent ---
	headSHA, err := repoActions.GetOriginMainSHA(repoPath)
	if err != nil {
		slog.Error("Failed to resolve origin/main", "error", err)
		return err
	}
	devflowCommitPath := filepath.Join(repoPath, ".devflow", "devflow-commit.txt")
	devflowSHA := ""
	if b, err := os.ReadFile(devflowCommitPath); err == nil {
		devflowSHA = strings.TrimSpace(string(b))
	}
	if devflowSHA != headSHA {
		slog.Info("Devflow stale; syncing", "devflow", devflowSHA, "head", headSHA)
		if err := repoActions.RunIncrementalDevflowSync(ctx, repoName, repoPath, headSHA); err != nil {
			slog.Error("Devflow incremental sync failed", "error", err)
			return err
		}
		// refresh HEAD just in case
		if _, err := repoActions.GetOriginMainSHA(repoPath); err != nil {
			slog.Warn("Post-sync fetch failed", "error", err)
		}
	}

	// Check if knowledge base exists
	repoStructureFile := cfg.GetDevflowPath(repoPath, cfg.Files.StructureFile)
	if _, err := os.Stat(repoStructureFile); os.IsNotExist(err) {
		slog.Error("Devflow knowledge base not initialized for repo", "repo", repoName)

		// Post a helpful comment on the issue instead of trying to initialize here
		issue := event.Issue
		owner := event.GetRepo().GetOwner().GetLogin()
		name := event.GetRepo().GetName()

		commentBody := `DevFlow isn't fully set up for this repository yet.

	Please merge the "Initialize Devflow Knowledge Base" PR (branch "devflow-init") that DevFlow created for this repo, and then re-apply the label to this issue.`

		_, _, cErr := ctx.GitHub.Issues.CreateComment(
			context.Background(),
			owner,
			name,
			int(issue.GetNumber()),
			&github.IssueComment{Body: &commentBody},
		)
		if cErr != nil {
			slog.Error("Failed to post missing-knowledge-base comment", "error", cErr)
		}

		return fmt.Errorf("devflow knowledge base not initialized for repo %s", repoName)
	}

	// Call Python Strands agent
	result, err := ai.CallPythonStrandsAgent(repoPath, event.Issue)
	if err != nil {
		slog.Error("Python agent failed", "error", err)
		return err
	}

	// Use the results
	for _, file := range result.ChangesMade {
		fmt.Printf("Changed: %s\n", file)
	}

	// Create branch and commit changes
	if len(result.ChangesMade) > 0 {
		if err := repoActions.CreateBranch(ctx, repoName, branchName); err != nil {
			slog.Error("Failed to create branch", "error", err)
			return err
		}

		commitMessage := fmt.Sprintf("Resolve issue #%d: %s\n\n%s", issueNumber, issueTitle, result.Summary)

		// Convert relative paths to absolute for commit
		absolutePaths := make([]string, len(result.ChangesMade))
		for i, relPath := range result.ChangesMade {
			absolutePaths[i] = filepath.Join(repoPath, relPath)
		}

		if err := repoActions.CommitMultipleFiles(ctx, repoName, branchName, commitMessage, absolutePaths, false, repoPath); err != nil {
			slog.Error("Failed to commit files", "error", err)
			return err
		}

		// Create PR with AI-generated body if available
		var pr *github.PullRequest
		if result.PRBodyFile != "" {
			// Read the generated PR body
			prBodyPath := filepath.Join(repoPath, result.PRBodyFile)
			slog.Info("Attempting to read AI-generated PR body", "path", prBodyPath)

			prBodyContent, err := os.ReadFile(prBodyPath)
			if err != nil {
				slog.Warn("Failed to read generated PR body, using fallback", "error", err, "path", prBodyPath)
				// Fallback to default PR creation
				pr, err = repoActions.CreateIssueResolutionPR(
					ctx,
					repoName,
					branchName,
					issueNumber,
					issueTitle,
					result.Summary,
					fmt.Sprintf("Modified files:\n- %s", strings.Join(result.ChangesMade, "\n- ")),
					"Please review the automated changes generated by the AI agent.",
				)
				if err != nil {
					slog.Error("Failed to create PR with fallback", "error", err)
					return err
				}
			} else {
				// Use the AI-generated PR body directly
				prTitle := fmt.Sprintf("[#%d] %s", issueNumber, issueTitle) // neutral title is fine
				bodyWithLink := ensureClosingLink(string(prBodyContent), issueNumber)

				slog.Info("Creating PR with AI-generated body", "length", len(bodyWithLink))
				pr, err = repoActions.CreatePullRequest(ctx, repoName, branchName, prTitle, bodyWithLink)

				if err != nil {
					slog.Error("Failed to create PR with AI-generated body", "error", err)
					return err
				}
				slog.Info("PR created successfully with AI-generated description")
			}
		} else {
			slog.Info("No PR body file returned by agent, composing PR body with closing link")

			prTitle := fmt.Sprintf("[#%d] %s", issueNumber, issueTitle)

			baseBody := fmt.Sprintf(
				"Summary:\n%s\n\nModified files:\n- %s\n\nPlease review the automated changes generated by the AI agent.",
				result.Summary,
				strings.Join(result.ChangesMade, "\n- "),
			)

			bodyWithLink := ensureClosingLink(baseBody, issueNumber)

			pr, err = repoActions.CreatePullRequest(ctx, repoName, branchName, prTitle, bodyWithLink)
			if err != nil {
				slog.Error("Failed to create PR", "error", err)
				return err
			}
		}

		slog.Info("Python agent workflow completed successfully",
			"issueNumber", issueNumber,
			"branch", branchName,
			"prNumber", pr.GetNumber(),
			"prURL", pr.GetHTMLURL(),
			"modifiedFiles", len(result.ChangesMade))
	} else {
		slog.Info("No files were modified by the agent", "issueNumber", issueNumber)
	}

	// Cleanup
	if cfg.Repository.CleanupTempRepos {
		if cleanupErr := repoActions.CleanupRepo(repoPath); cleanupErr != nil {
			slog.Error("Failed to cleanup temporary repository", "error", cleanupErr)
		} else {
			slog.Info("Temporary repository cleaned up", "repoPath", repoPath)
		}
	}

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
	if err := repoActions.CommitMultipleFiles(ctx, repoName, branchName, cfg.Installations.KnowledgeBaseCommit, devflowFiles, true, ""); err != nil {
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
