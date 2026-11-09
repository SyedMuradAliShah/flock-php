package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A small i18n/ in a temporary repository root, with the folders the apps' files go to.
func catalogs(t *testing.T, en, cs, es string) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"i18n", "macos/Resources", "windows/Flock.Kit", "windows/Flock.Tray", "windows/installer", "linux/po"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range map[string]string{"en.json": en, "cs.json": cs, "es.json": es} {
		if err := os.WriteFile(filepath.Join(root, "i18n", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func read(t *testing.T, root, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

const (
	sampleEN = `{
  "Update to {version}": {"note": "{version}: a PHP version", "macos": true, "windows": "Tray.General_UpdateTo", "linux": true},
  "{n} sites": {"one": "{n} site", "macos": "%lld sites", "windows": "Kit.Sites", "linux": true},
  "Settings": {"macos": true, "windows": "Tray.Settings", "linux": true},
  "Update [noun]": {"macos": "Update noun", "linux": true},
  "The update was not installed: {reason}": {"windows": "Setup.UpdateNotInstalled"},
  "PHP": {"translate": false, "macos": true}
}`
	sampleCS = `{
  "Update to {version}": "Aktualizovat na {version}",
  "{n} sites": {"one": "{n} web", "few": "{n} weby", "other": "{n} webů"},
  "Settings": "Nastavení",
  "Update [noun]": "Aktualizace",
  "The update was not installed: {reason}": "Aktualizace nebyla nainstalována: {reason}"
}`
	sampleES = `{
  "Update to {version}": "Actualizar a {version}",
  "{n} sites": {"one": "{n} sitio", "other": "{n} sitios"},
  "Settings": {"macos": "Ajustes", "default": "Configuración"},
  "Update [noun]": "Actualización",
  "The update was not installed: {reason}": "La actualización no se instaló: {reason}"
}`
)

func TestGeneratesEveryAppsFiles(t *testing.T) {
	root := catalogs(t, sampleEN, sampleCS, sampleES)
	if err := run(root, false); err != nil {
		t.Fatal(err)
	}
	if err := run(root, true); err != nil {
		t.Fatalf("a second run should find everything up to date: %v", err)
	}

	xcstrings := read(t, root, "macos/Resources/Localizable.xcstrings")
	for _, want := range []string{
		`"Update to %@" : {`, `"value" : "Aktualizovat na %@"`, `"comment" : "{version}: a PHP version"`,
		`"%lld sites" : {`, `"value" : "%lld webů"`, `"many" : {`, `"value" : "%lld site"`,
		`"Settings" : {`, `"value" : "Ajustes"`,
		`"Update noun" : {`, `"value" : "Update"`,
		`"PHP" : {` + "\n" + `      "shouldTranslate" : false`,
	} {
		if !strings.Contains(xcstrings, want) {
			t.Errorf("Localizable.xcstrings lacks %s:\n%s", want, xcstrings)
		}
	}
	if strings.Contains(xcstrings, "Configuración") || strings.Contains(xcstrings, "not installed") {
		t.Error("Localizable.xcstrings has another app's text")
	}

	kit := read(t, root, "windows/Flock.Kit/Strings.cs.resx")
	for _, want := range []string{`<data name="Sites_one"`, `<value>{0} web</value>`, `<data name="Sites_few"`, `<value>{0} webů</value>`} {
		if !strings.Contains(kit, want) {
			t.Errorf("Flock.Kit/Strings.cs.resx lacks %s", want)
		}
	}
	tray := read(t, root, "windows/Flock.Tray/Strings.resx")
	if !strings.Contains(tray, "<value>Update to {0}</value>") || !strings.Contains(tray, "<comment>{0}: a PHP version</comment>") {
		t.Errorf("Flock.Tray/Strings.resx:\n%s", tray)
	}
	if es := read(t, root, "windows/Flock.Tray/Strings.es.resx"); !strings.Contains(es, "<value>Configuración</value>") {
		t.Errorf("Flock.Tray/Strings.es.resx:\n%s", es)
	}
	if english := read(t, root, "windows/Flock.Kit/Strings.resx"); strings.Contains(english, "_few") {
		t.Error("English has a Czech plural form")
	}
	if iss := read(t, root, "windows/installer/Strings.iss"); !strings.Contains(iss, "cs.UpdateNotInstalled=Aktualizace nebyla nainstalována: %1\n") {
		t.Errorf("Strings.iss:\n%s", iss)
	}

	po := read(t, root, "linux/po/cs.po")
	for _, want := range []string{
		`"Plural-Forms: nplurals=3; plural=(n==1) ? 0 : (n>=2 && n<=4) ? 1 : 2;\n"`,
		"msgid \"{n} site\"\nmsgid_plural \"{n} sites\"\nmsgstr[0] \"{n} web\"\nmsgstr[1] \"{n} weby\"\nmsgstr[2] \"{n} webů\"\n",
		"msgctxt \"noun\"\nmsgid \"Update\"\nmsgstr \"Aktualizace\"\n",
		"#. {version}: a PHP version\nmsgid \"Update to {version}\"\n",
	} {
		if !strings.Contains(po, want) {
			t.Errorf("cs.po lacks %q:\n%s", want, po)
		}
	}
	if strings.Contains(po, "PHP\"") || strings.Contains(po, "not installed") {
		t.Error("cs.po has another app's text")
	}
}

func TestCheckFindsStaleFiles(t *testing.T) {
	root := catalogs(t, sampleEN, sampleCS, sampleES)
	if err := run(root, true); err == nil || !strings.Contains(err.Error(), "linux/po/cs.po") {
		t.Fatalf("got %v, want the files that aren't generated yet", err)
	}
	if err := run(root, false); err != nil {
		t.Fatal(err)
	}
	// A catalog out of its canonical order is stale too.
	if err := os.WriteFile(filepath.Join(root, "i18n", "cs.json"), []byte(strings.ReplaceAll(sampleCS, "\n  ", " ")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(root, true); err == nil || !strings.Contains(err.Error(), "i18n/cs.json") {
		t.Fatalf("got %v, want i18n/cs.json", err)
	}
}

func TestReportsProblems(t *testing.T) {
	for _, test := range []struct {
		name, en, cs, es, want string
	}{
		{
			name: "missing translation",
			en:   `{"Open": {"macos": true}}`, cs: `{}`, es: `{"Open": "Abrir"}`,
			want: `"Open": no Czech translation`,
		},
		{
			name: "lost placeholder",
			en:   `{"Open {name}": {"linux": true}}`, cs: `{"Open {name}": "Otevřít"}`, es: `{"Open {name}": "Abrir {name}"}`,
			want: `has other placeholders than the English`,
		},
		{
			name: "missing plural form",
			en:   `{"{n} sites": {"one": "{n} site", "linux": true}}`,
			cs:   `{"{n} sites": {"one": "{n} web", "other": "{n} webů"}}`,
			es:   `{"{n} sites": {"one": "{n} sitio", "other": "{n} sitios"}}`,
			want: `no "few" form`,
		},
		{
			name: "app without its translation",
			en:   `{"Settings": {"macos": true, "linux": true}}`,
			cs:   `{"Settings": "Nastavení"}`,
			es:   `{"Settings": {"macos": "Ajustes"}}`,
			want: `no Spanish translation for linux`,
		},
		{
			name: "translation of an unknown text",
			en:   `{"Open": {"macos": true}}`, cs: `{"Open": "Otevřít", "Close": "Zavřít"}`, es: `{"Open": "Abrir"}`,
			want: `cs.json "Close": not in en.json`,
		},
		{
			name: "unused",
			en:   `{"Open": {}}`, cs: `{"Open": "Otevřít"}`, es: `{"Open": "Abrir"}`,
			want: `no app uses it`,
		},
		{
			name: "same Windows key twice",
			en:   `{"Open": {"windows": "Tray.Open"}, "Open it": {"windows": "Tray.open"}}`,
			cs:   `{"Open": "Otevřít", "Open it": "Otevřít ho"}`, es: `{"Open": "Abrir", "Open it": "Abrirlo"}`,
			want: `Windows key "Tray.open" is also "Open"'s`,
		},
		{
			name: "duplicate key",
			en:   `{"Open": {"macos": true}, "Open": {"linux": true}}`, cs: `{}`, es: `{}`,
			want: `"Open" is there twice`,
		},
		{
			name: "stray brace",
			en:   `{"Use {{ and }}": {"linux": true}}`, cs: `{"Use {{ and }}": "x"}`, es: `{"Use {{ and }}": "x"}`,
			want: `a brace that isn't a {placeholder}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := run(catalogs(t, test.en, test.cs, test.es), true)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %q", err, test.want)
			}
		})
	}
}

func TestMacFormat(t *testing.T) {
	e := &Entry{ID: "x", Text: "{name} {version} stops and {name} {next} starts.", MacOS: &MacOSUse{Derived: true}}
	f, err := newMacFormat(e)
	if err != nil {
		t.Fatal(err)
	}
	if key := macKey(e); key != "%@ %@ stops and %@ %@ starts." {
		t.Errorf("key %q", key)
	}
	// A repeated name is one argument per place, in order.
	if got := f.render("{name} {version} končí, {name} {next} začíná."); got != "%1$@ %2$@ končí, %3$@ %4$@ začíná." {
		t.Errorf("got %q", got)
	}

	e = &Entry{ID: "y", Text: "Downloading… {0}%", MacOS: &MacOSUse{Key: "Downloading… %lld%%"}}
	if f, err = newMacFormat(e); err != nil {
		t.Fatal(err)
	}
	if got := f.render("Stahuje se… {0} %"); got != "Stahuje se… %lld %%" {
		t.Errorf("got %q", got)
	}
	if got := f.sequential(e.Text); got != macKey(e) {
		t.Errorf("sequential %q", got)
	}

	e = &Entry{ID: "z", Text: "Downloads PHP {version} ({size} MB)", MacOS: &MacOSUse{Key: "Downloads PHP %@ (%lld MB)"}}
	if f, err = newMacFormat(e); err != nil {
		t.Fatal(err)
	}
	if got := f.render("({size} MB) PHP {version}"); got != "(%2$lld MB) PHP %1$@" {
		t.Errorf("got %q", got)
	}

	e = &Entry{ID: "w", Text: "{a} and {b}", MacOS: &MacOSUse{Key: "%@ only"}}
	if _, err := newMacFormat(e); err == nil {
		t.Error("a key with fewer arguments than the text should fail")
	}
}

func TestWindowsArgs(t *testing.T) {
	for _, test := range []struct {
		entry Entry
		want  []string
	}{
		{Entry{Text: "{site} uses {version}"}, []string{"site", "version"}},
		{Entry{Text: "{1} of {0}"}, []string{"0", "1"}},
		{Entry{Text: "{0} then drops {2}"}, []string{"0", "", "2"}},
		{Entry{Text: "{n} sites in {folder}", One: "One site in {folder}"}, []string{"n", "folder"}},
		{Entry{Text: "{b} {a}", Windows: &WindowsUse{Args: []string{"a", "b"}}}, []string{"a", "b"}},
	} {
		if test.entry.Windows == nil {
			test.entry.Windows = &WindowsUse{}
		}
		if got := windowsArgs(&test.entry); !slices.Equal(got, test.want) {
			t.Errorf("%q: got %q, want %q", test.entry.Text, got, test.want)
		}
	}

	e := &Entry{Text: "{size} bytes of data", Windows: &WindowsUse{Formats: map[string]string{"size": "N0"}}}
	if got := windowsFormat(e, false)("{size} bajtů dat"); got != "{0:N0} bajtů dat" {
		t.Errorf("got %q", got)
	}
}

// The repository's catalogs are valid and the apps' files are what they generate.
func TestRepositoryIsUpToDate(t *testing.T) {
	if err := run(filepath.Join("..", ".."), true); err != nil {
		t.Fatal(err)
	}
}
