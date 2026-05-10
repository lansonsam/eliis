package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Root is the top-level configuration document.
type Root struct {
	Server Server `yaml:"server"`
	Log    Log    `yaml:"log"`
}

// Server holds HTTP listener settings.
type Server struct {
	Addr string `yaml:"addr"`
}

// Log controls the structured logger emitted by cmd/eliis.
type Log struct {
	// Level is one of: debug, info, warn, error. Defaults to "info".
	Level string `yaml:"level"`
	// Format is one of: text, json. Defaults to "text" for dev readability.
	Format string `yaml:"format"`
}

// Load reads and parses a YAML config file. Relative paths resolve from the process cwd.
func Load(path string) (*Root, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var root Root
	if err := yaml.Unmarshal(b, &root); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	root.applyDefaults()
	return &root, nil
}

func (r *Root) applyDefaults() {
	if r.Server.Addr == "" {
		r.Server.Addr = ":8080"
	}
	if r.Log.Level == "" {
		r.Log.Level = "info"
	} else {
		r.Log.Level = strings.ToLower(r.Log.Level)
	}
	if r.Log.Format == "" {
		r.Log.Format = "text"
	} else {
		r.Log.Format = strings.ToLower(r.Log.Format)
	}
}
