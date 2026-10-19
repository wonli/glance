# Desktop UI Internationalization Design

## Goal

Provide complete Simplified Chinese and English localization for Glance's app-owned desktop interface, with an optional system-language mode. Users can change the interface language from the existing basic settings page, and the choice persists across launches.

## Decisions and scope

- Supported selections are `system`, `zh-CN`, and `en-US`.
- A fresh installation selects `system`. An unsupported system language, an unreadable system preference, or an invalid persisted value resolves to `en-US`.
- The settings control belongs on the existing basic settings page represented by Pen node `l7WA9J`. Follow that design and add only the supported language setting; do not add unsupported settings as disabled or placeholder controls.
- Translation data is embedded JSON, with `zh-CN.json` and `en-US.json`. English is the complete fallback catalog.
- This feature localizes app-owned desktop UI, including home/workspace navigation, settings, menus, ribbons, tooltips and accessibility labels, dialogs, status/toast feedback, validation, and AI/MCP page chrome across Word, Excel, PowerPoint, Markdown, and JSON workflows.
- Native operating-system dialogs retain their platform-provided language. Document contents, user input, formulas, file paths, stable MCP tool names and protocol fields, and model-generated conversation text are not translated.
- The first release supports left-to-right layouts only.

## Architecture

Add `internal/i18n` as a UI-independent package. It owns locale identifiers, embedded catalogs, message lookup, named parameter interpolation, plural selection, locale matching, and UI number/date formatting. It does not depend on Gio or workspace storage. The package exposes a catalog/resolver that can be constructed with a selected locale and used by layout code without disk or network access.

Embed `internal/i18n/locales/en-US.json` and `internal/i18n/locales/zh-CN.json` with `go:embed`. Message IDs are stable semantic names such as `settings.language.title` and `workspace.save.failed`. Values are complete sentences; dynamic content uses named parameters. Entries may be strings or plural maps. For the supported locales, plural maps provide `one` and `other` forms. JSON parsing and template compilation happen once when the catalog is initialized, never in Gio layout.

The English catalog is the fallback for a missing Simplified Chinese entry. Catalog validation rejects duplicate or malformed IDs, invalid templates, mismatched parameter names, missing English entries, and incomplete plural forms. Unknown IDs in production resolve to the catalog's `common.translation_missing` message, while the missing ID is recorded through the existing diagnostic path. The validator makes an unknown ID a release-blocking defect rather than normal behavior.

Add a small system-locale provider behind a platform-neutral interface. Platform implementations read the user's preferred UI language without network access. Normalize tags with `golang.org/x/text/language`, then match supported locales: Chinese tags resolve to `zh-CN`; English tags resolve to `en-US`; any other tag resolves to `en-US`. The provider is read at startup and when the user chooses `system`; no live OS-language watcher is required.

## Preference and runtime behavior

Store the selection under workspace preference key `ui.language`, reusing the existing preferences table and coalesced background writer. Do not add a schema migration. An absent value means `system`; valid stored values are `system`, `zh-CN`, and `en-US`. Any other stored value resolves to `en-US` and is corrected to `en-US` through the existing preference writer.

The application keeps both the requested selection and the resolved locale. Choosing a language updates the resolved catalog in the UI event path, requests a frame, and queues persistence. Choosing `system` resolves through the platform provider first. If persistence fails, the current process keeps the selected locale and shows a localized error; the prior persisted setting remains available for the next launch.

Changing locale must preserve the active document, dirty state, drafts, editor and list instances, selection, cursor, scroll offsets, open modal identity, and modal draft values. Modal titles and controls re-render in the new locale. Background work publishes a message ID plus typed parameters/result data; the UI resolves it using the current locale when displaying the result, so delayed completions cannot show stale-language UI text.

UI-generated quantities use locale plural rules. UI-generated numeric and date/time labels use locale-aware helpers: `zh-CN` uses `2026-10-18 14:00` style dates and 24-hour time; `en-US` uses `Oct 18, 2026, 2:00 PM` style dates and 12-hour time. Office document values retain their source-format semantics.

## Behavioral separation

Localized text must not control application behavior. Replace comparisons against translated modal titles with stable dialog IDs. Replace status-string parsing for severity, side-effect classification, and toast visibility with explicit message metadata: message ID, named arguments, feedback kind, and notification policy. Keep the message ID and arguments in feedback state until rendering; do not make the translated sentence the source of truth.

Known application errors are mapped to localized message IDs at the operation boundary. Preserve the underlying error for diagnostics and a localized, user-accessible detail view; do not concatenate raw third-party error strings into the primary localized message. User-provided names and paths remain parameter values and are never treated as translation keys.

## UI migration

Migrate all app-owned user-visible strings in `internal/desktopui` by feature area: workspace/home and settings; shared dialogs, menus, and feedback; Word; Excel; PowerPoint; Markdown and JSON; AI/MCP chrome. Each area is complete only when labels, button names, tooltips, empty states, validation, statuses, toasts, and user-visible failures use message IDs in both catalogs. Preserve existing layout and interaction behavior while allowing English text to wrap or truncate according to established UI patterns.

The language selector is the only new settings control in this scope. It offers `Follow system`, `简体中文`, and `English`, with all option labels localized. Its dimensions and placement follow Pen node `l7WA9J`; no other proposed settings are introduced.

## Validation and release criteria

- Catalog checks prove both locales have identical message IDs, all templates parse, named arguments match, and plural entries are complete.
- Locale-resolution checks cover fresh install, each explicit selection, Chinese and English system tags, unsupported tags, provider errors, invalid persisted values, and English fallback.
- Preference checks cover persistence across restart and write failure without blocking Gio layout.
- Interaction checks prove locale switching preserves active editing state and open dialog drafts, and that toast severity, modal actions, and document mutations do not depend on translated wording.
- UI checks cover all feature areas in both locales, long labels and filenames, narrow windows, and 1x/2x rendering. GPU-dependent visual checks that skip are reported as unverified.
- A source/resource audit finds no app-owned hard-coded user-visible strings outside the catalogs, except user/document content, protocol identifiers, and native OS UI.
- Architecture and office-support documentation record the locale boundary and confirm that interface localization does not change document data or save behavior.

The release is complete only when every criterion above passes; a home/settings-only bilingual preview is not a completed release.
