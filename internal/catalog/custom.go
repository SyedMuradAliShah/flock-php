package catalog

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"flock/internal/binaries"
	"flock/internal/config"
)

// customPrefix starts the line (and version) of a MySQL or MariaDB the user already had.
const customPrefix = "custom-"

func isCustom(key string) bool { return strings.HasPrefix(key, customPrefix) }

// withCustom makes a version of the form "custom-10.11.2" its own line, so it lives next to the
// downloaded 10.11 and doesn't share its data.
func withCustom(line func(version string) string) func(string) string {
	return func(version string) string {
		if isCustom(version) {
			return version
		}
		return line(version)
	}
}

// customAt finds the custom version of service id with the line (or version) key.
func customAt(root, id, key string) (config.Custom, bool) {
	if !isCustom(key) {
		return config.Custom{}, false
	}
	cfg, err := config.Load(root)
	if err != nil {
		return config.Custom{}, false
	}
	return cfg.Services.FindCustom(id, key)
}

// DisplayVersion is the version to show for an installed version: "10.11.2" for the custom
// "custom-10.11.2", and any other version as it is.
func DisplayVersion(version string) string { return strings.TrimPrefix(version, customPrefix) }

// customTools are the programs of a MySQL or MariaDB folder Flock runs: the server and the tool that
// stops it on Windows.
var customTools = map[string][2]string{
	"mysql":   {"mysqld", "mysqladmin"},
	"mariadb": {"mariadbd", "mariadb-admin"},
}

var serverVersion = regexp.MustCompile(`(?i)\bVer\s+(\d+\.\d+\.\d+)`)

// AddCustom makes a MySQL or MariaDB the user already has (installed by another tool or by hand) one of the
// versions of service id: Flock runs the server in the base directory binary (the folder that holds bin/) on the
// data directory data, in place, and never deletes either. It becomes the active version when the
// service has none yet.
func (m *Manager) AddCustom(ctx context.Context, id, binary, data string) error {
	k, err := m.idle(id)
	if err != nil {
		return err
	}
	tools, ok := customTools[id]
	if !ok {
		return fmt.Errorf("%w: only MySQL and MariaDB can use a version you already have", ErrInvalid)
	}
	binary, data = filepath.Clean(strings.TrimSpace(binary)), filepath.Clean(strings.TrimSpace(data))
	if !filepath.IsAbs(binary) || !filepath.IsAbs(data) {
		return fmt.Errorf("%w: give the full path of both folders", ErrInvalid)
	}
	// Choosing the bin folder itself is an easy slip: the folder that holds it is meant.
	if _, err := os.Stat(filepath.Join(binary, exe(runtime.GOOS, tools[0]))); err == nil && strings.EqualFold(filepath.Base(binary), "bin") {
		binary = filepath.Dir(binary)
	}
	for _, tool := range tools {
		if _, err := os.Stat(filepath.Join(binary, "bin", exe(runtime.GOOS, tool))); err != nil {
			return fmt.Errorf("%w: %s has no bin%c%s — choose the base directory, the folder that contains bin", ErrInvalid, binary, filepath.Separator, exe(runtime.GOOS, tool))
		}
	}
	version, err := detectVersion(ctx, id, filepath.Join(binary, "bin", exe(runtime.GOOS, tools[0])))
	if err != nil {
		return err
	}
	if err := m.checkCustomData(data); err != nil {
		return err
	}

	return m.mutate(func(cfg *config.Config) error {
		for _, c := range cfg.Services.Custom {
			if samePath(c.DataDir, data) {
				return fmt.Errorf("%w: %s is already used by %s %s", ErrInvalid, data, c.Service, DisplayVersion(c.Line))
			}
		}
		line := customPrefix + version
		for n := 2; ; n++ {
			if _, taken := cfg.Services.FindCustom(id, line); !taken {
				break
			}
			line = fmt.Sprintf("%s%s-%d", customPrefix, version, n)
		}
		cfg.Services.AddCustom(config.Custom{Service: id, Line: line, Version: version, BaseDir: binary, DataDir: data})
		if x, active := cfg.Services.Extra(id); !active || !exists(Dir(m.root, id, x.Version)) {
			m.use(cfg, k, line)
		}
		return nil
	})
}

// detectVersion asks the server program for its version, and checks it is the flavor id names.
func detectVersion(ctx context.Context, id, server string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, server, "--version").CombinedOutput()
	text := string(out)
	match := serverVersion.FindStringSubmatch(text)
	if err != nil || match == nil {
		return "", fmt.Errorf("%w: %s doesn't say which version it is (%s)", ErrInvalid, filepath.Base(server), strings.TrimSpace(text))
	}
	maria := strings.Contains(strings.ToLower(text), "mariadb")
	switch {
	case id == "mariadb" && !maria:
		return "", fmt.Errorf("%w: that folder is MySQL, not MariaDB: add it to MySQL instead", ErrInvalid)
	case id == "mysql" && maria:
		return "", fmt.Errorf("%w: that folder is MariaDB, not MySQL: add it to MariaDB instead", ErrInvalid)
	}
	return match[1], nil
}

// checkCustomData refuses a data directory Flock would damage or can't use: one inside Flock's own
// services folder, or a folder with other things in it that isn't a MySQL or MariaDB data directory.
func (m *Manager) checkCustomData(data string) error {
	if inside(data, filepath.Join(m.root, "services")) {
		return fmt.Errorf("%w: %s is inside Flock's own folder, which Flock manages itself", ErrInvalid, data)
	}
	entries, err := os.ReadDir(data)
	if os.IsNotExist(err) || (err == nil && len(entries) == 0) {
		return nil // the server sets up a new data directory
	}
	if err != nil {
		return fmt.Errorf("%w: can't read %s: %v", ErrInvalid, data, err)
	}
	for _, e := range entries {
		if e.IsDir() && strings.EqualFold(e.Name(), "mysql") {
			return nil
		}
	}
	return fmt.Errorf("%w: %s has files in it but no mysql folder, so it isn't a MySQL or MariaDB data directory", ErrInvalid, data)
}

// removeCustom stops using a custom version. Its files are never touched: Flock didn't create them.
func (m *Manager) removeCustom(k *Kind, line string, deleteData bool) error {
	if deleteData {
		return fmt.Errorf("%w: Flock doesn't delete folders it didn't create; remove %s yourself if you want it gone", ErrInvalid, line)
	}
	cfg, err := config.Load(m.root)
	if err != nil {
		return err
	}
	if _, ok := cfg.Services.FindCustom(k.ID, line); !ok {
		return fmt.Errorf("%w: %s %s is not one of your own versions", ErrInvalid, k.Name, DisplayVersion(line))
	}
	versions := installedLines(m.root, k)
	return m.mutate(func(cfg *config.Config) error {
		if x, ok := cfg.Services.Extra(k.ID); ok && x.Version == line {
			next := ""
			for other, version := range versions {
				if other != line && (next == "" || binaries.CompareVersions(other, k.LineOf(next)) > 0) {
					next = version
				}
			}
			if next == "" {
				cfg.Services.RemoveExtra(k.ID)
			} else {
				m.use(cfg, k, next)
			}
		}
		cfg.Services.RemoveCustom(k.ID, line)
		return nil
	})
}

// inside reports whether path is dir or below it.
func inside(path, dir string) bool {
	rel, err := filepath.Rel(strings.ToLower(filepath.Clean(dir)), strings.ToLower(filepath.Clean(path)))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func samePath(a, b string) bool { return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) }
