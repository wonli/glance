package desktopui

import (
	"fmt"
	"image"
	"strings"

	"glance/internal/document"
	"glance/internal/format/markdowndoc"
)

// Count placements, including table cells, while keeping the prompt bounded.
func markdownImageWarnings(snapshot document.MarkdownSnapshot, resources map[string]image.Image, diagnostics map[string]string) (int, string) {
	count := 0
	var details []string
	visit := func(spans []document.MarkdownSpan) {
		for _, span := range spans {
			key := markdowndoc.ResourceKey(span)
			if span.Kind != "image" || resources[key] != nil {
				continue
			}
			count++
			if len(details) >= 5 {
				continue
			}
			label := span.Text
			if label == "" {
				label = "未命名图片"
			}
			reason := diagnostics[key]
			if reason == "" {
				reason = "图片未加载"
			}
			short := func(s string) string {
				r := []rune(s)
				if len(r) > 160 {
					return string(r[:160]) + "…"
				}
				return s
			}
			details = append(details, fmt.Sprintf("%s：%s\n%s", short(label), short(reason), short(span.Destination)))
		}
	}
	for _, block := range snapshot.Blocks {
		visit(block.Spans)
		for _, row := range block.TableSpans {
			for _, cell := range row {
				visit(cell)
			}
		}
	}
	if count > len(details) {
		details = append(details, fmt.Sprintf("另有 %d 张图片不可用", count-len(details)))
	}
	return count, strings.Join(details, "\n\n")
}
