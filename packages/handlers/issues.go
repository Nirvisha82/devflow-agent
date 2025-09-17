package handlers

import (
	"context"
	repo "devflow-agent/packages/repository"
	"fmt"
	"log/slog"
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
		return handleIssueOpened(ctx, event, repoName, issueNumber, issueTitle)
	case "labeled":
		return handleIssueLabeled(ctx, event, repoName, issueNumber, issueTitle)
	default:
		slog.Info("Skipping action", "action", action)
		return nil
	}
}

func handleIssueOpened(ctx *probot.Context, event *github.IssuesEvent, repoName string, issueNumber int, issueTitle string) error {
	// Check if issue already has required labels
	if hasRequiredLabels(event.Issue.Labels) {
		slog.Info(" Issue opened with required labels - proceeding with workflow", "issueNumber", issueNumber)
		return processIssue(ctx, repoName, issueNumber, issueTitle)
	}

	slog.Info(" Issue opened without required labels - waiting for labels", "issueNumber", issueNumber)
	return nil
}

func handleIssueLabeled(ctx *probot.Context, event *github.IssuesEvent, repoName string, issueNumber int, issueTitle string) error {
	// Check if the newly labeled issue now has required labels
	if !hasRequiredLabels(event.Issue.Labels) {
		slog.Info("Issue labeled but still missing required labels", "issueNumber", issueNumber)
		return nil
	}

	// Check if we've already processed this issue (deduplication)
	branchName := fmt.Sprintf("issue-%d-%s", issueNumber, repo.SanitizeBranchName(issueTitle))
	if branchExists(ctx, repoName, branchName) {
		slog.Info(" Issue already processed - branch exists", "issueNumber", issueNumber, "branch", branchName)
		return nil
	}

	slog.Info("Issue labeled with required labels - proceeding with workflow", "issueNumber", issueNumber)
	return processIssue(ctx, repoName, issueNumber, issueTitle)
}

func processIssue(ctx *probot.Context, repoName string, issueNumber int, issueTitle string) error {
	// Clone repository temporarily
	repoPath, err := repo.CloneRepository(repoName)
	if err != nil {
		slog.Error("Failed to clone repository", "error", err)
		return err
	}

	slog.Debug("Repository ready for analysis", "repoPath", repoPath)

	// Test authentication first
	repo.TestProbotAuth(ctx, repoName)

	// Create branch on GitHub
	if err := repo.CreateBranch(ctx, repoName, issueNumber, issueTitle); err != nil {
		slog.Error("Failed to create branch", "error", err)
		return err
	}
	if err := repo.CleanupRepo(repoPath); err != nil {
		slog.Error("Failed to cleanup ", "repoPath", repoPath)
	}

	slog.Info(" Issue processing completed", "issueNumber", issueNumber)
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
	// TODO: Make these dynamic - fetch from config/database/environment
	requiredLabels := []string{
		"auto-fix",
		"devflow-agent-automate",
		"bug-fix",
	}

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
