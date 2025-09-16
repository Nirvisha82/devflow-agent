package handlers

import (
	"devflow-agent/packages/repository"
	"log"

	repo "devflow-agent/packages/repository"

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

	log.Printf("📝 Issue Action: %s", action)
	log.Printf("📝 Issue #%d: %s", issueNumber, issueTitle)
	log.Printf("📝 Repository: %s", repoName)

	// Clone repository temporarily
	repoPath, cleanup, err := repository.CloneRepositoryTemp(repoName)
	if err != nil {
		log.Printf("Failed to clone repository: %v", err)
		return nil
	}
	defer cleanup()

	log.Printf("📁 Repository ready for analysis at: %s", repoPath)

	// Test authentication first
	repo.TestProbotAuth(ctx, repoName)

	// Create branch on GitHub
	if err := repo.CreateBranchWithProbot(ctx, repoName, issueNumber, issueTitle); err != nil {
		log.Printf("Failed to create branch: %v", err)
	}

	return nil
}
