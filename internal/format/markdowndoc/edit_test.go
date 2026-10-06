package markdowndoc

import "testing"

func TestMarkdownLineEditPreservesBOM(t *testing.T) {
	for _, tc := range []struct{ action, want string }{{"heading", "\ufeff## 中文\r\n"}, {"quote", "\ufeff> # 中文\r\n"}, {"list", "\ufeff- # 中文\r\n"}, {"todo", "\ufeff- [ ] # 中文\r\n"}} {
		out, err := ApplyEdit("\ufeff# 中文\r\n", tc.action, 3, 5, EditArgs{})
		if err != nil || out.Text != tc.want {
			t.Fatalf("%s: %q %v", tc.action, out.Text, err)
		}
	}
}

func TestMarkdownToolbarEdits(t *testing.T) {
	cases := []struct {
		id, source, want string
		start, end       int
		args             EditArgs
	}{
		{"bold", "中文", "**中文**", 0, 2, EditArgs{}},
		{"italic", "甲中文乙", "甲*中文*乙", 1, 3, EditArgs{}},
		{"heading", "中文\n正文", "# 中文\n正文", 0, 2, EditArgs{}},
		{"quote", "甲\n乙", "> 甲\n> 乙", 0, 3, EditArgs{}},
		{"code", "中文", "`中文`", 0, 2, EditArgs{}},
		{"code", "甲\n乙", "```\n甲\n乙\n```", 0, 3, EditArgs{}},
		{"link", "中文", "[中文](https://example.com)", 0, 2, EditArgs{Destination: "https://example.com"}},
		{"image", "", "![图](./a.png)", 0, 0, EditArgs{Label: "图", Destination: "./a.png"}},
		{"list", "甲\n乙", "- 甲\n- 乙", 0, 3, EditArgs{}},
		{"todo", "甲", "- [ ] 甲", 0, 1, EditArgs{}},
		{"math", "x^2", "$x^2$", 0, 3, EditArgs{}},
		{"math", "x^2", "$$\nx^2\n$$", 0, 3, EditArgs{Display: true}},
	}
	for _, c := range cases {
		t.Run(c.id+c.source, func(t *testing.T) {
			out, err := ApplyEdit(c.source, c.id, c.start, c.end, c.args)
			if err != nil {
				t.Fatal(err)
			}
			if out.Text != c.want {
				t.Fatalf("got %q want %q", out.Text, c.want)
			}
			if out.Start < 0 || out.End < out.Start || out.End > len([]rune(out.Text)) {
				t.Fatal("invalid selection")
			}
		})
	}
	out, err := ApplyEdit("", "bold", 0, 0, EditArgs{})
	if err != nil || out.Text != "****" || out.Start != 2 || out.End != 2 {
		t.Fatalf("empty insertion %+v %v", out, err)
	}
}

func TestMarkdownTodoToolbarTogglesExistingMarkers(t *testing.T) {
	for _, tc := range []struct{ source, want string }{{"- [ ] 中文", "- [x] 中文"}, {"- [X] 中文", "- [ ] 中文"}, {"  - [x] 子项", "  - [ ] 子项"}, {"> - [ ] 引用", "> - [x] 引用"}, {"1. [ ] 有序", "1. [x] 有序"}} {
		out, err := ApplyEdit(tc.source, "todo", len([]rune(tc.source)), len([]rune(tc.source)), EditArgs{})
		if err != nil || out.Text != tc.want {
			t.Errorf("todo %q got %q err=%v want %q", tc.source, out.Text, err, tc.want)
		}
	}
}
