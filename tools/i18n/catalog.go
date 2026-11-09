package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// The apps, as en.json names them.
const (
	MacOS   = "macos"
	Windows = "windows"
	Linux   = "linux"
)

var platforms = []string{MacOS, Windows, Linux}

// A language the apps are translated to: its catalog is i18n/<code>.json.
type Language struct {
	Code string
	Name string // in English, for the generated files' comments
	// The plural forms a plural text needs, for whole numbers (CLDR); Optional ones may be left out.
	Forms    []string
	Optional []string
	// gettext's Plural-Forms for Forms, in their order.
	PluralForms string
}

// The translations; English is the source. The apps' language lists match.
var languages = []Language{
	// Czech "many" is for fractions; only macOS asks for it, and gets "other" without it.
	{Code: "cs", Name: "Czech", Forms: []string{"one", "few", "other"}, Optional: []string{"many"},
		PluralForms: "nplurals=3; plural=(n==1) ? 0 : (n>=2 && n<=4) ? 1 : 2;"},
	{Code: "es", Name: "Spanish", Forms: []string{"one", "other"},
		PluralForms: "nplurals=2; plural=(n != 1);"},
}

// The plural forms in the order the catalogs list them.
var formOrder = []string{"one", "few", "many", "other"}

// Catalog is i18n/: every text with its translations.
type Catalog struct {
	Entries []*Entry // by ID
}

// Entry is a text of en.json.
type Entry struct {
	ID      string
	Text    string // the English, the "other" form of a plural
	Context string // tells apart texts that are the same in English: "Update [noun]"
	One     string // the English singular of a plural text
	Note    string // for translators
	// Not translated: every language shows the English, e.g. a format or an example address
	// that macOS's catalog has to list all the same.
	NoTranslate bool
	MacOS       *MacOSUse
	Windows     *WindowsUse
	Linux       bool
	// Each language's translation.
	Translations map[string]*Translation
}

func (e *Entry) Plural() bool { return e.One != "" }

// The apps that use the entry.
func (e *Entry) Platforms() []string {
	var used []string
	if e.MacOS != nil {
		used = append(used, MacOS)
	}
	if e.Windows != nil {
		used = append(used, Windows)
	}
	if e.Linux {
		used = append(used, Linux)
	}
	return used
}

// MacOSUse: the text's key in Localizable.xcstrings, the format string Swift makes of it
// ("Update to %@"). Derived keys are the text with every placeholder as %@.
type MacOSUse struct {
	Key     string
	Derived bool
}

// WindowsUse: the resource names the text has ("Tray.General_UpdateTo"; Setup is the
// installer's [CustomMessages]) and the index of each placeholder in String.Format's arguments.
type WindowsUse struct {
	Keys []string
	// The placeholder at each index, "" for an argument the text leaves out; nil for the
	// default order (see windowsArgs).
	Args    []string
	Formats map[string]string // a placeholder's format: {0:N0}
}

// Translation is a text's translation into a language: one for every app, or each app its own.
type Translation struct {
	Default  *Text
	Platform map[string]*Text
}

// For returns the translation an app uses, nil when there is none.
func (t *Translation) For(platform string) *Text {
	if text, ok := t.Platform[platform]; ok {
		return text
	}
	return t.Default
}

// Text is a translated text, or a plural's forms.
type Text struct {
	Plain string
	Forms map[string]string
}

var (
	placeholder = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)
	contextID   = regexp.MustCompile(`^(.+) \[([^\]]+)\]$`)
	windowsKey  = regexp.MustCompile(`^(Kit|Tray|Setup)\.[A-Za-z_][A-Za-z0-9_]*$`)
)

// The placeholders of a text, in order, with repeats.
func placeholders(text string) []string {
	var names []string
	for _, match := range placeholder.FindAllStringSubmatch(text, -1) {
		names = append(names, match[1])
	}
	return names
}

func nameSet(texts ...string) map[string]bool {
	set := map[string]bool{}
	for _, text := range texts {
		for _, name := range placeholders(text) {
			set[name] = true
		}
	}
	return set
}

func sameSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for name := range a {
		if !b[name] {
			return false
		}
	}
	return true
}

func sortedNames(set map[string]bool) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func load(dir string) (*Catalog, error) {
	raw, err := readObject(filepath.Join(dir, "en.json"))
	if err != nil {
		return nil, err
	}
	c := &Catalog{}
	byID := map[string]*Entry{}
	var problems []string
	for id, value := range raw {
		e, err := parseEntry(id, value)
		if err != nil {
			problems = append(problems, fmt.Sprintf("en.json %q: %v", id, err))
			continue
		}
		c.Entries = append(c.Entries, e)
		byID[id] = e
	}
	sort.Slice(c.Entries, func(i, j int) bool { return c.Entries[i].ID < c.Entries[j].ID })
	for _, language := range languages {
		name := language.Code + ".json"
		raw, err := readObject(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		for id, value := range raw {
			e := byID[id]
			if e == nil {
				problems = append(problems, fmt.Sprintf("%s %q: not in en.json", name, id))
				continue
			}
			t, err := parseTranslation(value)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s %q: %v", name, id, err))
				continue
			}
			if e.Translations == nil {
				e.Translations = map[string]*Translation{}
			}
			e.Translations[language.Code] = t
		}
	}
	problems = append(problems, c.validate()...)
	if len(problems) > 0 {
		sort.Strings(problems)
		return nil, errors.New("the catalogs have problems:\n  " + strings.Join(problems, "\n  "))
	}
	return c, nil
}

