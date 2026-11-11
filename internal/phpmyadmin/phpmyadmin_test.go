package phpmyadmin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeZip(t *testing.T, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	top := "phpMyAdmin-" + version + "-all-languages/"
	for name, body := range map[string]string{"index.php": "<?php echo 1;", "setup/index.php": "x", "libraries/a.php": "y"} {
		f, err := w.Create(top + name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(body))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func server(t *testing.T, version string, tamper bool) {
	t.Helper()
	data := fakeZip(t, version)
	sum := sha256.Sum256(data)
	if tamper {
		sum[0] ^= 0xff
	}
	mux := http.NewServeMux()
	name := "/" + version + "/phpMyAdmin-" + version + "-all-languages.zip"
	mux.HandleFunc(name, func(w http.ResponseWriter, _ *http.Request) { w.Write(data) })
	mux.HandleFunc(name+".sha256", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(hex.EncodeToString(sum[:]) + "  phpMyAdmin.zip\n"))
	})
	mux.HandleFunc("/version.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"version":"` + version + `","releases":[{"version":"` + version + `","php_versions":">=7.2,<8.4"}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	oldV, oldF := VersionURL, FilesURL
	VersionURL, FilesURL = srv.URL+"/version.json", srv.URL
	t.Cleanup(func() { VersionURL, FilesURL = oldV, oldF })
}

func TestLatestInstallMoveUninstall(t *testing.T) {
	server(t, "5.2.3", false)
	ctx := context.Background()

	rel, err := Latest(ctx, nil)
	if err != nil || rel.Version != "5.2.3" || rel.PHP != ">=7.2,<8.4" {
		t.Fatalf("Latest = %+v, %v", rel, err)
	}

	folder := filepath.Join(t.TempDir(), "tools", "pma")
	dir, err := Install(ctx, nil, folder, rel, nil)
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(folder, "5.2.3") || Installed(folder) != "5.2.3" {
		t.Errorf("dir %s, installed %q", dir, Installed(folder))
	}
	if _, err := os.Stat(filepath.Join(dir, "index.php")); err != nil {
		t.Error("the single top folder of the zip should be flattened")
	}
	if _, err := os.Stat(filepath.Join(dir, "setup")); err == nil {
		t.Error("setup should be removed")
	}

	if err := WriteConfig(folder, "5.2.3", []Server{{Name: "Maria'DB", Port: 3307}}); err != nil {
		t.Fatal(err)
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, "config.inc.php"))
	for _, want := range []string{"['port'] = 3307;", `'Maria\'DB'`, "AllowNoPassword", "flock_root_is_open(3307)", "config.user.inc.php"} {
		if !strings.Contains(string(cfg), want) {
			t.Errorf("config.inc.php lacks %q:\n%s", want, cfg)
		}
	}
	secret, _ := os.ReadFile(filepath.Join(folder, "blowfish_secret"))
	if len(secret) != 32 {
		t.Errorf("secret %q", secret)
	}

	to := filepath.Join(t.TempDir(), "elsewhere")
	moved, err := Move(folder, to, "5.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(moved, "index.php")); err != nil {
		t.Error("the version moved with its files")
	}
	if got, _ := os.ReadFile(filepath.Join(to, "blowfish_secret")); string(got) != string(secret) {
		t.Error("the secret moved too, so logins survive")
	}
	if _, err := os.Stat(folder); err == nil {
		t.Error("the old folder should be gone")
	}

	if err := Uninstall(to); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(to); err == nil {
		t.Error("an emptied folder is removed")
	}
}

func TestChecksumMismatchInstallsNothing(t *testing.T) {
	server(t, "5.2.3", true)
	folder := filepath.Join(t.TempDir(), "pma")
	if _, err := Install(context.Background(), nil, folder, Release{Version: "5.2.3"}, nil); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v", err)
	}
	if Installed(folder) != "" {
		t.Error("nothing should be installed")
	}
}

func TestFolderChecks(t *testing.T) {
	other := t.TempDir()
	os.WriteFile(filepath.Join(other, "notes.txt"), []byte("mine"), 0o644)
	if err := CheckFolder(other); err == nil {
		t.Error("a folder with other files was accepted")
	}
	if err := Uninstall(other); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(other, "notes.txt")); err != nil {
		t.Error("Uninstall touched a file that isn't Flock's")
	}
	if CheckFolder("relative/dir") == nil {
		t.Error("a relative folder was accepted")
	}
	if err := CheckFolder(filepath.Join(t.TempDir(), "new")); err != nil {
		t.Errorf("a folder that doesn't exist yet: %v", err)
	}
}

func TestPHPFor(t *testing.T) {
	installed := []string{"8.5", "8.3", "8.2", "7.4"}
	cases := []struct{ def, want string }{{"8.5", "8.3"}, {"8.2", "8.2"}, {"", "8.3"}}
	for _, c := range cases {
		if got := PHPFor(">=7.2,<8.4", installed, c.def); got != c.want {
			t.Errorf("default %q: got %q, want %q", c.def, got, c.want)
		}
	}
	if PHPFor(">=7.2,<8.4", []string{"8.5"}, "8.5") != "" {
		t.Error("no supported PHP installed should give \"\"")
	}
	if !Supports("", "8.5") || Supports(">=8.0", "7.4") || !Supports(">=7.2,<8.4", "7.2") || Supports(">=7.2,<8.4", "8.4") {
		t.Error("Supports bounds")
	}
}
