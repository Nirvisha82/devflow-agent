package repository

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/go-github/github"
	"github.com/swinton/go-probot/probot"
)

func CreateBranch(ctx *probot.Context, repoName string, issueNumber int, issueTitle string) error {

	// Split repo name
	parts := strings.Split(repoName, "/")
	owner := parts[0]
	repo := parts[1]

	// Get main branch reference
	mainRef, _, err := ctx.GitHub.Git.GetRef(context.Background(), owner, repo, "refs/heads/main")
	if err != nil {
		slog.Error("Clone Failed", "error", err)
		return err
	}

	branchName := fmt.Sprintf("issue-%d-%s", issueNumber, SanitizeBranchName(issueTitle))
	slog.Info("Creating branch on GitHub", "branch", branchName)
	// Create new branch reference
	newRef := &github.Reference{
		Ref: github.String("refs/heads/" + branchName),
		Object: &github.GitObject{
			SHA: mainRef.Object.SHA,
		},
	}

	_, _, err = ctx.GitHub.Git.CreateRef(context.Background(), owner, repo, newRef)
	if err != nil {
		slog.Error("Failed to create a Branch", "error", err)
		return err
	}

	slog.Info("Branch created on GitHub", "branch", branchName)
	return nil
}

func SanitizeBranchName(title string) string {
	sanitized := strings.ReplaceAll(title, " ", "-")
	sanitized = strings.ToLower(sanitized)
	if len(sanitized) > 20 {
		sanitized = sanitized[:20]
	}
	return sanitized
}
