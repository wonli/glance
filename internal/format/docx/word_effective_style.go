package docx

import (
	"glance/internal/document"
	"glance/internal/wordlayout"
)

// Caller holds d.mu; resolving inheritance reads the cached neutral stylesheet.
func (d *Document) effectiveWordTargetStyle(target wordTextTarget) document.WordTextStyle {
	properties := firstChild(target.run, "rPr")
	paragraph := document.WordParagraph{ID: target.paragraphID, Style: attrValue(firstChild(firstChild(target.paragraph, "pPr"), "pStyle"), "val")}
	run := document.WordTextRun{Style: parseWordTextStyle(properties), StyleSet: parseWordTextStyleSet(properties), CharacterStyle: attrValue(firstChild(properties, "rStyle"), "val")}
	return wordlayout.ResolveTextRunStyle(d.wordLayoutData, paragraph, run)
}
