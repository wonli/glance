package desktopui

import (
	"fmt"
	"strconv"
	"strings"

	"glance/internal/document"
	"glance/internal/i18n"
)

func wordNavigationCachedEntries(state *documentSessionState, blocks []document.WordBlock, query string) ([]wordNavigationEntry, int, int) {
	if state != nil {
		cache := state.wordNavigationEntryCache
		draftEpoch := state.wordDraftEpoch.Load()
		if cache.valid && cache.revision == state.wordRevisionAtomic.Load() && cache.draftEpoch == draftEpoch && cache.query == query {
			return cache.entries, cache.headingCount, cache.resultCount
		}
	}
	drafts := wordNavigationDrafts(state)
	styles := []document.WordStyle(nil)
	if state != nil && state.word != nil {
		if data, ok := wordLayoutDataForState(state); ok {
			styles = data.Styles
		}
	}
	entries, headingCount, resultCount := wordNavigationEntries(blocks, styles, query, drafts)
	if state != nil {
		state.wordNavigationEntryCache = wordNavigationEntryCache{
			valid: true, revision: state.wordRevisionAtomic.Load(), draftEpoch: state.wordDraftEpoch.Load(), query: query,
			entries: entries, headingCount: headingCount, resultCount: resultCount,
		}
	}
	return entries, headingCount, resultCount
}

func wordNavigationDrafts(state *documentSessionState) map[string]string {
	if state == nil {
		return nil
	}
	return state.wordDraftSnapshot()
}

func wordNavigationEntries(blocks []document.WordBlock, styles []document.WordStyle, query string, drafts map[string]string) ([]wordNavigationEntry, int, int) {
	paragraphs := wordNavigationParagraphRefs(blocks)
	styleParents := wordNavigationStyleParents(styles)
	entries := []wordNavigationEntry{{kind: wordNavigationGroup, key: "group:headings", label: "文档标题", labelMessageID: "word.navigation.group.headings"}}
	headingCount := 0
	for _, ref := range paragraphs {
		paragraph := ref.paragraph
		level := wordNavigationHeadingLevelFromParents(paragraph.Style, styleParents)
		if level == 0 {
			continue
		}
		label := strings.TrimSpace(wordNavigationParagraphText(*paragraph, drafts))
		if label == "" {
			continue
		}
		runID := wordNavigationFirstEditableRun(*paragraph)
		entries = append(entries, wordNavigationEntry{
			kind: wordNavigationHeading, key: "heading:" + paragraph.ID, label: wordNavigationShortLabel(label), level: level,
			blockIndex: ref.blockIndex, paragraphID: paragraph.ID, runID: runID, anchorRunID: wordNavigationFirstTextRun(*paragraph),
		})
		headingCount++
	}
	if headingCount == 0 {
		entries = append(entries, wordNavigationEntry{kind: wordNavigationGroup, key: "empty:headings", label: "未发现标题", labelMessageID: "word.navigation.empty.headings"})
	}
	resultCount := 0
	if query != "" {
		resultGroupIndex := len(entries)
		entries = append(entries, wordNavigationEntry{kind: wordNavigationGroup, key: "group:results", label: "搜索结果", labelMessageID: "word.navigation.group.results"})
		needle := lowerWordRunes([]rune(query))
		for _, ref := range paragraphs {
			paragraph := ref.paragraph
			for _, inline := range paragraph.Inlines {
				if inline.Text == nil || !inline.Text.Editable {
					continue
				}
				run := inline.Text
				value := run.Text
				if draft, ok := drafts[run.ID]; ok {
					value = draft
				}
				runes := []rune(value)
				lowered := lowerWordRunes(runes)
				for start := 0; start+len(needle) <= len(runes); {
					found := findWordRunes(lowered, needle, start)
					if found < 0 {
						break
					}
					end := found + len(needle)
					resultCount++
					if resultCount <= wordNavigationResultLimit {
						entries = append(entries, wordNavigationEntry{
							kind: wordNavigationResult, key: fmt.Sprintf("result:%s:%s:%d", paragraph.ID, run.ID, found),
							label: wordNavigationMatchSnippet(runes, found, end), blockIndex: ref.blockIndex,
							paragraphID: paragraph.ID, runID: run.ID, start: found, end: end,
						})
					}
					start = end
				}
			}
		}
		if resultCount == 0 {
			entries = append(entries, wordNavigationEntry{kind: wordNavigationGroup, key: "empty:results", label: "没有搜索结果", labelMessageID: "word.navigation.empty.results"})
		} else if resultCount > wordNavigationResultLimit {
			entries[resultGroupIndex].label = fmt.Sprintf("搜索结果（显示前 %d 个，共 %d 个）", wordNavigationResultLimit, resultCount)
			entries[resultGroupIndex].labelMessageID = "word.navigation.group.results_limited"
			entries[resultGroupIndex].labelArgs = i18n.Args{"Shown": wordNavigationResultLimit, "Total": resultCount}
		}
	}
	return entries, headingCount, resultCount
}

