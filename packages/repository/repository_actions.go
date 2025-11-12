package repository

import (
	"context"
	"devflow-agent/packages/config"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"github.com/swinton/go-probot/probot"
)

func CloneRepository(repoName string) (string, string, error) {
	cfg := config.GetConfig()
	cloneURL := fmt.Sprintf("https://github.com/%s.git", repoName)
	repoDir := fmt.Sprintf("%s%s_%d", cfg.Repository.TempRepoPrefix, strings.Replace(repoName, "/", "_", -1), time.Now().Unix())

	slog.Info("Cloning", "repo", repoName)

	cmd := exec.Command("git", "clone", fmt.Sprintf("--depth=%d", cfg.Repository.CloneDepth), cloneURL, repoDir)
	_, err := cmd.CombinedOutput()

	if err != nil {
		slog.Error("Clone Failed", "error", err)
		return "", "", err
	}

	slog.Info("Repository cloned to", "repoDir", repoDir)

	// Return cleanup function

	return repoDir, cloneURL, nil
}

func AnalyzeRepo(ctx *probot.Context, outputFile, LocalPath, repoURL string) error {

	fmt.Printf("Creating analysis of: %s\n", repoURL)

	analyzer := &RepoAnalyzer{
		LocalPath:  LocalPath,
		RepoURL:    repoURL,
		OutputFile: outputFile,
		Files:      make([]FileInfo, 0),
	}

	if err := analyzer.Generate(); err != nil {
		slog.Error("Failed to generate analysis", "error", err)
		return err
	}

	fmt.Printf("Repository analysis saved to: %s\n", outputFile)
	return nil
}
func CommitFile(ctx *probot.Context, repoName, branchName, commitMessage, filePath string) error {
	parts := strings.Split(repoName, "/")
	owner := parts[0]
	repo := parts[1]

	fileName := filepath.Base(filePath)

	// Read the analysis file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		slog.Error("Failed to read the file", "File Path", filePath, "error", err)
		return err
	}

	// Create file content for GitHub API
	fileContent := &github.RepositoryContentFileOptions{
		Message: github.String(commitMessage),
		Content: content,
		Branch:  github.String(branchName),
	}

	// Commit the file to the branch
	_, _, err = ctx.GitHub.Repositories.CreateFile(
		context.Background(),
		owner,
		repo,
		fileName, // File path in repo
		fileContent,
	)

	if err != nil {
		slog.Error("Failed to commit analysis file", "error", err)
		return err
	}

	slog.Info("Analysis file committed to branch", "branch", branchName, "file", fileName)
	return nil
}

