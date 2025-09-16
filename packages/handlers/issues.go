package handlers

import (
	"devflow-agent/packages/repository"
	repo "devflow-agent/packages/repository"
	"log"
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

	log.Printf("📝 Issue Action: %s", action)
	log.Printf("📝 Issue #%d: %s", issueNumber, issueTitle)
	log.Printf("📝 Repository: %s", repoName)

	// Check if issue has required labels before proceeding
	if !hasRequiredLabels(event.Issue.Labels) {
		log.Printf("⏭️ Skipping issue #%d - missing required labels", issueNumber)
		return nil
	}

	log.Printf("✅ Issue #%d has required labels - proceeding with workflow", issueNumber)

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

// hasRequiredLabels checks if the issue has any of the required labels
// Changed parameter type from []*github.Label to []github.Label
func hasRequiredLabels(labels []github.Label) bool {
	// TODO: Make these dynamic - fetch from config/database/environment
	requiredLabels := []string{
		"auto-fix",
		"devflow-agent-automate",
		"enhancement",
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
			log.Printf("🏷️ Found required label: %s", requiredLabel)
			return true
		}
	}

	log.Printf("🏷️ Required labels not found. Issue has labels: %v", getIssueLabelNames(labels))
	return false
}

// Helper function to get label names for logging
// Changed parameter type from []*github.Label to []github.Label
func getIssueLabelNames(labels []github.Label) []string {
	var labelNames []string
	for _, label := range labels {
		if label.Name != nil {
			labelNames = append(labelNames, *label.Name)
		}
	}
	return labelNames
}
