package proofhistory

import (
	"strings"
	"testing"
)

func TestChangedTextLinesOnlyShowsAffectedLines(t *testing.T) {
	for _, tc := range []struct{ before, after, want string }{
		{"第一行\n春眠不觉晓\n最后一行", "第一行\n春眠不觉晓新增\n最后一行", "春眠不觉晓新增\n"},
		{"第一行\n春眠不觉晓\n最后一行", "第一行\n春眠觉晓\n最后一行", "春眠不觉晓\n"},
		{"不变一\n修改一\n中间不变\n修改二\n不变二", "不变一\n修改一甲\n中间不变\n修改二乙\n不变二", "修改一甲\n…\n修改二乙\n"},
		{"相同\n正文", "相同\n正文", ""},
		{"前\n后", "前\n新增一行\n后", "新增一行\n"},
		{"前\n删除一行\n后", "前\n后", "删除一行\n"},
		{"旧行", "新行", "旧新行"},
	} {
		lines := ChangedTextLines(CompareText(tc.before, tc.after))
		var text strings.Builder
		for _, span := range lines {
			text.WriteString(span.Text)
		}
		if text.String() != tc.want {
			t.Fatalf("%q -> %q: display=%q want=%q", tc.before, tc.after, text.String(), tc.want)
		}
	}
}
