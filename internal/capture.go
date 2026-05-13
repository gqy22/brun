package internal

import "gopkg.in/yaml.v3"

type Config struct {
	Project string `yaml:"project"`
}

func ParseConfig(data []byte) (Config, error) {
	var cfg Config
	err := yaml.Unmarshal(data, &cfg)
	return cfg, err
}
