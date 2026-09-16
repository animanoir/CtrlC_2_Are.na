package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/png"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// -- Fonts (SIL Open Font License, see fonts/OFL-*.txt)
//
//go:embed fonts/SchibstedGrotesk-Regular.ttf
var uiRegularFontBytes []byte

//go:embed fonts/SchibstedGrotesk-SemiBold.ttf
var uiSemiBoldFontBytes []byte

//go:embed fonts/Literata-Regular.ttf
var passageFontBytes []byte

var (
	uiRegularFont  = fyne.NewStaticResource("SchibstedGrotesk-Regular.ttf", uiRegularFontBytes)
	uiSemiBoldFont = fyne.NewStaticResource("SchibstedGrotesk-SemiBold.ttf", uiSemiBoldFontBytes)
	passageFont    = fyne.NewStaticResource("Literata-Regular.ttf", passageFontBytes)
)

const (
	colorNameGraphite fyne.ThemeColorName = "graphite" // Secondary text
	sizeNameTitle     fyne.ThemeSizeName  = "title"
)

// Are.na is black and white, so the app is too. The only hue is red, reserved for errors.
type palette struct {
	paper, ink, graphite, hairline, wash, placeholder, disabled, disabledButton, danger color.Color
}

var lightPalette = palette{
	paper:          color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
	ink:            color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xff},
	graphite:       color.NRGBA{R: 0x66, G: 0x66, B: 0x66, A: 0xff},
	hairline:       color.NRGBA{R: 0xde, G: 0xde, B: 0xde, A: 0xff},
	wash:           color.NRGBA{R: 0xf4, G: 0xf4, B: 0xf4, A: 0xff},
	placeholder:    color.NRGBA{R: 0x70, G: 0x70, B: 0x70, A: 0xff},
	disabled:       color.NRGBA{R: 0xa3, G: 0xa3, B: 0xa3, A: 0xff},
	disabledButton: color.NRGBA{R: 0xed, G: 0xed, B: 0xed, A: 0xff},
	danger:         color.NRGBA{R: 0xb3, G: 0x26, B: 0x1e, A: 0xff},
}

var darkPalette = palette{
	paper:          color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xff},
	ink:            color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
	graphite:       color.NRGBA{R: 0x9e, G: 0x9e, B: 0x9e, A: 0xff},
	hairline:       color.NRGBA{R: 0x2e, G: 0x2e, B: 0x2e, A: 0xff},
	wash:           color.NRGBA{R: 0x14, G: 0x14, B: 0x14, A: 0xff},
	placeholder:    color.NRGBA{R: 0x8a, G: 0x8a, B: 0x8a, A: 0xff},
	disabled:       color.NRGBA{R: 0x5c, G: 0x5c, B: 0x5c, A: 0xff},
	disabledButton: color.NRGBA{R: 0x1a, G: 0x1a, B: 0x1a, A: 0xff},
	danger:         color.NRGBA{R: 0xff, G: 0x7b, B: 0x6e, A: 0xff},
}

// Mid-gray overlays read on black and white surfaces alike, so keyboard focus stays visible on every button.
var (
	hoverTint     = color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0x24}
	pressedTint   = color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0x40}
	focusTint     = color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0x66}
	selectionTint = color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0x4d}
)

// Are.na theme. Follows the system's light or dark appearance.
type arenaTheme struct{}

var _ fyne.Theme = (*arenaTheme)(nil)

func (m *arenaTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	p := lightPalette
	if variant == theme.VariantDark {
		p = darkPalette
	}

	switch name {
	case theme.ColorNameBackground, theme.ColorNameForegroundOnPrimary, theme.ColorNameForegroundOnError:
		return p.paper
	case theme.ColorNameForeground, theme.ColorNamePrimary, theme.ColorNameHyperlink:
		return p.ink
	case colorNameGraphite:
		return p.graphite
	case theme.ColorNameInputBorder, theme.ColorNameSeparator:
		return p.hairline
	case theme.ColorNameInputBackground, theme.ColorNameButton:
		return p.wash
	case theme.ColorNamePlaceHolder:
		return p.placeholder
	case theme.ColorNameDisabled:
		return p.disabled
	case theme.ColorNameDisabledButton:
		return p.disabledButton
	case theme.ColorNameError:
		return p.danger
	case theme.ColorNameHover:
		return hoverTint
	case theme.ColorNamePressed:
		return pressedTint
	case theme.ColorNameFocus:
		return focusTint
	case theme.ColorNameSelection:
		return selectionTint
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (m *arenaTheme) Font(style fyne.TextStyle) fyne.Resource {
	switch {
	case style.Monospace, style.Symbol:
		return theme.DefaultTheme().Font(style)
	case style.Bold:
		return uiSemiBoldFont
	}
	return uiRegularFont
}

func (m *arenaTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (m *arenaTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 4
	case theme.SizeNameInnerPadding:
		return 10
	case theme.SizeNameCaptionText:
		return 12
	case theme.SizeNameText:
		return 14
	case theme.SizeNameSubHeadingText:
		return 18
	case sizeNameTitle:
		return 21
	case theme.SizeNameInputRadius:
		return 6
	case theme.SizeNameSelectionRadius:
		return 3
	}
	return theme.DefaultTheme().Size(name)
}

// Cancels the padding a widget draws around its content (on the left, and on the top and bottom),
// so the content lines up with the edges of fields and buttons.
type outdentLayout struct {
	left, vertical float32
}

func (l outdentLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		o.Move(fyne.NewPos(-l.left, -l.vertical))
		o.Resize(size.AddWidthHeight(l.left*2, l.vertical*2))
	}
}

func (l outdentLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	min := fyne.NewSize(0, 0)
	for _, o := range objects {
		min = min.Max(o.MinSize().SubtractWidthHeight(l.left*2, l.vertical*2))
	}
	return min
}

// Text widgets are padded on every side
func flush(obj fyne.CanvasObject) *fyne.Container {
	pad := theme.Size(theme.SizeNameInnerPadding)
	return container.New(outdentLayout{left: pad, vertical: pad}, obj)
}

// Checkboxes draw their box a little in from the left
func flushCheck(check *widget.Check) *fyne.Container {
	return container.New(outdentLayout{left: theme.Size(theme.SizeNameInnerPadding)/2 + theme.Size(theme.SizeNameInputBorder)}, check)
}

// Recolors the white Are.na logo so it reads on light and dark backgrounds.
func tintedLogo(ink color.Color) image.Image {
	src, err := png.Decode(bytes.NewReader(arenaLogoBytes))
	if err != nil {
		return nil
	}
	r, g, b, _ := ink.RGBA()
	bounds := src.Bounds()
	out := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := src.At(x, y).RGBA()
			out.SetNRGBA(x, y, color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)})
		}
	}
	return out
}
