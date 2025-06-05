package main

import (
	"bytes"
	_ "embed"
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
)

// -- This magically will embed the image into the final application.
//
//go:embed arena-logo-white.png
var arenaLogoBytes []byte

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
var stopGUIChan chan bool
var arenaApiStatus chan string

// Main function
func main() {

	stopMonitoringChan = make(chan bool, 1)
	clipboardContentChan = make(chan string, 10)
	stopGUIChan = make(chan bool, 1)
	arenaApiStatus = make(chan string, 1)
	runGui()
}

func runGui() {
	var userArenaToken string
	var userSlugChannel string
	var blockTitle string
	var statusBox *fyne.Container
	var stopButton *widget.Button
	var saveDataButton *widget.Button

	// App and window settings
	a := app.New()
	a.Settings().SetTheme(&arenaTheme{})
	w := a.NewWindow("CTRL+C to Are.na")
	w.Resize(fyne.NewSize(600, 300))
	w.CenterOnScreen()

	// Images configuration
	// Convert arena logo bytes into a Fyne resource
	resource := fyne.NewStaticResource("arena-logo-white.png", arenaLogoBytes)
	arenaLogoImg := canvas.NewImageFromResource(resource)
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

	// copiedTextInfo := canvas.NewText("Last copied text: ", grayColor)
	// copiedText := widget.NewRichText()
	// copiedText.Wrapping = fyne.TextWrapWord
	// copiedTextBox := container.NewHBox(copiedTextInfo, copiedText)
	// copiedTextBox.Hide()

	// Form entries
	arenaTokenEntry := widget.NewPasswordEntry()
	arenaSlugEntry := widget.NewEntry()
	blockTitleEntry := widget.NewEntry()

	// Checks if the file arena_settings.json exists so it uses the data to fill the entries (cuz we lazy)
	if _, err := os.Stat("arena_settings.json"); err == nil {
		file, err := os.Open("arena_settings.json")
		if err == nil {
			defer file.Close()
			decoder := json.NewDecoder(file)
			var savedData struct {
				Token string `json:"token"`
				Slug  string `json:"slug"`
			}
			if err := decoder.Decode(&savedData); err == nil {
				arenaTokenEntry.Text = savedData.Token
				arenaTokenEntry.Refresh()
				arenaSlugEntry.Text = savedData.Slug
				arenaSlugEntry.Refresh()
			}
		}
	}

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
	saveDataButton = widget.NewButtonWithIcon("Save data?", theme.DocumentSaveIcon(), func() {
		saveDataToFile(arenaTokenEntry.Text, arenaSlugEntry.Text)
		arenaApiStatus <- "💾 Your data has been saved as arena_settings.json (no need to fill info again 🙈.)"
	})
	stopButton = widget.NewButtonWithIcon("Stop monitoring", theme.MediaStopIcon(), func() {
		// log.Println("stop button pressed")
		if isMonitoring {
			// Send stop signals to both monitoring and GUI
			select {
			case stopMonitoringChan <- true:
			default:
			}
			select {
			case stopGUIChan <- true:
			default:
			}
			isMonitoring = false
			form.Enable()
			statusBox.Hide()
			stopButton.Hide()
		}
	})
	// Status components
	statusIcon := widget.NewIcon(nil)
	statusText := canvas.NewText("", whiteColor)
	statusText.TextSize = 14
	arenaStatusString := "⏳ Waiting for new text..."
	arenaStatusText := canvas.NewText(arenaStatusString, whiteColor)
	arenaStatusText.Hide()

	statusBox = container.NewHBox(statusIcon, statusText, stopButton, saveDataButton)
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
		if userArenaToken != "" && userSlugChannel != "" {
			isMonitoring = true
			arenaTokenEntry.Disable()
			blockTitleEntry.Disable()
			arenaSlugEntry.Disable()

			// Clear any previous stop signals
			select {
			case <-stopMonitoringChan:
			default:
			}
			go clipboardMonitoring(userArenaToken, userSlugChannel, blockTitle)
			statusText.Text = "The software is now monitoring your clipboard—be careful... "
			statusText.Color = theme.WarningColor()
			statusText.Refresh()
			statusIcon.SetResource(theme.MediaVideoIcon())
			statusBox.Show()
			form.Disable()
			stopButton.Show()

			go func() {
				// Show copiedText on main thread
				fyne.DoAndWait(func() {
					// copiedTextBox.Show()
					arenaStatusText.Show()
				})

				for {
					select {
					// case content := <-clipboardContentChan:
					// 	// Update GUI on main thread using fyne.Do()
					// 	fyne.Do(func() {
					// 		copiedText.ParseMarkdown(strings.ReplaceAll(content, "\r\n", ""))
					// 		copiedText.Refresh()
					// 		arenaStatusText.Show()
					// 	})

					case <-stopGUIChan:
						// Hide text on main thread
						fyne.Do(func() {
							// copiedTextBox.Hide()
							arenaStatusText.Text = arenaStatusString
							arenaStatusText.Hide()
							arenaTokenEntry.Enable()
							blockTitleEntry.Enable()
							arenaSlugEntry.Enable()
						})
						return // Exit the goroutine

					default:
						// Non-blocking check if monitoring stopped
						if !isMonitoring {
							fyne.Do(func() {
								// copiedTextBox.Hide()
								arenaStatusText.Text = arenaStatusString
								arenaStatusText.Hide()
							})
							return
						}
						time.Sleep(100 * time.Millisecond) // Prevents busy-waiting by pausing the loop when no channels are ready, reducing CPU usage
					}
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
	// For now, make block title optional.
	// blockTitleEntry.Validator = func(text string) error {
	// 	if len(strings.TrimSpace(text)) == 0 {
	// 		return fmt.Errorf("block title entry cannot be empty")
	// 	}
	// 	return nil
	// }

	// Arena status text
	go func() {
		for status := range arenaApiStatus {
			fyne.Do(func() {
				arenaStatusText.Text = status
				arenaStatusText.Refresh()
			})
		}
	}()

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
		// copiedTextBox,
		arenaStatusText,
	)

	// Adds padding to the container
	paddedContent := container.NewPadded(content)

	// Final Fyne functions to make this shyte work
	w.SetContent(paddedContent)
	w.ShowAndRun()
}

func clipboardMonitoring(_accessToken string, _channelSlug string, _blockTitle string) {

	// fmt.Print("clipboardMonitoring func executing...")
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
				// fmt.Printf("✨ New content detected: ")
				arenaApiStatus <- "✨ New content detected! Sending..."
				// fmt.Println(strings.ReplaceAll(currentClipboardContent, "\r\n", " "))
				clipboardContentChan <- currentClipboardContent
				lastClipboardContent = currentClipboardContent // Update the last content

				// Send to Are.na in a goroutine to avoid blocking the check
				go sendToArena(_accessToken, _channelSlug, lastClipboardContent, _blockTitle)
			}

		case <-stopMonitoringChan:
			// fmt.Println("\n🛑 Stopping the monitor...")
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
		Content: formattedContent,
	}

	// Checks if block title has something. If so, adds it to the blockData to send to Are.na's API.
	if blockTitle != "" {
		blockData.Title = blockTitle
	}

	jsonData, err := json.Marshal(blockData)
	if err != nil {
		//log.Printf("❌ Error encoding JSON: %v\n", err)
		return
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		//log.Printf("❌ Error creating HTTP request: %v\n", err)
		return
	}

	// Set Headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Go CTRL+C2Arena Connector (https://github.com/animanoir)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		arenaApiStatus <- "❌ Error sending request to Are.na: %v\n"
		//log.Printf("❌ Error sending request to Are.na: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {

		arenaApiStatus <- fmt.Sprintf("✅ Copied text successfully sent to your Are.na channel! — Last content: %s at %s", formattedContent[:20]+"...", time.Now().Format("15:04:05"))
		// fmt.Printf("✅ Sent to Are.na! (Status: %d)\n", resp.StatusCode)
	} else {
		// Read response body for more error details
		// var bodyBytes []byte
		// if resp.Body != nil {
		// 	bodyBytes, _ = ReadAll(resp.Body)
		// }
		arenaApiStatus <- fmt.Sprintf("❌ There was an error sending to Are.na (re-check your token/slug). Status: %d", resp.StatusCode)
		// log.Printf("❌ Error sending to Are.na. Status: %d, Response: %s\n", resp.StatusCode, string(bodyBytes))
	}
}

// ReadAll helper (if using Go < 1.16, use ioutil.ReadAll)
func ReadAll(r io.Reader) ([]byte, error) {
	b := bytes.NewBuffer(make([]byte, 0, 512))
	_, err := io.Copy(b, r)
	return b.Bytes(), err
}

func saveDataToFile(arenaToken string, channelSlug string) {

	type fileData struct {
		Token string `json:"token"`
		Slug  string `json:"slug"`
	}

	data := fileData{
		Token: arenaToken,
		Slug:  channelSlug,
	}

	file, err := os.Create("arena_settings.json")
	if err != nil {
		fmt.Println("Error creating file: ", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(data); err != nil {
		fmt.Println("Error encoding data: ", err)
		return
	}
}
