package main

import (
	"log"
	"os"

	"devflow-agent/packages/handlers"

	"github.com/joho/godotenv"
	"github.com/swinton/go-probot/probot"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Load private key
	loadPrivateKey()

	// Log app ID
	appID := os.Getenv("GITHUB_APP_ID")
	log.Printf("App ID: %s", appID)

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
			log.Printf("Failed to read private key: %v", err)
		} else {
			os.Setenv("GITHUB_APP_PRIVATE_KEY", string(keyData))
			log.Printf("Private key loaded from: %s", keyPath)
			log.Printf("Private key starts with: %s", string(keyData)[:50])
		}
	}
}
