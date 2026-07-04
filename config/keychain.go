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
		return "", fmt.Errorf("keyring error: %w", err)
	}

	if v := os.Getenv("GURT_API_KEY"); v != "" {
		return v, nil
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
	if err := keyring.Delete(keyringService, account); err != nil {
		return fmt.Errorf("deleting API key from keychain: %w", err)
	}
	return nil
}