func CommitMultipleFiles(ctx *probot.Context, repoName, branchName, commitMessage string, filePaths []string, init bool, repoPath string) error {
	parts := strings.Split(repoName, "/")
	if len(parts) != 2 {
		slog.Error("Invalid repository name format", "repoName", repoName)
		return errors.New("invalid repository name format, expected 'owner/repo'")
	}
	owner := parts[0]
	repo := parts[1]

	slog.Info("Committing multiple files to branch", "branch", branchName, "fileCount", len(filePaths))

	// ✅ Use "heads/<branch>" (NOT "refs/heads/<branch>")
	ref, _, err := ctx.GitHub.Git.GetRef(context.Background(), owner, repo, "heads/"+branchName)
	if err != nil {
		slog.Error("Failed to get branch reference", "error", err, "branch", branchName)
		return err
	}

	// Get the tree SHA from the current commit
	commit, _, err := ctx.GitHub.Git.GetCommit(context.Background(), owner, repo, ref.Object.GetSHA())
	if err != nil {
		slog.Error("Failed to get commit", "error", err, "sha", ref.Object.GetSHA())
		return err
	}

	// Create tree entries for all files
	var entries []*github.TreeEntry
	for _, filePath := range filePaths {
		// Read file content from the local repo checkout
		content, err := os.ReadFile(filePath)
		if err != nil {
			slog.Error("Failed to read file locally", "file", filePath, "error", err)
			return err
		}

		// Compute repo-relative path
		repoFilePath, err := filepath.Rel(repoPath, filePath)
		if err != nil {
			return fmt.Errorf("failed to calculate relative path for %s using root %s: %w", filePath, repoPath, err)
		}

		// If this is the "init" case, place files under .devflow/
		if init {
			fileName := filepath.Base(filePath)
			repoFilePath = ".devflow/" + fileName
		}

		// ✅ CRITICAL: normalize path to POSIX (Git tree paths must use forward slashes)
		repoFilePath = filepath.ToSlash(repoFilePath)
		// Trim any accidental "./"
		repoFilePath = strings.TrimPrefix(repoFilePath, "./")
		// Safety: do not allow escaping the repo root
		if strings.HasPrefix(repoFilePath, "../") {
			return fmt.Errorf("refusing to commit path outside repo: %s", repoFilePath)
		}

		contentStr := string(content)

		// Create blob
		blob := &github.Blob{
			Content:  &contentStr,
			Encoding: github.String("utf-8"),
		}
		createdBlob, _, err := ctx.GitHub.Git.CreateBlob(context.Background(), owner, repo, blob)
		if err != nil {
			slog.Error("Failed to create blob for content", "repoPath", repoFilePath, "error", err)
			return err
		}

		// Create tree entry (path MUST be POSIX style)
		entry := &github.TreeEntry{
			Path: github.String(repoFilePath),
			Mode: github.String("100644"),
			Type: github.String("blob"),
			SHA:  createdBlob.SHA,
		}
		entries = append(entries, entry)
	}

	// Create new tree against current base tree
	treeEntries := make([]github.TreeEntry, len(entries))
	for i, entry := range entries {
		treeEntries[i] = *entry
	}
	newTree, _, err := ctx.GitHub.Git.CreateTree(context.Background(), owner, repo, commit.Tree.GetSHA(), treeEntries)
	if err != nil {
		slog.Error("Failed to create tree", "error", err)
		return err
	}

	// Create new commit
	newCommit := &github.Commit{
		Message: github.String(commitMessage),
		Tree:    newTree,
		Parents: []github.Commit{*commit},
	}
	createdCommit, _, err := ctx.GitHub.Git.CreateCommit(context.Background(), owner, repo, newCommit)
	if err != nil {
		slog.Error("Failed to create commit", "error", err)
		return err
	}

	// Move branch to the new commit
	ref.Object.SHA = createdCommit.SHA
	_, _, err = ctx.GitHub.Git.UpdateRef(context.Background(), owner, repo, ref, false)
	if err != nil {
		slog.Error("Failed to update branch reference", "error", err)
		return err
	}

	slog.Info("Successfully committed multiple files",
		"branch", branchName, "fileCount", len(filePaths), "commit", createdCommit.GetSHA())
	return nil
}

// CreatePullRequest creates a pull request from the specified branch to the default branch
func CreatePullRequest(ctx *probot.Context, repoName, branchName, title, body string) (*github.PullRequest, error) {
	cfg := config.GetConfig()
	parts := strings.Split(repoName, "/")
	owner := parts[0]
	repo := parts[1]

	slog.Info("Creating pull request", "repo", repoName, "branch", branchName, "title", title)

	// Create the pull request
	newPR := &github.NewPullRequest{
		Title:               github.String(title),
		Head:                github.String(branchName),
		Base:                github.String(cfg.Repository.DefaultBranch),
		Body:                github.String(body),
		MaintainerCanModify: github.Bool(true),
	}

	pr, _, err := ctx.GitHub.PullRequests.Create(context.Background(), owner, repo, newPR)
	if err != nil {
		slog.Error("Failed to create pull request", "error", err)
		return nil, err
	}

	slog.Info("Pull request created successfully",
		"prNumber", pr.GetNumber(),
		"prURL", pr.GetHTMLURL(),
		"branch", branchName)

	return pr, nil
}

