package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"flock/internal/config"
)

// The test binary stands in for a server program: copied to bin/mariadbd it answers --version.
func TestMain(m *testing.M) {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	if len(os.Args) == 2 && os.Args[1] == "--version" && (name == "mariadbd" || name == "mysqld") {
		if name == "mariadbd" {
			fmt.Println("mariadbd  Ver 10.11.2-MariaDB for Win64 on AMD64 (mariadb.org binary distribution)")
		} else {
			fmt.Println("mysqld  Ver 8.4.1 for Win64 on x86_64 (MySQL Community Server - GPL)")
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fakeInstall(t *testing.T, server, admin string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "other-tool", "bin", "mariadb-10.11.2-winx64")
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	if err := os.WriteFile(filepath.Join(bin, server+suffix), data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, admin+suffix), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func customManager(t *testing.T) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	mutate := func(change func(*config.Config) error) error {
		cfg, err := config.Load(root)
		if err != nil {
			return err
		}
		if err := change(cfg); err != nil {
			return err
		}
		return cfg.Save(root)
	}
	return NewManager(root, Kinds(runtime.GOOS), nil, mutate), root
}

func TestAddACustomVersionAndRemoveItWithoutTouchingFiles(t *testing.T) {
	m, root := customManager(t)
	binary := fakeInstall(t, "mariadbd", "mariadb-admin")
	data := filepath.Join(t.TempDir(), "other-tool", "data", "mariadb")
	if err := os.MkdirAll(filepath.Join(data, "mysql"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(data, "ibdata1")
	if err := os.WriteFile(marker, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := m.AddCustom(context.Background(), "mariadb", binary, data); err != nil {
		t.Fatal(err)
	}

	cfg, _ := config.Load(root)
	x, ok := cfg.Services.Extra("mariadb")
	if !ok || x.Version != "custom-10.11.2" {
		t.Fatalf("the first version becomes the active one: %+v", x)
	}
	k, _ := m.Kind("mariadb")
	if k.LineOf(x.Version) != "custom-10.11.2" {
		t.Errorf("line = %q", k.LineOf(x.Version))
	}
	if Dir(root, "mariadb", x.Version) != binary || DataDir(root, "mariadb", "custom-10.11.2") != data {
		t.Errorf("folders: %s, %s", Dir(root, "mariadb", x.Version), DataDir(root, "mariadb", "custom-10.11.2"))
	}
	spec, err := Spec(root, k, x)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Dir != binary || !strings.HasPrefix(spec.Path, binary) {
		t.Errorf("the server runs from the user's folder: %+v", spec)
	}

	entry, _ := m.Catalog().Find("mariadb")
	if entry.Installed != "10.11.2" || entry.Line != "custom-10.11.2" {
		t.Errorf("entry: %q %q", entry.Installed, entry.Line)
	}
	var line LineState
	for _, l := range entry.Lines {
		if l.ID == "custom-10.11.2" {
			line = l
		}
	}
	if !line.Custom || !line.Active || line.Version != "10.11.2" || line.Label != "10.11.2 (custom)" || line.BaseDir != binary || line.DataDir != data || !line.HasData {
		t.Errorf("line: %+v", line)
	}

	if err := m.Uninstall("mariadb", "custom-10.11.2", true); !errors.Is(err, ErrInvalid) {
		t.Errorf("deleting the data of a version you added must be refused: %v", err)
	}
	if err := m.Uninstall("mariadb", "custom-10.11.2", false); err != nil {
		t.Fatal(err)
	}
	cfg, _ = config.Load(root)
	if _, ok := cfg.Services.Extra("mariadb"); ok || len(cfg.Services.Custom) != 0 {
		t.Errorf("nothing left in config: %+v", cfg.Services)
	}
	if b, _ := os.ReadFile(marker); string(b) != "precious" {
		t.Error("the user's data was touched")
	}
	if _, err := os.Stat(filepath.Join(binary, "bin")); err != nil {
		t.Error("the user's base directory was touched")
	}
}

func TestACustomVersionIsRefusedWhenItCannotWork(t *testing.T) {
	m, root := customManager(t)
	binary := fakeInstall(t, "mariadbd", "mariadb-admin")
	mysqlBinary := fakeInstall(t, "mysqld", "mysqladmin")
	data := t.TempDir()
	stray := t.TempDir()
	os.WriteFile(filepath.Join(stray, "notes.txt"), []byte("mine"), 0o644)
	ctx := context.Background()

	for name, call := range map[string]func() error{
		"relative paths":   func() error { return m.AddCustom(ctx, "mariadb", "other-tool/bin", "data") },
		"no program":       func() error { return m.AddCustom(ctx, "mariadb", t.TempDir(), data) },
		"other service":    func() error { return m.AddCustom(ctx, "valkey", binary, data) },
		"mariadb as mysql": func() error { return m.AddCustom(ctx, "mysql", binary, data) },
		"mysql as mariadb": func() error { return m.AddCustom(ctx, "mariadb", mysqlBinary, data) },
		"not a data dir":   func() error { return m.AddCustom(ctx, "mariadb", binary, stray) },
		"inside flock": func() error {
			return m.AddCustom(ctx, "mariadb", binary, filepath.Join(root, "services", "mariadb", "data", "x"))
		},
	} {
		if err := call(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if cfg, _ := config.Load(root); len(cfg.Services.Custom) != 0 {
		t.Errorf("nothing should be registered: %+v", cfg.Services.Custom)
	}
	if err := m.AddCustom(ctx, "mariadb", binary, data); err != nil { // an empty or new folder is fine
		t.Fatal(err)
	}
	if err := m.AddCustom(ctx, "mariadb", binary, data); err == nil {
		t.Error("the same data folder twice was accepted")
	}
	other := t.TempDir()
	if err := m.AddCustom(ctx, "mariadb", binary, other); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Load(root)
	if len(cfg.Services.Custom) != 2 || cfg.Services.Custom[1].Line != "custom-10.11.2-2" {
		t.Errorf("a second one with the same version gets its own line: %+v", cfg.Services.Custom)
	}
	if x, _ := cfg.Services.Extra("mariadb"); x.Version != "custom-10.11.2" {
		t.Errorf("a second version doesn't take over: %+v", x)
	}
}

func TestChoosingTheBinFolderItselfWorks(t *testing.T) {
	m, root := customManager(t)
	binary := fakeInstall(t, "mariadbd", "mariadb-admin")

	if err := m.AddCustom(context.Background(), "mariadb", filepath.Join(binary, "bin"), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Load(root)
	if len(cfg.Services.Custom) != 1 || cfg.Services.Custom[0].BaseDir != binary {
		t.Errorf("the folder that holds bin is what gets stored: %+v", cfg.Services.Custom)
	}
}

func TestACustomVersionWhoseFolderIsGoneIsStillListedAndRemovable(t *testing.T) {
	m, root := customManager(t)
	binary := fakeInstall(t, "mariadbd", "mariadb-admin")
	if err := m.AddCustom(context.Background(), "mariadb", binary, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(binary); err != nil {
		t.Fatal(err)
	}

	entry, _ := m.Catalog().Find("mariadb")
	var line LineState
	for _, l := range entry.Lines {
		if l.ID == "custom-10.11.2" {
			line = l
		}
	}
	if !line.Custom || !line.Missing || !line.Installed {
		t.Errorf("still listed, and marked as missing: %+v", line)
	}
	k, _ := m.Kind("mariadb")
	cfg, _ := config.Load(root)
	x, _ := cfg.Services.Extra("mariadb")
	if _, err := Spec(root, k, x); err == nil || !strings.Contains(err.Error(), "base directory") {
		t.Errorf("the start error says what is wrong: %v", err)
	}

	if err := m.Uninstall("mariadb", "custom-10.11.2", false); err != nil {
		t.Fatal(err)
	}
	cfg, _ = config.Load(root)
	if _, ok := cfg.Services.Extra("mariadb"); ok || len(cfg.Services.Custom) != 0 {
		t.Errorf("removed: %+v", cfg.Services)
	}
}
