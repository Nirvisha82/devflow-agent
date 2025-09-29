package repository

import (
	"context"
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
	cloneURL := fmt.Sprintf("https://github.com/%s.git", repoName)
	repoDir := fmt.Sprintf("temp_repo_%s_%d", strings.Replace(repoName, "/", "_", -1), time.Now().Unix())

	slog.Info("Cloning", "repo", repoName)

	cmd := exec.Command("git", "clone", "--depth=1", cloneURL, repoDir)
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
