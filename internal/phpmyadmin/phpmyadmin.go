// Package phpmyadmin installs phpMyAdmin from phpmyadmin.net into <folder>/<version>/ (the folder
// defaults to <root>/phpmyadmin) and writes the configuration that connects it to Flock's MySQL and
// MariaDB. Flock serves it as the site config.PhpMyAdminName.
package phpmyadmin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"flock/internal/archive"
	"flock/internal/binaries"
	"flock/internal/download"
	"flock/internal/fsutil"
)

// Where phpMyAdmin publishes its releases (tests replace them).
var (
	VersionURL = "https://www.phpmyadmin.net/home_page/version.json"
	FilesURL   = "https://files.phpmyadmin.net/phpMyAdmin"
)

const (
	secretFile = "blowfish_secret"
	tempDir    = "tmp"
	stagingPre = ".install-"
)

// DefaultDir is the folder phpMyAdmin is installed into unless the user picks another.
func DefaultDir(root string) string { return filepath.Join(root, "phpmyadmin") }

// Folder is custom, or DefaultDir when custom is empty.
func Folder(root, custom string) string {
	if custom != "" {
		return filepath.Clean(custom)
	}
	return DefaultDir(root)
}

// VersionDir is where version is installed in folder.
func VersionDir(folder, version string) string { return filepath.Join(folder, version) }

// ours reports whether an entry of the install folder is something Flock made there.
func ours(name string, isDir bool) bool {
	switch {
	case name == secretFile:
		return !isDir
	case name == tempDir:
		return isDir
	case strings.HasPrefix(name, stagingPre):
		return isDir
	}
	return isDir && validVersion(name)
}

// CheckFolder refuses a folder that can't hold phpMyAdmin: a relative path, a file, or a folder with
// other things in it (Flock deletes what it installed there, and must never touch anything else).
func CheckFolder(folder string) error {
	if !filepath.IsAbs(folder) {
		return fmt.Errorf("%s is not an absolute path", folder)
	}
	entries, err := os.ReadDir(folder)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s is not a folder Flock can use: %w", folder, err)
	}
	for _, e := range entries {
		if !ours(e.Name(), e.IsDir()) {
			return fmt.Errorf("%s has other files in it (%s): choose an empty folder", folder, e.Name())
		}
	}
	return nil
}

// Release is a phpMyAdmin release.
type Release struct {
	Version string `json:"version"`
	// PHP is the PHP versions it supports, e.g. ">=7.2,<8.4".
	PHP string `json:"php_versions"`
}

