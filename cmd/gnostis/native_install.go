package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/quonaro/lota/engine"
)

const systemdUnit = `[Unit]
Description=Gnostis MCP HTTP server

[Service]
Type=simple
WorkingDirectory=%h/.gnostis
ExecStart=%h/.local/bin/gnostis run
Restart=unless-stopped
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=default.target
`

func installHandler(_ context.Context, nctx engine.NativeContext) error {
	bin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("user home dir: %w", err)
	}

	localBin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(localBin, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", localBin, err)
	}

	target := filepath.Join(localBin, "gnostis")
	if _, err := os.Stat(target); err == nil && isInteractive() && !confirm(nctx.Stdout, fmt.Sprintf("%s already exists. Overwrite?", target)) {
		_, _ = fmt.Fprintln(nctx.Stdout, "cancelled")
		return nil
	}

	if err := copyFile(bin, target); err != nil {
		return fmt.Errorf("copy binary: %w", err)
	}
	_ = os.Chmod(target, 0o755)

	unitDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", unitDir, err)
	}

	unitPath := filepath.Join(unitDir, "gnostis.service")
	if err := os.WriteFile(unitPath, []byte(systemdUnit), 0o644); err != nil {
		return fmt.Errorf("write unit file: %w", err)
	}

	if _, err := exec.LookPath("systemctl"); err == nil {
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
		_ = exec.Command("systemctl", "--user", "stop", "gnostis").Run()
		_ = exec.Command("systemctl", "--user", "enable", "gnostis").Run()
		_ = exec.Command("systemctl", "--user", "start", "gnostis").Run()
	}

	_, _ = fmt.Fprintf(nctx.Stdout, "Installed %s and started gnostis user service.\n", target)
	_, _ = fmt.Fprintf(nctx.Stdout, "Status: %s\n", systemdStatus())
	return nil
}

func isInteractive() bool {
	return isatty.IsTerminal(os.Stdin.Fd())
}

func systemdStatus() string {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return "unknown"
	}
	out, err := exec.Command("systemctl", "--user", "is-active", "gnostis").Output()
	if err != nil {
		return "inactive"
	}
	return strings.TrimSpace(string(out))
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return nil
}
