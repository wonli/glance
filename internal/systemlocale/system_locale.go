package systemlocale

import (
	"strings"

	"golang.org/x/text/language"
)

func parsePreferredLanguages(values []string) []language.Tag {
	tags := make([]language.Tag, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if end := strings.IndexAny(value, ".@"); end >= 0 {
			value = value[:end]
		}
		if strings.EqualFold(value, "C") || strings.EqualFold(value, "POSIX") {
			continue
		}
		value = strings.ReplaceAll(value, "_", "-")
		tag, err := language.Parse(value)
		if err != nil {
			continue
		}
		base, _ := tag.Base()
		if base.String() == "und" {
			continue
		}
		tags = append(tags, tag)
	}
	return tags
}

func preferredLanguagesFromEnvironment(getenv func(string) (string, bool)) []language.Tag {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		value, exists := getenv(key)
		if !exists || strings.TrimSpace(value) == "" {
			continue
		}
		return parsePreferredLanguages([]string{value})
	}
	return nil
}
