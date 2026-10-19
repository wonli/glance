package i18n

import "golang.org/x/text/language"

type Selection string

const (
	SelectionSystem  Selection = "system"
	SelectionChinese Selection = "zh-CN"
	SelectionEnglish Selection = "en-US"
)

type Locale string

const (
	LocaleChinese Locale = "zh-CN"
	LocaleEnglish Locale = "en-US"
)

type Args map[string]any

type Message struct {
	ID   string
	Args Args
}

// NormalizePreference returns the stored selection and whether an invalid
// persisted value should be repaired. An absent value intentionally means
// that the user follows the operating system language.
func NormalizePreference(value string, found bool) (Selection, bool) {
	if !found {
		return SelectionSystem, false
	}
	switch Selection(value) {
	case SelectionSystem, SelectionChinese, SelectionEnglish:
		return Selection(value), false
	default:
		return SelectionEnglish, true
	}
}

func Resolve(selection Selection, preferred []language.Tag) Locale {
	switch selection {
	case SelectionChinese:
		return LocaleChinese
	case SelectionEnglish:
		return LocaleEnglish
	case SelectionSystem:
		for _, tag := range preferred {
			base, _ := tag.Base()
			switch base.String() {
			case "zh":
				return LocaleChinese
			case "en":
				return LocaleEnglish
			}
		}
	}
	return LocaleEnglish
}
