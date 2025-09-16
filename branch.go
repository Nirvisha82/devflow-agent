package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/google/go-github/github"
	"github.com/swinton/go-probot/probot"
)

func createBranchWithProbot(ctx *probot.Context, repoName string, issueNumber int, issueTitle string) error {
	branchName := fmt.Sprintf("issue-%d-%s", issueNumber, sanitizeBranchName(issueTitle))

	log.Printf("🌿 Creating branch on GitHub: %s", branchName)

	// Split repo name
	parts := strings.Split(repoName, "/")
	owner := parts[0]
	repo := parts[1]

	// Get main branch reference
	mainRef, _, err := ctx.GitHub.Git.GetRef(context.Background(), owner, repo, "refs/heads/main")
	if err != nil {
		log.Printf("❌ Failed to get main ref: %v", err)
		return err
	}

	// Create new branch reference
	newRef := &github.Reference{
		Ref: github.String("refs/heads/" + branchName),
		Object: &github.GitObject{
			SHA: mainRef.Object.SHA,
		},
	}

	_, _, err = ctx.GitHub.Git.CreateRef(context.Background(), owner, repo, newRef)
	if err != nil {
		log.Printf("❌ Failed to create branch: %v", err)
		return err
	}

	log.Printf("✅ Branch created on GitHub: %s", branchName)
	return nil
}

func sanitizeBranchName(title string) string {
	sanitized := strings.ReplaceAll(title, " ", "-")
	sanitized = strings.ToLower(sanitized)
	if len(sanitized) > 20 {
		sanitized = sanitized[:20]
	}
	return sanitized
}
