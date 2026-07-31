package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var environmentPlaceholder = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)

// Load parses, substitutes, validates, and compiles a configuration file.
func Load(path string) (*Config, error) {
	return load(path, os.LookupEnv)
}

func load(path string, lookup func(string) (string, bool)) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := substituteEnvironment(&document, lookup); err != nil {
		return nil, err
	}

	var substituted bytes.Buffer
	encoder := yaml.NewEncoder(&substituted)
	if err := encoder.Encode(&document); err != nil {
		return nil, fmt.Errorf("prepare config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("prepare config: %w", err)
	}

	decoder := yaml.NewDecoder(&substituted)
	decoder.KnownFields(true)
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, fmt.Errorf("decode config: %w", err)
		}
		return nil, fmt.Errorf("decode config: multiple YAML documents are not supported")
	}

	config.applyDefaults()
	if err := config.validateAndCompile(); err != nil {
		return nil, err
	}
	return &config, nil
}

func substituteEnvironment(node *yaml.Node, lookup func(string) (string, bool)) error {
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		match := environmentPlaceholder.FindStringSubmatch(node.Value)
		if len(match) == 2 {
			value, ok := lookup(match[1])
			if !ok {
				return fmt.Errorf("environment variable %s is not set", match[1])
			}
			node.Value = value
			return nil
		}
		if strings.Contains(node.Value, "${") {
			return fmt.Errorf("environment placeholders must occupy an entire YAML scalar")
		}
	}
	for _, child := range node.Content {
		if err := substituteEnvironment(child, lookup); err != nil {
			return err
		}
	}
	return nil
}
