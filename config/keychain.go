package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/zalando/go-keyring"
)

const keyringService = "gurtcli"

func KeychainAccount(provider, customBaseURL string) string {
	if customBaseURL != "" {
		return "custom:" + customBaseURL
	}
	return provider
}

func GetAPIKey(provider, customBaseURL string) (string, error) {
	account := KeychainAccount(provider, customBaseURL)
	key, err := keyring.Get(keyringService, account)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, keyring.ErrNotFound) {
		return "", fmt.Errorf("keyring error: %w", err)
	}

	if v := os.Getenv("GURT_API_KEY"); v != "" {
		return v, nil
	}

	return "", nil
}

func SetAPIKey(provider, customBaseURL, key string) error {
	account := KeychainAccount(provider, customBaseURL)
	if err := keyring.Set(keyringService, account, key); err != nil {
		return fmt.Errorf("setting API key in keychain: %w", err)
	}
	return nil
}