// Latest asks phpmyadmin.net for the newest release.
func Latest(ctx context.Context, client *http.Client) (Release, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, VersionURL, nil)
	if err != nil {
		return Release{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GET %s: %s", VersionURL, resp.Status)
	}
	var feed struct {
		Version  string    `json:"version"`
		Releases []Release `json:"releases"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&feed); err != nil {
		return Release{}, fmt.Errorf("read %s: %w", VersionURL, err)
	}
	if !validVersion(feed.Version) {
		return Release{}, fmt.Errorf("%s names no phpMyAdmin version", VersionURL)
	}
	for _, r := range feed.Releases {
		if r.Version == feed.Version {
			return r, nil
		}
	}
	return Release{Version: feed.Version}, nil
}

// validVersion accepts 5.2.3 and nothing that could leave the folder or the download URL.
func validVersion(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) < 2 || len(parts) > 4 {
		return false
	}
	for _, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return false
		}
	}
	return true
}

// Installed returns the newest version installed in folder, "" when there is none.
func Installed(folder string) string {
	entries, _ := os.ReadDir(folder)
	best := ""
	for _, e := range entries {
		if e.IsDir() && validVersion(e.Name()) && (best == "" || binaries.CompareVersions(e.Name(), best) > 0) {
			best = e.Name()
		}
	}
	return best
}

// Install downloads release into folder, checking its SHA-256 (published next to it). It leaves the
// other installed versions for RemoveOlder. progress gets the bytes so far and the total (-1 when unknown).
func Install(ctx context.Context, client *http.Client, folder string, release Release, progress func(done, total int64)) (string, error) {
	if !validVersion(release.Version) {
		return "", fmt.Errorf("%q is not a phpMyAdmin version", release.Version)
	}
	if err := CheckFolder(folder); err != nil {
		return "", err
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(folder, stagingPre)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	name := fmt.Sprintf("phpMyAdmin-%s-all-languages.zip", release.Version)
	url := fmt.Sprintf("%s/%s/%s", FilesURL, release.Version, name)
	sha, err := checksum(ctx, client, url+".sha256")
	if err != nil {
		return "", err
	}
	zip := filepath.Join(tmp, name)
	if err := download.File(ctx, client, url, zip, sha, progress); err != nil {
		return "", err
	}
	unpacked := filepath.Join(tmp, "out")
	if err := os.Mkdir(unpacked, 0o755); err != nil {
		return "", err
	}
	if err := archive.Extract(zip, unpacked); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(unpacked, "index.php")); err != nil {
		return "", errors.New("the download has no index.php")
	}
	for _, d := range []string{"setup", "doc", "examples"} {
		_ = os.RemoveAll(filepath.Join(unpacked, d))
	}

	dest := VersionDir(folder, release.Version)
	if _, err := os.Stat(dest); err == nil {
		// The site is being served from it, and Windows won't delete a folder that is in use.
		if err := overwriteTree(unpacked, dest); err != nil {
			return "", err
		}
		return dest, nil
	}
	if err := os.Rename(unpacked, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// overwriteTree copies src over dst file by file, leaving files dst has and src doesn't (config.inc.php,
// config.user.inc.php).
func overwriteTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return fmt.Errorf("replace %s: %w (is something using it?)", rel, err)
		}
		return nil
	})
}

// checksum reads the hex SHA-256 from a "<sha>  <file>" document.
func checksum(ctx context.Context, client *http.Client, url string) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(body))
	if len(fields) == 0 || len(fields[0]) != 64 {
		return "", fmt.Errorf("%s has no SHA-256", url)
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return "", fmt.Errorf("%s has no SHA-256", url)
	}
	return fields[0], nil
}

// RemoveOlder deletes the versions in folder other than keep.
func RemoveOlder(folder, keep string) {
	entries, _ := os.ReadDir(folder)
	for _, e := range entries {
		if e.IsDir() && validVersion(e.Name()) && e.Name() != keep {
			_ = os.RemoveAll(filepath.Join(folder, e.Name()))
		}
	}
}

// Uninstall removes what Flock installed into folder, and the folder itself once it is empty.
func Uninstall(folder string) error {
	entries, err := os.ReadDir(folder)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		if ours(e.Name(), e.IsDir()) {
			errs = append(errs, os.RemoveAll(filepath.Join(folder, e.Name())))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	if rest, err := os.ReadDir(folder); err == nil && len(rest) == 0 {
		return os.Remove(folder)
	}
	return nil
}

// Move moves the installed version (and the login secret) from folder to another, and returns the
// version's new directory. The old folder is cleaned up like Uninstall does.
func Move(from, to, version string) (string, error) {
	if !validVersion(version) {
		return "", fmt.Errorf("%q is not a phpMyAdmin version", version)
	}
	if filepath.Clean(from) == filepath.Clean(to) {
		return VersionDir(to, version), nil
	}
	if err := CheckFolder(to); err != nil {
		return "", err
	}
	if err := os.MkdirAll(to, 0o755); err != nil {
		return "", err
	}
	dest := VersionDir(to, version)
	if err := os.RemoveAll(dest); err != nil {
		return "", err
	}
	if err := os.Rename(VersionDir(from, version), dest); err != nil {
		if err := fsutil.InstallTree(VersionDir(from, version), dest); err != nil {
			return "", err
		}
	}
	if data, err := os.ReadFile(filepath.Join(from, secretFile)); err == nil {
		_ = os.WriteFile(filepath.Join(to, secretFile), data, 0o600)
	}
	return dest, Uninstall(from)
}

// Server is a MySQL or MariaDB server phpMyAdmin offers.
type Server struct {
	Name string // shown in phpMyAdmin's server list
	Port int
}

// WriteConfig writes the config.inc.php of version in folder. phpMyAdmin asks for the login: root
// without a password, as Flock sets the databases up.
func WriteConfig(folder, version string, servers []Server) error {
	secret, err := blowfishSecret(folder)
	if err != nil {
		return err
	}
	temp := filepath.Join(folder, tempDir)
	if err := os.MkdirAll(temp, 0o700); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("<?php\n// Rewritten by Flock whenever its configuration changes: put your own settings in config.user.inc.php.\n")
	fmt.Fprintf(&b, "$cfg['blowfish_secret'] = '%s';\n", secret)
	fmt.Fprintf(&b, "$cfg['TempDir'] = %s;\n", phpString(filepath.ToSlash(temp)))
	b.WriteString("$cfg['DefaultLang'] = 'en';\n$cfg['VersionCheck'] = false;\n")
	b.WriteString(rootProbe)
	for i, s := range servers {
		n := i + 1
		fmt.Fprintf(&b, "$cfg['Servers'][%d]['verbose'] = %s;\n", n, phpString(s.Name))
		fmt.Fprintf(&b, "$cfg['Servers'][%d]['host'] = '127.0.0.1';\n", n)
		fmt.Fprintf(&b, "$cfg['Servers'][%d]['port'] = %d;\n", n, s.Port)
		fmt.Fprintf(&b, "$cfg['Servers'][%d]['user'] = 'root';\n", n)
		fmt.Fprintf(&b, "$cfg['Servers'][%d]['password'] = '';\n", n)
		fmt.Fprintf(&b, "$cfg['Servers'][%d]['auth_type'] = flock_root_is_open(%d) ? 'config' : 'cookie';\n", n, s.Port)
		fmt.Fprintf(&b, "$cfg['Servers'][%d]['AllowNoPassword'] = true;\n", n)
	}
	b.WriteString("if (is_file(__DIR__ . '/config.user.inc.php')) {\n    require __DIR__ . '/config.user.inc.php';\n}\n")
	return os.WriteFile(filepath.Join(VersionDir(folder, version), "config.inc.php"), []byte(b.String()), 0o644)
}

// rootProbe signs in by itself while root has no password, and shows the login form once it has one.
const rootProbe = `if (!function_exists('flock_root_is_open')) {
    function flock_root_is_open($port) {
        mysqli_report(MYSQLI_REPORT_OFF);
        $link = @mysqli_connect('127.0.0.1', 'root', '', '', $port);
        if ($link) {
            mysqli_close($link);
            return true;
        }
        return false;
    }
}
`

// phpString quotes s as a PHP single-quoted string.
func phpString(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

// blowfishSecret is the 32-character key of phpMyAdmin's login cookie, kept so an update doesn't log everyone out.
func blowfishSecret(folder string) (string, error) {
	path := filepath.Join(folder, secretFile)
	if data, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(data))) == 32 {
		return strings.TrimSpace(string(data)), nil
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(raw)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return "", err
	}
	return secret, os.WriteFile(path, []byte(secret), 0o600)
}

// PHPFor picks the PHP version phpMyAdmin runs on: def when supported, else the newest installed
// supported one, "" when none is.
func PHPFor(supported string, installed []string, def string) string {
	if def != "" && Supports(supported, def) {
		return def
	}
	best := ""
	for _, v := range installed {
		if Supports(supported, v) && (best == "" || binaries.CompareVersions(v, best) > 0) {
			best = v
		}
	}
	return best
}

// Supports reports whether PHP version satisfies constraint (">=7.2,<8.4"); an empty constraint accepts all.
func Supports(constraint, version string) bool {
	for _, part := range strings.Split(constraint, ",") {
		part = strings.TrimSpace(part)
		var op string
		for _, o := range []string{">=", "<=", ">", "<", "="} {
			if strings.HasPrefix(part, o) {
				op = o
				break
			}
		}
		if op == "" {
			continue
		}
		c := binaries.CompareVersions(version, strings.TrimSpace(part[len(op):]))
		if !map[string]bool{">=": c >= 0, "<=": c <= 0, ">": c > 0, "<": c < 0, "=": c == 0}[op] {
			return false
		}
	}
	return true
}
