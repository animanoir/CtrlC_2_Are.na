package main

import (
	"bytes"
	"encoding/json"
	"errors"
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

// Structure for the Are.na API payload (simplified)
const (
	arenaAPIEndpoint = "https://api.are.na/v2/channels/%s/blocks" // %s will be replaced by the channelSlug
	checkInterval    = 2 * time.Second                            // Interval for checking the clipboard
)

type ArenaBlock struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

var isMonitoring bool = false
var stopMonitoringChan chan bool
var clipboardContentChan chan string

// Main function
func main() {
	stopMonitoringChan = make(chan bool, 1)
	clipboardContentChan = make(chan string, 10)
	runGui()
}

func clipboardMonitoring(_accessToken string, _channelSlug string, _blockTitle string) {

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
				clipboardContentChan <- currentClipboardContent
				lastClipboardContent = currentClipboardContent // Update the last content

				// Send to Are.na in a goroutine to avoid blocking the check
				go sendToArena(_accessToken, _channelSlug, lastClipboardContent, _blockTitle)
			}

		case <-stopMonitoringChan:
			fmt.Println("\n🛑 Stopping the monitor...")
			isMonitoring = false
			return // Exit the program
		}

	}
}
func sendToArena(token, channelSlug, content string, blockTitle string) {
	// Formats the text before sending
	formattedContent := strings.ReplaceAll(content, "\r\n", " ")

	apiURL := fmt.Sprintf(arenaAPIEndpoint, channelSlug)

	blockData := ArenaBlock{
		Title:   blockTitle,
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

func runGui() {
	var userArenaToken string
	var userSlugChannel string
	var blockTitle string

	// App and window settings
	a := app.New()
	a.Settings().SetTheme(&arenaTheme{})
	w := a.NewWindow("CTRL+C to Are.na")
	w.Resize(fyne.NewSize(600, 500))
	w.CenterOnScreen()

	// Images configuration
	arenaLogoImg := canvas.NewImageFromFile("arena-logo-white.png")
	arenaLogoImg.FillMode = canvas.ImageFillContain
	arenaLogoImg.SetMinSize(fyne.NewSize(70, 50))

	// Buttons, form and styles configuration

	// Colors
	grayColor := color.NRGBA{R: 178, G: 178, B: 178, A: 255}
	whiteColor := color.NRGBA{R: 255, G: 255, B: 255, A: 255}

	// Title and info text
	title := canvas.NewText("Ctrl+C to Are.na", whiteColor)
	title.TextSize = 42
	infoText := canvas.NewText("This lil' software will monitor and send whatever TEXT you copy (CTRL+C) into a specified channel in your Are.na account.", grayColor)
	copiedText := canvas.NewText("The text that will be send.", whiteColor)

	// Form entries
	arenaTokenEntry := widget.NewPasswordEntry()
	arenaSlugEntry := widget.NewEntry()
	blockTitleEntry := widget.NewEntry()

	// External links
	parsedURL, err := url.Parse("https://dev.are.na/")
	if err != nil {
		return
	}
	arenaApiUrl := widget.NewHyperlink("Click here to get your Are.na API token.", parsedURL)

	// Form configuration
	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Are.na token:", Widget: arenaTokenEntry},
			{Text: "Channel slug:", Widget: arenaSlugEntry},
			{Text: "Block title:", Widget: blockTitleEntry}},
		SubmitText: "Connect",
	}
	stopButton := widget.NewButtonWithIcon("Stop monitoring", theme.MediaStopIcon(), func() {
		log.Println("stop button pressed")
		if isMonitoring {
			// Send stop signal (non-blocking)
			select {
			case stopMonitoringChan <- true:
			default:
			}
			isMonitoring = false
			form.Enable()
		}
	})
	// Status components
	statusIcon := widget.NewIcon(nil)
	statusText := canvas.NewText("", whiteColor)
	statusText.TextSize = 14

	statusBox := container.NewHBox(statusIcon, statusText, stopButton)
	statusBox.Hide()
	if isMonitoring {
		stopButton.Show()
	} else {
		stopButton.Hide()
	}

	form.OnSubmit = func() {
		userArenaToken = arenaTokenEntry.Text
		userSlugChannel = arenaSlugEntry.Text
		blockTitle = blockTitleEntry.Text
		if userArenaToken != "" && userSlugChannel != "" && blockTitle != "" {
			isMonitoring = true

			// Clear any previous stop signals
			select {
			case <-stopMonitoringChan:
			default:
			}
			go clipboardMonitoring(userArenaToken, userSlugChannel, blockTitle)
			statusText.Text = "The software is now monitoring your clipboard—be careful..."
			statusText.Color = theme.WarningColor()
			statusText.Refresh()
			statusIcon.SetResource(theme.MediaVideoIcon())
			statusBox.Show()
			form.Disable()
			stopButton.Show()

			go func() {
				for content := range clipboardContentChan {
					copiedText.Text = "Last copied:" + strings.ReplaceAll(content, "\r\n", "")
					copiedText.Refresh()
				}
			}()
		}
	}

	// Entry validators
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
	blockTitleEntry.Validator = func(text string) error {
		if len(strings.TrimSpace(text)) == 0 {
			return fmt.Errorf("block title entry cannot be empty")
		}
		return nil
	}

	// Final layout container
	layoutHeader := container.NewHBox(title, arenaLogoImg)
	content := container.NewVBox(
		layoutHeader,
		widget.NewSeparator(),
		infoText,
		widget.NewSeparator(),
		arenaApiUrl,
		form,
		statusBox,
		copiedText,
	)

	paddedContent := container.NewPadded(content)

	// Final Fyne functions to make this shyte work
	w.SetContent(paddedContent)
	w.ShowAndRun()
}

func settingsFileExist(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