// A catalog file's object, refusing a key that's there twice (the second would win unseen).
func readObject(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("%s: not a JSON object", path)
	}
	object := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		key := token.(string)
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("%s %q: %w", path, key, err)
		}
		if _, ok := object[key]; ok {
			return nil, fmt.Errorf("%s: %q is there twice", path, key)
		}
		object[key] = value
	}
	return object, nil
}

func strict(data []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}

func parseEntry(id string, data json.RawMessage) (*Entry, error) {
	var raw struct {
		One       string          `json:"one"`
		Note      string          `json:"note"`
		Translate *bool           `json:"translate"`
		MacOS     json.RawMessage `json:"macos"`
		Windows   json.RawMessage `json:"windows"`
		Linux     bool            `json:"linux"`
	}
	if err := strict(data, &raw); err != nil {
		return nil, err
	}
	e := &Entry{ID: id, Text: id, One: raw.One, Note: raw.Note, Linux: raw.Linux}
	if match := contextID.FindStringSubmatch(id); match != nil {
		e.Text, e.Context = match[1], match[2]
	}
	if raw.Translate != nil {
		if *raw.Translate {
			return nil, errors.New(`"translate" is only ever false`)
		}
		e.NoTranslate = true
	}
	if raw.MacOS != nil {
		var derived bool
		var key string
		switch {
		case json.Unmarshal(raw.MacOS, &derived) == nil && derived:
			e.MacOS = &MacOSUse{Derived: true}
		case json.Unmarshal(raw.MacOS, &key) == nil && key != "":
			e.MacOS = &MacOSUse{Key: key}
		default:
			return nil, errors.New(`"macos" is true or the key`)
		}
	}
	if raw.Windows != nil {
		var key string
		var keys []string
		var use WindowsUse
		switch {
		case json.Unmarshal(raw.Windows, &key) == nil:
			use.Keys = []string{key}
		case json.Unmarshal(raw.Windows, &keys) == nil:
			use.Keys = keys
		default:
			var object struct {
				Keys    []string          `json:"keys"`
				Args    []string          `json:"args"`
				Formats map[string]string `json:"formats"`
			}
			if err := strict(raw.Windows, &object); err != nil {
				return nil, fmt.Errorf(`"windows": %w`, err)
			}
			use = WindowsUse{Keys: object.Keys, Args: object.Args, Formats: object.Formats}
		}
		e.Windows = &use
	}
	return e, nil
}

func parseText(data json.RawMessage) (*Text, error) {
	var plain string
	if json.Unmarshal(data, &plain) == nil {
		return &Text{Plain: plain}, nil
	}
	var forms map[string]string
	if err := strict(data, &forms); err != nil {
		return nil, errors.New("a translation is a string or plural forms")
	}
	for form := range forms {
		if !slices.Contains(formOrder, form) {
			return nil, fmt.Errorf("unknown plural form %q", form)
		}
	}
	return &Text{Forms: forms}, nil
}

func parseTranslation(data json.RawMessage) (*Translation, error) {
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) == nil {
		perPlatform := false
		for key := range object {
			if key == "default" || slices.Contains(platforms, key) {
				perPlatform = true
			}
		}
		if perPlatform {
			t := &Translation{Platform: map[string]*Text{}}
			for key, value := range object {
				if key != "default" && !slices.Contains(platforms, key) {
					return nil, fmt.Errorf("%q is neither an app nor \"default\"", key)
				}
				text, err := parseText(value)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", key, err)
				}
				if key == "default" {
					t.Default = text
				} else {
					t.Platform[key] = text
				}
			}
			return t, nil
		}
	}
	text, err := parseText(data)
	if err != nil {
		return nil, err
	}
	return &Translation{Default: text}, nil
}

// The text an app shows in a language.
func (e *Entry) translation(language, platform string) *Text {
	if e.NoTranslate {
		return &Text{Plain: e.Text}
	}
	return e.Translations[language].For(platform)
}

// The count of a plural text: {n}, or {0} in a text without names.
func (e *Entry) count() string {
	names := nameSet(e.Text, e.One)
	switch {
	case names["n"]:
		return "n"
	case names["0"]:
		return "0"
	}
	return ""
}

