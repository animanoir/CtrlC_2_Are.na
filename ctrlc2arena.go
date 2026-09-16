package main

import (
	"bytes"
	_ "embed"
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
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/atotto/clipboard"
)

// -- This magically will embed the image into the final application.
//
//go:embed arena-logo-white.png
var arenaLogoBytes []byte

// Are.na API v3 (https://www.are.na/developers/explore)
const (
	arenaAPIEndpoint = "https://api.are.na/v3/blocks" // Creates a block and connects it to the given channels
	checkInterval    = 2 * time.Second                // Interval for checking the clipboard
	settingsFileName = "arena_settings.json"
)

// Block preview
const (
	tileSize        = 300
	tilePadding     = 24
	passageTextSize = 16
	passageLeading  = 0   // Extra space between lines of copied text
	maxPreviewRunes = 600 // More than fits in the tile, so long copies aren't measured in full
)

// Structure for the Are.na API payload (simplified)
type ArenaBlock struct {
	Value    string         `json:"value"`           // Text/markdown creates a Text block; a URL creates an Image/Link/Embed block
	Title    string         `json:"title,omitempty"` // Optional
	Channels []ArenaChannel `json:"channels"`
}

// Target channel for a new block. The ID accepts either a numeric channel ID or a channel slug.
type ArenaChannel struct {
	ID string `json:"id"`
}

// Error body returned by the Are.na API
type ArenaError struct {
	Error   string `json:"error"`
	Details struct {
		Message string `json:"message"`
	} `json:"details"`
}

// Where a copied text is on its way to Are.na
type blockState int

const (
	blockSending blockState = iota
	blockSent
	blockFailed
)

type arenaUpdate struct {
	State   blockState
	Channel string
	Content string
	Message string // Explains what went wrong when State is blockFailed
}

var isMonitoring bool = false
var stopMonitoringChan chan bool
var arenaUpdates chan arenaUpdate

// Main function
func main() {
	stopMonitoringChan = make(chan bool, 1)
	arenaUpdates = make(chan arenaUpdate, 8)
	runGui()
}

func runGui() {
	// App and window settings
	a := app.New()
	a.Settings().SetTheme(&arenaTheme{})
	w := a.NewWindow("Ctrl+C to Are.na")

	ui := newAppUI(a)
	w.SetContent(ui.content)
	w.Resize(fyne.NewSize(780, 540))
	w.CenterOnScreen()

	// Show what happens to each copied text, on the main thread
	go func() {
		for update := range arenaUpdates {
			fyne.Do(func() {
				ui.showUpdate(update)
			})
		}
	}()

	w.ShowAndRun()
}

// Text styles
var (
	titleStyle   = widget.RichTextStyle{ColorName: theme.ColorNameForeground, SizeName: sizeNameTitle, TextStyle: fyne.TextStyle{Bold: true}}
	headingStyle = widget.RichTextStyle{ColorName: theme.ColorNameForeground, SizeName: theme.SizeNameSubHeadingText, TextStyle: fyne.TextStyle{Bold: true}}
	labelStyle   = widget.RichTextStyle{ColorName: theme.ColorNameForeground, SizeName: theme.SizeNameText, TextStyle: fyne.TextStyle{Bold: true}}
	bodyStyle    = widget.RichTextStyle{ColorName: theme.ColorNameForeground, SizeName: theme.SizeNameText}
	quietStyle   = widget.RichTextStyle{ColorName: colorNameGraphite, SizeName: theme.SizeNameText}
	captionStyle = widget.RichTextStyle{ColorName: colorNameGraphite, SizeName: theme.SizeNameCaptionText, Alignment: fyne.TextAlignCenter}
)

// Everything on screen that changes while the app runs
type appUI struct {
	app     fyne.App
	content fyne.CanvasObject
	logo    *canvas.Image

	// Setup
	setupView     *fyne.Container
	tokenEntry    *widget.Entry
	slugEntry     *widget.Entry
	titleEntry    *widget.Entry
	rememberCheck *widget.Check
	startButton   *widget.Button

	// Listening
	listeningView   *fyne.Container
	listeningDot    *canvas.Circle
	listeningBlink  *fyne.Animation // Running while listening, nil otherwise
	listeningDetail *widget.RichText
	sentCountText   *widget.RichText
	sentCount       int

	// Block preview: the last copied text, shown the way Are.na shows a text block
	tileBorder     *canvas.Rectangle
	tileHint       *widget.RichText
	passage        *fyne.Container
	blockTitle     *widget.RichText
	blockStatus    *widget.RichText
	blockTitleText string
	tileFailed     bool
}

