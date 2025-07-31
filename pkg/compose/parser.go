package compose

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ComposeFile represents a docker-compose.yml file
type ComposeFile struct {
	Services map[string]Service `yaml:"services"`
}

// Service represents a service in a docker-compose file
type Service struct {
	Image string `yaml:"image"`
}

// ParseComposeFile parses a docker-compose file
func ParseComposeFile(filename string) (*ComposeFile, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var compose ComposeFile
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &compose, nil
}

// GetImages returns all images from a compose file
func (c *ComposeFile) GetImages() map[string]string {
	images := make(map[string]string)
	for serviceName, service := range c.Services {
		if service.Image != "" {
			images[serviceName] = service.Image
		}
	}
	return images
}
