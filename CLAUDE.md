# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A single-binary desktop app (Go + Fyne) that polls the clipboard and posts each new text copy as a Text block to an Are.na channel via the Are.na API v3 (`POST https://api.are.na/v3/blocks`). Everything is in `package main` at the repo root.

## Commands

```bash
go run .                                        # run the app
go build                                        # build for the current OS
go test ./...                                   # run all tests
go test -run 'TestUnformatted/proper_name' -v . # one table case (spaces in case names become _)
```

- Fyne needs cgo. On Linux, install `libgl1-mesa-dev xorg-dev` first. On Windows, build with `-ldflags="-H=windowsgui"` so no console window opens.
- `layout_test.go` builds the whole UI headlessly with `fyne.io/fyne/v2/test` plus `keyring.MockInit()`. Use the same setup to measure `MinSize()`s when changing the layout: it needs no window and never touches the real keychain (launching the real binary does, and prompts if a token is saved).
- `RENDER_DIR=/some/folder go test -run TestRenderStates .` renders both states in light and dark to PNGs in that folder (`render_test.go`, skipped otherwise). That is how to check the layout visually without launching the app.
- Cross-OS `go vet` fails on Fyne, so `clipboard_windows.go` can't be checked from macOS directly. Copy it into a scratch module whose `go.mod` requires `golang.org/x/sys`, add a stub `main`, and run `GOOS=windows go vet -C <dir> .`.
- CI (`.github/workflows/build.yml`) only builds the three platforms; it does not run tests or `go vet`. Run them locally.

## Releasing

Pushing a `vX.Y.Z` tag builds Linux, Windows, and a macOS universal binary (amd64 + arm64 merged with `lipo`), zips them, and creates a GitHub release. The release notes are the `## vX.Y.Z` section of `CHANGELOG.md`; the workflow fails on purpose if that section is missing, so add it before tagging. Versions follow semver as described in the README.

The About dialog shows `version` (`var version = "dev"` in `ctrlc2arena.go`). Each build job sets it with `-ldflags "-X main.version=$VERSION"`, where `VERSION` is the tag without its leading `v`, or `dev` on non-tag builds. Pass the same flag to a local `go build` to test the dialog's version line.

## Architecture

**Files**
- `ctrlc2arena.go`: UI, clipboard loop, text cleanup, API call.
- `secrets.go`: the heuristic that holds back keys, tokens and passwords.
- `hold.go`: the queue of copies waiting out the hold before they're sent.
- `hint.go`: the "?" icon and the hint it opens, drawn inside the content rather than as a Fyne overlay (an overlay takes the mouse, so the icon would lose hover at once).
- `clipboard_darwin.go` / `clipboard_windows.go` / `clipboard_other.go`: build-tagged `clipboardIsConcealed()`.
- `settings.go`: storing the token and slug.
- `theme.go`: custom Fyne theme, embedded fonts, layout helpers.

**Concurrency flow**
- `ui.start()` launches `clipboardMonitoring`, which polls every 2s and stops via `stopMonitoringChan`.
- Each new copy spawns a `sendToArena` goroutine.
- `sendToArena` reports progress (`blockSending` → `blockSent`/`blockFailed`) by sending `arenaUpdate` values on the buffered `arenaUpdates` channel.
- One goroutine started in `runGui` drains that channel and applies updates through `fyne.Do`. UI changes from background goroutines must go through this path (or `fyne.Do`), never directly.
- Every new copy gets a number (`copyCount`) and goes to `handleCopy`, which runs the secret filter and the blank check first and then either calls `sendToArena` or, with the hold on, `pendingSends.hold` (`hold.go`): the copy's own goroutine waits out the hold, sending a `blockPending` tick each second, then sends. `undo()` closes the newest entry's cancel channel and `cancelAll()` (called from `ui.stop()`, so the auto-stop too) every one, which makes the goroutine send `blockCancelled` instead. The UI keeps the newest copy on the tile (`shownCopy`) and ignores updates from older ones except to count them. Esc is wired in `runGui`. `hold_test.go` drives the queue with a fast tick and fake senders.
- `clipboardMonitoring` also owns the inactivity timer: a `time.Timer` reset on every new copy (sent, held back or failed), with a nil channel when the timer is off. When it fires, the goroutine calls the `onIdle` callback that `start()` passed, which `fyne.Do`s `ui.stopAfterIdle`; that goes through `ui.stop()` so the UI ends exactly as after a manual stop, then shows the note under Start. `listeningRun` is compared first so a timer from an earlier run can't stop a later one.

