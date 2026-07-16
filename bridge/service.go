// Primary author: Navjyot Nishant
// Created on: 2026-07-16
// Last updated: 2026-07-16
// Description: `relayent-bridge install|uninstall` — registers the bridge as a
//
//	per-user background service (launchd on macOS, systemd --user on Linux) so
//	it starts at login and restarts on failure. Deliberately per-user, never
//	root: the bridge must run as the user whose CLI sessions it uses, and it
//	needs no privileges beyond that.
//
// AI usage: Built with assistance from AI tools for implementation acceleration,
//
//	review, and refactoring.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const launchdLabel = "com.relayent.bridge"

// servicePaths returns the unit/plist path for the current platform.
func servicePaths() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist"), nil
	case "linux":
		return filepath.Join(home, ".config", "systemd", "user", "relayent-bridge.service"), nil
	default:
		return "", fmt.Errorf("service install is not supported on %s — run the bridge manually", runtime.GOOS)
	}
}

// logPaths returns stdout/stderr log destinations for the service.
func logPaths() (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(home, configDirName, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	return filepath.Join(dir, "bridge.out.log"), filepath.Join(dir, "bridge.err.log"), nil
}

// InstallService registers and starts the bridge as a login service.
func InstallService() error {
	// Require a working config first: a service that boots into a crash loop
	// every login is worse than no service at all.
	if _, err := LoadConfig(); err != nil {
		return fmt.Errorf("%w\n\nRun 'relayent-bridge setup' first", err)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate this binary: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("resolve binary path: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		return installLaunchd(exe)
	case "linux":
		return installSystemd(exe)
	default:
		return fmt.Errorf("service install is not supported on %s", runtime.GOOS)
	}
}

func installLaunchd(exe string) error {
	plistPath, err := servicePaths()
	if err != nil {
		return err
	}
	outLog, errLog, err := logPaths()
	if err != nil {
		return err
	}
	cfgPath, err := ConfigPath()
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()

	// PATH matters: launchd gives an agent a minimal PATH, so the CLIs the bridge
	// shells out to (installed via brew or ~/.local/bin) would not be found.
	svcPath := strings.Join([]string{
		filepath.Join(home, ".local", "bin"),
		"/opt/homebrew/bin",
		"/usr/local/bin",
		"/usr/bin",
		"/bin",
	}, ":")

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key><string>%s</string>
    <key>RELAYENT_CONFIG</key><string>%s</string>
  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key>
  <dict>
    <key>SuccessfulExit</key><false/>
  </dict>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
  <key>WorkingDirectory</key><string>%s</string>
  <key>ProcessType</key><string>Background</string>
</dict>
</plist>
`, launchdLabel, exe, svcPath, cfgPath, outLog, errLog, home)

	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}
	if err := os.WriteFile(plistPath, []byte(plist), 0o644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}

	// Unload any previous copy so install is repeatable (bootout fails harmlessly
	// when nothing is loaded, hence the ignored error).
	uid := fmt.Sprint(os.Getuid())
	_ = exec.Command("launchctl", "bootout", "gui/"+uid+"/"+launchdLabel).Run()

	if out, err := exec.Command("launchctl", "bootstrap", "gui/"+uid, plistPath).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("launchctl", "kickstart", "-k", "gui/"+uid+"/"+launchdLabel).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl kickstart: %w: %s", err, strings.TrimSpace(string(out)))
	}

	fmt.Println()
	fmt.Println("  ✓ Installed as a login service (launchd)")
	fmt.Printf("    plist: %s\n", plistPath)
	fmt.Printf("    logs:  %s\n", outLog)
	fmt.Println()
	fmt.Println("  It is running now and will start automatically at login.")
	fmt.Println()
	fmt.Println("    relayent-bridge status      check it")
	fmt.Println("    relayent-bridge uninstall   remove it")
	fmt.Println()
	return nil
}

func installSystemd(exe string) error {
	unitPath, err := servicePaths()
	if err != nil {
		return err
	}
	cfgPath, err := ConfigPath()
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	svcPath := strings.Join([]string{
		filepath.Join(home, ".local", "bin"),
		"/usr/local/bin", "/usr/bin", "/bin",
	}, ":")

	unit := fmt.Sprintf(`[Unit]
Description=Relayent bridge — runs AI jobs on this machine's CLI subscription
After=network-online.target

[Service]
Type=simple
ExecStart=%s
Environment=PATH=%s
Environment=RELAYENT_CONFIG=%s
Restart=always
RestartSec=10
# The bridge needs no privileges beyond the user's own CLI sessions.
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=default.target
`, exe, svcPath, cfgPath)

	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		return fmt.Errorf("create systemd user dir: %w", err)
	}
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return fmt.Errorf("write unit: %w", err)
	}
	if out, err := exec.Command("systemctl", "--user", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("systemctl", "--user", "enable", "--now", "relayent-bridge.service").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable: %w: %s", err, strings.TrimSpace(string(out)))
	}

	fmt.Println()
	fmt.Println("  ✓ Installed as a user service (systemd)")
	fmt.Printf("    unit: %s\n", unitPath)
	fmt.Println()
	fmt.Println("  Logs:  journalctl --user -u relayent-bridge -f")
	fmt.Println("  Tip:   'loginctl enable-linger' keeps it running when logged out.")
	fmt.Println()
	return nil
}

// UninstallService stops and removes the login service. Config is left in place:
// removing a service should not silently destroy the user's pairing.
func UninstallService() error {
	path, err := servicePaths()
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		uid := fmt.Sprint(os.Getuid())
		_ = exec.Command("launchctl", "bootout", "gui/"+uid+"/"+launchdLabel).Run()
	case "linux":
		_ = exec.Command("systemctl", "--user", "disable", "--now", "relayent-bridge.service").Run()
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove service file: %w", err)
	}
	if runtime.GOOS == "linux" {
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	}
	fmt.Println()
	fmt.Println("  ✓ Service removed. The bridge is no longer running.")
	cfg, _ := ConfigPath()
	fmt.Printf("    Your pairing is still saved at %s\n", cfg)
	fmt.Println("    Delete that file to remove the pairing key from this machine.")
	fmt.Println()
	return nil
}

// ServiceStatus reports whether the login service is registered and running.
func ServiceStatus() error {
	path, err := servicePaths()
	if err != nil {
		return err
	}
	installed := true
	if _, err := os.Stat(path); os.IsNotExist(err) {
		installed = false
	}

	fmt.Println()
	if !installed {
		fmt.Println("  · Not installed as a login service.")
		fmt.Println("    Install with: relayent-bridge install")
		fmt.Println()
		return nil
	}
	fmt.Printf("  ✓ Service installed: %s\n", path)

	switch runtime.GOOS {
	case "darwin":
		uid := fmt.Sprint(os.Getuid())
		out, err := exec.Command("launchctl", "print", "gui/"+uid+"/"+launchdLabel).CombinedOutput()
		if err != nil {
			fmt.Println("  · Registered but not currently loaded.")
		} else {
			state := "unknown"
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "state = ") {
					state = strings.TrimSpace(strings.SplitN(line, "=", 2)[1])
					break
				}
			}
			fmt.Printf("  ✓ launchd state: %s\n", state)
		}
		o, e, _ := logPaths()
		fmt.Printf("    logs: %s\n          %s\n", o, e)
	case "linux":
		out, _ := exec.Command("systemctl", "--user", "is-active", "relayent-bridge.service").CombinedOutput()
		fmt.Printf("  ✓ systemd state: %s\n", strings.TrimSpace(string(out)))
		fmt.Println("    logs: journalctl --user -u relayent-bridge -f")
	}
	fmt.Println()
	return nil
}