// CreateInstallationPR creates a PR for the installation workflow
func CreateInstallationPR(ctx *probot.Context, repoName, branchName string) (*github.PullRequest, error) {
	cfg := config.GetConfig()

	// Read title from file
	titleBytes, err := os.ReadFile(cfg.PullRequests.Installation.TitleFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read PR title template: %w", err)
	}
	title := strings.TrimSpace(string(titleBytes))

	// Read body from file
	bodyBytes, err := os.ReadFile(cfg.PullRequests.Installation.BodyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read PR body template: %w", err)
	}
	body := string(bodyBytes)

	return CreatePullRequest(ctx, repoName, branchName, title, body)
}

// CreateIssueResolutionPR creates a PR for issue resolution workflow
func CreateIssueResolutionPR(ctx *probot.Context, repoName, branchName string, issueNumber int, issueTitle, changesSummary, implementationDetails, testingNotes string) (*github.PullRequest, error) {
	cfg := config.GetConfig()

	// Read title template from file
	titleBytes, err := os.ReadFile(cfg.PullRequests.IssueResolution.TitleFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read PR title template: %w", err)
	}
	title := strings.TrimSpace(string(titleBytes))

	// Read body template from file
	bodyBytes, err := os.ReadFile(cfg.PullRequests.IssueResolution.BodyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read PR body template: %w", err)
	}
	body := string(bodyBytes)

	// Replace template variables in title
	title = strings.ReplaceAll(title, "{issue_number}", fmt.Sprintf("%d", issueNumber))
	title = strings.ReplaceAll(title, "{issue_title}", issueTitle)

	// Replace template variables in body
	body = strings.ReplaceAll(body, "{issue_number}", fmt.Sprintf("%d", issueNumber))
	body = strings.ReplaceAll(body, "{issue_title}", issueTitle)
	body = strings.ReplaceAll(body, "{changes_summary}", changesSummary)
	body = strings.ReplaceAll(body, "{implementation_details}", implementationDetails)
	body = strings.ReplaceAll(body, "{testing_notes}", testingNotes)

	return CreatePullRequest(ctx, repoName, branchName, title, body)
}

// CreateIssueResolutionPRSimple creates a PR for issue resolution with minimal info (for current workflow)
func CreateIssueResolutionPRSimple(ctx *probot.Context, repoName, branchName string, issueNumber int, issueTitle string) (*github.PullRequest, error) {
	changesSummary := "Knowledge base initialization and analysis files"
	implementationDetails := "Generated comprehensive repository analysis and knowledge base files"
	testingNotes := "Auto-generated files - no manual testing required"

	return CreateIssueResolutionPR(ctx, repoName, branchName, issueNumber, issueTitle, changesSummary, implementationDetails, testingNotes)
}

func TestProbotAuth(ctx *probot.Context, repoName string) {
	parts := strings.Split(repoName, "/")
	owner := parts[0]
	repo := parts[1]

	slog.Info("Testing probot authentication.")

	// Try a simple API call
	repository, _, err := ctx.GitHub.Repositories.Get(context.Background(), owner, repo)
	if err != nil {
		slog.Error("Auth Test Failed", "error", err)
		return
	}

	slog.Info("Auth test passed! Repo: %s, Default branch: %s",
		repository.GetFullName(), repository.GetDefaultBranch())
}

func CleanupRepo(repoDir string) error {
	err := os.RemoveAll(repoDir)
	if err == nil {
		slog.Info("Cleaned up", "repoDir", repoDir)
		return nil
	}
	return err
}

func SaveAnalysisToFile(content, filePath string) error {
	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		slog.Error("Failed to save analysis file", "error", err)
		return err
	}
	slog.Info("Analysis saved successfully", "file", filePath)
	return nil
}
