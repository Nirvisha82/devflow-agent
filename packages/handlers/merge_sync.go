package handlers

import (
	"log/slog"

	"devflow-agent/packages/repository"

	"github.com/google/go-github/github"
	"github.com/swinton/go-probot/probot"
)

// Triggered on PR close; if merged into default branch, sync .devflow incrementally.
func HandlePullRequest(ctx *probot.Context) error {
	ev := ctx.Payload.(*github.PullRequestEvent)
	if ev.GetAction() != "closed" || !ev.PullRequest.GetMerged() {
		return nil
	}
	if ev.PullRequest.Base.GetRef() != "main" { // optional: only if merged into main
		return nil
	}

	baseRef := ev.PullRequest.Base.GetRef() // e.g., "main"
	repoName := ev.Repo.GetFullName()

	slog.Info("PR closed event", "repo", repoName, "base", baseRef, "merged", true)

	// Clone and sync against origin/main
	repoPath, _, err := repository.CloneRepository(repoName)
	if err != nil {
		slog.Error("Clone failed for merge sync", "error", err)
		return err
	}
	defer func() { _ = repository.CleanupRepo(repoPath) }()

	headSHA, err := repository.GetOriginMainSHA(repoPath)
	if err != nil {
		slog.Error("Resolve origin/main failed", "error", err)
		return err
	}
	if err := repository.RunIncrementalDevflowSync(ctx, repoName, repoPath, headSHA); err != nil {
		slog.Error("Incremental devflow sync (PR) failed", "error", err)
		return err
	}
	return nil
}

// Triggered on any push; if branch is main, sync .devflow incrementally.
func HandlePush(ctx *probot.Context) error {
	ev := ctx.Payload.(*github.PushEvent)
	ref := ev.GetRef() // e.g., "refs/heads/main"
	repoName := ev.Repo.GetFullName()

	if ref != "refs/heads/main" {
		return nil
	}

	slog.Info("Push to main detected", "repo", repoName)

	repoPath, _, err := repository.CloneRepository(repoName)
	if err != nil {
		slog.Error("Clone failed for push sync", "error", err)
		return err
	}
	defer func() { _ = repository.CleanupRepo(repoPath) }()

	headSHA, err := repository.GetOriginMainSHA(repoPath)
	if err != nil {
		slog.Error("Resolve origin/main failed", "error", err)
		return err
	}
	if err := repository.RunIncrementalDevflowSync(ctx, repoName, repoPath, headSHA); err != nil {
		slog.Error("Incremental devflow sync (push) failed", "error", err)
		return err
	}
	return nil
}
