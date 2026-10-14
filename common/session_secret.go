package common

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// loadSessionSecret preserves cookie signatures across restarts. Publish a
// complete file atomically without replacing another process's secret.
func loadSessionSecret(path string) (string, error) {
	readSecret := func() (string, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		secret := strings.TrimSpace(string(data))
		if len(secret) < 32 || secret == "random_string" {
			return "", fmt.Errorf("session secret file %q must contain at least 32 characters", path)
		}
		return secret, nil
	}
	secret, err := readSecret()
	if !os.IsNotExist(err) {
		return secret, err
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".session-secret-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	secret = hex.EncodeToString(key)
	_, writeErr := file.WriteString(secret + "\n")
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := os.Link(file.Name(), path); err != nil && !os.IsExist(err) {
		return "", err
	}
	return readSecret()
}
