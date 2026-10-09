package desktopui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"

	"strings"
	"testing"

	"glance/internal/document"
	"glance/internal/format/docx"
)

func hasWordAction(actions []ribbonAction, id string) bool {
	for _, action := range actions {
		if action.id == id {
			return true
		}
	}
	return false
}

func TestWordToolbarTabsActionsAndIcons(t *testing.T) {
	state := newDocumentSessionState("sample.docx")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(false)
	state.word = word
	if tabs := wordToolbarTabs(state); len(tabs) != 7 {
		t.Fatalf("Word tabs without an image selection = %#v; want the 7 Pen categories", tabs)
	}
	state.selectedWordImageID = "img-7"
	if tabs := wordToolbarTabs(state); len(tabs) != 8 || tabs[7] != wordTabPicture {
		t.Fatalf("Word tabs with an image selection = %#v; want contextual Picture tab", tabs)
	}

	want := map[string]bool{
		"undo": true, "redo": true, "cut": true, "copy": true, "paste": true,
		"bold": true, "italic": true, "underline": true, "font-color": true,
		"align-left": true, "align-center": true, "align-right": true, "align-justify": true,
	}
	seen := make(map[string]bool)
	for _, action := range wordToolbarActions(wordTabHome) {
		seen[action.id] = true
		if action.label == "" || action.icon == "" {
			t.Errorf("Word action %q is missing its accessible label or icon", action.id)
		}
		if _, ok := toolbarIconData(action.icon); !ok {
			t.Errorf("Word action %q references missing icon %q", action.id, action.icon)
		}
		if action.group == "单元格" || action.group == "工作表" || strings.Contains(action.id, "row-") || strings.Contains(action.id, "sheet-") {
			t.Errorf("spreadsheet-only action leaked into Word Home: %#v", action)
		}
	}
	for _, action := range wordHistoryActions {
		seen[action.id] = true
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("Word Home is missing action %q", id)
		}
	}
	if got := wordToolbarActions(wordTabInsert); len(got) != 16 || !hasWordAction(got, "insert-image") {
		t.Fatalf("Word Insert actions = %#v; want Pen insert actions including image insertion", got)
	}
	if got := wordToolbarActions(wordTabFile); len(got) != 12 || !hasWordAction(got, "save") {
		t.Fatalf("Word File actions = %#v; want Pen file actions including Save", got)
	}
	if got := wordToolbarActions(wordTabPicture); len(got) != 5 || got[0].id != "replace-image" || got[1].id != "delete-image" || got[2].id != "resize-image" || got[3].id != "rotate-left" || got[4].id != "rotate-right" {
		t.Fatalf("Word Picture actions = %#v; want replace, delete, resize and 90° rotation", got)
	}
}

func TestWordToolbarExposesPenReferenceCategories(t *testing.T) {
	state := newDocumentSessionState("sample.docx")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state.word = word

	if got := len(wordToolbarTabs(state)); got != 7 {
		t.Fatalf("Word category count = %d; want the 7 categories in Pen node kiBim", got)
	}
	if got := len(wordToolbarTabLabels); got != 8 {
		t.Fatalf("Word category labels = %d; want 7 reference labels and contextual Picture", got)
	}
	want := "文件,开始,插入,设计,布局,审阅,视图"
	if got := strings.Join(wordToolbarTabLabels, ","); got != want+",图片" {
		t.Fatalf("Word category labels = %q; want %q plus contextual Picture", got, want)
	}
	wantActions := map[wordToolbarTab]string{
		wordTabFile:   "新建,打开,最近使用,保存,另存为,自动保存,打印,导出 PDF,共享,文档信息,保护文档,版本历史",
		wordTabHome:   "粘贴,剪切,复制,格式刷,字体,减小字号,增大字号,加粗,斜体,下划线,高亮,字体颜色,项目符号,编号,左对齐,居中,右对齐,两端对齐,减少缩进,增加缩进,查找,替换,选择,编辑器,听写,加载项,设计器",
		wordTabInsert: "封面,空白页,分页,表格,图片,形状,图表,链接,书签,页眉,页脚,页码,文本框,艺术字,公式,符号",
		wordTabDesign: "主题,颜色,字体,效果,紧凑,宽松,水印,页面颜色,页面边框",
		wordTabLayout: "页边距,方向,纸张大小,分栏,分隔符,行号,缩进,段前,段后,位置,环绕文字,对齐",
		wordTabReview: "编辑器,字数统计,翻译,语言,新建批注,删除批注,修订,接受,拒绝,比较,限制编辑",
		wordTabView:   "阅读模式,打印布局,Web版式,标尺,网格线,导航窗格,缩放,单页,多页,新建窗口,并排查看,宏",
	}
	for tab, want := range wantActions {
		var labels []string
		for _, action := range wordToolbarActions(tab) {
			labels = append(labels, action.label)
		}
		if got := strings.Join(labels, ","); got != want {
			t.Errorf("Word tab %q actions = %q; want Pen order %q", wordToolbarTabLabels[tab], got, want)
		}
	}
	for tab := wordTabFile; tab <= wordTabPicture; tab++ {
		for _, action := range wordToolbarActions(tab) {
			if action.label == "" || action.icon == "" {
				t.Errorf("Word tab %q has an action without a label or icon: %#v", wordToolbarTabLabels[tab], action)
			}
			for _, enabled := range []bool{true, false} {
				if icon, ok := wordRibbonIcon(action.icon, enabled); !ok || icon == nil {
					t.Errorf("Word tab %q action %q references an icon outside the shared Excel set: %q", wordToolbarTabLabels[tab], action.id, action.icon)
				}
			}
		}
	}
}

