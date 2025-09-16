package main

import (
	"log"
	"os"

	"github.com/google/go-github/github"
	"github.com/joho/godotenv"
	"github.com/swinton/go-probot/probot"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	keyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")
	if keyPath != "" {
		keyData, err := os.ReadFile(keyPath)
		if err != nil {
			log.Printf("Failed to read private key: %v", err)
		} else {
			os.Setenv("GITHUB_APP_PRIVATE_KEY", string(keyData))
			log.Printf("Private key loaded from: %s", keyPath)
			log.Printf("Private key starts with: %s", string(keyData)[:50]) // First 50 characters
		}
	}

	// Now you can access environment variables
	appID := os.Getenv("GITHUB_APP_ID")
	log.Printf("App ID: %s", appID)

	probot.HandleEvent("issues", func(ctx *probot.Context) error {
		// Because we're listening for "issues" we know the payload is a *github.IssuesEvent
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
		repoPath, cleanup, err := cloneRepositoryTemp(repoName)
		if err != nil {
			log.Printf("Failed to clone repository: %v", err)
			return nil
		}
		defer cleanup()

		log.Printf("📁 Repository ready for analysis at: %s", repoPath)

		// Test authentication first
		testProbotAuth(ctx, repoName)

		// Create branch on GitHub
		if err := createBranchWithProbot(ctx, repoName, issueNumber, issueTitle); err != nil {
			log.Printf("Failed to create branch: %v", err)
		}

		return nil
	})
	probot.Start()
}
