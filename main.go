package main

import (
    "log"
    "net/http"
    "os"
    
    "github.com/joho/godotenv"
)

func main() {
    // Load .env file
    if err := godotenv.Load(); err != nil {
        log.Println("No .env file found")
    }

    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    http.HandleFunc("/", handleHome)
    http.HandleFunc("/install", handleInstall) 
    http.HandleFunc("/auth/callback", handleCallback)
    http.HandleFunc("/webhook", handleWebhook)

    log.Printf("Server starting on http://localhost:%s", port)
    log.Fatal(http.ListenAndServe(":"+port, nil))
}