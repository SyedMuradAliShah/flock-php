package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Format specifiers of a macOS key, in argument order: Swift's interpolations.
var macSpecifier = regexp.MustCompile(`%(?:(\d+)\$)?(lld|ld|d|@)`)

// The entry's key in Localizable.xcstrings.
func macKey(e *Entry) string {
	if !e.MacOS.Derived {
		return e.MacOS.Key
	}
	key := e.Text
	if placeholder.MatchString(key) {
		key = strings.ReplaceAll(key, "%", "%%")
	}
	return placeholder.ReplaceAllString(key, "%@")
}

// macFormat turns texts with {name} placeholders into format strings of the entry's key: each
// placeholder becomes the specifier of its argument, the arguments in the order the key's text has
// them. A name the text has twice ("{name} … {name}") is two arguments; a translation's first
// {name} is the first of them, and so on.
type macFormat struct {
	types []string         // each argument's specifier: "@", "lld"
	args  map[string][]int // a name's arguments, 1-based
}

func newMacFormat(e *Entry) (*macFormat, error) {
	key := macKey(e)
	f := &macFormat{args: map[string][]int{}}
	for i, match := range macSpecifier.FindAllStringSubmatch(key, -1) {
		if match[1] != "" && match[1] != strconv.Itoa(i+1) {
			return nil, fmt.Errorf("%q: macOS key %q has its arguments out of order", e.ID, key)
		}
		f.types = append(f.types, match[2])
	}
	names := placeholders(e.Text)
	if len(names) != len(f.types) {
		return nil, fmt.Errorf("%q: macOS key %q has %d arguments, the text %d placeholders", e.ID, key, len(f.types), len(names))
	}
	for i, name := range names {
		f.args[name] = append(f.args[name], i+1)
	}
	return f, nil
}

// The text as a format string. Several arguments are numbered ("%2$@"), so a translation can
// reorder them or, in a singular, leave the count out.
func (f *macFormat) render(text string) string {
	if len(f.types) == 0 {
		return text
	}
	numbered := len(f.types) > 1
	seen := map[string]int{}
	text = strings.ReplaceAll(text, "%", "%%")
	return placeholder.ReplaceAllStringFunc(text, func(match string) string {
		name := match[1 : len(match)-1]
		args := f.args[name]
		arg := args[min(seen[name], len(args)-1)]
		seen[name]++
		if numbered {
			return fmt.Sprintf("%%%d$%s", arg, f.types[arg-1])
		}
		return "%" + f.types[arg-1]
	})
}

// The text as Swift writes its key: the specifiers unnumbered, in the text's order.
func (f *macFormat) sequential(text string) string {
	if len(f.types) == 0 {
		return text
	}
	i := 0
	return placeholder.ReplaceAllStringFunc(strings.ReplaceAll(text, "%", "%%"), func(string) string {
		i++
		return "%" + f.types[min(i, len(f.types))-1]
	})
}

func stringUnit(value string) map[string]any {
	return map[string]any{"stringUnit": map[string]any{"state": "translated", "value": value}}
}

func generateMacOS(c *Catalog) (file, error) {
	strs := map[string]any{}
	for _, e := range c.Entries {
		if e.MacOS == nil {
			continue
		}
		key := macKey(e)
		if e.NoTranslate {
			strs[key] = map[string]any{"shouldTranslate": false}
			continue
		}
		f, err := newMacFormat(e)
		if err != nil {
			return file{}, err
		}
		localizations := map[string]any{}
		if e.Plural() {
			localizations["en"] = map[string]any{"variations": map[string]any{"plural": map[string]any{
				"one": stringUnit(f.render(e.One)), "other": stringUnit(f.render(e.Text)),
			}}}
		} else if f.sequential(e.Text) != key {
			// The key isn't the English text: "Database category".
			localizations["en"] = stringUnit(f.render(e.Text))
		}
		for _, language := range languages {
			text := e.Translations[language.Code].For(MacOS)
			if !e.Plural() {
				localizations[language.Code] = stringUnit(f.render(text.Plain))
				continue
			}
			plural := map[string]any{}
			for form, value := range text.Forms {
				plural[form] = stringUnit(f.render(value))
			}
			// macOS asks for every form the language has, Czech "many" too.
			for _, form := range language.Optional {
				if _, ok := plural[form]; !ok {
					plural[form] = stringUnit(f.render(text.Forms["other"]))
				}
			}
			localizations[language.Code] = map[string]any{"variations": map[string]any{"plural": plural}}
		}
		entry := map[string]any{"localizations": localizations}
		if e.Note != "" {
			entry["comment"] = e.Note
		}
		strs[key] = entry
	}
	content := xcodeJSON(map[string]any{"sourceLanguage": "en", "strings": strs, "version": "1.0"})
	return file{path: "macos/Resources/Localizable.xcstrings", content: content}, nil
}

// JSON the way Xcode writes a string catalog: keys sorted, " : " between key and value.
func xcodeJSON(value any) []byte {
	var b strings.Builder
	writeXcodeJSON(&b, value, "")
	b.WriteString("\n")
	return []byte(b.String())
}

func writeXcodeJSON(b *strings.Builder, value any, indent string) {
	switch v := value.(type) {
	case map[string]any:
		if len(v) == 0 {
			b.WriteString("{\n\n" + indent + "}")
			return
		}
		b.WriteString("{\n")
		for i, key := range sortedKeys(v) {
			if i > 0 {
				b.WriteString(",\n")
			}
			b.WriteString(indent + "  " + quoteJSON(key) + " : ")
			writeXcodeJSON(b, v[key], indent+"  ")
		}
		b.WriteString("\n" + indent + "}")
	case string:
		b.WriteString(quoteJSON(v))
	case bool:
		b.WriteString(strconv.FormatBool(v))
	default:
		panic(fmt.Sprintf("xcodeJSON: %T", value))
	}
}
