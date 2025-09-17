package main

import (
	"log/slog"
	"os"

	"devflow-agent/packages/handlers"

	"github.com/joho/godotenv"
	"github.com/swinton/go-probot/probot"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		slog.Error("No .env file found")
	}

	// Load private key
	loadPrivateKey()

	// Log app ID
	appID := os.Getenv("GITHUB_APP_ID")
	slog.Info("App ID: ", "appID", appID)

	// Register event handlers
	probot.HandleEvent("issues", handlers.HandleIssues)

	// Start the bot
	probot.Start()
}

func loadPrivateKey() {
	keyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")
	if keyPath != "" {
		keyData, err := os.ReadFile(keyPath)
		if err != nil {
			slog.Error("Failed to read private key", "error", err)
		} else {
			os.Setenv("GITHUB_APP_PRIVATE_KEY", string(keyData))
			slog.Info("Private key loaded from", "keyPath", keyPath)
			slog.Info("Private key starts with", "keyData", string(keyData)[:50])
		}
	}
}
