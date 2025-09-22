package repository

import (
	"context"
	"log/slog"

	"github.com/google/go-github/github"
	"github.com/swinton/go-probot/probot"
)

// Define your custom labels
var customLabels = []*github.Label{
	{
		Name:        github.String("devflow-agent-automate"),
		Color:       github.String("d73a4a"),
		Description: github.String("NewBranch-Analysis-Suggestions-PR"),
	},
	{
		Name:        github.String("devflow-agent-bug-fix"),
		Color:       github.String("a2eeef"),
		Description: github.String("NewBranch-Analysis-Action-PR"),
	},
	// more labels as needed
}

func AddCustomLabels(ctx *probot.Context, owner, repo string) error {
	client := ctx.GitHub

	for _, label := range customLabels {
		// Check if label exists, create if it doesn't
		_, _, err := client.Issues.GetLabel(context.Background(), owner, repo, label.GetName())
		if err != nil {
			// Label doesn't exist, create it
			_, _, err := client.Issues.CreateLabel(context.Background(), owner, repo, label)
			if err != nil {
				slog.Error("Failed to create label", "label", label.GetName(), "error", err)
				continue
			}
			slog.Info("Created label", "label", label.GetName(), "repo", owner+"/"+repo)
		} else {
			slog.Info("Label already exists", "label", label.GetName(), "repo", owner+"/"+repo)
		}
	}

	return nil
}

func RemoveCustomLabels(ctx *probot.Context, owner, repo string) error {
	client := ctx.GitHub

	for _, label := range customLabels {
		labelName := label.GetName()

		// Check if label exists before trying to delete
		_, resp, err := client.Issues.GetLabel(context.Background(), owner, repo, labelName)
		if err != nil {
			if resp != nil && resp.StatusCode == 404 {
				slog.Info("Label doesn't exist (already removed)", "label", labelName, "repo", owner+"/"+repo)
				continue
			}
			slog.Error("Error checking label", "label", labelName, "error", err)
			continue
		}

		// Delete the label
		_, err = client.Issues.DeleteLabel(context.Background(), owner, repo, labelName)
		if err != nil {
			slog.Error("Failed to delete label", "label", labelName, "repo", owner+"/"+repo, "error", err)
			continue
		}

		slog.Info("Deleted label", "label", labelName, "repo", owner+"/"+repo)
	}

	return nil
}
