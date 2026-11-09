# Translations

The macOS, Windows and Linux apps share one translation of each text. The catalogs here are the
source: `en.json` lists every English text and which apps use it, and `cs.json` (Czech) and
`es.json` (Spanish) translate them. `tools/i18n` generates each app's own files from them:

| App | Generated files |
|---|---|
| macOS | `macos/Resources/Localizable.xcstrings` |
| Windows | `windows/Flock.Kit/Strings*.resx`, `windows/Flock.Tray/Strings*.resx`, `windows/installer/Strings.iss` |
| Linux | `linux/po/cs.po`, `linux/po/es.po` |

Don't edit those files. Edit the catalogs instead, then regenerate:

```bash
go run ./tools/i18n
```

The tool also sorts the catalogs, so a new text can go anywhere in them. CI runs
`go run ./tools/i18n -check`, which fails when a catalog has a problem (a missing translation or
plural form, a placeholder a translation drops) or a generated file is out of date. Each app's
tests then check that its code and its generated files agree: `macos/check-strings.sh`,
`ResourcesTests` in `windows/Flock.Kit.Tests`, and `cargo test` in `linux/`.

## en.json

Each key is a text's ID: the English text itself, with `{name}` placeholders. A ` [context]`
suffix tells apart texts that are the same in English but not in other languages, such as
`"Update [noun]"`. The value says where the text is used:

```json
"Update to {version}": {"note": "{version}: a PHP version", "macos": true, "windows": "Tray.General_UpdateTo", "linux": true},
"{n} sites": {"one": "{n} site", "macos": "%lld sites", "windows": "Kit.Sites", "linux": true},
```

- `one`: the English singular of a plural text. The ID is the plural.
- `note`: a hint for translators: what a placeholder stands for, or where the text shows.
- `macos`: the text's key in Localizable.xcstrings, the format string Swift makes of the
  interpolated text (`Text("Update to \(version)")` is `"Update to %@"`). `true` stands for the
  text with every placeholder as `%@`. Give the key when it differs: a number (`%lld`), a plural,
  or a key that isn't the English (`String(localized: "Database category", defaultValue: "Database")`).
- `windows`: the resource name, with the project first: `Kit.` and `Tray.` for Strings.resx, and
  `Setup.` for the installer's `[CustomMessages]`. It can also be a list of names, or an object:
  - `keys`: the names;
  - `args`: the placeholder at each `String.Format` index, when that isn't the order the text
    has them in;
  - `formats`: a placeholder's format, e.g. `{"size": "N0"}` for `{0:N0}`.

  A plural becomes `Name_one` and `Name_other`, plus `Name_few` in Czech.
- `linux`: `true` when `linux/src` uses it (`tr!`, `trn!`, and `trc!` with the context).
- `"translate": false`: every language shows the English, for a format like `"%@ (%@)"` or a
  product name that macOS's catalog lists all the same.

## cs.json, es.json

Each text's translation, under its ID: a string, or a plural's forms. Czech has `one` (1), `few`
(2–4) and `other` (0, 5 and more), and optionally `many`, for fractions, which only macOS uses.
Spanish has `one` and `other`. A translation keeps the English placeholders. A singular may leave
out the count (`"Show its worktree"`).

```json
"{n} sites": {"one": "{n} web", "few": "{n} weby", "other": "{n} webů"},
"Settings": {"macos": "Ajustes", "default": "Configuración"},
```

An app can have its own translation when the shared one would be wrong there. Spanish macOS says
"Ajustes" where Windows and Linux say "Configuración", for example, and a text can serve as a
noun in one app and a verb in another. Use this only for such cases: the point of the shared
catalog is that the apps say the same thing the same way.

## Adding a language

1. Add the language to `languages` in `tools/i18n/catalog.go`, with its plural forms.
2. Add its catalog, e.g. `de.json`.
3. Add it to each app's languages: `AppLanguage.swift` and the `.lproj` list in `macos/build-app.sh`,
   `Localization.cs`, and `i18n.rs` with its catalog and plural rule.
4. Add it to `[Languages]` in `windows/installer/Flock.iss`.
