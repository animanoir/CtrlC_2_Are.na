package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zalando/go-keyring"
)

// The token goes in the system's password manager (Keychain on macOS, Credential Manager on Windows,
// the Secret Service keyring on Linux). Only the channel slug and the timer settings are written to a file.
const (
	keyringService   = "Ctrl+C to Are.na"
	keyringUser      = "personal-access-token"
	settingsDirName  = "ctrlc2arena"
	settingsFileName = "settings.json"

	// Older versions saved the token in plain text next to the app
	legacySettingsFileName = "arena_settings.json"
)

// A whole-number setting, with what it is until changed and the range it's kept within
type numberSetting struct {
	fallback, lo, hi int
}

var (
	autoStopMinutes = numberSetting{fallback: 30, lo: 1, hi: 720} // Listening stops by itself after this long without a copy; twelve hours at most
	holdSeconds     = numberSetting{fallback: 5, lo: 1, hi: 30}   // How long a copy waits before it's sent, when the hold is on
)

func (n numberSetting) clamp(value int) int {
	return min(max(value, n.lo), n.hi)
}

// Reads a value typed into an entry. Nothing, or something that isn't a whole number, means the default;
// a number out of range is clamped.
func (n numberSetting) parse(text string) int {
	text = strings.TrimSpace(text)
	if text == "" || strings.ContainsFunc(text, func(r rune) bool { return r < '0' || r > '9' }) {
		return n.fallback
	}
	if len(strings.TrimLeft(text, "0")) > len(strconv.Itoa(n.hi)) {
		return n.hi // More digits than the maximum has is past it, however long the number
	}
	value, _ := strconv.Atoi(text)
	return n.clamp(value)
}

type settings struct {
	Slug            string `json:"slug"`
	AutoStop        bool   `json:"autoStop"`
	AutoStopMinutes int    `json:"autoStopMinutes"`
	Hold            bool   `json:"hold"`
	HoldSeconds     int    `json:"holdSeconds"`
}

// What a fresh install uses, and what fills in for keys an older settings file doesn't have
func defaultSettings() settings {
	return settings{AutoStop: true, AutoStopMinutes: autoStopMinutes.fallback, HoldSeconds: holdSeconds.fallback}
}

// Per-user settings folder: Application Support on macOS, AppData on Windows, ~/.config on Linux
func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, settingsDirName, settingsFileName), nil
}

// Returns whatever was remembered. The token and slug are empty if nothing was saved for them;
// the rest falls back to the defaults.
func loadSettings() (token string, saved settings) {
	migrateLegacySettings()

	saved = defaultSettings()
	if path, err := settingsPath(); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			// Unmarshal only sets the keys that are present, so a file from an older version keeps the defaults for the rest
			if json.Unmarshal(data, &saved) != nil {
				saved = defaultSettings()
			}
		}
	}
	saved.AutoStopMinutes = autoStopMinutes.clamp(saved.AutoStopMinutes)
	saved.HoldSeconds = holdSeconds.clamp(saved.HoldSeconds)

	token, err := keyring.Get(keyringService, keyringUser)
	if err != nil {
		token = "" // Not saved, or the password manager isn't available
	}
	return token, saved
}

// Remembers the token and the other settings. The returned error explains what couldn't be saved.
func saveSettings(token string, saved settings) error {
	var problems []error

	if err := keyring.Set(keyringService, keyringUser, token); err != nil {
		problems = append(problems, fmt.Errorf("token not saved to the system’s password manager: %w", err))
	}

	path, err := settingsPath()
	if err == nil {
		err = os.MkdirAll(filepath.Dir(path), 0o700)
	}
	if err == nil {
		var data []byte
		data, err = json.Marshal(saved)
		if err == nil {
			err = os.WriteFile(path, data, 0o600) // Readable only by this user
		}
	}
	if err != nil {
		problems = append(problems, fmt.Errorf("channel not saved: %w", err))
	}

	return errors.Join(problems...)
}

func forgetSettings() {
	if err := keyring.Delete(keyringService, keyringUser); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		fmt.Println("Error removing saved token: ", err)
	}
	if path, err := settingsPath(); err == nil {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Println("Error removing saved settings: ", err)
		}
	}
}

// Moves a plain-text arena_settings.json from an older version into the new storage, then deletes it
func migrateLegacySettings() {
	data, err := os.ReadFile(legacySettingsFileName)
	if err != nil {
		return
	}
	var legacy struct {
		Token string `json:"token"`
		Slug  string `json:"slug"`
	}
	if json.Unmarshal(data, &legacy) != nil {
		return
	}
	saved := defaultSettings()
	saved.Slug = legacy.Slug
	if err := saveSettings(legacy.Token, saved); err != nil {
		fmt.Println("Old settings kept, they couldn't be moved: ", err)
		return // Leave the old file so the token isn't lost
	}
	if err := os.Remove(legacySettingsFileName); err != nil {
		fmt.Println("Error removing old settings file: ", err)
	}
}