func (c *Catalog) validate() []string {
	var problems []string
	macKeys := map[string]string{}
	windowsKeys := map[string]string{}
	for _, e := range c.Entries {
		report := func(format string, args ...any) {
			problems = append(problems, fmt.Sprintf("%q: %s", e.ID, fmt.Sprintf(format, args...)))
		}
		used := e.Platforms()
		if len(used) == 0 {
			report("no app uses it")
		}
		for _, text := range []string{e.Text, e.One} {
			if strings.ContainsAny(placeholder.ReplaceAllString(text, ""), "{}") {
				report("a brace that isn't a {placeholder} in %q", text)
			}
		}
		if e.Plural() {
			for name := range nameSet(e.One) {
				if !nameSet(e.Text)[name] {
					report("the singular has {%s}, which the plural lacks", name)
				}
			}
		}
		if e.NoTranslate {
			if e.Plural() {
				report(`a text with plural forms is translated`)
			}
			if len(e.Translations) > 0 {
				report("it isn't translated, but has translations")
			}
		}
		if m := e.MacOS; m != nil {
			if m.Derived && (e.Context != "" || e.Plural()) {
				report(`a text with a context or plural forms names its macOS key ("macos": "…")`)
			}
			key := macKey(e)
			if other, ok := macKeys[key]; ok {
				report("macOS key %q is also %q's", key, other)
			}
			macKeys[key] = e.ID
		}
		if w := e.Windows; w != nil {
			if len(w.Keys) == 0 {
				report("no Windows key")
			}
			for _, key := range w.Keys {
				if !windowsKey.MatchString(key) {
					report("Windows key %q isn't Kit.Name, Tray.Name or Setup.Name", key)
				}
				// A .resx can't hold two names that differ only in case.
				folded := strings.ToLower(key)
				if other, ok := windowsKeys[folded]; ok {
					report("Windows key %q is also %q's", key, other)
				}
				windowsKeys[folded] = e.ID
				if strings.HasPrefix(key, "Setup.") && (e.Plural() || strings.Contains(e.Text, "%")) {
					report("installer messages have no plural forms or %% signs")
				}
			}
			names := nameSet(e.Text, e.One)
			if w.Args != nil {
				for name := range names {
					if !slices.Contains(w.Args, name) {
						report("Windows args lack {%s}", name)
					}
				}
				for _, name := range w.Args {
					if name != "" && !names[name] {
						report("Windows args have {%s}, which the text lacks", name)
					}
				}
			}
			for name := range w.Formats {
				if !names[name] {
					report("Windows formats {%s}, which the text lacks", name)
				}
			}
		}
		if e.NoTranslate {
			continue
		}
		for _, language := range languages {
			t := e.Translations[language.Code]
			if t == nil {
				report("no %s translation", language.Name)
				continue
			}
			for platform := range t.Platform {
				if !slices.Contains(used, platform) {
					report("%s has a translation for %s, which doesn't use it", language.Name, platform)
				}
			}
			if t.Default != nil && len(t.Platform) == len(used) {
				report("%s has a default no app uses", language.Name)
			}
			for _, platform := range used {
				text := t.For(platform)
				if text == nil {
					report("no %s translation for %s", language.Name, platform)
					continue
				}
				for _, problem := range e.checkText(language, platform, text) {
					report("%s (%s): %s", language.Name, platform, problem)
				}
			}
		}
	}
	return problems
}

// What's wrong with a translation of the entry.
func (e *Entry) checkText(language Language, platform string, text *Text) []string {
	var problems []string
	if !e.Plural() {
		if text.Forms != nil {
			return []string{"plural forms for a text without them"}
		}
		if strings.TrimSpace(text.Plain) == "" {
			return []string{"empty"}
		}
		if !sameSet(nameSet(text.Plain), nameSet(e.Text)) {
			problems = append(problems, fmt.Sprintf("%q has other placeholders than the English", text.Plain))
		}
		return problems
	}
	if text.Forms == nil {
		return []string{"a plural text needs its plural forms"}
	}
	for _, form := range language.Forms {
		if _, ok := text.Forms[form]; !ok {
			problems = append(problems, fmt.Sprintf("no %q form", form))
		}
	}
	for form, value := range text.Forms {
		if !slices.Contains(language.Forms, form) && !slices.Contains(language.Optional, form) {
			problems = append(problems, fmt.Sprintf("%s has no %q form", language.Name, form))
			continue
		}
		if strings.TrimSpace(value) == "" {
			problems = append(problems, fmt.Sprintf("the %q form is empty", form))
			continue
		}
		found, english := nameSet(value), nameSet(e.Text)
		withoutCount := nameSet(e.Text)
		delete(withoutCount, e.count())
		switch {
		case form != "one" && !sameSet(found, english):
			problems = append(problems, fmt.Sprintf("%q has other placeholders than the English", value))
		case form == "one" && !sameSet(found, english) && !sameSet(found, withoutCount):
			problems = append(problems, fmt.Sprintf("%q has other placeholders than the English", value))
		// Windows checks a singular against the English singular.
		case form == "one" && platform == Windows && !sameSet(found, nameSet(e.One)):
			problems = append(problems, fmt.Sprintf("%q has other placeholders than the English singular", value))
		}
	}
	return problems
}
