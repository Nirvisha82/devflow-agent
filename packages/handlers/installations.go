package handlers

import (
	"log/slog"
	"strings"

	repoActions "devflow-agent/packages/repository"

	"github.com/google/go-github/github"
	"github.com/swinton/go-probot/probot"
)

func HandleInstallations(ctx *probot.Context) error {
	event := ctx.Payload.(*github.InstallationRepositoriesEvent)
	action := event.GetAction()

	slog.Info("Installation Action:", "action", action)

	switch action {
	case "added":
		return handleRepositoriesAdded(ctx, event.RepositoriesAdded)
	case "removed":
		return handleRepositoriesRemoved(ctx, event.RepositoriesRemoved)
	}

	return nil
}

func handleRepositoriesAdded(ctx *probot.Context, repos []*github.Repository) error {
	for _, repo := range repos {
		fullName := repo.GetFullName()

		// Parse owner from full name
		parts := strings.Split(fullName, "/")
		if len(parts) != 2 {
			slog.Error("Invalid repository full name", "fullName", fullName)
			continue
		}

		owner := parts[0]
		name := parts[1]

		slog.Info("Repository details:",
			"fullName", fullName,
			"owner", owner,
			"name", name)
		// Add custom labels to newly installed repositories
		if err := repoActions.AddCustomLabels(ctx, owner, name); err != nil {
			slog.Error("Failed to add labels", "repo", repo.GetFullName(), "error", err)
			continue
		}
	}
	return nil
}

func handleRepositoriesRemoved(ctx *probot.Context, repos []*github.Repository) error {
	for _, repo := range repos {
		fullName := repo.GetFullName()

		// Parse owner from full name
		parts := strings.Split(fullName, "/")
		if len(parts) != 2 {
			slog.Error("Invalid repository full name", "fullName", fullName)
			continue
		}

		owner := parts[0]
		name := parts[1]

		slog.Info("Repository removed",
			"fullName", fullName,
			"owner", owner,
			"name", name)

		// Labels can't be cleaned up since access to the repo is removed.
		// if err := repoActions.RemoveCustomLabels(ctx, owner, name); err != nil {
		// 	slog.Error("Failed to cleanup repository", "repo", fullName, "error", err)
		// 	continue
		// }
	}
	return nil
}