**Text cleanup**
- `unformatted()` runs before sending: `dehyphenated()` rejoins words split by a line-end hyphen, then all whitespace collapses to single spaces.
- The heuristics in `hyphenReplacement` decide when to keep the hyphen (capital after, digit touching, single-letter head, multi-hyphen compound) or keep a trailing space for hanging hyphens before conjunctions listed in `hangingHyphenNext`.
- `ctrlc2arena_test.go` is a table of these cases, including documented known limits (e.g. `well-\nknown` → `wellknown`). Add a case there for any change to this logic.

**Secret filtering** (two independent layers)
- `clipboardIsConcealed()` reads the flag password managers put on the pasteboard (`org.nspasteboard.ConcealedType`/`TransientType` on macOS, the `ExcludeClipboardContentFromMonitorProcessing` format on Windows; Linux has none and always returns false). `clipboardMonitoring` calls it before *and* after each `ReadAll`, so a concealed copy is normally never read, and one that lands mid-read is dropped. Nothing is shown or sent.
- `looksLikeSecret()` runs on the raw text at the top of `sendToArena`, before `unformatted()`. Whole-word shape matches come first (known key prefixes, JWT, private-key header, `Bearer`, password in a URL), then an entropy check that only applies when the whole copy is a single word. A match sends a `blockSkipped` update with `Content` left empty; the tile shows the note in place of a preview.
- Any change to the heuristic needs a case in `secrets_test.go`. That table also documents the known limits, both secrets that pass on purpose and harmless strings that are held back, so keep those sections honest when behaviour shifts.

**Settings storage**
- The token goes only into the OS keyring (`github.com/zalando/go-keyring`, service `"Ctrl+C to Are.na"`, user `"personal-access-token"`).
- `os.UserConfigDir()/ctrlc2arena/settings.json` (0600) holds `slug`, `autoStop` (bool, default true), `autoStopMinutes` (default 30, clamped 1–720), `hold` (bool, default false) and `holdSeconds` (default 5, clamped 1–30). `loadSettings` fills a `defaultSettings()` value before `json.Unmarshal`, so keys missing from an older file keep their defaults. The numbers are `numberSetting`s (`autoStopMinutes`, `holdSeconds`) whose `parse`/`clamp` handle both the entries and the file, with a table test in `settings_test.go`.
- `migrateLegacySettings` moves an old plain-text `arena_settings.json` from the working directory, and deletes it only if the save succeeded.
- Never add a plain-text fallback for the token. The README promises there isn't one, and it documents the exact keyring names and paths, so keep the README in sync if those change.

**Theming quirks (Fyne-specific)**
- The palette is black/white with red reserved for errors, with light and dark variants in `theme.go`.
- The window is fixed-size (`SetFixedSize(true)`), `windowWidth` wide and as tall as the taller state needs: `ui.windowSize()` lays the content out at that width in each state and takes the max, so new controls change the height automatically (`TestWindowFitsBothStates` guards it). A wrapped `RichText` reports a one-line `MinSize` until it has been given a width, so always `Resize` before measuring; `aboutDialog()` does the same.
- Fyne does not recolor raw `canvas.*` objects or images when the system switches light/dark. Anything like that (the logo tint, listening dot, preview lines, tile border, the hint icons and their box) has to be updated in `appUI.refreshColors()`, which runs on a settings listener.
- The block preview tile sets copied text in Literata. Fyne's own wrapping measures with the UI font, so `setPassage`/`wrapText` measure and wrap lines manually.
- `flush()`/`flushCheck()` cancel widget padding so text lines up with field edges.
- `breakableSlug` adds a hair space after hyphens so long slugs can wrap.
- Animations check `Settings().ShowAnimations()` first.
- Fonts (`fonts/`, OFL-licensed) and `arena-logo-white.png` are compiled in with `//go:embed`.

## Conventions

- User-facing strings use plain, non-technical language with typographic punctuation (`’`, `…`). Error messages say what to do next (see the status-code switch in `sendToArena`).
- Code comments are short, sentence-case, and explain *why*, in the same plain voice.
- `README.md` and `CHANGELOG.md` are written for end users, not developers.
