package main

import (
	"strings"
)

// linux/po/<code>.po, which linux/src/i18n.rs builds into the app: msgid is the English, with a
// msgctxt for a text's context.
func generateLinux(c *Catalog) []file {
	var files []file
	for _, language := range languages {
		var b strings.Builder
		b.WriteString("# " + language.Name + " translations of the Flock Linux app (linux/src), keyed by the English\n")
		b.WriteString("# text. " + generatedNote + "\n")
		b.WriteString("# src/i18n.rs builds this file into the app; its tests fail when a text of the app is missing.\n")
		b.WriteString("msgid \"\"\nmsgstr \"\"\n")
		for _, header := range []string{
			"Project-Id-Version: flock-app", "Language: " + language.Code, "MIME-Version: 1.0",
			"Content-Type: text/plain; charset=UTF-8", "Content-Transfer-Encoding: 8bit", "Plural-Forms: " + language.PluralForms,
		} {
			b.WriteString(quotePO(header+"\n") + "\n")
		}
		for _, e := range c.Entries {
			if !e.Linux {
				continue
			}
			b.WriteString("\n")
			if e.Note != "" {
				for line := range strings.SplitSeq(e.Note, "\n") {
					b.WriteString("#. " + line + "\n")
				}
			}
			if e.Context != "" {
				b.WriteString("msgctxt " + quotePO(e.Context) + "\n")
			}
			text := e.translation(language.Code, Linux)
			if !e.Plural() {
				b.WriteString("msgid " + quotePO(e.Text) + "\n")
				b.WriteString("msgstr " + quotePO(text.Plain) + "\n")
				continue
			}
			b.WriteString("msgid " + quotePO(e.One) + "\n")
			b.WriteString("msgid_plural " + quotePO(e.Text) + "\n")
			for i, form := range language.Forms {
				b.WriteString("msgstr[" + string(rune('0'+i)) + "] " + quotePO(text.Forms[form]) + "\n")
			}
		}
		files = append(files, file{path: "linux/po/" + language.Code + ".po", content: []byte(b.String())})
	}
	return files
}

func quotePO(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`).Replace(s) + `"`
}
