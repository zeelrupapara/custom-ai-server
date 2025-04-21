package gpt

import (
	"io/ioutil"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// GPTConfig represents one agent
type GPTConfig struct {
	Slug         string   `yaml:"slug"`
	Name         string   `yaml:"name"`
	Model        string   `yaml:"model"`
	SystemPrompt string   `yaml:"system_prompt"`
	Files        []string `yaml:"files"`
	RateLimit    string   `yaml:"rate_limit"`
	Temperature  float32  `yaml:"temperature"`
}

// Store loaded configs
var Configs = map[string]*GPTConfig{}

// LoadConfigs reads all YAML files in dir
func LoadConfigs(dir string) error {
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, f := range entries {
		if filepath.Ext(f.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, f.Name())
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		var cfg GPTConfig
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return err
		}
		Configs[cfg.Slug] = &cfg
	}
	return nil
}
