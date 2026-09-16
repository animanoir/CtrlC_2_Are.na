package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

// The token goes in the system's password manager (Keychain on macOS, Credential Manager on Windows,
// the Secret Service keyring on Linux). Only the channel slug is written to a file.
const (
	keyringService   = "Ctrl+C to Are.na"
	keyringUser      = "personal-access-token"
	settingsDirName  = "ctrlc2arena"
	settingsFileName = "settings.json"

	// Older versions saved the token in plain text next to the app
	legacySettingsFileName = "arena_settings.json"
)

type settings struct {
	Slug string `json:"slug"`
}

// Per-user settings folder: Application Support on macOS, AppData on Windows, ~/.config on Linux
func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, settingsDirName, settingsFileName), nil
}

// Returns whatever was remembered. Either value can be empty if nothing was saved for it.
func loadSettings() (token string, slug string) {
	migrateLegacySettings()

	if path, err := settingsPath(); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			var saved settings
			if json.Unmarshal(data, &saved) == nil {
				slug = saved.Slug
			}
		}
	}

	token, err := keyring.Get(keyringService, keyringUser)
	if err != nil {
		token = "" // Not saved, or the password manager isn't available
	}
	return token, slug
}

// Remembers the token and slug. The returned error explains what couldn't be saved.
func saveSettings(token string, slug string) error {
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
		data, err = json.Marshal(settings{Slug: slug})
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
	if err := saveSettings(legacy.Token, legacy.Slug); err != nil {
		fmt.Println("Old settings kept, they couldn't be moved: ", err)
		return // Leave the old file so the token isn't lost
	}
	if err := os.Remove(legacySettingsFileName); err != nil {
		fmt.Println("Error removing old settings file: ", err)
	}
}