func newAppUI(a fyne.App) *appUI {
	ui := &appUI{app: a}

	// Header
	ui.logo = canvas.NewImageFromImage(nil)
	ui.logo.FillMode = canvas.ImageFillContain
	ui.logo.SetMinSize(fyne.NewSize(34, 20))
	header := container.New(layout.NewCustomPaddedHBoxLayout(12), ui.logo, flush(newText("Ctrl+C to Are.na", titleStyle)))

	// Setup
	ui.tokenEntry = widget.NewPasswordEntry()
	ui.slugEntry = widget.NewEntry()
	ui.slugEntry.SetPlaceHolder("Last part of the channel’s URL")
	ui.titleEntry = widget.NewEntry()
	ui.titleEntry.SetPlaceHolder("Optional")
	ui.rememberCheck = widget.NewCheck("Remember token and channel on this computer", nil)
	ui.startButton = widget.NewButton("Start listening", ui.start)
	ui.startButton.Importance = widget.HighImportance

	for _, entry := range []*widget.Entry{ui.tokenEntry, ui.slugEntry, ui.titleEntry} {
		entry.OnChanged = func(string) { ui.updateStartButton() }
		entry.OnSubmitted = func(string) { ui.start() }
	}

	// Fills the entries with saved data (cuz we lazy)
	if token, slug, ok := loadSavedData(); ok {
		ui.tokenEntry.SetText(token)
		ui.slugEntry.SetText(slug)
		ui.rememberCheck.SetChecked(true)
	}
	ui.updateStartButton()

	tokenURL, _ := url.Parse("https://www.are.na/settings/personal-access-tokens")
	tokenHint := widget.NewRichText(
		&widget.TextSegment{Text: "Needs write access. ", Style: inline(quietStyle)},
		&widget.HyperlinkSegment{Text: "Create a token", URL: tokenURL},
	)
	tokenHint.Wrapping = fyne.TextWrapWord

	ui.setupView = container.New(layout.NewCustomPaddedVBoxLayout(0),
		flush(newWrappedText("Everything you copy becomes a text block in your Are.na channel.", quietStyle)),
		gap(24),
		field("Personal access token", ui.tokenEntry, flush(tokenHint)),
		gap(16),
		field("Channel slug", ui.slugEntry, nil),
		gap(16),
		field("Block title", ui.titleEntry, nil),
		gap(14),
		flushCheck(ui.rememberCheck),
		gap(14),
		container.NewHBox(ui.startButton),
	)

	// Listening
	ui.listeningDot = canvas.NewCircle(color.Transparent)
	ui.listeningDetail = newWrappedText("", quietStyle)
	ui.sentCountText = newText("", bodyStyle)
	stopButton := widget.NewButton("Stop listening", ui.stop)

	ui.listeningView = container.New(layout.NewCustomPaddedVBoxLayout(0),
		container.New(layout.NewCustomPaddedHBoxLayout(10),
			container.NewCenter(container.NewGridWrap(fyne.NewSquareSize(10), ui.listeningDot)),
			flush(newText("Listening", headingStyle)),
		),
		gap(8),
		flush(ui.listeningDetail),
		gap(24),
		flush(ui.sentCountText),
		gap(24),
		container.NewHBox(stopButton),
	)
	ui.listeningView.Hide()

	// Block preview
	ui.tileBorder = canvas.NewRectangle(color.Transparent)
	ui.tileBorder.StrokeWidth = 1
	ui.tileBorder.SetMinSize(fyne.NewSquareSize(tileSize))
	ui.tileHint = newWrappedText("Copied text shows up here.", captionStyle)
	ui.passage = container.New(layout.NewCustomPaddedVBoxLayout(passageLeading))
	ui.passage.Hide()
	ui.blockTitle = newWrappedText("", widget.RichTextStyle{ColorName: theme.ColorNameForeground, SizeName: theme.SizeNameCaptionText, Alignment: fyne.TextAlignCenter, TextStyle: fyne.TextStyle{Bold: true}})
	ui.blockTitle.Hide()
	ui.blockStatus = newWrappedText("", captionStyle)
	ui.blockStatus.Hide()

	tile := container.NewStack(
		ui.tileBorder,
		container.New(layout.NewCustomPaddedLayout(tilePadding, tilePadding, tilePadding, tilePadding),
			container.NewStack(
				container.NewVBox(layout.NewSpacer(), flush(ui.tileHint), layout.NewSpacer()),
				ui.passage,
			),
		),
	)
	tileColumn := container.New(layout.NewCustomPaddedVBoxLayout(4), tile, gap(4), flush(ui.blockTitle), flush(ui.blockStatus))

	// Final layout
	body := container.NewBorder(nil, nil, nil,
		container.New(layout.NewCustomPaddedLayout(0, 0, 40, 0), tileColumn),
		container.NewStack(ui.setupView, ui.listeningView),
	)
	ui.content = container.New(layout.NewCustomPaddedLayout(28, 28, 32, 32),
		container.NewBorder(container.New(layout.NewCustomPaddedLayout(0, 24, 0, 0), header), nil, nil, nil, body),
	)

	ui.refreshColors()
	a.Settings().AddListener(func(fyne.Settings) {
		fyne.Do(func() {
			ui.refreshColors()
			if ui.listeningBlink != nil {
				ui.startBlinking() // Blink in the new colors
			}
		})
	})
	return ui
}