func TestWordRibbonUsesSemanticallyClosestExcelGlyphs(t *testing.T) {
	cases := []struct {
		tab  wordToolbarTab
		id   string
		want string
	}{
		{tab: wordTabHome, id: "font-family", want: "type"},
		{tab: wordTabFile, id: "share", want: "copy"},
		{tab: wordTabFile, id: "doc-info", want: "file-text"},
	}
	for _, test := range cases {
		action, found := ribbonAction{}, false
		for _, candidate := range wordToolbarActions(test.tab) {
			if candidate.id == test.id {
				action, found = candidate, true
				break
			}
		}
		if !found {
			t.Fatalf("Word action %q was not found in tab %q", test.id, wordToolbarTabLabels[test.tab])
		}
		if got := wordExcelIconName(action.icon); got != test.want {
			t.Errorf("Word action %q uses Excel glyph %q; want semantic match %q", test.id, got, test.want)
		}
	}
}

func TestWordRibbonActionSlotsMatchPenNodeClpgm(t *testing.T) {
	want := map[wordToolbarTab][]int{
		wordTabFile:    {34, 84, 134, 203, 253, 303, 372, 422, 472, 541, 591, 641},
		wordTabHome:    {34, 84, 134, 184, 253, 303, 353, 403, 453, 503, 553, 603, 672, 722, 772, 822, 872, 922, 972, 1022, 1091, 1141, 1191, 1260, 1310, 1360, 1410},
		wordTabInsert:  {34, 84, 134, 203, 272, 322, 372, 441, 491, 560, 610, 660, 729, 779, 848, 898},
		wordTabDesign:  {34, 84, 134, 184, 253, 303, 372, 422, 472},
		wordTabLayout:  {34, 84, 134, 184, 253, 303, 372, 422, 472, 541, 591, 641},
		wordTabReview:  {34, 84, 153, 203, 272, 322, 391, 441, 491, 560, 610},
		wordTabView:    {34, 84, 134, 203, 253, 303, 372, 422, 472, 541, 591, 660},
		wordTabPicture: {34, 84, 134, 184, 234},
	}
	for tab, expected := range want {
		got := wordPenActionPositions[tab]
		if len(got) != len(expected) {
			t.Fatalf("Pen ribbon tab %q has %d positions, want %d", wordToolbarTabLabels[tab], len(got), len(expected))
		}
		for index, position := range expected {
			if got[index] != position {
				t.Errorf("Pen ribbon tab %q action slot %d x=%d, want %d dp", wordToolbarTabLabels[tab], index, got[index], position)
			}
		}
	}
	wantDividers := map[wordToolbarTab][]int{
		wordTabFile:   {190, 359, 528},
		wordTabHome:   {240, 659, 1078, 1247},
		wordTabInsert: {190, 259, 428, 547, 716, 835},
		wordTabDesign: {240, 359},
		wordTabLayout: {240, 359, 528},
		wordTabReview: {140, 259, 378, 547},
		wordTabView:   {190, 359, 528, 647},
	}
	for tab, expected := range wantDividers {
		got := wordPenGroupDividers[tab]
		if len(got) != len(expected) {
			t.Fatalf("Pen ribbon tab %q has %d group dividers, want %d", wordToolbarTabLabels[tab], len(got), len(expected))
		}
		for index, position := range expected {
			if got[index] != position {
				t.Errorf("Pen ribbon tab %q group divider %d x=%d, want %d dp", wordToolbarTabLabels[tab], index, got[index], position)
			}
		}
	}
}

