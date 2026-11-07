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
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/atotto/clipboard"
)

// -- This magically will embed the image into the final application.
//
//go:embed arena-logo-white.png
var arenaLogoBytes []byte

// Set at build time from the git tag with -ldflags "-X main.version=2.1.0" (see the README). Shown in the About dialog.
var version = "dev"

const (
	repoURL   = "https://github.com/animanoir/CtrlC_2_Are.na"
	issuesURL = repoURL + "/issues"
)

// The window is this wide and as tall as its content needs (see windowSize). One gutter separates header, form and tile.
const (
	windowWidth = 780
	gutter      = 24
)

// Are.na API v3 (https://www.are.na/developers/explore)
const (
	arenaAPIEndpoint = "https://api.are.na/v3/blocks" // Creates a block and connects it to the given channels
	checkInterval    = 2 * time.Second                // Interval for checking the clipboard
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
	blockSkipped   // Held back because it looks like a secret. Content stays empty, so the text never reaches the UI.
	blockPending   // Waiting out the hold; SecondsLeft and Waiting say how long and how many
	blockCancelled // Undone during the hold, or called off because listening stopped
)

type arenaUpdate struct {
	State       blockState
	Copy        int // Number of the copy since launch, so the tile can keep showing the newest one
	Channel     string
	Content     string
	Message     string // Explains what went wrong when State is blockFailed, or why nothing was sent when it's blockSkipped
	SecondsLeft int    // While pending
	Waiting     int    // Copies waiting out the hold, this one included, while pending
}

var copyCount int // Numbers every copy since launch. Only the monitoring goroutine touches it.
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

	ui := newAppUI(a, w)
	w.SetContent(ui.content)
	w.Resize(ui.windowSize())
	w.SetFixedSize(true) // Sized to fit, so there's nothing to gain from resizing, and the frame never jumps
	w.CenterOnScreen()

	// Esc closes an open hint, or else undoes the newest copy waiting out the hold. The canvas only gets keys
	// while nothing has focus, which showListening sees to.
	w.Canvas().SetOnTypedKey(func(key *fyne.KeyEvent) {
		if key.Name == fyne.KeyEscape && !ui.hints.hide() {
			ui.undo()
		}
	})

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
	errorStyle   = widget.RichTextStyle{ColorName: theme.ColorNameError, SizeName: theme.SizeNameText}
	captionStyle = widget.RichTextStyle{ColorName: colorNameGraphite, SizeName: theme.SizeNameCaptionText, Alignment: fyne.TextAlignCenter}
	// Block titles and channel names under the block preview
	captionBoldStyle = widget.RichTextStyle{ColorName: theme.ColorNameForeground, SizeName: theme.SizeNameCaptionText, Alignment: fyne.TextAlignCenter, TextStyle: fyne.TextStyle{Bold: true}}
)

// Everything on screen that changes while the app runs
type appUI struct {
	app       fyne.App
	window    fyne.Window
	content   fyne.CanvasObject
	logo      *canvas.Image
	aboutLogo *canvas.Image // Tinted along with the header logo, for the About dialog

	// Setup
	setupView     *fyne.Container
	tokenEntry    *widget.Entry
	slugEntry     *widget.Entry
	titleEntry    *widget.Entry
	rememberCheck *widget.Check
	autoStopCheck *widget.Check
	autoStopEntry *widget.Entry
	minutesText   *widget.RichText
	holdCheck     *widget.Check
	holdEntry     *widget.Entry
	secondsText   *widget.RichText
	autoStopHint  *hintIcon // At the end of each number row, saying what the number does
	holdHint      *hintIcon
	hints         *hintLayer // Where the hints open
	startButton   *widget.Button
	stopNote      *widget.RichText // Says so when listening stopped by itself
	listeningRun  int              // Counts starts, so a timer from an earlier run can't stop a later one

	// Listening
	listeningView    *fyne.Container
	listeningDot     *canvas.Circle
	listeningBlink   *fyne.Animation // Running while listening, nil otherwise
	listeningDetail  *widget.RichText
	listeningWarning *widget.RichText // Reminder to stop before copying anything private, on its own line
	saveProblem      *widget.RichText // Shown when the token or channel couldn't be remembered
	sentCountText    *widget.RichText
	sentCount        int

	// Block preview: the last copied text, shown the way Are.na shows a text block
	tileBorder     *canvas.Rectangle
	tileHint       *widget.RichText
	passage        *fyne.Container
	blockTitle     *widget.RichText
	blockStatus    *widget.RichText
	pendingRow     *fyne.Container // Countdown and Undo, in place of the status while a copy waits out the hold
	pendingText    *widget.RichText
	blockTitleText string
	tileFailed     bool
	shownCopy      int // Number of the copy on the tile. Updates about older copies leave the tile alone.
}

