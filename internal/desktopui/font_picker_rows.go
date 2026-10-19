package desktopui

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"

	"glance/internal/fontcatalog"
	"glance/internal/fontdownload"
)

type fontPickerRow struct {
	faceID                                           fontcatalog.ResourceID
	title, family, displayFamily, source, substitute string
	titleMessageID, sourceMessageID                  string
	aliases                                          []string
	missing                                          bool
	online                                           *fontdownload.Entry
}

func fontPickerRows(snapshot fontcatalog.Snapshot, documentNames []string, online []fontdownload.Entry, query string) []fontPickerRow {
	all := snapshot.Families()
	query = strings.ToLower(strings.TrimSpace(query))
	families := make([]fontcatalog.Family, 0, len(all))
	available := map[string]fontcatalog.Family{}
	for _, f := range all {
		available[strings.ToLower(f.Name)] = f
		for _, name := range f.Aliases {
			available[strings.ToLower(name)] = f
		}
		for _, name := range f.Aliases {
			if strings.Contains(strings.ToLower(name), query) {
				families = append(families, f)
				break
			}
		}
	}
	collator := collate.New(language.SimplifiedChinese, collate.IgnoreCase)
	sort.SliceStable(families, func(i, j int) bool {
		a, b := fontPickerLocalizedFamilyName(families[i]), fontPickerLocalizedFamilyName(families[j])
		if order := fontPickerCompareNames(collator, a, b); order != 0 {
			return order < 0
		}
		return strings.ToLower(families[i].Name) < strings.ToLower(families[j].Name)
	})
	var rows []fontPickerRow
	matches := func(name string) bool {
		if strings.Contains(strings.ToLower(name), query) {
			return true
		}
		if f, ok := available[strings.ToLower(name)]; ok {
			for _, alias := range f.Aliases {
				if strings.Contains(strings.ToLower(alias), query) {
					return true
				}
			}
		}
		return false
	}
	names := append([]string(nil), documentNames...)
	sort.SliceStable(names, func(i, j int) bool {
		a, b := names[i], names[j]
		if family, ok := available[strings.ToLower(strings.TrimSpace(a))]; ok {
			a = fontPickerLocalizedFamilyName(family)
		}
		if family, ok := available[strings.ToLower(strings.TrimSpace(b))]; ok {
			b = fontPickerLocalizedFamilyName(family)
		}
		if order := fontPickerCompareNames(collator, a, b); order != 0 {
			return order < 0
		}
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	seen := map[string]bool{}
	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || seen[key] || !matches(name) {
			continue
		}
		seen[key] = true
		if len(rows) == 0 {
			rows = append(rows, fontPickerRow{title: "当前文档使用", titleMessageID: "font.picker.section.document"})
		}
		f, ok := available[key]
		usable := false
		source := "本机"
		sourceID := "font.picker.source.local"
		for _, face := range f.Faces {
			if face.Unavailable == "" {
				usable = true
				if face.Source == fontcatalog.Private {
					source = "已下载"
					sourceID = "font.picker.source.downloaded"
				}
				break
			}
		}
		row := fontPickerRow{family: name, source: source, sourceMessageID: sourceID, missing: !ok || !usable}
		if ok {
			row.displayFamily = fontPickerLocalizedFamilyName(f)
			row.aliases = f.Aliases
		}
		if row.missing {
			row.source = "缺失"
			row.sourceMessageID = "font.picker.source.missing"
			if len(all) > 0 {
				row.substitute = all[0].Name
			}
		}
		rows = append(rows, row)
	}
	if len(families) > 0 {
		rows = append(rows, fontPickerRow{title: "全部本机字体", titleMessageID: "font.picker.section.local"})
		for _, f := range families {
			source := "本机"
			sourceID := "font.picker.source.local"
			usable := false
			for _, face := range f.Faces {
				if face.Unavailable == "" {
					usable = true
					if face.Source == fontcatalog.Private {
						source = "已下载"
						sourceID = "font.picker.source.downloaded"
					}
					break
				}
			}
			groups := map[string]bool{}
			for _, face := range f.Faces {
				group := string(face.Source)
				if face.Source == fontcatalog.Private {
					group += ":" + face.Path
				}
				if groups[group] {
					continue
				}
				groups[group] = true
				best := face
				for _, other := range f.Faces {
					if other.Source == face.Source && (face.Source != fontcatalog.Private || other.Path == face.Path) && fontWeightDistance(other.Weight) < fontWeightDistance(best.Weight) {
						best = other
					}
				}
				label := source
				labelID := sourceID
				if best.Source == fontcatalog.Private {
					label = "已下载"
					labelID = "font.picker.source.downloaded"
				} else {
					label = "本机"
					labelID = "font.picker.source.local"
				}
				missing := best.Unavailable != ""
				if missing {
					label, labelID = "缺失", "font.picker.source.missing"
				}
				rows = append(rows, fontPickerRow{family: f.Name, displayFamily: fontPickerLocalizedFamilyName(f), source: label, sourceMessageID: labelID, aliases: f.Aliases, missing: missing, faceID: best.ID})
			}
			_ = usable
		}
	}
	first := true
	for _, entry := range online {
		if _, ok := available[strings.ToLower(entry.Family)]; ok {
			continue
		}
		if !strings.Contains(strings.ToLower(entry.Family+" "+entry.ID), query) {
			continue
		}
		if first {
			rows = append(rows, fontPickerRow{title: "更多字体", titleMessageID: "font.picker.section.online"})
			first = false
		}
		e := entry
		rows = append(rows, fontPickerRow{family: e.Family, source: "在线", sourceMessageID: "font.picker.source.online", online: &e})
	}
	return rows
}

func fontPickerLocalizedFamilyName(family fontcatalog.Family) string {
	// Keep the canonical name for edits; prefer a Simplified Chinese alias for display.
	name := family.Name
	bestScore := 0
	for _, alias := range family.Aliases {
		if strings.EqualFold(alias, family.Name) || alias == "" {
			continue
		}
		score := 0
		hasHan := false
		for _, r := range alias {
			if unicode.Is(unicode.Han, r) {
				hasHan = true
			}
			if unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
				score -= 100
			}
		}
		if !hasHan {
			continue
		}
		score += 100
		if strings.Contains(alias, "简") {
			score += 50
		}
		if strings.Contains(alias, "繁") {
			score -= 20
		}
		if fields := strings.Fields(alias); len(fields) > 1 {
			last := fields[len(fields)-1]
			if len(last) == 2 && last[0] == 'W' && last[1] >= '0' && last[1] <= '9' {
				score -= 10
			}
		}
		if score > bestScore {
			name, bestScore = alias, score
		}
	}
	return name
}

