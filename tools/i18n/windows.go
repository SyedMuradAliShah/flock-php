package main

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// The placeholder at each of String.Format's argument indices: Args when given, else every
// placeholder in the order the texts first have it (the singular first), a plural's count {n}
// first (Plural passes it as {0}). Texts whose placeholders are all numbers ({0}, {1}) use those
// as the indices.
func windowsArgs(e *Entry) []string {
	if e.Windows.Args != nil {
		return e.Windows.Args
	}
	seen := append(placeholders(e.One), placeholders(e.Text)...)
	numbered := len(seen) > 0
	highest := -1
	for _, name := range seen {
		n, err := strconv.Atoi(name)
		if err != nil {
			numbered = false
			break
		}
		highest = max(highest, n)
	}
	if numbered {
		args := make([]string, highest+1)
		for _, name := range seen {
			n, _ := strconv.Atoi(name)
			args[n] = name
		}
		return args
	}
	var args []string
	if e.Plural() && slices.Contains(seen, "n") {
		args = append(args, "n")
	}
	for _, name := range seen {
		if !slices.Contains(args, name) {
			args = append(args, name)
		}
	}
	return args
}

// windowsFormat turns {name} into the argument's {index}, or into %index for the installer
// (1-based).
func windowsFormat(e *Entry, installer bool) func(string) string {
	index := map[string]int{}
	for i, name := range windowsArgs(e) {
		if _, ok := index[name]; !ok && name != "" {
			index[name] = i
		}
	}
	return func(text string) string {
		return placeholder.ReplaceAllStringFunc(text, func(match string) string {
			name := match[1 : len(match)-1]
			i := index[name]
			if installer {
				return "%" + strconv.Itoa(i+1)
			}
			if format := e.Windows.Formats[name]; format != "" {
				return fmt.Sprintf("{%d:%s}", i, format)
			}
			return "{" + strconv.Itoa(i) + "}"
		})
	}
}

type resxString struct {
	name, value, comment string
}

func generateWindows(c *Catalog) ([]file, error) {
	// Project -> language ("" for English) -> strings.
	resx := map[string]map[string][]resxString{"Kit": {}, "Tray": {}}
	var installer []string
	for _, e := range c.Entries {
		if e.Windows == nil {
			continue
		}
		format := windowsFormat(e, false)
		for _, key := range e.Windows.Keys {
			project, name, _ := strings.Cut(key, ".")
			if project == "Setup" {
				installer = append(installer, installerMessages(e, name)...)
				continue
			}
			add := func(language, name, value, comment string) {
				resx[project][language] = append(resx[project][language], resxString{name, format(value), format(comment)})
			}
			if !e.Plural() {
				add("", name, e.Text, e.Note)
			} else {
				add("", name+"_one", e.One, e.Note)
				add("", name+"_other", e.Text, e.Note)
			}
			for _, language := range languages {
				text := e.translation(language.Code, Windows)
				if !e.Plural() {
					add(language.Code, name, text.Plain, "")
					continue
				}
				for _, form := range language.Forms {
					add(language.Code, name+"_"+form, text.Forms[form], "")
				}
			}
		}
	}
	var files []file
	for _, project := range []string{"Kit", "Tray"} {
		for _, language := range append([]string{""}, codes()...) {
			path := fmt.Sprintf("windows/Flock.%s/Strings.resx", project)
			if language != "" {
				path = fmt.Sprintf("windows/Flock.%s/Strings.%s.resx", project, language)
			}
			files = append(files, file{path: path, content: resxFile(resx[project][language])})
		}
	}
	files = append(files, file{path: "windows/installer/Strings.iss", content: []byte(installerFile(installer))})
	return files, nil
}

func codes() []string {
	var codes []string
	for _, language := range languages {
		codes = append(codes, language.Code)
	}
	return codes
}

const generatedNote = "Generated from i18n/ by tools/i18n: edit i18n/*.json, then run go run ./tools/i18n."

func resxFile(strs []resxString) []byte {
	sort.Slice(strs, func(i, j int) bool { return strs[i].name < strs[j].name })
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n")
	b.WriteString("<!-- " + generatedNote + " -->\n")
	b.WriteString(resxHeader)
	for _, s := range strs {
		b.WriteString(`    <data name="` + escapeXML(s.name) + `" xml:space="preserve">` + "\n")
		b.WriteString("        <value>" + escapeXML(s.value) + "</value>\n")
		if s.comment != "" {
			b.WriteString("        <comment>" + escapeXML(s.comment) + "</comment>\n")
		}
		b.WriteString("    </data>\n")
	}
	b.WriteString("</root>\n")
	return []byte(b.String())
}

func escapeXML(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// An installer message in every language: en.Name=…, cs.Name=…
func installerMessages(e *Entry, name string) []string {
	format := windowsFormat(e, true)
	var lines []string
	if e.Note != "" {
		lines = append(lines, "; "+format(e.Note))
	}
	lines = append(lines, "en."+name+"="+format(e.Text))
	for _, language := range languages {
		lines = append(lines, language.Code+"."+name+"="+format(e.translation(language.Code, Windows).Plain))
	}
	return lines
}

func installerFile(lines []string) string {
	return "; " + generatedNote + "\n; Flock.iss includes it; %1 is a message's first argument.\n\n[CustomMessages]\n" +
		strings.Join(lines, "\n") + "\n"
}

// The standard .resx schema and headers, as Visual Studio writes them.
const resxHeader = `<root>
    <xsd:schema id="root" xmlns="" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns:msdata="urn:schemas-microsoft-com:xml-msdata">
        <xsd:element name="root" msdata:IsDataSet="true">
            <xsd:complexType>
                <xsd:choice maxOccurs="unbounded">
                    <xsd:element name="data">
                        <xsd:complexType>
                            <xsd:sequence>
                                <xsd:element name="value" type="xsd:string" minOccurs="0" msdata:Ordinal="1" />
                                <xsd:element name="comment" type="xsd:string" minOccurs="0" msdata:Ordinal="2" />
                            </xsd:sequence>
                            <xsd:attribute name="name" type="xsd:string" msdata:Ordinal="1" />
                            <xsd:attribute name="type" type="xsd:string" msdata:Ordinal="3" />
                            <xsd:attribute name="mimetype" type="xsd:string" msdata:Ordinal="4" />
                        </xsd:complexType>
                    </xsd:element>
                    <xsd:element name="resheader">
                        <xsd:complexType>
                            <xsd:sequence>
                                <xsd:element name="value" type="xsd:string" minOccurs="0" msdata:Ordinal="1" />
                            </xsd:sequence>
                            <xsd:attribute name="name" type="xsd:string" use="required" />
                        </xsd:complexType>
                    </xsd:element>
                </xsd:choice>
            </xsd:complexType>
        </xsd:element>
    </xsd:schema>
    <resheader name="resmimetype">
        <value>text/microsoft-resx</value>
    </resheader>
    <resheader name="version">
        <value>2.0</value>
    </resheader>
    <resheader name="reader">
        <value>System.Resources.ResXResourceReader, System.Windows.Forms, Version=4.0.0.0, Culture=neutral, PublicKeyToken=b77a5c561934e089</value>
    </resheader>
    <resheader name="writer">
        <value>System.Resources.ResXResourceWriter, System.Windows.Forms, Version=4.0.0.0, Culture=neutral, PublicKeyToken=b77a5c561934e089</value>
    </resheader>
`
