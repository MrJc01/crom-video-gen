package types

import (
	"encoding/json"
	"os"
)

// ParseConfig lê um arquivo JSON e retorna a struct ConfigInput já validada
func ParseConfig(filePath string) (*ConfigInput, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var config ConfigInput
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &config, nil
}
