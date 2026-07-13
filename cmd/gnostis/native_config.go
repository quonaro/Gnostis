package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/quonaro/lota/engine"
	"gopkg.in/yaml.v3"

	"github.com/quonaro/gnostis/internal/config"
	"github.com/quonaro/gnostis/internal/discover"
)

func configValidateHandler(_ context.Context, nctx engine.NativeContext) error {
	_, restore, err := loadConfigForCLI()
	if err != nil {
		return err
	}
	defer restore()

	_, _ = fmt.Fprintln(nctx.Stdout, "Configuration is valid")
	return nil
}

func configShowHandler(_ context.Context, nctx engine.NativeContext) error {
	cfg, restore, err := loadConfigForCLI()
	if err != nil {
		return err
	}
	defer restore()

	cfg.Embeddings.APIKey = maskValue(cfg.Embeddings.APIKey)

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	_, _ = nctx.Stdout.Write(data)
	return nil
}

func configDiscoverHandler(_ context.Context, nctx engine.NativeContext) error {
	cfg, restore, err := loadConfigForCLI()
	if err != nil {
		return err
	}
	defer restore()

	root := nctx.Args["path"]
	if root == "" {
		return fmt.Errorf("path argument is required")
	}

	opts := discover.Options{
		Git:  nctx.Args["git"] == "true",
		Go:   nctx.Args["go"] == "true",
		NM:   nctx.Args["nm"] == "true",
		Venv: nctx.Args["venv"] == "true",
	}

	existingPaths := make(map[string]bool)
	existingNames := make(map[string]bool)
	for _, d := range cfg.Directories {
		existingPaths[d.Path] = true
		existingNames[d.Name] = true
	}

	result, err := discover.FindProjects(root, opts, existingPaths)
	if err != nil {
		return fmt.Errorf("discover projects: %w", err)
	}

	if len(result.New) == 0 {
		_, _ = fmt.Fprintln(nctx.Stdout, "No new projects found.")
		if len(result.AlreadyAdded) > 0 {
			_, _ = fmt.Fprintf(nctx.Stdout, "Already configured: %d\n", len(result.AlreadyAdded))
		}
		return nil
	}

	newProjects := discover.UniqueNames(result.New, existingNames)
	_, _ = fmt.Fprintln(nctx.Stdout, "The following projects will be added:")
	_, _ = nctx.Stdout.Write([]byte(discover.ToYAML(newProjects)))

	if !confirm(nctx.Stdout, "Apply changes?") {
		_, _ = fmt.Fprintln(nctx.Stdout, "cancelled")
		return nil
	}

	cfgPath, err := config.ResolvePath("")
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}

	if nctx.Args["backup"] == "true" {
		backupPath, err := backupConfig(cfgPath)
		if err != nil {
			return fmt.Errorf("create backup: %w", err)
		}
		_, _ = fmt.Fprintf(nctx.Stdout, "Backup created: %s\n", backupPath)
	}

	for _, p := range newProjects {
		cfg.Directories = append(cfg.Directories, config.Directory{Path: p.Path, Name: p.Name})
	}

	if err := saveConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	_, _ = fmt.Fprintln(nctx.Stdout, "Configuration updated.")
	return nil
}

func maskValue(v string) string {
	if v == "" {
		return ""
	}
	return "***"
}

func backupConfig(path string) (string, error) {
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s.%d", path, i)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			if err := copyFile(path, candidate); err != nil {
				return "", err
			}
			return candidate, nil
		} else if err != nil {
			return "", fmt.Errorf("stat backup candidate: %w", err)
		}
	}
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read source file: %w", err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return fmt.Errorf("write backup file: %w", err)
	}
	return nil
}

func saveConfig(path string, cfg config.Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
