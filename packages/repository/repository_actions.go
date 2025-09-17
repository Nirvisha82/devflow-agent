package repository

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/swinton/go-probot/probot"
)

func CloneRepository(repoName string) (string, error) {
	cloneURL := fmt.Sprintf("https://github.com/%s.git", repoName)
	repoDir := fmt.Sprintf("temp_%s_%d", strings.Replace(repoName, "/", "_", -1), time.Now().Unix())

	slog.Info("Cloning", "repo", repoName)

	cmd := exec.Command("git", "clone", "--depth=1", cloneURL, repoDir)
	_, err := cmd.CombinedOutput()

	if err != nil {
		slog.Error("Clone Failed", "error", err)
		return "", err
	}

	slog.Info("Repository cloned to", "repoDir", repoDir)

	// Return cleanup function

	return repoDir, nil
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
