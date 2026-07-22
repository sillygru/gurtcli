package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/zalando/go-keyring"
)

const keyringService = "gurtcli"

func KeychainAccount(provider, customBaseURL, savedEndpointName string) string {
	if savedEndpointName != "" {
		return "saved:" + savedEndpointName
	}
	if customBaseURL != "" {
		return "custom:" + customBaseURL
	}
	return provider
}

func GetAPIKey(provider, customBaseURL, savedEndpointName string) (string, error) {
	account := KeychainAccount(provider, customBaseURL, savedEndpointName)
	key, err := keyring.Get(keyringService, account)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, keyring.ErrNotFound) {
		logf("keychain unavailable (%v), falling back to credentials file/env/.env", err)
	}

	// Machines with no keychain (headless SSH boxes) keep the key here instead,
	// but only ever because the user opted in on the keychain-failure prompt.
	if v, ok := GetCredFileKey(account); ok {
		return v, nil
	}

	if v := os.Getenv("GURT_API_KEY"); v != "" {
		return v, nil
	}

	if err := LoadDotenv(); err == nil {
		if v := os.Getenv("GURT_API_KEY"); v != "" {
			return v, nil
		}
		if cfg, cfgErr := Load(); cfgErr == nil && cfg.DotenvKeyName != "" {
			if v := os.Getenv(cfg.DotenvKeyName); v != "" {
				return v, nil
			}
		}
	}

	return "", nil
}

func SetAPIKey(provider, customBaseURL, savedEndpointName, key string) error {
	account := KeychainAccount(provider, customBaseURL, savedEndpointName)
	if err := keyring.Set(keyringService, account, key); err != nil {
		return fmt.Errorf("setting API key in keychain: %w", err)
	}
	return nil
}

func DeleteAPIKey(provider, customBaseURL, savedEndpointName string) error {
	account := KeychainAccount(provider, customBaseURL, savedEndpointName)
	// Clear the fallback store too, or a "deleted" key keeps working.
	if err := DeleteCredFileKey(account); err != nil {
		return fmt.Errorf("deleting API key from credentials file: %w", err)
	}
	if err := keyring.Delete(keyringService, account); err != nil {
		return fmt.Errorf("deleting API key from keychain: %w", err)
	}
	return nil
}

// SetCredFileAPIKey stores a key in the fallback credential file, for when the
// OS keychain is unavailable and the user has chosen this over a project .env.
func SetCredFileAPIKey(provider, customBaseURL, savedEndpointName, key string) error {
	return SetCredFileKey(KeychainAccount(provider, customBaseURL, savedEndpointName), key)
}