func TestWordToolbarEnablesCommandsForTheirSelectedWordObjects(t *testing.T) {
	state := newDocumentSessionState("sample.docx")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	word.SetDirty(false)
	state.word = word
	app := &App{documentSessionState: state}
	findAction := func(tab wordToolbarTab, id string) wordToolbarAction {
		t.Helper()
		if tab == wordTabHome && (id == "undo" || id == "redo") {
			for _, action := range wordHistoryActions {
				if action.id == id {
					return action
				}
			}
		}
		for _, action := range wordToolbarActions(tab) {
			if action.id == id {
				return action
			}
		}
		t.Fatalf("Word toolbar action %q not found", id)
		return wordToolbarAction{}
	}
	for _, id := range []string{"bold", "align-center", "copy", "paste", "undo"} {
		if app.wordToolbarActionEnabled(findAction(wordTabHome, id)) {
			t.Errorf("Word action %q should be disabled without its required selection/history", id)
		}
	}
	if app.wordToolbarActionEnabled(findAction(wordTabInsert, "insert-image")) {
		t.Fatal("image insertion should be disabled without a selected paragraph")
	}
	blocks := word.WordBlocks()
	paragraph := blocks[0].Paragraph
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = paragraph.Inlines[0].Text.ID
	for _, id := range []string{"bold", "align-center", "copy", "paste"} {
		if !app.wordToolbarActionEnabled(findAction(wordTabHome, id)) {
			t.Errorf("Word action %q should be enabled for selected editable text", id)
		}
	}
	if !app.wordToolbarActionEnabled(findAction(wordTabInsert, "insert-image")) {
		t.Fatal("image insertion should be enabled for a selected paragraph")
	}
	if app.wordToolbarActionEnabled(findAction(wordTabFile, "save")) {
		t.Fatal("save should remain disabled for a clean document")
	}
	if err := word.ReplaceTextRun(state.selectedWordRunID, "Edited"); err != nil {
		t.Fatal(err)
	}
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	if !app.wordToolbarActionEnabled(findAction(wordTabFile, "save")) || !app.wordToolbarActionEnabled(findAction(wordTabHome, "undo")) {
		t.Fatal("dirty Word document should enable save and undo")
	}
	state.selectedWordRunID = ""
	if app.wordToolbarActionEnabled(findAction(wordTabHome, "bold")) || app.wordToolbarActionEnabled(findAction(wordTabHome, "copy")) {
		t.Fatal("run-only Word commands should disable when no text run is selected")
	}

	imageID, err := word.InsertInlineImage(paragraph.ID, "", "image/png", tinyToolbarPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordImageID = imageID
	for _, id := range []string{"replace-image", "delete-image", "resize-image", "rotate-left", "rotate-right"} {
		if !app.wordToolbarActionEnabled(findAction(wordTabPicture, id)) {
			t.Errorf("Picture action %q should be enabled for a selected inline image", id)
		}
	}
	state.selectedWordImageID = ""
	for _, action := range wordToolbarActions(wordTabPicture) {
		if app.wordToolbarActionEnabled(action) {
			t.Errorf("Picture action %q should disable without an image selection", action.id)
		}
	}
}

func findWordTestAction(t *testing.T, id string) ribbonAction {
	t.Helper()
	for _, action := range wordToolbarActions(wordTabHome) {
		if action.id == id {
			return action
		}
	}
	t.Fatalf("Word Home action %q not found", id)
	return ribbonAction{}
}

func tinyToolbarPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 20, G: 80, B: 180, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

var _ document.WordHistory = (*docx.Document)(nil)
