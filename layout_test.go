package main

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"github.com/zalando/go-keyring"
)

// The window is fixed at windowSize(), so each state has to fit in it, including the note shown after an
// automatic stop, and the preview tile has to stay square.
func TestWindowFitsBothStates(t *testing.T) {
	keyring.MockInit() // Settings are loaded on construction; leave the real password manager alone
	a := test.NewApp()
	a.Settings().SetTheme(&arenaTheme{})
	w := a.NewWindow("")
	ui := newAppUI(a, w)
	setText(ui.stopNote, "Stopped after 720 minutes without copies.")
	ui.stopNote.Show()
	w.SetContent(ui.content)
	size := ui.windowSize()
	w.Resize(size)
	pad := theme.Padding() * 2

	for _, state := range []struct {
		name      string
		listening bool
	}{{"setup", false}, {"listening", true}} {
		if state.listening {
			ui.showListening("reading-notes")
		} else {
			ui.stop()
		}
		ui.content.Resize(fyne.NewSize(size.Width-pad, size.Height-pad))

		if need := ui.content.MinSize(); need.Width+pad > size.Width || need.Height+pad > size.Height {
			t.Errorf("%s needs %v, but the window is fixed at %v", state.name, need, size)
		}
		if tile := ui.tileBorder.Size(); tile != fyne.NewSquareSize(tileSize) {
			t.Errorf("%s: the preview tile is %v, want a square of %v", state.name, tile, tileSize)
		}
	}

	// Both number rows are built on one layout, so their entries line up and match in size
	if a, h := ui.autoStopEntry.Position(), ui.holdEntry.Position(); a.X != h.X {
		t.Errorf("the number entries start at x=%v and x=%v, want the same", a.X, h.X)
	}
	if a, h := ui.autoStopEntry.Size(), ui.holdEntry.Size(); a != h {
		t.Errorf("the number entries are %v and %v, want the same size", a, h)
	}

	// Turning a row off only dims it; nothing moves
	entryBefore, hintBefore := ui.holdEntry.Position(), ui.holdHint.Position()
	ui.holdCheck.SetChecked(!ui.holdCheck.Checked)
	ui.content.Resize(fyne.NewSize(size.Width-pad, size.Height-pad))
	if entryAfter, hintAfter := ui.holdEntry.Position(), ui.holdHint.Position(); entryAfter != entryBefore || hintAfter != hintBefore {
		t.Errorf("toggling the hold row moved its entry from %v to %v and its hint from %v to %v", entryBefore, entryAfter, hintBefore, hintAfter)
	}

	// Hovering the icon, found by Fyne's own hit-testing, opens its hint inside the window; leaving closes it.
	// Back on the setup screen first: the loop above ends in the listening state, where the icon is hidden.
	ui.stop()
	ui.content.Resize(fyne.NewSize(size.Width-pad, size.Height-pad))
	icon := a.Driver().AbsolutePositionForObject(ui.holdHint)
	test.MoveMouse(w.Canvas(), icon.Add(fyne.NewPos(hintIconSize/2, hintIconSize/2)))
	if !ui.hints.box.Visible() {
		t.Fatal("hovering the hint icon opened nothing")
	}
	box := ui.hints.box
	if box.Position().X < 0 || box.Position().X+box.Size().Width > size.Width || box.Position().Y+box.Size().Height > size.Height {
		t.Errorf("the hint box at %v with size %v runs outside the %v window", box.Position(), box.Size(), size)
	}
	test.MoveMouse(w.Canvas(), fyne.NewPos(10, 10))
	if ui.hints.box.Visible() {
		t.Error("leaving the hint icon left its hint open")
	}
}