func wordNavigationParagraphRefs(blocks []document.WordBlock) []wordNavigationParagraphRef {
	var paragraphs []wordNavigationParagraphRef
	for blockIndex := range blocks {
		block := &blocks[blockIndex]
		if block.Paragraph != nil {
			paragraphs = append(paragraphs, wordNavigationParagraphRef{paragraph: block.Paragraph, blockIndex: blockIndex})
		}
		if block.Table == nil {
			continue
		}
		for rowIndex := range block.Table.Rows {
			for cellIndex := range block.Table.Rows[rowIndex].Cells {
				cell := &block.Table.Rows[rowIndex].Cells[cellIndex]
				for paragraphIndex := range cell.Paragraphs {
					paragraphs = append(paragraphs, wordNavigationParagraphRef{paragraph: &cell.Paragraphs[paragraphIndex], blockIndex: blockIndex})
				}
			}
		}
	}
	return paragraphs
}

func wordNavigationHeadingLevel(styleID string, styles []document.WordStyle) int {
	return wordNavigationHeadingLevelFromParents(styleID, wordNavigationStyleParents(styles))
}

func wordNavigationStyleParents(styles []document.WordStyle) map[string]string {
	styleByID := make(map[string]string, len(styles))
	for _, style := range styles {
		styleByID[wordNavigationNormalizeStyleID(style.ID)] = style.BasedOn
	}
	return styleByID
}

func wordNavigationHeadingLevelFromParents(styleID string, styleByID map[string]string) int {
	seen := make(map[string]struct{}, len(styleByID)+1)
	for styleID != "" {
		normalized := wordNavigationNormalizeStyleID(styleID)
		if level := wordNavigationBuiltinHeadingLevel(normalized); level > 0 {
			return level
		}
		if _, duplicate := seen[normalized]; duplicate {
			return 0
		}
		seen[normalized] = struct{}{}
		basedOn, ok := styleByID[normalized]
		if !ok {
			return 0
		}
		styleID = basedOn
	}
	return 0
}

func wordNavigationNormalizeStyleID(styleID string) string {
	return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(styleID)))
}

func wordNavigationBuiltinHeadingLevel(styleID string) int {
	if !strings.HasPrefix(styleID, "heading") {
		return 0
	}
	level, err := strconv.Atoi(strings.TrimPrefix(styleID, "heading"))
	if err != nil || level < 1 || level > 9 {
		return 0
	}
	return level
}

func wordNavigationParagraphText(paragraph document.WordParagraph, drafts map[string]string) string {
	var text strings.Builder
	for _, inline := range paragraph.Inlines {
		if inline.Text == nil {
			continue
		}
		value := inline.Text.Text
		if draft, ok := drafts[inline.Text.ID]; ok {
			value = draft
		}
		text.WriteString(value)
	}
	return text.String()
}

func wordNavigationFirstEditableRun(paragraph document.WordParagraph) string {
	for _, inline := range paragraph.Inlines {
		if inline.Text != nil && inline.Text.Editable {
			return inline.Text.ID
		}
	}
	return ""
}

func wordNavigationFirstTextRun(paragraph document.WordParagraph) string {
	for _, inline := range paragraph.Inlines {
		if inline.Text != nil {
			return inline.Text.ID
		}
	}
	return ""
}

func wordNavigationShortLabel(value string) string {
	runes := []rune(value)
	if len(runes) <= 64 {
		return value
	}
	return string(runes[:61]) + "…"
}

func wordNavigationMatchSnippet(runes []rune, start, end int) string {
	left, right := max(0, start-26), min(len(runes), end+26)
	value := string(runes[left:right])
	if left > 0 {
		value = "…" + value
	}
	if right < len(runes) {
		value += "…"
	}
	return value
}
