package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/swinton/go-probot/probot"
)

func cloneRepositoryTemp(repoName string) (string, func(), error) {
	cloneURL := fmt.Sprintf("https://github.com/%s.git", repoName)
	repoDir := fmt.Sprintf("temp_%s_%d", strings.Replace(repoName, "/", "_", -1), time.Now().Unix())

	log.Printf("🔄 Cloning: %s", repoName)

	cmd := exec.Command("git", "clone", "--depth=1", cloneURL, repoDir)
	_, err := cmd.CombinedOutput()

	if err != nil {
		log.Printf("❌ Clone failed: %v", err)
		return "", nil, err
	}

	log.Printf("✅ Repository cloned to: %s", repoDir)

	// Return cleanup function
	cleanup := func() {
		os.RemoveAll(repoDir)
		log.Printf("🗑️ Cleaned up: %s", repoDir)
	}

	return repoDir, cleanup, nil
}

func testProbotAuth(ctx *probot.Context, repoName string) {
	parts := strings.Split(repoName, "/")
	owner := parts[0]
	repo := parts[1]

	log.Printf("🔍 Testing probot authentication...")

	// Try a simple API call
	repository, _, err := ctx.GitHub.Repositories.Get(context.Background(), owner, repo)
	if err != nil {
		log.Printf("❌ Auth test failed: %v", err)
		return
	}

	log.Printf("✅ Auth test passed! Repo: %s, Default branch: %s",
		repository.GetFullName(), repository.GetDefaultBranch())
}
