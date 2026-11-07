package main

import (
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"github.com/zalando/go-keyring"
)

// Renders the window in both states and both appearances to PNG files, to check the layout without launching
// the app (which reads the keychain). Skipped unless RENDER_DIR names the folder to write into:
//
//	RENDER_DIR=/tmp/ctrlc2arena go test -run TestRenderStates .
func TestRenderStates(t *testing.T) {
	dir := os.Getenv("RENDER_DIR")
	if dir == "" {
		t.Skip("set RENDER_DIR to a folder to render the window into")
	}
	keyring.MockInit()

	for name, variant := range map[string]fyne.ThemeVariant{"light": theme.VariantLight, "dark": theme.VariantDark} {
		a := test.NewApp()
		a.Settings().SetTheme(fixedVariant{&arenaTheme{}, variant})
		w := a.NewWindow("")
		ui := newAppUI(a, w)
		w.SetContent(ui.content)
		w.Resize(ui.windowSize())
		ui.holdCheck.SetChecked(false) // Whatever the saved settings say, show both states of the row
		render(t, w, filepath.Join(dir, "setup-hold-off-"+name+".png"))
		ui.holdCheck.SetChecked(true)
		render(t, w, filepath.Join(dir, "setup-hold-on-"+name+".png"))
		ui.holdHint.MouseIn(nil)
		render(t, w, filepath.Join(dir, "setup-hint-"+name+".png"))
		ui.holdHint.MouseOut()

		ui.showListening("reading-notes-2024")
		ui.showUpdate(arenaUpdate{State: blockSent, Channel: "reading-notes-2024",
			Content: "It is a truth universally acknowledged, that a single man in possession of a good fortune, must be in want of a wife."})
		render(t, w, filepath.Join(dir, "listening-"+name+".png"))

		ui.showUpdate(arenaUpdate{State: blockPending, Copy: 2, Channel: "reading-notes-2024", SecondsLeft: 4, Waiting: 2,
			Content: "Whatever our souls are made of, his and mine are the same."})
		render(t, w, filepath.Join(dir, "pending-"+name+".png"))
	}
}

func render(t *testing.T, w fyne.Window, path string) {
	t.Helper()
	img := software.RenderCanvas(w.Canvas(), fyne.CurrentApp().Settings().Theme())
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("rendered %s (%v)", path, img.Bounds().Size())
}

// The test app has no light/dark setting, so the variant is pinned by the theme itself
type fixedVariant struct {
	fyne.Theme
	variant fyne.ThemeVariant
}

func (t fixedVariant) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	return t.Theme.Color(name, t.variant)
}
