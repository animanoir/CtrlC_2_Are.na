package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/atotto/clipboard"
	"github.com/joho/godotenv"
)

const (
	arenaAPIEndpoint = "https://api.are.na/v2/channels/%s/blocks" // %s will be replaced by the channelSlug
	checkInterval    = 2 * time.Second                            // Interval for checking the clipboard
)

// Structure for the Are.na API payload (simplified)
type ArenaBlock struct {
	Content string `json:"content"`
	// You could add "source" if you could get the URL/path of the PDF,
	// but it's difficult to get it automatically from the clipboard.
	// Source string `json:"source,omitempty"`
}

func main() {
	// --- Configuration ---
	envErr := godotenv.Load()
	if envErr != nil {
		log.Fatal(".env file couldn't be loaded.")
	}
	accessToken := os.Getenv("ARENA_PERSONAL_ACCESS_TOKEN")
	channelSlug := os.Getenv("ARENA_CHANNEL_SLUG")

	if accessToken == "" || channelSlug == "" {
		log.Fatal("Error: You must set the ARENA_PERSONAL_ACCESS_TOKEN and ARENA_CHANNEL_SLUG environment variables.")
	}

	fmt.Println("🚀 Starting clipboard monitor for Are.na...")
	fmt.Printf("➡️  Sending to channel: %s\n", channelSlug)
	fmt.Println("📋 Highlight text in SumatraPDF, copy it (Ctrl+C), and it will be sent to Are.na.")
	fmt.Println("ℹ️  Press Ctrl+C in this terminal to stop.")

	// --- Clipboard Monitoring ---
	var lastClipboardContent string
	var err error

	// Initialize with the current content to avoid sending at startup
	lastClipboardContent, err = clipboard.ReadAll()
	if err != nil {
		log.Printf("Warning: Could not read the initial clipboard content: %v\n", err)
	}

	// Channel for handling interrupt signal (Ctrl+C)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			currentClipboardContent, err := clipboard.ReadAll()
			if err != nil {
				// Ignore temporary read errors, but log if useful
				// log.Printf("Error reading clipboard: %v\n", err)
				continue
			}

			// If the content changed and is not empty
			if currentClipboardContent != lastClipboardContent && currentClipboardContent != "" {
				fmt.Printf("✨ New content detected: \"%s...\"\n", currentClipboardContent)
				lastClipboardContent = currentClipboardContent // Update the last content

				// Send to Are.na in a goroutine to avoid blocking the check
				go sendToArena(accessToken, channelSlug, currentClipboardContent)
			}

		case <-sigChan:
			fmt.Println("\n🛑 Stopping the monitor...")
			return // Exit the program
		}
	}
}

// sendToArena sends the text as a block to the specified Are.na channel
func sendToArena(token, channelSlug, content string) {
	apiURL := fmt.Sprintf(arenaAPIEndpoint, channelSlug)

	blockData := ArenaBlock{
		Content: content,
	}

	jsonData, err := json.Marshal(blockData)
	if err != nil {
		log.Printf("❌ Error encoding JSON: %v\n", err)
		return
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("❌ Error creating HTTP request: %v\n", err)
		return
	}

	// Set Headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Go CTRL-C-Arena Connector (https://github.com/animanoir)") // Good practice

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Error sending request to Are.na: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		fmt.Printf("✅ Sent to Are.na! (Status: %d)\n", resp.StatusCode)
	} else {
		// Read response body for more error details
		var bodyBytes []byte
		if resp.Body != nil {
			bodyBytes, _ = ReadAll(resp.Body) // Use io.ReadAll in Go 1.16+ or ioutil.ReadAll
		}
		log.Printf("❌ Error sending to Are.na. Status: %d, Response: %s\n", resp.StatusCode, string(bodyBytes))
	}
}

// ReadAll helper (if using Go < 1.16, use ioutil.ReadAll)
func ReadAll(r io.Reader) ([]byte, error) {
	b := bytes.NewBuffer(make([]byte, 0, 512))
	_, err := io.Copy(b, r)
	return b.Bytes(), err
}