// Single line of text (wrap it in flush() when placing it, to line it up with fields and buttons)
func newText(text string, style widget.RichTextStyle) *widget.RichText {
	return widget.NewRichText(&widget.TextSegment{Text: text, Style: style})
}

// Text that wraps to the width it's given
func newWrappedText(text string, style widget.RichTextStyle) *widget.RichText {
	rt := newText(text, style)
	rt.Wrapping = fyne.TextWrapWord
	return rt
}

func setText(rt *widget.RichText, text string) {
	rt.Segments[0].(*widget.TextSegment).Text = text
	rt.Refresh()
}

func inline(style widget.RichTextStyle) widget.RichTextStyle {
	style.Inline = true
	return style
}

// Empty vertical space
func gap(height float32) fyne.CanvasObject {
	space := canvas.NewRectangle(color.Transparent)
	space.SetMinSize(fyne.NewSize(0, height))
	return space
}

// A form field with its label above it and an optional hint below it
func field(label string, entry fyne.CanvasObject, hint fyne.CanvasObject) fyne.CanvasObject {
	parts := []fyne.CanvasObject{flush(newText(label, labelStyle)), entry}
	if hint != nil {
		parts = append(parts, hint)
	}
	return container.New(layout.NewCustomPaddedVBoxLayout(6), parts...)
}

func (ui *appUI) updateStartButton() {
	if strings.TrimSpace(ui.tokenEntry.Text) != "" && strings.TrimSpace(ui.slugEntry.Text) != "" {
		ui.startButton.Enable()
	} else {
		ui.startButton.Disable()
	}
}

func (ui *appUI) start() {
	token := strings.TrimSpace(ui.tokenEntry.Text)
	slug := strings.TrimSpace(ui.slugEntry.Text)
	if isMonitoring || token == "" || slug == "" {
		return
	}

	if ui.rememberCheck.Checked {
		saveDataToFile(token, slug)
	} else {
		forgetSavedData()
	}

	// Clear any previous stop signals
	select {
	case <-stopMonitoringChan:
	default:
	}
	isMonitoring = true
	ui.blockTitleText = strings.TrimSpace(ui.titleEntry.Text)
	go clipboardMonitoring(token, slug, ui.blockTitleText)

	ui.sentCount = 0
	setText(ui.sentCountText, blocksSentText(ui.sentCount))
	setText(ui.listeningDetail, fmt.Sprintf("Everything you copy is sent to %s. Stop listening before you copy anything private.", slug))
	if ui.passage.Hidden {
		setText(ui.tileHint, "Copy some text to send your first block.")
	}
	ui.setupView.Hide()
	ui.listeningView.Show()
	ui.startBlinking()
}

func (ui *appUI) stop() {
	if isMonitoring {
		select {
		case stopMonitoringChan <- true:
		default:
		}
		isMonitoring = false
	}
	if ui.passage.Hidden {
		setText(ui.tileHint, "Copied text shows up here.")
	}
	ui.stopBlinking()
	ui.listeningView.Hide()
	ui.setupView.Show()
}

func (ui *appUI) showUpdate(update arenaUpdate) {
	ui.tileHint.Hide()
	ui.setPassage(update.Content)
	ui.passage.Show()
	setText(ui.blockTitle, ui.blockTitleText)
	ui.blockTitle.Hidden = ui.blockTitleText == ""

	status := ui.blockStatus.Segments[0].(*widget.TextSegment)
	status.Style.ColorName = colorNameGraphite
	ui.tileFailed = false
	switch update.State {
	case blockSending:
		status.Text = fmt.Sprintf("Sending to %s…", update.Channel)
	case blockSent:
		status.Text = fmt.Sprintf("Sent to %s at %s", update.Channel, time.Now().Format("15:04"))
		ui.sentCount++
		setText(ui.sentCountText, blocksSentText(ui.sentCount))
	case blockFailed:
		status.Text = update.Message
		status.Style.ColorName = theme.ColorNameError
		ui.tileFailed = true
	}
	ui.blockStatus.Show()
	ui.blockStatus.Refresh()
	ui.refreshColors()

	if update.State == blockSent {
		ui.flashTile()
	}
}

