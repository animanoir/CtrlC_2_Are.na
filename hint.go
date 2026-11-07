package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	hintIconSize = 14  // A little under the text's line height, so it sits in the row like a character
	hintWidth    = 300 // Of the box a hint opens
)

// A small "?" in a circle at the end of a row. The sentence it stands for opens next to it on hover or tap.
type hintIcon struct {
	widget.BaseWidget
	text   string
	hints  *hintLayer
	circle *canvas.Circle
	glyph  *canvas.Text
	dimmed bool // Coloured like the rest of its row, by refreshColors
}

func newHintIcon(hints *hintLayer, text string) *hintIcon {
	h := &hintIcon{text: text, hints: hints}
	h.circle = canvas.NewCircle(color.Transparent)
	h.circle.StrokeWidth = 1
	h.glyph = canvas.NewText("?", color.Transparent)
	h.glyph.TextSize = 10
	h.glyph.TextStyle = fyne.TextStyle{Bold: true}
	h.ExtendBaseWidget(h)
	return h
}

func (h *hintIcon) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewStack(h.circle, container.NewCenter(h.glyph)))
}

func (h *hintIcon) MinSize() fyne.Size {
	return fyne.NewSquareSize(hintIconSize)
}

func (h *hintIcon) recolor(c color.Color) {
	h.circle.StrokeColor = c
	h.glyph.Color = c
	h.circle.Refresh()
	h.glyph.Refresh()
}

func (h *hintIcon) MouseIn(*desktop.MouseEvent) {
	h.hints.show(h)
}

func (h *hintIcon) MouseMoved(*desktop.MouseEvent) {}

func (h *hintIcon) MouseOut() {
	h.hints.hideFor(h)
}

func (h *hintIcon) Tapped(*fyne.PointEvent) {
	if h.hints.shownFor == h {
		h.hints.hide()
	} else {
		h.hints.show(h)
	}
}

// Draws one hint at a time, next to the icon that asked for it. It lives inside the window's content rather than
// as a Fyne overlay: an overlay takes the mouse, so the icon would lose it the moment the hint appeared.
type hintLayer struct {
	layer    *fyne.Container // Covers the content; the box inside is placed by hand
	box      *fyne.Container
	bg       *canvas.Rectangle
	text     *widget.RichText
	shownFor *hintIcon
}

func newHintLayer() *hintLayer {
	l := &hintLayer{}
	l.bg = canvas.NewRectangle(color.Transparent)
	l.bg.StrokeWidth = 1
	l.bg.CornerRadius = theme.Size(theme.SizeNameInputRadius)
	l.text = newWrappedText("", bodyStyle)
	l.box = container.NewStack(l.bg, container.New(layout.NewCustomPaddedLayout(8, 8, 10, 10), flush(l.text)))
	l.box.Hide()
	l.layer = container.NewWithoutLayout(l.box)
	return l
}

func (l *hintLayer) show(icon *hintIcon) {
	setText(l.text, icon.text)
	// Wrapped text only knows its height once it has a width
	l.box.Resize(fyne.NewSize(hintWidth, l.box.MinSize().Height))
	l.box.Resize(fyne.NewSize(hintWidth, l.box.MinSize().Height))

	driver := fyne.CurrentApp().Driver()
	pos := driver.AbsolutePositionForObject(icon).Subtract(driver.AbsolutePositionForObject(l.layer))
	pos.Y += icon.Size().Height + theme.Padding()
	pos.X = max(0, min(pos.X, l.layer.Size().Width-hintWidth)) // Kept inside the window
	l.box.Move(pos)
	l.box.Show()
	l.shownFor = icon
	l.repaint(icon)
}

// Closes the hint. Reports whether one was open.
func (l *hintLayer) hide() bool {
	if l.shownFor == nil {
		return false
	}
	l.box.Hide()
	icon := l.shownFor
	l.shownFor = nil
	l.repaint(icon)
	return true
}

// Showing or hiding a container doesn't redraw the window by itself, and the box isn't known to the canvas until
// it has been painted once, so the redraw is asked for through objects that are: the layer and the icon.
func (l *hintLayer) repaint(icon *hintIcon) {
	l.layer.Refresh()
	icon.Refresh()
}

func (l *hintLayer) hideFor(icon *hintIcon) {
	if l.shownFor == icon {
		l.hide()
	}
}

func (l *hintLayer) recolor(th fyne.Theme, variant fyne.ThemeVariant) {
	l.bg.FillColor = th.Color(theme.ColorNameOverlayBackground, variant)
	l.bg.StrokeColor = th.Color(theme.ColorNameSeparator, variant)
	l.bg.Refresh()
}
