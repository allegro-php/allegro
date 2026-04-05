package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

// ComputeLockHash returns "sha256:{hex}" of the raw file bytes.
func ComputeLockHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("compute lock hash: %w", err)
	}
	h := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(h[:]), nil
}