// Colors that Fyne doesn't update by itself when the system switches between light and dark
func (ui *appUI) refreshColors() {
	th := ui.app.Settings().Theme()
	variant := ui.app.Settings().ThemeVariant()

	ink := th.Color(theme.ColorNameForeground, variant)

	ui.logo.Image = tintedLogo(ink)
	ui.logo.Refresh()

	if ui.listeningBlink == nil { // While blinking, the animation sets the color
		ui.listeningDot.FillColor = ink
		ui.listeningDot.Refresh()
	}

	for _, line := range ui.passage.Objects {
		line.(*canvas.Text).Color = ink
		line.Refresh()
	}

	ui.tileBorder.StrokeColor = th.Color(theme.ColorNameSeparator, variant)
	if ui.tileFailed {
		ui.tileBorder.StrokeColor = th.Color(theme.ColorNameError, variant)
	}
	ui.tileBorder.Refresh()
}

// Blinks the dot next to "Listening", like a recording light, while the clipboard is being watched
func (ui *appUI) startBlinking() {
	ui.stopBlinking()
	if !ui.app.Settings().ShowAnimations() {
		return
	}
	ink := ui.app.Settings().Theme().Color(theme.ColorNameForeground, ui.app.Settings().ThemeVariant())
	ui.listeningBlink = canvas.NewColorRGBAAnimation(ink, color.Transparent, 700*time.Millisecond, func(c color.Color) {
		ui.listeningDot.FillColor = c
		ui.listeningDot.Refresh()
	})
	ui.listeningBlink.AutoReverse = true
	ui.listeningBlink.RepeatCount = fyne.AnimationRepeatForever
	ui.listeningBlink.Curve = fyne.AnimationEaseInOut
	ui.listeningBlink.Start()
}

func (ui *appUI) stopBlinking() {
	if ui.listeningBlink == nil {
		return
	}
	ui.listeningBlink.Stop()
	ui.listeningBlink = nil
	ui.refreshColors()
}

// Confirms a block was sent: the tile's border fades from ink back to its resting color
func (ui *appUI) flashTile() {
	if !ui.app.Settings().ShowAnimations() {
		return
	}
	th := ui.app.Settings().Theme()
	variant := ui.app.Settings().ThemeVariant()
	flash := canvas.NewColorRGBAAnimation(th.Color(theme.ColorNameForeground, variant), th.Color(theme.ColorNameSeparator, variant), 900*time.Millisecond, func(c color.Color) {
		ui.tileBorder.StrokeColor = c
		ui.tileBorder.Refresh()
	})
	flash.Curve = fyne.AnimationEaseOut
	flash.Start()
}

func blocksSentText(count int) string {
	switch count {
	case 0:
		return "No blocks sent yet"
	case 1:
		return "1 block sent"
	}
	return fmt.Sprintf("%d blocks sent", count)
}

// Shows copied text in the tile, set in Literata, a serif made for reading books on screen, so it reads like the passage it came from.
// Fyne's own text wrapping measures with the app's font, so the lines are wrapped here instead.
func (ui *appUI) setPassage(text string) {
	measure := func(s string) float32 {
		size, _ := ui.app.Driver().RenderedTextSize(s, passageTextSize, fyne.TextStyle{}, passageFont)
		return size.Width
	}
	lineHeight, _ := ui.app.Driver().RenderedTextSize("Ag", passageTextSize, fyne.TextStyle{}, passageFont)
	width := float32(tileSize - 2*tilePadding)
	maxLines := int((width + passageLeading) / (lineHeight.Height + passageLeading)) // The tile is square

	if utf8.RuneCountInString(text) > maxPreviewRunes {
		text = string([]rune(text)[:maxPreviewRunes])
	}
	lines := wrapText(strings.Fields(text), width, maxLines+1, measure)

	// Too long for the tile: end the last line with an ellipsis
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		last := lines[maxLines-1]
		for measure(last+"…") > width {
			if i := strings.LastIndex(last, " "); i > 0 {
				last = last[:i]
			} else {
				runes := []rune(last)
				last = string(runes[:len(runes)-1])
			}
		}
		lines[maxLines-1] = last + "…"
	}

	ui.passage.RemoveAll()
	for _, line := range lines {
		text := canvas.NewText(line, color.Transparent) // Colored by refreshColors
		text.TextSize = passageTextSize
		text.FontSource = passageFont
		ui.passage.Add(text)
	}
}

