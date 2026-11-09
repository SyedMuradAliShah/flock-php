// Command i18n generates the apps' translation files from the shared catalogs in i18n/, so the
// macOS, Windows and Linux apps share one translation of each text. Run it from the repository
// root after editing a catalog:
//
//	go run ./tools/i18n
//
// It writes macos/Resources/Localizable.xcstrings, Strings*.resx of windows/Flock.Kit and
// windows/Flock.Tray, windows/installer/Strings.iss and linux/po/*.po, and puts the catalogs in
// their canonical order. With -check it writes nothing and fails when the catalogs have a problem
// or a file isn't what they generate; CI runs it. i18n/README.md describes the catalogs.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	check := flag.Bool("check", false, "write nothing; fail when a file is out of date")
	flag.Parse()
	if err := run(".", *check); err != nil {
		fmt.Fprintln(os.Stderr, "i18n:", err)
		os.Exit(1)
	}
}

func run(root string, check bool) error {
	catalog, err := load(filepath.Join(root, "i18n"))
	if err != nil {
		return err
	}
	files, err := generate(catalog)
	if err != nil {
		return err
	}
	var stale []string
	for _, file := range files {
		path := filepath.Join(root, filepath.FromSlash(file.path))
		old, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if bytes.Equal(old, file.content) {
			continue
		}
		if check {
			stale = append(stale, file.path)
			continue
		}
		if err := os.WriteFile(path, file.content, 0o644); err != nil {
			return err
		}
		fmt.Println("wrote", file.path)
	}
	if len(stale) > 0 {
		return fmt.Errorf("not up to date, run go run ./tools/i18n:\n  %s", strings.Join(stale, "\n  "))
	}
	return nil
}

// A file the catalogs generate, by its path from the repository root.
type file struct {
	path    string
	content []byte
}

func generate(c *Catalog) ([]file, error) {
	var files []file
	source, err := formatCatalogs(c)
	if err != nil {
		return nil, err
	}
	files = append(files, source...)
	xcstrings, err := generateMacOS(c)
	if err != nil {
		return nil, err
	}
	files = append(files, xcstrings)
	windows, err := generateWindows(c)
	if err != nil {
		return nil, err
	}
	files = append(files, windows...)
	return append(files, generateLinux(c)...), nil
}
