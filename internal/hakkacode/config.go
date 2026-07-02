package hakkacode

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// FileConfig is the on-disk shape of ~/.hakka-code.json. All fields are
// optional; CLI flags always take precedence over file values.
type FileConfig struct {
	Addr       string `json:"addr,omitempty"`
	EnableTags string `json:"enable_tags,omitempty"`
}

// DefaultConfigPath returns ~/.hakka-code.json, or "" if $HOME can't be
// resolved.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".hakka-code.json")
}

// LoadFileConfig reads and parses the config file at path. A missing file
// is not an error — it returns a zero-value FileConfig.
func LoadFileConfig(path string) (FileConfig, error) {
	var fc FileConfig
	if path == "" {
		return fc, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fc, nil
		}
		return fc, err
	}
	if err := json.Unmarshal(data, &fc); err != nil {
		return fc, err
	}
	return fc, nil
}