// Breaks words into lines no wider than width, stopping after maxLines
func wrapText(words []string, width float32, maxLines int, measure func(string) float32) []string {
	var lines []string
	line := ""
	for _, word := range words {
		// Break words too long for a line of their own, like URLs
		for measure(word) > width && len(lines) < maxLines {
			runes := []rune(word)
			fits := sort.Search(len(runes), func(n int) bool {
				return measure(string(runes[:n+1])) > width
			})
			fits = max(fits, 1)
			if line != "" {
				lines = append(lines, line)
				line = ""
			}
			lines = append(lines, string(runes[:fits]))
			word = string(runes[fits:])
		}

		switch {
		case len(lines) >= maxLines:
			return lines[:maxLines]
		case line == "":
			line = word
		case measure(line+" "+word) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines[:min(len(lines), maxLines)]
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

	update := arenaUpdate{State: blockSending, Channel: channelSlug, Content: formattedContent}
	arenaUpdates <- update

	fail := func(message string) {
		update.State = blockFailed
		update.Message = message
		arenaUpdates <- update
	}

	blockData := ArenaBlock{
		Value:    formattedContent,
		Channels: []ArenaChannel{{ID: channelSlug}},
	}

	// Checks if block title has something. If so, adds it to the blockData to send to Are.na's API.
	if blockTitle != "" {
		blockData.Title = blockTitle
	}

	jsonData, err := json.Marshal(blockData)
	if err != nil {
		fail("This text couldn’t be prepared for Are.na.")
		return
	}

	req, err := http.NewRequest("POST", arenaAPIEndpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		fail("This text couldn’t be prepared for Are.na.")
		return
	}

	// Set Headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "Go CTRL+C2Arena Connector (https://github.com/animanoir)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fail("Couldn’t reach Are.na. Check your internet connection.")
		//log.Printf("❌ Error sending request to Are.na: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		update.State = blockSent
		arenaUpdates <- update
		return
	}

	// Read response body for more error details
	var apiError ArenaError
	_ = json.NewDecoder(resp.Body).Decode(&apiError)
	errorMessage := apiError.Details.Message
	if errorMessage == "" {
		errorMessage = apiError.Error
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		fail("Are.na didn’t accept your token. Stop listening and check it.")
	case http.StatusForbidden:
		fail(fmt.Sprintf("Are.na won’t let this token add blocks to %s. Check that the token has write access and that you can add to the channel.", channelSlug))
	case http.StatusNotFound:
		fail(fmt.Sprintf("There’s no channel called %s. Stop listening and check the slug.", channelSlug))
	case http.StatusTooManyRequests:
		if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
			fail(fmt.Sprintf("Are.na’s limit on new blocks was reached. Copies are accepted again in %s seconds.", retryAfter))
		} else {
			fail("Are.na’s limit on new blocks was reached. Copies are accepted again in about a minute.")
		}
	default:
		fail(strings.TrimSpace(fmt.Sprintf("Are.na couldn’t save this block (status %d). %s", resp.StatusCode, errorMessage)))
	}
	// log.Printf("❌ Error sending to Are.na. Status: %d, Response: %s\n", resp.StatusCode, errorMessage)
}

// ReadAll helper (if using Go < 1.16, use ioutil.ReadAll)
func ReadAll(r io.Reader) ([]byte, error) {
	b := bytes.NewBuffer(make([]byte, 0, 512))
	_, err := io.Copy(b, r)
	return b.Bytes(), err
}

type savedData struct {
	Token string `json:"token"`
	Slug  string `json:"slug"`
}

// Reads the token and slug from arena_settings.json, if it exists
func loadSavedData() (token string, slug string, ok bool) {
	file, err := os.Open(settingsFileName)
	if err != nil {
		return "", "", false
	}
	defer file.Close()

	var data savedData
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return "", "", false
	}
	return data.Token, data.Slug, true
}

func saveDataToFile(arenaToken string, channelSlug string) {
	file, err := os.Create(settingsFileName)
	if err != nil {
		fmt.Println("Error creating file: ", err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(savedData{Token: arenaToken, Slug: channelSlug}); err != nil {
		fmt.Println("Error encoding data: ", err)
		return
	}
}

func forgetSavedData() {
	if err := os.Remove(settingsFileName); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Println("Error removing saved data: ", err)
	}
}
