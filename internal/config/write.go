package config

import (
	"bytes"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func GlobalConfigExists() (bool, error) {
	path, err := GlobalConfigPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func WriteGlobal(cfg GlobalConfig) error {
	path, err := GlobalConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := marshalYAML(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

func WriteRepoConfig(dir string, filename string, rc RepoConfig) error {
	data, err := marshalYAML(rc)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, filename), data, 0o644)
}

func marshalYAML(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func AddGlobalRepoEntry(orgRepo string, rc RepoConfig) error {
	cfg, err := LoadGlobal()
	if err != nil {
		return err
	}
	if cfg.Repos == nil {
		cfg.Repos = make(map[string]RepoConfig)
	}
	cfg.Repos[orgRepo] = rc
	return WriteGlobal(cfg)
}

func GlobalRepoEntryExists(orgRepo string) (bool, error) {
	cfg, err := LoadGlobal()
	if err != nil {
		return false, err
	}
	_, ok := cfg.Repos[orgRepo]
	return ok, nil
}
