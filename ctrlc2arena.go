package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/atotto/clipboard"
	"github.com/joho/godotenv"
)

// Are.na theme
type arenaTheme struct{}

var _ fyne.Theme = (*arenaTheme)(nil)

func (m *arenaTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameBackground {
		return color.Black
	}
	return theme.DarkTheme().Color(name, variant)
}

func (m *arenaTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DarkTheme().Font(style)
}

func (m *arenaTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DarkTheme().Icon(name)
}

func (m *arenaTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInnerPadding:
		return 5
	}
	return theme.DarkTheme().Size(name)
}

const (
	arenaAPIEndpoint = "https://api.are.na/v2/channels/%s/blocks" // %s will be replaced by the channelSlug
	checkInterval    = 2 * time.Second                            // Interval for checking the clipboard
)

// Structure for the Are.na API payload (simplified)
type ArenaBlock struct {
	Content string `json:"content"`
}

var isMonitoring bool = false

func main() {

	var userArenaToken string
	var userSlugChannel string

	// -- GUI Stuff --
	a := app.New()
	a.Settings().SetTheme(&arenaTheme{})
	w := a.NewWindow("CTRL+C to Are.na")
	w.Resize(fyne.NewSize(600, 500))
	w.CenterOnScreen()

	arenaLogoImg := canvas.NewImageFromFile("arena-logo-white.png")
	arenaLogoImg.FillMode = canvas.ImageFillContain
	arenaLogoImg.SetMinSize(fyne.NewSize(70, 50))

	grayColor := color.NRGBA{R: 178, G: 178, B: 178, A: 255}
	whiteColor := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	title := canvas.NewText("Ctrl+C to Are.na", whiteColor)
	statusIcon := widget.NewIcon(nil)
	statusText := canvas.NewText("", whiteColor)
	statusText.TextSize = 14
	stopButton := widget.NewButtonWithIcon("Stop monitoring", theme.MediaStopIcon(), func() {
		log.Print("stop button pressed")
		isMonitoring = false
	})
	stopButton.Hide()
	statusBox := container.NewHBox(statusIcon, statusText, stopButton)
	statusBox.Hide()
	stopButton.Hide()
	title.TextSize = 42
	infoText := canvas.NewText("This lil' software will monitor and send whatever TEXT you copy (CTRL+C) into a specified channel in your Are.na account.", grayColor)
	arenaTokenEntry := widget.NewEntry()
	arenaSlugEntry := widget.NewEntry()
	blockTitle := widget.NewEntry()
	parsedURL, err := url.Parse("https://dev.are.na/")
	if err != nil {
		return
	}

	arenaApiUrl := widget.NewHyperlink("Click here to get your Are.na API token.", parsedURL)

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Are.na token:", Widget: arenaTokenEntry},
			{Text: "Channel slug:", Widget: arenaSlugEntry},
			{Text: "Block title:", Widget: blockTitle}},
		SubmitText: "Connect",
	}
	form.OnSubmit = func() {
		userArenaToken = arenaTokenEntry.Text
		userSlugChannel = arenaSlugEntry.Text
		log.Println("Are.na token: ", arenaTokenEntry.Text)
		log.Println("Are.na slug channel: ", arenaSlugEntry.Text)
		if userArenaToken != "" && userSlugChannel != "" {
			isMonitoring = true
			go clipboardMonitoring(userArenaToken, userSlugChannel)

			statusText.Text = "The software is now monitoring your clipboard—be careful..."
			statusText.Color = theme.WarningColor()
			statusText.Refresh()
			statusIcon.SetResource(theme.MediaVideoIcon())
			statusBox.Show()
			form.Disable()
			stopButton.Show()
		}
	}

	arenaTokenEntry.Validator = func(text string) error {
		if len(strings.TrimSpace(text)) == 0 {
			return fmt.Errorf("token is required")
		}
		return nil
	}

	arenaSlugEntry.Validator = func(text string) error {
		if len(strings.TrimSpace(text)) == 0 {
			return fmt.Errorf("channel slug is required")
		}
		return nil
	}
	layoutHeader := container.NewHBox(title, arenaLogoImg)
	content := container.NewVBox(
		layoutHeader,
		widget.NewSeparator(),
		infoText,
		widget.NewSeparator(),
		arenaApiUrl,
		form,
		statusBox,
	)

	paddedContent := container.NewPadded(content)

	w.SetContent(paddedContent)

	w.ShowAndRun()
}

func clipboardMonitoring(_accessToken string, _channelSlug string, _blocklTitle string) {
	if isMonitoring {
		fmt.Print("clipboardMonitoring func executing...")
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
					fmt.Printf("✨ New content detected: ")
					fmt.Println(strings.ReplaceAll(currentClipboardContent, "\r\n", " "))
					lastClipboardContent = currentClipboardContent // Update the last content

					// Send to Are.na in a goroutine to avoid blocking the check
					go sendToArena(_accessToken, _channelSlug, lastClipboardContent)
				}

			case <-sigChan:
				fmt.Println("\n🛑 Stopping the monitor...")
				isMonitoring = false
				return // Exit the program
			}
		}
	} else {
		log.Print("clipboard function not executing.")
		return
	}

}

// sendToArena sends the text as a block to the specified Are.na channel
func sendToArena(token, channelSlug, content string) {
	// Formats the text before sending
	formattedContent := strings.ReplaceAll(content, "\r\n", " ")

	apiURL := fmt.Sprintf(arenaAPIEndpoint, channelSlug)

	blockData := ArenaBlock{
		Content: formattedContent,
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
	req.Header.Set("User-Agent", "Go CTRL+C2Arena Connector (https://github.com/animanoir)")

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
			bodyBytes, _ = ReadAll(resp.Body)
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

func configuration(_accessToken string, _channelSlug string) {
	// --- Configuration ---
	envErr := godotenv.Load()
	if envErr != nil {
		log.Fatal(".env file couldn't be loaded.")
	}
	//accessToken := os.Getenv("ARENA_PERSONAL_ACCESS_TOKEN")
	//channelSlug := os.Getenv("ARENA_CHANNEL_SLUG")

	if _accessToken == "" || _channelSlug == "" {
		log.Fatal("Error: You must set the ARENA_PERSONAL_ACCESS_TOKEN and ARENA_CHANNEL_SLUG environment variables.")
	}

	fmt.Println("🚀 Starting clipboard monitor for Are.na...")
	fmt.Printf("➡️  Sending to channel: %s\n", _channelSlug)
	fmt.Println("📋 Copy text (Ctrl+C) and it will be sent to Are.na.")
	fmt.Println("ℹ️  Press Ctrl+C in this terminal to stop.")
}
