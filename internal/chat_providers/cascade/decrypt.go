package cascade

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// key is the hardcoded AES-256-GCM key used by Windsurf Cascade clients.
	key       = "safeCodeiumworldKeYsecretBalloon"
	nonceSize = 12
	tagSize   = 16
)

// DecryptFile reads a Windsurf Cascade .pb file and returns the decrypted plaintext.
// The file layout is nonce (12 bytes) || ciphertext || tag (16 bytes), matching
// AES-256-GCM as implemented by the cryptography Python package.
func DecryptFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	if len(data) < nonceSize+tagSize {
		return nil, fmt.Errorf("file too short: %d bytes", len(data))
	}
	return Decrypt(data)
}

// Decrypt decrypts a Windsurf Cascade payload.
func Decrypt(data []byte) ([]byte, error) {
	if len(data) < nonceSize+tagSize {
		return nil, fmt.Errorf("data too short: %d bytes", len(data))
	}
	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

// DiscoverDir returns the first existing Cascade trajectory directory among the
// known Windsurf/Next/Devin variants. If none exists, it returns the default
// legacy path so callers can produce a predictable error message.
func DiscoverDir() string {
	home := os.Getenv("HOME")
	if home == "" {
		return defaultCascadeDir()
	}

	candidates := []string{
		filepath.Join(home, ".codeium", "windsurf", "cascade"),
		filepath.Join(home, ".codeium", "windsurf-next", "cascade"),
		filepath.Join(home, ".codeium", "devin", "cascade"),
		filepath.Join(home, ".codeium", "devin-desktop", "cascade"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return candidates[0]
}

func defaultCascadeDir() string {
	return filepath.Join("~", ".codeium", "windsurf", "cascade")
}

// SourceDirs returns the list of known Cascade source directories. Only
// directories that currently exist are returned.
func SourceDirs() []string {
	home := os.Getenv("HOME")
	if home == "" {
		return nil
	}
	base := filepath.Join(home, ".codeium")
	all := []string{
		filepath.Join(base, "windsurf", "cascade"),
		filepath.Join(base, "windsurf-next", "cascade"),
		filepath.Join(base, "devin", "cascade"),
		filepath.Join(base, "devin-desktop", "cascade"),
	}
	var existing []string
	for _, d := range all {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			existing = append(existing, d)
		}
	}
	return existing
}
