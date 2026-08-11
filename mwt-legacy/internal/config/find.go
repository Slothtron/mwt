package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ConfigFileName is the meta-root config basename.
const ConfigFileName = ".mwt.yaml"

// FindConfigFile walks up from startDir looking for .mwt.yaml.
// Returns the absolute path to the config file.
func FindConfigFile(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolve start dir: %w", err)
	}

	for {
		candidate := filepath.Join(dir, ConfigFileName)
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("stat %s: %w", candidate, err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s found from %s upward", ConfigFileName, startDir)
		}
		dir = parent
	}
}