func newAppUI(a fyne.App, w fyne.Window) *appUI {
	ui := &appUI{app: a, window: w}

	// Header
	ui.logo = canvas.NewImageFromImage(nil)
	ui.logo.FillMode = canvas.ImageFillContain
	ui.logo.SetMinSize(fyne.NewSize(34, 20))
	ui.aboutLogo = canvas.NewImageFromImage(nil)
	ui.aboutLogo.FillMode = canvas.ImageFillContain
	ui.aboutLogo.SetMinSize(fyne.NewSize(44, 26))
	header := container.New(layout.NewCustomPaddedHBoxLayout(12), ui.logo, flush(newText("Ctrl+C to Are.na", titleStyle)))

	// Setup
	ui.tokenEntry = widget.NewPasswordEntry()
	ui.slugEntry = widget.NewEntry()
	ui.slugEntry.SetPlaceHolder("Last part of the channel’s URL")
	ui.titleEntry = widget.NewEntry()
	ui.titleEntry.SetPlaceHolder("Optional")
	ui.rememberCheck = widget.NewCheck("Remember token and channel on this computer", nil)
	ui.hints = newHintLayer()
	ui.autoStopEntry = ui.numberEntry()
	ui.minutesText = newText("minutes", quietStyle)
	ui.autoStopHint = newHintIcon(ui.hints, "Stops on its own after that long without a copy. Each copy restarts the count.")
	ui.autoStopCheck = widget.NewCheck("Stop listening after", func(on bool) { ui.enableNumberRow(ui.autoStopEntry, ui.minutesText, ui.autoStopHint, on) })
	ui.holdEntry = ui.numberEntry()
	ui.secondsText = newText("seconds", quietStyle)
	ui.holdHint = newHintIcon(ui.hints, "Waits a few seconds before each send so you can stop a copy you didn’t mean to share.")
	ui.holdCheck = widget.NewCheck("Hold before sending", func(on bool) { ui.enableNumberRow(ui.holdEntry, ui.secondsText, ui.holdHint, on) })
	ui.startButton = widget.NewButton("Start listening", ui.start)
	ui.startButton.Importance = widget.HighImportance
	ui.stopNote = newWrappedText("", quietStyle)
	ui.stopNote.Hide()

	for _, entry := range []*widget.Entry{ui.tokenEntry, ui.slugEntry, ui.titleEntry} {
		entry.OnChanged = func(string) { ui.updateStartButton() }
		entry.OnSubmitted = func(string) { ui.start() }
	}

	// Fills the entries with saved data (cuz we lazy)
	token, saved := loadSettings()
	if token != "" || saved.Slug != "" {
		ui.tokenEntry.SetText(token)
		ui.slugEntry.SetText(saved.Slug)
		ui.rememberCheck.SetChecked(true)
	}
	ui.autoStopEntry.SetText(strconv.Itoa(saved.AutoStopMinutes))
	ui.autoStopCheck.SetChecked(saved.AutoStop)
	ui.enableNumberRow(ui.autoStopEntry, ui.minutesText, ui.autoStopHint, saved.AutoStop) // SetChecked only calls back when the value changes
	ui.holdEntry.SetText(strconv.Itoa(saved.HoldSeconds))
	ui.holdCheck.SetChecked(saved.Hold)
	ui.enableNumberRow(ui.holdEntry, ui.secondsText, ui.holdHint, saved.Hold)
	ui.updateStartButton()

	tokenURL, _ := url.Parse("https://www.are.na/settings/personal-access-tokens")
	tokenHint := widget.NewRichText(
		&widget.TextSegment{Text: "Needs write access. ", Style: inline(quietStyle)},
		&widget.HyperlinkSegment{Text: "Create a token", URL: tokenURL},
	)
	tokenHint.Wrapping = fyne.TextWrapWord

	// Both number rows share one layout, so their entries line up however wide the checkbox labels are
	rowLayout := numberRowLayout{
		labelWidth: max(ui.autoStopCheck.MinSize().Width, ui.holdCheck.MinSize().Width) - 2*checkOutdent(),
		entryWidth: fyne.MeasureText("000", theme.TextSize(), fyne.TextStyle{}).Width + 2*theme.InnerPadding() + 2*theme.InputBorderSize() + 2*theme.Padding(),
		gap:        8,
	}

	ui.setupView = container.New(layout.NewCustomPaddedVBoxLayout(0),
		flush(newWrappedText("Everything you copy becomes a text block in your Are.na channel. BEWARE OF COPYING CONFIDENTIAL DATA.", quietStyle)),
		gap(24),
		field("Personal access token", ui.tokenEntry, flush(tokenHint)),
		gap(16),
		field("Channel slug", ui.slugEntry, nil),
		gap(16),
		field("Block title", ui.titleEntry, nil),
		gap(14),
		flushCheck(ui.rememberCheck),
		gap(6),
		numberRow(rowLayout, ui.autoStopCheck, ui.autoStopEntry, ui.minutesText, ui.autoStopHint),
		gap(6),
		numberRow(rowLayout, ui.holdCheck, ui.holdEntry, ui.secondsText, ui.holdHint),
		gap(14),
		container.NewHBox(ui.startButton),
		gap(12),
		flush(ui.stopNote),
	)

	// Listening
	ui.listeningDot = canvas.NewCircle(color.Transparent)
	ui.listeningDetail = newWrappedText("", quietStyle)
	ui.listeningWarning = newWrappedText("Stop listening before you copy anything private.", labelStyle)
	ui.saveProblem = newWrappedText("", errorStyle)
	ui.saveProblem.Hide()
	ui.sentCountText = newText("", bodyStyle)
	stopButton := widget.NewButton("Stop listening", ui.stop)

	ui.listeningView = container.New(layout.NewCustomPaddedVBoxLayout(0),
		container.New(layout.NewCustomPaddedHBoxLayout(10),
			container.NewCenter(container.NewGridWrap(fyne.NewSquareSize(10), ui.listeningDot)),
			flush(newText("Listening", headingStyle)),
		),
		gap(8),
		flush(ui.listeningDetail),
		flush(ui.listeningWarning), // Straight under the sentence above, so the two read as one paragraph
		gap(8),
		flush(ui.saveProblem),
		gap(16),
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
	ui.blockTitle = newWrappedText("", captionBoldStyle)
	ui.blockTitle.Hide()
	ui.blockStatus = newWrappedText("", captionStyle)
	ui.blockStatus.Hide()
	ui.pendingText = newText("", captionStyle)
	undoButton := widget.NewButton("Undo", ui.undo) // Plain, not the black primary: it's a way out, not the main action
	ui.pendingRow = container.NewCenter(container.New(layout.NewCustomPaddedHBoxLayout(8), container.NewCenter(flush(ui.pendingText)), undoButton))
	ui.pendingRow.Hide()

	tile := container.NewStack(
		ui.tileBorder,
		container.New(layout.NewCustomPaddedLayout(tilePadding, tilePadding, tilePadding, tilePadding),
			container.NewStack(
				container.NewVBox(layout.NewSpacer(), flush(ui.tileHint), layout.NewSpacer()),
				ui.passage,
			),
		),
	)
	// The status and the countdown share one slot under the tile; only one of them shows at a time
	tileColumn := container.New(layout.NewCustomPaddedVBoxLayout(4), tile, gap(4), flush(ui.blockTitle), container.NewStack(flush(ui.blockStatus), ui.pendingRow))

	// Final layout
	body := container.NewBorder(nil, nil, nil,
		container.New(layout.NewCustomPaddedLayout(0, 0, gutter, 0), tileColumn),
		container.NewStack(ui.setupView, ui.listeningView),
	)
	aboutButton := widget.NewButton("About", ui.showAbout)
	aboutButton.Importance = widget.LowImportance // Reads as a text link, so it doesn't compete with Start
	// Two paddings below the body in all: this one, plus the one the Border layout puts between its regions
	footer := container.New(layout.NewCustomPaddedLayout(theme.Padding(), 0, 0, 0),
		container.NewHBox(layout.NewSpacer(), container.New(outdentLayout{left: theme.Size(theme.SizeNameInnerPadding)}, aboutButton)),
	)

	// The hint layer covers everything, so a hint can open next to whichever icon asked for it
	ui.content = container.NewStack(
		container.New(layout.NewCustomPaddedLayout(28, 28, 32, 32),
			container.NewBorder(container.New(layout.NewCustomPaddedLayout(0, gutter, 0, 0), header), footer, nil, nil, body),
		),
		ui.hints.layer,
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

// The size the window is fixed at: wide enough by decree, and as tall as the taller of the two states needs,
// so switching between them never moves the frame. Wrapped text only knows its height once it has a width,
// so each state is laid out at the window's width before it's measured.
func (ui *appUI) windowSize() fyne.Size {
	pad := theme.Padding() * 2 // The window pads its content on every side
	var tallest float32
	for _, view := range []*fyne.Container{ui.setupView, ui.listeningView} {
		ui.setupView.Hide()
		ui.listeningView.Hide()
		view.Show()
		ui.content.Resize(fyne.NewSize(windowWidth-pad, ui.content.MinSize().Height))
		tallest = max(tallest, ui.content.MinSize().Height)
	}
	ui.listeningView.Hide()
	ui.setupView.Show()
	return fyne.NewSize(windowWidth, tallest+pad)
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

// An entry for a whole number: anything that isn't a digit is dropped as it's typed, and Enter starts listening
func (ui *appUI) numberEntry() *widget.Entry {
	entry := widget.NewEntry()
	entry.OnChanged = func(text string) {
		if digits := onlyDigits(text); digits != text {
			entry.SetText(digits) // Comes back through here, now digits only
		}
	}
	entry.OnSubmitted = func(string) { ui.start() }
	return entry
}

// A checkbox, a short number entry, its unit and a hint on one line, as in "Stop listening after [30] minutes ?"
func numberRow(rowLayout numberRowLayout, check *widget.Check, entry *widget.Entry, unit *widget.RichText, hint *hintIcon) *fyne.Container {
	return container.New(rowLayout, flushCheck(check), entry, flush(unit), hint)
}

// The number only matters while its checkbox is on, so the entry, the unit and the hint are dimmed along with it.
// The hint still opens, in case the sentence is what decides someone to turn the row on.
func (ui *appUI) enableNumberRow(entry *widget.Entry, unit *widget.RichText, hint *hintIcon, on bool) {
	style := quietStyle
	if on {
		entry.Enable()
	} else {
		entry.Disable()
		style.ColorName = theme.ColorNameDisabled
	}
	unit.Segments[0].(*widget.TextSegment).Style = style
	unit.Refresh()
	hint.dimmed = !on
	ui.recolorHints()
}

// The hint icons and the box they open are raw canvas objects, so their colours are set by hand, like the logo's
func (ui *appUI) recolorHints() {
	th := ui.app.Settings().Theme()
	variant := ui.app.Settings().ThemeVariant()
	for _, hint := range []*hintIcon{ui.autoStopHint, ui.holdHint} {
		name := colorNameGraphite
		if hint.dimmed {
			name = theme.ColorNameDisabled
		}
		hint.recolor(th.Color(name, variant))
	}
	ui.hints.recolor(th, variant)
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

	// Whatever was typed comes back clamped, so the entries show the numbers actually used
	minutes := autoStopMinutes.parse(ui.autoStopEntry.Text)
	ui.autoStopEntry.SetText(strconv.Itoa(minutes))
	seconds := holdSeconds.parse(ui.holdEntry.Text)
	ui.holdEntry.SetText(strconv.Itoa(seconds))

	ui.saveProblem.Hide()
	if ui.rememberCheck.Checked {
		saved := settings{Slug: slug, AutoStop: ui.autoStopCheck.Checked, AutoStopMinutes: minutes, Hold: ui.holdCheck.Checked, HoldSeconds: seconds}
		if err := saveSettings(token, saved); err != nil {
			setText(ui.saveProblem, "Some settings couldn’t be remembered, so you may need to enter them again next time. "+err.Error())
			ui.saveProblem.Show()
		}
	} else {
		forgetSettings()
	}

	// Clear any previous stop signals
	select {
	case <-stopMonitoringChan:
	default:
	}
	isMonitoring = true
	ui.listeningRun++
	run := ui.listeningRun
	ui.blockTitleText = strings.TrimSpace(ui.titleEntry.Text)
	var idleLimit, hold time.Duration
	if ui.autoStopCheck.Checked {
		idleLimit = time.Duration(minutes) * time.Minute
	}
	if ui.holdCheck.Checked {
		hold = time.Duration(seconds) * time.Second
	}
	go clipboardMonitoring(token, slug, ui.blockTitleText, idleLimit, hold, func() {
		fyne.Do(func() { ui.stopAfterIdle(run, minutes) })
	})

	ui.showListening(slug)
}

// Switches the screen to the listening state for the given channel
func (ui *appUI) showListening(slug string) {
	ui.stopNote.Hide()
	ui.sentCount = 0
	setText(ui.sentCountText, blocksSentText(ui.sentCount))
	ui.listeningDetail.Segments = []widget.RichTextSegment{
		&widget.TextSegment{Text: "Everything you copy is sent to ", Style: inline(quietStyle)},
		&widget.TextSegment{Text: breakableSlug(slug), Style: inline(labelStyle)}, // Bold, so it's clear where copies go
		&widget.TextSegment{Text: ".", Style: inline(quietStyle)},
	}
	ui.listeningDetail.Refresh()
	if ui.passage.Hidden {
		setText(ui.tileHint, "Copy some text to send your first block.")
	}
	ui.setupView.Hide()
	ui.listeningView.Show()
	ui.window.Canvas().Unfocus() // Enter in a field got here with the field still focused; Esc needs the keys
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
	// Copies still waiting out the hold aren't sent. The tile is cleared and told why here; their own cancelled
	// updates arrive a moment later and must leave that message alone, hence no copy is on the tile any more.
	if n := pendingSends.cancelAll(); n > 0 {
		message := "Listening stopped, so this copy wasn’t sent."
		if n > 1 {
			message = fmt.Sprintf("Listening stopped, so %d copies weren’t sent.", n)
		}
		ui.clearTile(message)
		ui.shownCopy = 0
	}
	if ui.passage.Hidden {
		setText(ui.tileHint, "Copied text shows up here.")
	}
	ui.stopBlinking()
	ui.listeningView.Hide()
	ui.setupView.Show()
}

// The timer ran out: stop exactly as the button does, then say so under the Start button
func (ui *appUI) stopAfterIdle(run int, minutes int) {
	if run != ui.listeningRun {
		return // Listening was stopped and started again in the meantime
	}
	ui.stop()
	setText(ui.stopNote, fmt.Sprintf("Stopped after %s without copies.", minutesText(minutes)))
	ui.stopNote.Show()
}

func minutesText(minutes int) string {
	if minutes == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", minutes)
}

// Calls off the newest copy waiting out the hold. The tile changes when its cancelled update comes through.
func (ui *appUI) undo() {
	pendingSends.undo()
}

func onlyDigits(text string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1 // Dropped
	}, text)
}

// Who made the app, and where to send feedback
func (ui *appUI) showAbout() {
	ui.aboutDialog().Show()
}

const aboutWidth = 460

func (ui *appUI) aboutDialog() dialog.Dialog {
	versionText := "Development build"
	if version != "dev" {
		versionText = "Version " + version
	}
	repo, _ := url.Parse(repoURL)
	issues, _ := url.Parse(issuesURL)

	content := container.New(layout.NewCustomPaddedVBoxLayout(0),
		container.New(layout.NewCustomPaddedHBoxLayout(12),
			container.NewCenter(ui.aboutLogo),
			container.New(layout.NewCustomPaddedVBoxLayout(2), flush(newText("Ctrl+C to Are.na", headingStyle)), flush(newText(versionText, quietStyle))),
		),
		gap(16),
		flush(newWrappedText("Made to archive interesting passages from books into an Are.na channel while reading, instead of copying and pasting each one by hand.", bodyStyle)),
		gap(12),
		flush(widget.NewHyperlink("Source code on GitHub", repo)),
		gap(4),
		flush(widget.NewHyperlink("Report a problem or suggest something", issues)),
	)
	about := dialog.NewCustom("About", "Close", container.New(layout.NewCustomPaddedLayout(8, 8, 8, 8), content), ui.window)

	// Wrapped text only knows its height once it has a width, so the width is set first and the height read back
	size := fyne.NewSize(aboutWidth, about.MinSize().Height)
	about.Resize(size)
	size.Height = about.MinSize().Height
	about.Resize(size)
	return about
}

func (ui *appUI) showUpdate(update arenaUpdate) {
	switch update.State {
	case blockSkipped:
		ui.shownCopy = update.Copy
		ui.showSkipped(update.Message)
		return
	case blockPending:
		ui.showPending(update)
		return
	case blockCancelled:
		if update.Copy == ui.shownCopy {
			ui.clearTile("Not sent.")
			ui.shownCopy = 0 // So an older copy still waiting can take the tile
		}
		return
	case blockSent:
		ui.sentCount++
		setText(ui.sentCountText, blocksSentText(ui.sentCount))
	}
	if update.Copy < ui.shownCopy && update.State != blockFailed {
		return // A newer copy is on the tile. This one has been counted, and that's enough.
	}
	ui.shownCopy = update.Copy
	ui.pendingRow.Hide()

	ui.tileHint.Hide()
	ui.setPassage(update.Content)
	ui.passage.Show()
	setText(ui.blockTitle, ui.blockTitleText)
	ui.blockTitle.Hidden = ui.blockTitleText == ""

	// The channel name is bold, so it's clear where copies go
	channel := &widget.TextSegment{Text: breakableSlug(update.Channel), Style: inline(captionBoldStyle)}
	ui.tileFailed = false
	switch update.State {
	case blockSending:
		ui.blockStatus.Segments = []widget.RichTextSegment{
			&widget.TextSegment{Text: "Sending to ", Style: inline(captionStyle)},
			channel,
			&widget.TextSegment{Text: "…", Style: captionStyle},
		}
	case blockSent:
		ui.blockStatus.Segments = []widget.RichTextSegment{
			&widget.TextSegment{Text: "Sent to ", Style: inline(captionStyle)},
			channel,
			&widget.TextSegment{Text: " at " + time.Now().Format("15:04"), Style: captionStyle},
		}
	case blockFailed:
		errorStyle := captionStyle
		errorStyle.ColorName = theme.ColorNameError
		ui.blockStatus.Segments = []widget.RichTextSegment{
			&widget.TextSegment{Text: update.Message, Style: errorStyle},
		}
		ui.tileFailed = true
	}
	ui.blockStatus.Show()
	ui.blockStatus.Refresh()
	ui.refreshColors()

	if update.State == blockSent {
		ui.flashTile()
	}
}

// A held-back copy isn't shown, so the tile says why nothing was sent, in its usual quiet caption rather than
// in red: nothing went wrong. The next copy that's sent takes the tile back.
func (ui *appUI) showSkipped(message string) {
	ui.passage.Hide()
	ui.blockTitle.Hide()
	ui.blockStatus.Hide()
	ui.pendingRow.Hide()
	ui.tileFailed = false
	setText(ui.tileHint, message)
	ui.tileHint.Show()
	ui.refreshColors()
}

// A copy waiting out the hold is shown like any other, with the countdown and Undo under it instead of a status.
// Ticks from a copy older than the one on the tile are ignored, so the newest stays in view.
func (ui *appUI) showPending(update arenaUpdate) {
	if update.Copy < ui.shownCopy {
		return
	}
	if update.Copy != ui.shownCopy {
		ui.shownCopy = update.Copy
		ui.tileHint.Hide()
		ui.setPassage(update.Content)
		ui.passage.Show()
		setText(ui.blockTitle, ui.blockTitleText)
		ui.blockTitle.Hidden = ui.blockTitleText == ""
		ui.tileFailed = false
		ui.refreshColors()
	}
	text := fmt.Sprintf("Sending in %d s…", update.SecondsLeft)
	if update.Waiting > 1 {
		text += fmt.Sprintf(" %d copies waiting", update.Waiting)
	}
	setText(ui.pendingText, text)
	ui.blockStatus.Hide()
	ui.pendingRow.Show()
}

// Empties the tile and says why under it, in the usual quiet caption: nothing went wrong
func (ui *appUI) clearTile(status string) {
	ui.passage.Hide()
	ui.blockTitle.Hide()
	ui.pendingRow.Hide()
	ui.tileFailed = false
	setText(ui.tileHint, "")
	ui.tileHint.Show()
	ui.blockStatus.Segments = []widget.RichTextSegment{&widget.TextSegment{Text: status, Style: captionStyle}}
	ui.blockStatus.Show()
	ui.blockStatus.Refresh()
	ui.refreshColors()
}

// Colors that Fyne doesn't update by itself when the system switches between light and dark
func (ui *appUI) refreshColors() {
	th := ui.app.Settings().Theme()
	variant := ui.app.Settings().ThemeVariant()

	ink := th.Color(theme.ColorNameForeground, variant)

	ui.logo.Image = tintedLogo(ink)
	ui.logo.Refresh()
	ui.aboutLogo.Image = ui.logo.Image
	ui.aboutLogo.Refresh()

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

	ui.recolorHints()
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

// Fyne only wraps lines at spaces, so a long slug would break mid-word.
// A hair space after each hyphen is nearly invisible, and lets the slug wrap at its hyphens instead.
func breakableSlug(slug string) string {
	return strings.ReplaceAll(slug, "-", "- ")
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

// Watches the clipboard until told to stop, or until idleLimit passes without a new copy (never, when zero),
// in which case onIdle is called from this goroutine before it returns. Copies wait out hold before they're
// sent, unless it's zero.
func clipboardMonitoring(_accessToken string, _channelSlug string, _blockTitle string, idleLimit time.Duration, hold time.Duration, onIdle func()) {

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

	// A nil channel never fires, so when the timer is off there simply isn't one
	var idleTimer *time.Timer
	var idle <-chan time.Time
	if idleLimit > 0 {
		idleTimer = time.NewTimer(idleLimit)
		defer idleTimer.Stop()
		idle = idleTimer.C
	}

	for {
		select {
		case <-ticker.C:
			// Copies from password managers are marked as concealed. Checked before the read, so the
			// secret isn't even read, and again after it, in case one was copied while the read ran.
			if clipboardIsConcealed() {
				continue
			}
			currentClipboardContent, err := clipboard.ReadAll()
			if err != nil {
				// Ignore temporary read errors, but log if useful
				// log.Printf("Error reading clipboard: %v\n", err)
				continue
			}
			if clipboardIsConcealed() {
				continue
			}

			// If the content changed and is not empty
			if currentClipboardContent != lastClipboardContent && currentClipboardContent != "" {
				lastClipboardContent = currentClipboardContent // Update the last content
				if idleTimer != nil {
					idleTimer.Reset(idleLimit) // Every new copy counts, whether it's then sent, held back or fails
				}

				// Handled in a goroutine to avoid blocking the check
				copyCount++
				go handleCopy(_accessToken, _channelSlug, lastClipboardContent, _blockTitle, copyCount, hold)
			}

		case <-idle:
			isMonitoring = false
			onIdle()
			return

		case <-stopMonitoringChan:
			// fmt.Println("\n🛑 Stopping the monitor...")
			isMonitoring = false
			return // Exit the program
		}

	}
}

// Removes the text's formatting so it reads cleanly in a block: words hyphenated at the end of
// a line are joined again, then line breaks, tabs and runs of spaces (including non-breaking ones)
// all become a single space.
func unformatted(text string) string {
	return strings.Join(strings.Fields(dehyphenated(text)), " ")
}

const softHyphen = '\u00ad' // Marks where a word may be broken; invisible unless it is

// Words that follow a hanging hyphen, as in "pre- and post-war" or "corto- y largo plazo"
var hangingHyphenNext = map[string]bool{
	"and": true, "or": true, "nor": true, "to": true, // English
	"y": true, "o": true, "e": true, "u": true, "ou": true, "et": true, // Spanish, Portuguese, Italian, French
	"und": true, "oder": true, // German
}

// Joins words that were hyphenated at the end of a line, as happens when text is copied from a PDF
// or a justified column: "he-\nllo" becomes "hello" instead of "he- llo".
//
// The hyphen is kept when it most likely belongs to the word rather than to the line break: a capital
// follows it ("Jean-\nPaul"), a digit touches it ("COVID-\n19"), a single letter precedes it ("e-\nmail"),
// the word has another hyphen ("mother-in-\nlaw"), or it hangs before a conjunction ("pre-\nand post-war").
// Soft hyphens only mark where a word may be broken, so they're removed wherever they are.
func dehyphenated(text string) string {
	runes := []rune(text)
	var out strings.Builder
	out.Grow(len(text))
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r != softHyphen && !isHyphen(r) {
			out.WriteRune(r)
			continue
		}
		next, broken := lineBreakAfter(runes, i)
		insideWord := i > 0 && isWordRune(runes[i-1]) && next < len(runes) && isWordRune(runes[next])
		switch {
		case r == softHyphen:
			if broken && insideWord {
				i = next - 1 // The break happened here, so it goes with the soft hyphen
			}
		case broken && insideWord:
			out.WriteString(hyphenReplacement(runes, i, next))
			i = next - 1
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

// What takes the place of a hyphen at the end of a line and of the line break after it
func hyphenReplacement(runes []rune, hyphen, next int) string {
	head := wordRunesBefore(runes, hyphen) // "mother-in-\nlaw": "in"
	tail := wordRunesAfter(runes, next)    // "law"
	beforeHead := hyphen - len(head) - 1
	afterTail := next + len(tail)
	kept := string(runes[hyphen])
	switch {
	case hangingHyphenNext[strings.ToLower(string(tail))]:
		return kept + " " // "pre- and post-war"
	case unicode.IsUpper(tail[0]), // "Jean-Paul"
		unicode.IsDigit(runes[hyphen-1]), unicode.IsDigit(tail[0]), // "COVID-19", "mid-1990s"
		len(head) == 1, // "e-mail", "X-ray"
		beforeHead >= 1 && isHyphen(runes[beforeHead]) && isWordRune(runes[beforeHead-1]), // "mother-in-law"
		afterTail+1 < len(runes) && isHyphen(runes[afterTail]) && isWordRune(runes[afterTail+1]):
		return kept
	default:
		return "" // "hel-" + "lo" → "hello"
	}
}

// Index of the first rune after the whitespace that follows position i, and whether that whitespace
// holds exactly one line break: a paragraph break never splits a word.
func lineBreakAfter(runes []rune, i int) (next int, oneBreak bool) {
	breaks := 0
	for next = i + 1; next < len(runes) && unicode.IsSpace(runes[next]); next++ {
		if isLineBreak(runes[next]) && !(runes[next] == '\n' && runes[next-1] == '\r') {
			breaks++
		}
	}
	return next, breaks == 1
}

func wordRunesBefore(runes []rune, i int) []rune {
	start := i
	for start > 0 && isWordRune(runes[start-1]) {
		start--
	}
	return runes[start:i]
}

func wordRunesAfter(runes []rune, i int) []rune {
	end := i
	for end < len(runes) && isWordRune(runes[end]) {
		end++
	}
	return runes[i:end]
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// The hyphen-minus and the typographic hyphen; dashes aren't used to break words
func isHyphen(r rune) bool { return r == '-' || r == '\u2010' }

func isLineBreak(r rune) bool {
	switch r {
	case '\n', '\r', '\v', '\f', '\u0085', '\u2028', '\u2029':
		return true
	}
	return false
}

// What becomes of a new copy: held back if it looks like a secret, dropped if it's blank, and otherwise sent,
// after waiting out the hold when there is one. Nothing enters the queue before those checks.
func handleCopy(token, channelSlug, content, blockTitle string, copyNumber int, hold time.Duration) {
	// Keys and passwords are held back before anything else is done with the text
	if secret, what := looksLikeSecret(content); secret {
		arenaUpdates <- arenaUpdate{State: blockSkipped, Copy: copyNumber, Channel: channelSlug, Message: fmt.Sprintf("Looks like %s, so it wasn’t sent.", what)}
		return
	}

	// Formats the text before sending
	text := unformatted(content)
	if text == "" {
		return // Nothing but spaces or line breaks was copied
	}

	if hold == 0 {
		sendToArena(token, channelSlug, text, blockTitle, copyNumber)
		return
	}
	update := arenaUpdate{State: blockPending, Copy: copyNumber, Channel: channelSlug, Content: text}
	pendingSends.hold(copyNumber, hold,
		func(left, waiting int) {
			update.SecondsLeft, update.Waiting = left, waiting
			arenaUpdates <- update
		},
		func() { sendToArena(token, channelSlug, text, blockTitle, copyNumber) },
		func() { arenaUpdates <- arenaUpdate{State: blockCancelled, Copy: copyNumber, Channel: channelSlug} },
	)
}

// Posts one cleaned-up text as a block, reporting progress through arenaUpdates
func sendToArena(token, channelSlug, content string, blockTitle string, copyNumber int) {
	update := arenaUpdate{State: blockSending, Copy: copyNumber, Channel: channelSlug, Content: content}
	arenaUpdates <- update

	fail := func(message string) {
		update.State = blockFailed
		update.Message = message
		arenaUpdates <- update
	}

	blockData := ArenaBlock{
		Value:    content,
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
