# Changelog

Each section's heading is a git tag. When that tag is pushed, the section becomes the notes of the GitHub release.

## v2.0.0

A redesign, a move to the current Are.na API, and safer storage of your token. **Existing users need to check two things after updating**, see "Upgrading" below.

### Are.na API v3

- Copied text is now sent through Are.na's API v3 (`POST /v3/blocks`); v2 is no longer used.
- Tokens must have the **write** scope. Create one at https://www.are.na/settings/personal-access-tokens if yours is read-only.
- Errors from Are.na now show what to do: a bad token, a read-only token, a channel that doesn't exist, or the rate limit being reached.

### New look

- A clean black-and-white interface in the spirit of Are.na, following your system's light or dark mode.
- A square preview shows the last text you copied, set the way Are.na shows a text block, with the block title and status underneath.
- While listening, the app shows the channel name, a count of blocks sent, and a blinking dot. A border flash confirms each block.
- Copied text is sent as a single clean line: line breaks, tabs and runs of spaces are collapsed. Copies that are only whitespace are ignored.
- Pressing Enter in any field starts listening. Start stays disabled until a token and slug are filled in.

### Token storage

- The "Save data?" button is replaced by a **Remember token and channel on this computer** checkbox.
- The token is kept in the operating system's password manager (Keychain on macOS, Credential Manager on Windows, Secret Service on Linux) instead of a plain-text file. Only the channel slug is written to a file, in your user settings folder. See the README for the exact locations.

### Upgrading

1. Make sure your token has the **write** scope, or create a new one.
2. If you had an `arena_settings.json` next to the app, the first launch moves its contents into the new storage and deletes the file. If that can't be done (for example no keyring on Linux), the file is left in place.

### Downloads

- **macOS:** `ctrl2arena-macos-universal.zip` runs on both Intel and Apple Silicon. The app isn't signed, so on first launch right-click it, choose Open, and confirm.
- **Windows:** `ctrl2arena-windows-amd64.zip`
- **Linux:** `ctrl2arena-linux-amd64.zip` — needs a Secret Service keyring (GNOME Keyring, KWallet) to remember the token.
