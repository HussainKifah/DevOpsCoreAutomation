package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func SaveJSON(folder, filename string, v any) (string, error){
	if folder == "" {
		folder = "json"
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", folder, err)
	}

	if filepath.Ext(filename) != ".json" {
		filename += ".json"
	}

	path := filepath.Join(folder,filename)

	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal json: %w", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return path, nil
}