func (a *App) fontPickerDisplayFamilyName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}

	snapshot := a.fontPicker.snapshot
	if current := fontcatalog.Default(); current.Version > snapshot.Version {
		snapshot = current
	}
	if snapshot.Version == 0 {
		return name
	}

	p := &a.fontPicker
	if p.displayNames == nil || p.displayNamesVersion != snapshot.Version {
		families := snapshot.Families()
		displayNames := make(map[string]string)
		for _, family := range families {
			displayName := fontPickerLocalizedFamilyName(family)
			displayNames[strings.ToLower(family.Name)] = displayName
		}
		for _, family := range families {
			displayName := fontPickerLocalizedFamilyName(family)
			for _, alias := range family.Aliases {
				key := strings.ToLower(alias)
				if _, exists := displayNames[key]; !exists {
					displayNames[key] = displayName
				}
			}
		}
		p.displayNames = displayNames
		p.displayNamesVersion = snapshot.Version
	}
	if displayName, ok := p.displayNames[strings.ToLower(name)]; ok {
		return displayName
	}
	return name
}

func fontPickerCompareNames(collator *collate.Collator, left, right string) int {
	// Put Chinese-labeled fonts first, then use Chinese collation within each group.
	leftHan, rightHan := fontPickerNameHasHan(left), fontPickerNameHasHan(right)
	if leftHan != rightHan {
		if leftHan {
			return -1
		}
		return 1
	}
	return collator.CompareString(left, right)
}

func fontPickerNameHasHan(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func fontWeightDistance(weight int) int {
	d := weight - 400
	if d < 0 {
		return -d
	}
	return d
}
