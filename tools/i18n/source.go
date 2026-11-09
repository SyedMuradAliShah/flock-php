package main

import (
	"bytes"
	"encoding/json"
	"slices"
	"sort"
	"strings"
)

// The catalogs in their canonical form: texts sorted by ID, fields in a fixed order, two-space
// indent. A text can be added anywhere; the next run puts it in its place.
func formatCatalogs(c *Catalog) ([]file, error) {
	var en strings.Builder
	en.WriteString("{")
	for i, e := range c.Entries {
		var fields []string
		if e.One != "" {
			fields = append(fields, `"one": `+quoteJSON(e.One))
		}
		if e.Note != "" {
			fields = append(fields, `"note": `+quoteJSON(e.Note))
		}
		if e.NoTranslate {
			fields = append(fields, `"translate": false`)
		}
		if m := e.MacOS; m != nil {
			if m.Derived {
				fields = append(fields, `"macos": true`)
			} else {
				fields = append(fields, `"macos": `+quoteJSON(m.Key))
			}
		}
		if w := e.Windows; w != nil {
			fields = append(fields, `"windows": `+windowsJSON(w))
		}
		if e.Linux {
			fields = append(fields, `"linux": true`)
		}
		en.WriteString(separator(i) + "  " + quoteJSON(e.ID) + ": {" + strings.Join(fields, ", ") + "}")
	}
	en.WriteString("\n}\n")
	files := []file{{path: "i18n/en.json", content: []byte(en.String())}}
	for _, language := range languages {
		var b strings.Builder
		b.WriteString("{")
		i := 0
		for _, e := range c.Entries {
			t := e.Translations[language.Code]
			if t == nil {
				continue
			}
			b.WriteString(separator(i) + "  " + quoteJSON(e.ID) + ": " + translationJSON(t))
			i++
		}
		b.WriteString("\n}\n")
		files = append(files, file{path: "i18n/" + language.Code + ".json", content: []byte(b.String())})
	}
	return files, nil
}

func separator(i int) string {
	if i == 0 {
		return "\n"
	}
	return ",\n"
}

func windowsJSON(w *WindowsUse) string {
	if w.Args == nil && len(w.Formats) == 0 {
		if len(w.Keys) == 1 {
			return quoteJSON(w.Keys[0])
		}
		return stringsJSON(w.Keys)
	}
	fields := []string{`"keys": ` + stringsJSON(w.Keys)}
	if w.Args != nil {
		fields = append(fields, `"args": `+stringsJSON(w.Args))
	}
	if len(w.Formats) > 0 {
		var formats []string
		for _, name := range sortedKeys(w.Formats) {
			formats = append(formats, quoteJSON(name)+": "+quoteJSON(w.Formats[name]))
		}
		fields = append(fields, `"formats": {`+strings.Join(formats, ", ")+"}")
	}
	return "{" + strings.Join(fields, ", ") + "}"
}

func stringsJSON(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = quoteJSON(value)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func translationJSON(t *Translation) string {
	if len(t.Platform) == 0 {
		return textJSON(t.Default)
	}
	var fields []string
	if t.Default != nil {
		fields = append(fields, `"default": `+textJSON(t.Default))
	}
	for _, platform := range platforms {
		if text, ok := t.Platform[platform]; ok {
			fields = append(fields, quoteJSON(platform)+": "+textJSON(text))
		}
	}
	return "{" + strings.Join(fields, ", ") + "}"
}

func textJSON(t *Text) string {
	if t.Forms == nil {
		return quoteJSON(t.Plain)
	}
	var fields []string
	for _, form := range formOrder {
		if value, ok := t.Forms[form]; ok {
			fields = append(fields, quoteJSON(form)+": "+quoteJSON(value))
		}
	}
	return "{" + strings.Join(fields, ", ") + "}"
}

// A JSON string, with <, > and & as they are.
func quoteJSON(s string) string {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return slices.Clip(keys)
}
