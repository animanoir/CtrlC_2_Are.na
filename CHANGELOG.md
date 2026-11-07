# Changelog

Each section's heading is a git tag. When that tag is pushed, the section becomes the notes of the GitHub release.

## v2.1.0

Two safeguards against sending a secret by accident, listening that stops by itself, and an About dialog.

- Copies made by password managers are ignored on macOS and Windows. They mark what they copy as concealed, and the app now leaves those copies alone: nothing is sent or shown. Linux has no such mark, so nothing changes there.
- Text that looks like a key, token or password is held back: keys and tokens from common services (OpenAI, Stripe, GitHub, Slack, AWS, Google, GitLab, npm, PyPI), login tokens, private keys, passwords inside web addresses, and lone random-looking strings. The preview says "Looks like a secret, so it wasn’t sent." instead of showing the text. Quotes and sentences are never held back, and the safeguards can't be turned off.
- Listening stops by itself after 30 minutes without a new copy, and the window says so. The row under "Remember token and channel" turns this off or changes the minutes (1 to 720); the choice is remembered along with the channel.
- An **About** button in the bottom-right corner shows the version and links to the source code and to the page for reporting a problem or suggesting something.
- A small "?" at the end of the "Stop listening after" and "Hold before sending" rows explains the number when you hover or tap it (Esc closes it); it dims along with its row when the row is off.
- Those two rows line up: their number fields start at the same place and are the same width.
- An optional hold before sending, off unless you turn it on: each copy waits a few seconds (1 to 30, 5 to begin with) with a countdown and an Undo button under the preview, and Undo or Esc drops it before anything reaches Are.na. Stopping listening drops whatever is still waiting.
- The window has a fixed size that fits its content, the About button sits closer to it, and the reminder to stop listening before copying anything private stands on its own line, in bold.
- The README has new sections, "What gets sent" and "Stopping automatically".

## v2.0.1

- Words that were hyphenated at the end of a line, as happens when copying from a PDF, are joined again: "he-" at the end of one line and "llo" at the start of the next now arrive as "hello" instead of "he- llo".
- Hyphens that belong to the word are kept: names ("Jean-Paul"), numbers ("COVID-19"), single letters ("e-mail"), compounds with more than one hyphen ("mother-in-law") and hanging hyphens ("pre- and post-war"). A compound broken at its only hyphen ("well-known") can't be told apart from a syllable break and is joined too.
- Invisible soft hyphens are removed from copied text.

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
