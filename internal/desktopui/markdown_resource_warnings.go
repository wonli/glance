package desktopui

import (
	"fmt"
	"image"
	"strings"

	"glance/internal/document"
	"glance/internal/format/markdowndoc"
	"glance/internal/i18n"
)

// Count placements, including table cells, while keeping the prompt bounded.
func markdownImageWarnings(snapshot document.MarkdownSnapshot, resources map[string]image.Image, diagnostics map[string]string, localize ...func(i18n.Message) string) (int, string) {
	text := func(message i18n.Message, fallback string) string {
		if len(localize) > 0 && localize[0] != nil {
			return localize[0](message)
		}
		return fallback
	}
	count := 0
	var details []string
	visit := func(spans []document.MarkdownSpan) {
		for _, span := range spans {
			key := markdowndoc.ResourceKey(span)
			if (span.Kind != "image" && span.Kind != "diagram") || resources[key] != nil {
				continue
			}
			count++
			if len(details) >= 5 {
				continue
			}
			label := span.Text
			if label == "" {
				label = text(i18n.Message{ID: "markdown.image.warning.unnamed"}, "未命名图片")
			}
			reason := diagnostics[key]
			if reason == "" {
				reason = text(i18n.Message{ID: "markdown.image.warning.not_loaded"}, "图片未加载")
			}
			short := func(s string) string {
				r := []rune(s)
				if len(r) > 160 {
					return string(r[:160]) + "…"
				}
				return s
			}
			message := i18n.Message{ID: "markdown.image.warning.detail", Args: i18n.Args{"Label": short(label), "Reason": short(reason), "Destination": short(span.Destination)}}
			details = append(details, text(message, fmt.Sprintf("%s：%s\n%s", short(label), short(reason), short(span.Destination))))
		}
	}
	for _, block := range snapshot.Blocks {
		if block.Kind == "diagram" {
			visit([]document.MarkdownSpan{markdowndoc.DiagramSpan(block)})
		}
		visit(block.Spans)
		for _, row := range block.TableSpans {
			for _, cell := range row {
				visit(cell)
			}
		}
	}
	if count > len(details) {
		message := i18n.Message{ID: "markdown.image.warning.more", Args: i18n.Args{"Count": count - len(details)}}
		details = append(details, text(message, fmt.Sprintf("另有 %d 张图片不可用", count-len(details))))
	}
	return count, strings.Join(details, "\n\n")
}
