package desktopui

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestWordFontSizeActionsRequireAndUseSelectedEditableRun(t *testing.T) {
	state := newDocumentSessionState("sample.docx")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state.word = word
	app := &App{documentSessionState: state}
	findAction := func(id string) wordToolbarAction {
		t.Helper()
		for _, action := range wordToolbarActions(wordTabHome) {
			if action.id == id {
				return action
			}
		}
		t.Fatalf("Word Home action %q not found", id)
		return wordToolbarAction{}
	}
	for _, id := range []string{"font-size-decrease", "font-size-increase"} {
		if app.wordToolbarActionEnabled(findAction(id)) {
			t.Errorf("Word font-size action %q should be disabled without a selected run", id)
		}
	}
	paragraph := word.WordBlocks()[0].Paragraph
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = paragraph.Inlines[0].Text.ID
	for _, id := range []string{"font-size-decrease", "font-size-increase"} {
		if !app.wordToolbarActionEnabled(findAction(id)) {
			t.Errorf("Word font-size action %q should be enabled for a selected editable run", id)
		}
	}
}

func TestWordFontSizeToolbarActionsChangeSelectedRunByOnePoint(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	runID := paragraph.Inlines[0].Text.ID
	state := newDocumentSessionState("font-size.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = runID
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	for _, step := range []struct {
		action string
		want   float64
	}{
		{action: "font-size-increase", want: 12},
		{action: "font-size-decrease", want: 11},
	} {
		app.executeWordToolbarAction(step.action)
		if !state.formatRunning.Load() {
			t.Fatalf("%s did not start a Word formatting command", step.action)
		}
		select {
		case result := <-app.wordCommandResults:
			if result.err != nil {
				t.Fatalf("%s: %v", step.action, result.err)
			}
			app.wordCommandResults <- result
			app.processAsyncResults()
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %s", step.action)
		}
		run, ok := selectedWordRun(state.wordBlocks, runID)
		if !ok || run.Style.FontSize != step.want {
			t.Fatalf("%s font size = %g, found=%v; want %g", step.action, run.Style.FontSize, ok, step.want)
		}
	}
}

func TestWordListToolbarActionsApplyAndToggleOffLists(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	state := newDocumentSessionState("word-list.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = paragraph.ID
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	for _, step := range []struct {
		action string
		kind   document.WordListKind
		marker string
	}{
		{action: "bullet-list", kind: document.WordListBullet, marker: "•"},
		{action: "numbered-list", kind: document.WordListNumbered, marker: "1."},
	} {
		action := findWordTestAction(t, step.action)
		if !app.wordToolbarActionEnabled(action) {
			t.Fatalf("Word list action %q should be enabled for a selected paragraph", step.action)
		}
		app.executeWordToolbarAction(step.action)
		if !state.formatRunning.Load() {
			t.Fatalf("%s did not start a Word list command", step.action)
		}
		select {
		case result := <-app.wordCommandResults:
			if result.err != nil {
				t.Fatalf("%s: %v", step.action, result.err)
			}
			app.wordCommandResults <- result
			app.processAsyncResults()
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %s", step.action)
		}
		updated, ok := selectedWordParagraph(state.wordBlocks, paragraph.ID)
		if !ok || updated.ListKind != step.kind || updated.ListMarker != step.marker {
			t.Fatalf("%s paragraph list = %#v, found=%v; want %q marker %q", step.action, updated, ok, step.kind, step.marker)
		}
		if !wordToolbarActionActive(state, action) {
			t.Errorf("%s is not shown as active after applying the list", step.action)
		}
		app.executeWordToolbarAction(step.action)
		select {
		case result := <-app.wordCommandResults:
			if result.err != nil {
				t.Fatalf("toggle off %s: %v", step.action, result.err)
			}
			app.wordCommandResults <- result
			app.processAsyncResults()
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out toggling off %s", step.action)
		}
		updated, ok = selectedWordParagraph(state.wordBlocks, paragraph.ID)
		if !ok || updated.ListKind != document.WordListNone || updated.ListMarker != "" {
			t.Fatalf("toggle off %s left list %#v, found=%v", step.action, updated, ok)
		}
	}
}

func TestWordListToolbarDisablesUnsupportedNestedLists(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	state := newDocumentSessionState("word-list.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = paragraph.ID
	blocks := wordBlocksForState(state)
	blocks[0].Paragraph.ListKind = document.WordListUnsupported
	blocks[0].Paragraph.ListLevel = 1
	state.wordBlocks = blocks
	app := &App{documentSessionState: state}
	for _, id := range []string{"bullet-list", "numbered-list"} {
		if app.wordToolbarActionEnabled(findWordTestAction(t, id)) {
			t.Errorf("list action %q should be disabled for unsupported nested numbering", id)
		}
	}
}

func TestWordIndentToolbarMovesListThroughNestedLevels(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	state := newDocumentSessionState("word-nested-list.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = paragraph.ID
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	apply := func(action string) {
		t.Helper()
		app.executeWordToolbarAction(action)
		select {
		case result := <-app.wordCommandResults:
			if result.err != nil {
				t.Fatalf("%s: %v", action, result.err)
			}
			app.wordCommandResults <- result
			app.processAsyncResults()
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %s", action)
		}
	}
	apply("bullet-list")
	apply("increase-indent")
	updated, ok := selectedWordParagraph(state.wordBlocks, paragraph.ID)
	if !ok || updated.ListKind != document.WordListBullet || updated.ListLevel != 1 || updated.ListMarker != "◦" {
		t.Fatalf("increase list level = %#v, found=%v; want bullet level 1 marker ◦", updated, ok)
	}
	apply("decrease-indent")
	updated, ok = selectedWordParagraph(state.wordBlocks, paragraph.ID)
	if !ok || updated.ListKind != document.WordListBullet || updated.ListLevel != 0 || updated.ListMarker != "•" {
		t.Fatalf("decrease list level = %#v, found=%v; want bullet level 0 marker •", updated, ok)
	}
	apply("decrease-indent")
	updated, ok = selectedWordParagraph(state.wordBlocks, paragraph.ID)
	if !ok || updated.ListKind != document.WordListNone || updated.ListMarker != "" {
		t.Fatalf("decrease at list level 0 = %#v, found=%v; want plain paragraph", updated, ok)
	}
}

func TestWordFindCyclesCaseInsensitivelyAcrossEditableRunsAndDrafts(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	blocks := word.WordBlocks()
	firstRunID := blocks[0].Paragraph.Inlines[0].Text.ID
	textEditor, ok := any(word).(document.WordTextEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose Word text editing")
	}
	if err := textEditor.ReplaceTextRun(firstRunID, "there World world"); err != nil {
		t.Fatal(err)
	}
	bodyEditor, ok := any(word).(document.WordBodyEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose body paragraph insertion")
	}
	if _, err := bodyEditor.InsertBodyParagraph(); err != nil {
		t.Fatal(err)
	}
	blocks = word.WordBlocks()
	secondRunID := blocks[1].Paragraph.Inlines[0].Text.ID
	if err := textEditor.ReplaceTextRun(secondRunID, "other world"); err != nil {
		t.Fatal(err)
	}
	blocks = cloneWordBlockSnapshot(word.WordBlocks())
	find := func(currentRun string, currentEnd int, drafts map[string]string) wordTextMatch {
		t.Helper()
		match, found := findWordTextMatch(blocks, "world", currentRun, currentEnd, drafts)
		if !found {
			t.Fatal("expected to find world")
		}
		return match
	}
	if got := find("", 0, nil); got.RunID != firstRunID || got.Start != 6 || got.End != 11 {
		t.Fatalf("first match = %#v; want first run [6,11)", got)
	}
	if got := find(firstRunID, 11, nil); got.RunID != firstRunID || got.Start != 12 || got.End != 17 {
		t.Fatalf("next match = %#v; want second match in first run [12,17)", got)
	}
	if got := find(firstRunID, 17, nil); got.RunID != secondRunID || got.Start != 6 || got.End != 11 {
		t.Fatalf("cross-run match = %#v; want second run [6,11)", got)
	}
	if got := find(secondRunID, 11, nil); got.RunID != firstRunID || got.Start != 6 || got.End != 11 {
		t.Fatalf("wrapped match = %#v; want first run [6,11)", got)
	}
	if got, found := findWordTextMatch(blocks, "keyword", "", 0, map[string]string{secondRunID: "other keyword"}); !found || got.RunID != secondRunID || got.Start != 6 {
		t.Fatalf("draft match = %#v found=%v; want second run draft at 6", got, found)
	}
}

func TestWordFindToolbarOpensAndNavigatesToSelectedMatch(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	runID := paragraph.Inlines[0].Text.ID
	textEditor, ok := any(word).(document.WordTextEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose Word text editing")
	}
	if err := textEditor.ReplaceTextRun(runID, "find this"); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("word-find.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app := &App{documentSessionState: state}
	if !app.wordToolbarActionEnabled(findWordTestAction(t, "find")) {
		t.Fatal("Word Find should be enabled for a document with editable text")
	}
	app.executeWordToolbarAction("find")
	if !app.modal.Visible || !app.modal.HasInput {
		t.Fatal("Word Find did not open its query dialog")
	}
	app.modal.Input.SetText("this")
	app.modal.OnAction("confirm")
	if app.modal.Visible {
		t.Fatal("Word Find left the query dialog visible after confirming")
	}
	if state.selectedWordRunID != runID || state.selectedWordParagraphID != paragraph.ID {
		t.Fatalf("Word Find selection = %q/%q; want %q/%q", state.selectedWordRunID, state.selectedWordParagraphID, runID, paragraph.ID)
	}
	if editor := state.wordRunEditors[runID]; editor == nil {
		t.Fatal("Word Find did not initialize the persistent run editor")
	} else if start, end := editor.Selection(); start != 5 || end != 9 {
		t.Fatalf("Word Find selection = [%d,%d); want [5,9)", start, end)
	}
	if state.wordPendingFocusRunID != runID {
		t.Fatalf("pending focus run = %q; want %q", state.wordPendingFocusRunID, runID)
	}
}

func TestWordReplaceToolbarOpensReplacementDialog(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	runID := paragraph.Inlines[0].Text.ID
	if err := word.ReplaceTextRun(runID, "replace this"); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("word-replace.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordRunID = runID
	state.selectedWordParagraphID = paragraph.ID
	app := &App{documentSessionState: state}

	app.executeWordToolbarAction("replace")
	if !app.modal.Visible || !app.modal.HasInput {
		t.Fatal("Word Replace did not open its search and replacement dialog")
	}
}

func TestWordReplaceReplacesSelectionAndSelectsNextMatch(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	runID := paragraph.Inlines[0].Text.ID
	if err := word.ReplaceTextRun(runID, "apple apple"); err != nil {
		t.Fatal(err)
	}
	paragraph = word.WordBlocks()[0].Paragraph
	state := newDocumentSessionState("word-replace.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordRunID = runID
	state.selectedWordParagraphID = paragraph.ID
	textEditor := wordRunEditor(state, *paragraph.Inlines[0].Text)
	textEditor.SetCaret(0, 5)
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	app.executeWordToolbarAction("replace")
	if !app.modal.HasSecondInput {
		t.Fatal("Word Replace did not show a replacement-value input")
	}
	app.modal.Input.SetText("APPLE")
	app.modal.SecondInput.SetText("pear")
	app.modal.OnAction("replace")
	select {
	case result := <-app.wordCommandResults:
		app.wordCommandResults <- result
	case <-time.After(time.Second):
		t.Fatal("Word Replace did not publish its edit result")
	}
	app.processAsyncResults()

	blocks := word.WordBlocks()
	got := blocks[0].Paragraph.Inlines[0].Text.Text
	if got != "pear apple" {
		t.Fatalf("first selected match replacement = %q; want %q", got, "pear apple")
	}
	if start, end := textEditor.Selection(); start != 5 || end != 10 {
		t.Fatalf("next match selection = [%d,%d); want [5,10)", start, end)
	}
	if !app.modal.Visible {
		t.Fatal("Word Replace closed its dialog after replacing one match")
	}

	app.modal.OnAction("replace")
	select {
	case result := <-app.wordCommandResults:
		app.wordCommandResults <- result
	case <-time.After(time.Second):
		t.Fatal("Word Replace did not publish the second edit result")
	}
	app.processAsyncResults()
	if got := word.WordBlocks()[0].Paragraph.Inlines[0].Text.Text; got != "pear pear" {
		t.Fatalf("second selected match replacement = %q; want %q", got, "pear pear")
	}
}

func TestWordIndentToolbarActionsStepLeftIndentAndKeepRightIndent(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	textEditor, ok := any(word).(document.WordTextEditor)
	if !ok {
		t.Fatal("DOCX adapter does not expose paragraph indent editing")
	}
	const initialLeft, right int64 = 914400, 228600
	if err := textEditor.SetParagraphIndents(paragraph.ID, initialLeft, right); err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("word-indent.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = paragraph.ID
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	for _, step := range []struct {
		action string
		want   int64
	}{
		{action: "decrease-indent", want: initialLeft - 457200},
		{action: "increase-indent", want: initialLeft},
	} {
		action := findWordTestAction(t, step.action)
		if !app.wordToolbarActionEnabled(action) {
			t.Fatalf("Word indent action %q should be enabled for the selected paragraph", step.action)
		}
		app.executeWordToolbarAction(step.action)
		select {
		case result := <-app.wordCommandResults:
			if result.err != nil {
				t.Fatalf("%s: %v", step.action, result.err)
			}
			app.wordCommandResults <- result
			app.processAsyncResults()
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %s", step.action)
		}
		updated, ok := selectedWordParagraph(state.wordBlocks, paragraph.ID)
		if !ok || updated.LeftIndentEMU != step.want || updated.RightIndentEMU != right {
			t.Fatalf("%s indents = %d/%d, found=%v; want %d/%d EMU", step.action, updated.LeftIndentEMU, updated.RightIndentEMU, ok, step.want, right)
		}
	}
	output := filepath.Join(t.TempDir(), "word-indent.docx")
	if err := word.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := docx.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedParagraph := reopened.WordBlocks()[0].Paragraph
	if reopenedParagraph.LeftIndentEMU != initialLeft || reopenedParagraph.RightIndentEMU != right {
		t.Fatalf("saved/reopened paragraph indents = %d/%d; want %d/%d EMU", reopenedParagraph.LeftIndentEMU, reopenedParagraph.RightIndentEMU, initialLeft, right)
	}
}

func TestWordFontFamilyActionRequiresSelectedEditableRun(t *testing.T) {
	state := newDocumentSessionState("font-family.docx")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state.word = word
	app := &App{documentSessionState: state}
	var action ribbonAction
	for _, candidate := range wordToolbarActions(wordTabHome) {
		if candidate.id == "font-family" {
			action = candidate
			break
		}
	}
	if action.id == "" {
		t.Fatal("font-family action not found")
	}
	if app.wordToolbarActionEnabled(action) {
		t.Fatal("font-family action should be disabled without a selected run")
	}
	paragraph := word.WordBlocks()[0].Paragraph
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = paragraph.Inlines[0].Text.ID
	if !app.wordToolbarActionEnabled(action) {
		t.Fatal("font-family action should be enabled for a selected editable run")
	}
}

func TestWordFontFamilyToolbarActionOpensDialogAndChangesSelectedRun(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	runID := paragraph.Inlines[0].Text.ID
	state := newDocumentSessionState("font-family.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = runID
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}

	app.executeWordToolbarAction("font-family")
	if !app.modal.Visible || !app.modal.HasInput {
		t.Fatal("font-family action did not open a text input dialog")
	}
	app.modal.Input.SetText("Noto Sans CJK SC")
	app.modal.OnAction("confirm")
	if app.modal.Visible {
		t.Fatal("font-family dialog remained open after confirm")
	}
	if !state.formatRunning.Load() {
		t.Fatal("confirm did not start a Word formatting command")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatal(result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for font-family update")
	}
	run, ok := selectedWordRun(state.wordBlocks, runID)
	if !ok || run.Style.FontFamily != "Noto Sans CJK SC" {
		t.Fatalf("selected run font family = %q, found=%v; want Noto Sans CJK SC", run.Style.FontFamily, ok)
	}
}

func TestWordFormatPainterAppliesSourceFormattingToNextFocusedRun(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	sourceParagraph := word.WordBlocks()[0].Paragraph
	targetParagraphID, err := any(word).(document.WordBodyEditor).InsertBodyParagraph()
	if err != nil {
		t.Fatal(err)
	}
	blocks := word.WordBlocks()
	targetParagraph, ok := selectedWordParagraph(blocks, targetParagraphID)
	if !ok {
		t.Fatalf("inserted paragraph %q was not projected", targetParagraphID)
	}
	sourceRunID := sourceParagraph.Inlines[0].Text.ID
	targetRunID := targetParagraph.Inlines[0].Text.ID
	textEditor := any(word).(document.WordTextEditor)
	if err := textEditor.ReplaceTextRun(sourceRunID, "Source"); err != nil {
		t.Fatal(err)
	}
	if err := textEditor.ReplaceTextRun(targetRunID, "Target"); err != nil {
		t.Fatal(err)
	}
	yes := true
	if err := textEditor.SetTextRunStyle(sourceRunID, document.WordTextFormatPatch{Bold: &yes}); err != nil {
		t.Fatal(err)
	}
	if err := textEditor.SetParagraphAlignment(sourceParagraph.ID, document.WordAlignCenter); err != nil {
		t.Fatal(err)
	}

	state := newDocumentSessionState("format-painter.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = sourceParagraph.ID
	state.selectedWordRunID = sourceRunID
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	action := ribbonAction{id: "format-painter"}
	if !app.wordToolbarActionEnabled(action) {
		t.Fatal("format painter should be enabled for an editable source run")
	}
	app.executeWordToolbarAction("format-painter")
	if app.status != "已启动格式刷；单击目标文字应用，再点格式刷取消" {
		t.Fatalf("arming format painter status = %q", app.status)
	}
	targetSelector, ok := any(app).(interface {
		queueWordFormatPainterTarget(paragraphID, runID string)
	})
	if !ok {
		t.Fatal("Word editor does not route the next focused run to the active format painter")
	}
	targetSelector.queueWordFormatPainterTarget(targetParagraph.ID, targetRunID)
	if !state.formatRunning.Load() {
		t.Fatal("selecting a target run did not start the formatting operation")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatal(result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for format painter")
	}
	paintedParagraph, ok := selectedWordParagraph(state.wordBlocks, targetParagraph.ID)
	if !ok {
		t.Fatal("painted paragraph is missing")
	}
	paintedRun := paintedParagraph.Inlines[0].Text
	if paintedRun.Text != "Target" || !paintedRun.Style.Bold || paintedParagraph.Alignment != document.WordAlignCenter {
		t.Fatalf("painted target text/style/alignment = %q/%#v/%q", paintedRun.Text, paintedRun.Style, paintedParagraph.Alignment)
	}
}

func TestFailedDraftFlushCancelsDeferredFormatPainter(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	sourceParagraph := word.WordBlocks()[0].Paragraph
	targetParagraphID, err := any(word).(document.WordBodyEditor).InsertBodyParagraph()
	if err != nil {
		t.Fatal(err)
	}
	blocks := word.WordBlocks()
	targetParagraph, ok := selectedWordParagraph(blocks, targetParagraphID)
	if !ok {
		t.Fatalf("inserted paragraph %q was not projected", targetParagraphID)
	}
	state := newDocumentSessionState("format-painter.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(blocks)
	state.selectedWordParagraphID = sourceParagraph.ID
	state.selectedWordRunID = sourceParagraph.Inlines[0].Text.ID
	state.wordRunDrafts = map[string]string{"missing-run": "draft"}
	state.wordRunCommitted = map[string]string{"missing-run": ""}
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	app.toggleWordFormatPainter()
	app.queueWordFormatPainterTarget(targetParagraph.ID, targetParagraph.Inlines[0].Text.ID)
	if state.wordDeferredToolbarAction != "apply-format-painter" {
		t.Fatalf("deferred action = %q; want format painter while drafts flush", state.wordDeferredToolbarAction)
	}
	app.requestWordTextFlush(true)
	select {
	case result := <-app.wordCommandResults:
		if result.err == nil {
			t.Fatal("invalid draft run should fail before the format painter starts")
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for invalid draft commit")
	}
	if state.wordDeferredToolbarAction != "" || state.wordFormatPainterSourceRunID != "" || state.wordFormatPainterTargetRunID != "" {
		t.Fatalf("failed draft flush left a deferred format painter: action=%q source=%q target=%q", state.wordDeferredToolbarAction, state.wordFormatPainterSourceRunID, state.wordFormatPainterTargetRunID)
	}
}

func TestWordHighlightActionRequiresSelectedEditableRun(t *testing.T) {
	state := newDocumentSessionState("word-highlight.docx")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state.word = word
	app := &App{documentSessionState: state}
	var action ribbonAction
	for _, candidate := range wordToolbarActions(wordTabHome) {
		if candidate.id == "text-highlight" {
			action = candidate
			break
		}
	}
	if action.id == "" {
		t.Fatal("text-highlight action not found")
	}
	if app.wordToolbarActionEnabled(action) {
		t.Fatal("text-highlight action should be disabled without a selected run")
	}
	paragraph := word.WordBlocks()[0].Paragraph
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = paragraph.Inlines[0].Text.ID
	if !app.wordToolbarActionEnabled(action) {
		t.Fatal("text-highlight action should be enabled for a selected editable run")
	}
}

func TestWordHighlightToolbarActionOpensPaletteAndAppliesHighlight(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	runID := paragraph.Inlines[0].Text.ID
	state := newDocumentSessionState("word-highlight.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordParagraphID = paragraph.ID
	state.selectedWordRunID = runID
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}

	app.executeWordToolbarAction("text-highlight")
	if !state.wordToolbarHighlightPicker || state.wordToolbarColorPicker {
		t.Fatal("text-highlight action did not open its highlight palette")
	}
	if wordToolbarActionActive(state, findWordTestAction(t, "text-highlight")) {
		t.Fatal("opening the highlight palette marked the run highlighted")
	}
	app.chooseWordHighlightColor(0)
	if state.wordToolbarHighlightPicker {
		t.Fatal("highlight palette remained open after choosing a color")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatal(result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for text highlight")
	}
	run, ok := selectedWordRun(state.wordBlocks, runID)
	if !ok || run.Style.Highlight != "yellow" {
		t.Fatalf("selected run highlight = %q, found=%v; want yellow", run.Style.Highlight, ok)
	}
	if !wordToolbarActionActive(state, findWordTestAction(t, "text-highlight")) {
		t.Fatal("highlight ribbon action is not active after applying a highlight")
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

func TestRotateSelectedWordImageRunsAsyncAndKeepsSelection(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	paragraph := word.WordBlocks()[0].Paragraph
	imageID, err := word.InsertInlineImage(paragraph.ID, "", "image/png", tinyToolbarPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	state := newDocumentSessionState("rotate.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	state.selectedWordImageID = imageID
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}
	app.rotateSelectedWordImage(90)
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("rotate selected image: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Word image rotation")
	}
	rotated := findWordImage(state.wordBlocks, imageID)
	if rotated == nil || rotated.RotationDegrees != 90 {
		t.Fatalf("selected image after rotation = %#v; want 90 degrees", rotated)
	}
	if state.selectedWordImageID != imageID {
		t.Fatalf("selected image after rotation = %q; want %q", state.selectedWordImageID, imageID)
	}
	if !word.CanUndo() {
		t.Fatal("rotating an image did not add an undo step")
	}
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

func TestWordToolbarParagraphSpacingBeforeAndAfter(t *testing.T) {
	state := newDocumentSessionState("sample.docx")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state.word = word
	state.setWordBlocks(word.WordBlocks())
	paragraph := allWordParagraphs(state.wordBlocks)[0]
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}

	beforeAction := ribbonAction{id: "layout-before", label: "段前"}
	afterAction := ribbonAction{id: "layout-after", label: "段后"}

	state.selectedWordParagraphID = ""
	if app.wordToolbarActionEnabled(beforeAction) || app.wordToolbarActionEnabled(afterAction) {
		t.Fatal("paragraph spacing should be disabled when no paragraph is selected")
	}

	state.selectedWordParagraphID = paragraph.ID
	if !app.wordToolbarActionEnabled(beforeAction) || !app.wordToolbarActionEnabled(afterAction) {
		t.Fatal("paragraph spacing should be enabled when paragraph is selected")
	}

	// Test layout-before
	app.executeWordToolbarAction("layout-before")
	if !app.modal.Visible || app.modal.Title != "段前间距" {
		t.Fatalf("expected modal dialog for layout-before, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("12")
	app.modal.OnAction("confirm")
	if app.modal.Visible {
		t.Fatal("modal should be hidden after confirm")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("layout-before failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for layout-before command result")
	}

	updated := allWordParagraphs(state.wordBlocks)[0]
	if updated.Layout.BeforePT != 12 {
		t.Fatalf("expected BeforePT=12, got %g", updated.Layout.BeforePT)
	}

	// Test layout-after
	app.executeWordToolbarAction("layout-after")
	if !app.modal.Visible || app.modal.Title != "段后间距" {
		t.Fatalf("expected modal dialog for layout-after, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("18")
	app.modal.OnAction("confirm")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("layout-after failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for layout-after command result")
	}

	updated = allWordParagraphs(state.wordBlocks)[0]
	if updated.Layout.AfterPT != 18 {
		t.Fatalf("expected AfterPT=18, got %g", updated.Layout.AfterPT)
	}

	// Test undo restores AfterPT
	app.applyWordHistory(true)
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("undo failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for undo result")
	}
	updated = allWordParagraphs(state.wordBlocks)[0]
	if updated.Layout.AfterPT != 0 {
		t.Fatalf("expected AfterPT restored to 0 after undo, got %g", updated.Layout.AfterPT)
	}

	// Test invalid input
	app.executeWordToolbarAction("layout-before")
	app.modal.Input.SetText("2000")
	app.modal.OnAction("confirm")
	if !strings.Contains(app.status, "必须在 0 到 1584 磅之间") {
		t.Fatalf("expected out-of-range status error, got %q", app.status)
	}
}

func TestWordPageSetupDialogsAndCommands(t *testing.T) {
	state := newDocumentSessionState("sample.docx")
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	state.word = doc
	state.setWordBlocks(doc.WordBlocks())
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}

	orientAction := ribbonAction{id: "layout-orientation", label: "方向"}
	marginsAction := ribbonAction{id: "layout-margins", label: "页边距"}
	sizeAction := ribbonAction{id: "layout-size", label: "纸张大小"}

	if !app.wordToolbarActionEnabled(orientAction) || !app.wordToolbarActionEnabled(marginsAction) || !app.wordToolbarActionEnabled(sizeAction) {
		t.Fatal("expected page setup actions to be enabled")
	}

	// 1. Orientation Dialog and Command
	app.executeWordToolbarAction("layout-orientation")
	if !app.modal.Visible || app.modal.Title != "页面方向" {
		t.Fatalf("expected modal dialog for layout-orientation, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("landscape")
	if app.modal.Visible {
		t.Fatal("modal should be hidden after landscape action")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("layout-orientation command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for layout-orientation command result")
	}

	layout := doc.WordLayoutData()
	if len(layout.Sections) == 0 || layout.Sections[0].PageSetup.Orientation != "landscape" {
		t.Fatalf("expected orientation landscape, got %#v", layout.Sections)
	}

	// 2. Margins Dialog and Command
	app.executeWordToolbarAction("layout-margins")
	if !app.modal.Visible || app.modal.Title != "页边距设置" {
		t.Fatalf("expected modal dialog for layout-margins, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("narrow")
	if app.modal.Visible {
		t.Fatal("modal should be hidden after narrow action")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("layout-margins command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for layout-margins command result")
	}

	layout = doc.WordLayoutData()
	setup := layout.Sections[0].PageSetup
	if setup.MarginTopPT != 36 || setup.MarginLeftPT != 36 {
		t.Fatalf("expected narrow margins (36pt), got top=%f left=%f", setup.MarginTopPT, setup.MarginLeftPT)
	}

	// 3. Page Size Dialog and Command
	app.executeWordToolbarAction("layout-size")
	if !app.modal.Visible || app.modal.Title != "纸张大小设置" {
		t.Fatalf("expected modal dialog for layout-size, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("letter")
	if app.modal.Visible {
		t.Fatal("modal should be hidden after letter action")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("layout-size command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for layout-size command result")
	}

	layout = doc.WordLayoutData()
	setup = layout.Sections[0].PageSetup
	// Since current orientation is landscape, width > height (792 x 612)
	if (math.Abs(setup.WidthPT-792) > 1.0 && math.Abs(setup.WidthPT-612) > 1.0) || (math.Abs(setup.HeightPT-792) > 1.0 && math.Abs(setup.HeightPT-612) > 1.0) {
		t.Fatalf("expected Letter page size dimensions, got width=%f height=%f", setup.WidthPT, setup.HeightPT)
	}

	// 4. Columns Dialog and Command
	columnsAction := ribbonAction{id: "layout-columns", label: "分栏"}
	if !app.wordToolbarActionEnabled(columnsAction) {
		t.Fatal("expected layout-columns action to be enabled")
	}
	app.executeWordToolbarAction("layout-columns")
	if !app.modal.Visible || app.modal.Title != "分栏设置" {
		t.Fatalf("expected modal dialog for layout-columns, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("col2")
	if app.modal.Visible {
		t.Fatal("modal should be hidden after col2 action")
	}
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("layout-columns command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for layout-columns command result")
	}
	snap, err := doc.Snapshot()
	if err != nil {
		t.Fatalf("doc.Snapshot failed: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(snap), int64(len(snap)))
	if err != nil {
		t.Fatal(err)
	}
	var docXML string
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			docXML = string(b)
			break
		}
	}
	if !strings.Contains(docXML, `w:cols`) || !strings.Contains(docXML, `w:num="2"`) {
		t.Fatalf("expected 2 columns in word/document.xml, got: %s", docXML)
	}
}

func TestWordParagraphIndentDialogAndCommand(t *testing.T) {
	state := newDocumentSessionState("sample.docx")
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	state.word = doc
	state.setWordBlocks(doc.WordBlocks())
	app := &App{documentSessionState: state, wordCommandResults: make(chan wordCommandResult, 1)}

	blocks := doc.WordBlocks()
	if len(blocks) == 0 || blocks[0].Paragraph == nil {
		t.Fatal("expected at least one paragraph block")
	}
	state.selectedWordParagraphID = blocks[0].Paragraph.ID
	if len(blocks[0].Paragraph.Inlines) > 0 && blocks[0].Paragraph.Inlines[0].Text != nil {
		state.selectedWordRunID = blocks[0].Paragraph.Inlines[0].Text.ID
	}

	indentAction := ribbonAction{id: "layout-indent", label: "缩进"}
	if !app.wordToolbarActionEnabled(indentAction) {
		t.Fatal("expected layout-indent action to be enabled")
	}

	// 1. Open dialog and set custom indent
	app.executeWordToolbarAction("layout-indent")
	if !app.modal.Visible || app.modal.Title != "段落缩进" {
		t.Fatalf("expected modal dialog for layout-indent, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	if len(app.modal.FormInputs) < 2 {
		t.Fatalf("expected at least 2 form inputs, got %d", len(app.modal.FormInputs))
	}
	app.modal.FormInputs[0].SetText("72")
	app.modal.FormInputs[1].SetText("36")
	app.modal.OnAction("confirm")
	if app.modal.Visible {
		t.Fatal("modal should be hidden after confirm action")
	}

	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("layout-indent command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for layout-indent command result")
	}

	p := doc.WordBlocks()[0].Paragraph
	const wantLeftEMU, wantRightEMU int64 = 72 * 12700, 36 * 12700
	if p.LeftIndentEMU != wantLeftEMU || p.RightIndentEMU != wantRightEMU {
		t.Fatalf("expected paragraph indents %d/%d EMU, got %d/%d EMU", wantLeftEMU, wantRightEMU, p.LeftIndentEMU, p.RightIndentEMU)
	}

	// 2. Open dialog and click "none" (reset to 0)
	app.executeWordToolbarAction("layout-indent")
	if !app.modal.Visible {
		t.Fatal("modal should be visible")
	}
	app.modal.OnAction("none")
	if app.modal.Visible {
		t.Fatal("modal should be hidden after none action")
	}

	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("layout-indent none command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for layout-indent none command result")
	}

	p = doc.WordBlocks()[0].Paragraph
	if p.LeftIndentEMU != 0 || p.RightIndentEMU != 0 {
		t.Fatalf("expected indents reset to 0, got %d/%d EMU", p.LeftIndentEMU, p.RightIndentEMU)
	}
}

func TestWordDesignSpacingPresets(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	state := newDocumentSessionState("design.docx")
	state.word = doc
	state.wordBlocks = cloneWordBlockSnapshot(doc.WordBlocks())
	state.selectedWordParagraphID = doc.WordBlocks()[0].Paragraph.ID
	app := &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 4),
		shutdown:             make(chan struct{}),
	}

	var compactAction, spaciousAction ribbonAction
	for _, a := range wordToolbarActions(wordTabDesign) {
		if a.id == "design-compact" {
			compactAction = a
		} else if a.id == "design-spacious" {
			spaciousAction = a
		}
	}
	if compactAction.id == "" || spaciousAction.id == "" {
		t.Fatal("design actions not found in wordTabDesign")
	}

	if !app.wordToolbarActionEnabled(compactAction) {
		t.Fatal("design-compact should be enabled")
	}
	if !app.wordToolbarActionEnabled(spaciousAction) {
		t.Fatal("design-spacious should be enabled")
	}

	// 1. Apply compact spacing
	app.executeWordToolbarAction("design-compact")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("design-compact failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for design-compact result")
	}

	p := doc.WordBlocks()[0].Paragraph
	if p.Layout.BeforePT != 0 || p.Layout.AfterPT != 4 {
		t.Fatalf("expected compact spacing 0/4 pt, got %g/%g pt", p.Layout.BeforePT, p.Layout.AfterPT)
	}

	// 2. Apply spacious spacing
	app.executeWordToolbarAction("design-spacious")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("design-spacious failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for design-spacious result")
	}

	p = doc.WordBlocks()[0].Paragraph
	if p.Layout.BeforePT != 0 || p.Layout.AfterPT != 12 {
		t.Fatalf("expected spacious spacing 0/12 pt, got %g/%g pt", p.Layout.BeforePT, p.Layout.AfterPT)
	}
}

func TestWordReviewRestrictProtection(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("failed to create blank docx: %v", err)
	}
	defer doc.Close()

	state := newDocumentSessionState("test.docx")
	state.word = doc
	state.wordBlocks = cloneWordBlockSnapshot(doc.WordBlocks())
	state.wordToolbarTab = wordTabReview

	app := &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 4),
	}

	var restrictAction ribbonAction
	for _, a := range wordToolbarActions(wordTabReview) {
		if a.id == "review-restrict" {
			restrictAction = a
			break
		}
	}
	if restrictAction.id == "" {
		t.Fatal("review-restrict action not found in wordTabReview")
	}

	if !app.wordToolbarActionEnabled(restrictAction) {
		t.Fatal("review-restrict should be enabled")
	}
	if wordToolbarActionActive(state, restrictAction) {
		t.Fatal("review-restrict should initially be inactive")
	}

	// 1. Show protection dialog -> Click "protect"
	app.showWordDocumentProtectionDialog()
	if !app.modal.Visible || app.modal.Title != "限制编辑" {
		t.Fatalf("expected modal dialog for review-restrict, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}

	app.modal.OnAction("protect")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("protect command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for protect command result")
	}

	// State should now be protected
	if !state.wordProtected {
		t.Fatal("expected state.wordProtected to be true")
	}
	if !wordToolbarActionActive(state, restrictAction) {
		t.Fatal("review-restrict should now be active")
	}
	prot, err := doc.GetWordDocumentProtection()
	if err != nil || !prot {
		t.Fatalf("expected doc to be protected, got %v (err=%v)", prot, err)
	}

	// 2. Show unprotect dialog -> Click "unprotect"
	app.showWordDocumentProtectionDialog()
	if !app.modal.Visible || app.modal.Title != "限制编辑" {
		t.Fatalf("expected modal dialog for unprotect, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}

	app.modal.OnAction("unprotect")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("unprotect command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for unprotect command result")
	}

	if state.wordProtected {
		t.Fatal("expected state.wordProtected to be false after unprotect")
	}
	if wordToolbarActionActive(state, restrictAction) {
		t.Fatal("review-restrict should be inactive after unprotect")
	}
	prot, err = doc.GetWordDocumentProtection()
	if err != nil || prot {
		t.Fatalf("expected doc to be unprotected, got %v (err=%v)", prot, err)
	}
}

func TestWordLineNumbersDialogAndCommand(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("failed to create blank docx: %v", err)
	}
	defer doc.Close()

	state := newDocumentSessionState("test.docx")
	state.word = doc
	state.wordBlocks = cloneWordBlockSnapshot(doc.WordBlocks())
	state.wordToolbarTab = wordTabLayout

	app := &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 4),
	}

	var lineNumAction ribbonAction
	for _, a := range wordToolbarActions(wordTabLayout) {
		if a.id == "layout-line-numbers" {
			lineNumAction = a
			break
		}
	}
	if lineNumAction.id == "" {
		t.Fatal("layout-line-numbers action not found in wordTabLayout")
	}

	if !app.wordToolbarActionEnabled(lineNumAction) {
		t.Fatal("layout-line-numbers should be enabled")
	}

	// 1. Show line numbers dialog -> Click "continuous"
	app.showWordLineNumbersDialog()
	if !app.modal.Visible || app.modal.Title != "行号设置" {
		t.Fatalf("expected modal dialog for line numbers, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}

	app.modal.OnAction("continuous")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("set line numbers command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for line numbers command result")
	}

	readXML := func() string {
		snap, err := doc.Snapshot()
		if err != nil {
			t.Fatalf("doc.Snapshot failed: %v", err)
		}
		zr, err := zip.NewReader(bytes.NewReader(snap), int64(len(snap)))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range zr.File {
			if f.Name == "word/document.xml" {
				rc, err := f.Open()
				if err != nil {
					t.Fatal(err)
				}
				b, _ := io.ReadAll(rc)
				_ = rc.Close()
				return string(b)
			}
		}
		return ""
	}

	xml := readXML()
	if !strings.Contains(xml, `w:restart="continuous"`) {
		t.Fatalf("expected continuous line numbers in xml: %s", xml)
	}

	// 2. Clear line numbers
	app.showWordLineNumbersDialog()
	app.modal.OnAction("none")
	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("clear line numbers command failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for clear line numbers command result")
	}

	xml = readXML()
	if strings.Contains(xml, "lnNumType") {
		t.Fatalf("expected lnNumType removed from xml: %s", xml)
	}
}

func TestWordSelectAllAndPageColor(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatalf("NewBlank failed: %v", err)
	}
	defer doc.Close()

	textEditor, _ := any(doc).(document.WordTextEditor)
	blocks := doc.WordBlocks()
	for _, b := range blocks {
		if b.Paragraph != nil {
			for _, in := range b.Paragraph.Inlines {
				if in.Kind == document.WordTextInline && in.Text != nil && in.Text.Editable {
					_ = textEditor.ReplaceTextRun(in.Text.ID, "Hello Word Document")
					break
				}
			}
		}
	}

	state := newDocumentSessionState("test.docx")
	state.word = doc
	state.wordBlocks = doc.WordBlocks()
	app := &App{documentSessionState: state}

	// 1. Verify toolbar action enabled
	if !app.wordToolbarActionEnabled(ribbonAction{id: "select"}) {
		t.Fatal("select action should be enabled")
	}
	if !app.wordToolbarActionEnabled(ribbonAction{id: "design-page-color"}) {
		t.Fatal("design-page-color action should be enabled")
	}

	// 2. Test Select All
	app.selectAllWordText()
	if app.status != "已全选文档正文" {
		t.Fatalf("unexpected status: %q", app.status)
	}
	if state.wordPageTextSelection == nil {
		t.Fatal("expected active wordPageTextSelection after select all")
	}
	selectedText, ok := wordPageSelectedText(state)
	if !ok || len(selectedText) == 0 {
		t.Fatalf("expected non-empty selected text, got ok=%v text=%q", ok, selectedText)
	}

	// 3. Test Page Color Dialog & Parchment
	app.showWordPageColorDialog()
	if !app.modal.Visible || app.modal.Title != "页面颜色" {
		t.Fatalf("expected 页面颜色 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("parchment")
	if state.wordBackgroundColor == nil {
		t.Fatal("expected wordBackgroundColor to be set")
	}
	if state.wordBackgroundColor.R != 0xFA || state.wordBackgroundColor.G != 0xF8 || state.wordBackgroundColor.B != 0xF5 {
		t.Fatalf("unexpected wordBackgroundColor: %+v", *state.wordBackgroundColor)
	}
	editor, _ := any(doc).(document.WordPageBackgroundEditor)
	bg, err := editor.GetPageBackgroundColor()
	if err != nil || bg != "FAF8F5" {
		t.Fatalf("expected doc background FAF8F5, got %q (err=%v)", bg, err)
	}

	// 4. Clear Page Color (White)
	app.showWordPageColorDialog()
	app.modal.OnAction("white")
	if state.wordBackgroundColor != nil {
		t.Fatalf("expected wordBackgroundColor nil after white, got %+v", *state.wordBackgroundColor)
	}
	bg, _ = editor.GetPageBackgroundColor()
	if bg != "" {
		t.Fatalf("expected empty background color after white, got %q", bg)
	}
}

func TestWordToolbar_TableAndPageBorder(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	state := newDocumentSessionState("test.docx")
	state.word = doc
	state.wordBlocks = doc.WordBlocks()
	state.selectedWordParagraphID = state.wordBlocks[0].Paragraph.ID
	app := &App{documentSessionState: state}

	// 1. Verify availability
	if !app.wordToolbarActionEnabled(ribbonAction{id: "insert-table"}) {
		t.Fatal("expected insert-table to be available")
	}
	if !app.wordToolbarActionEnabled(ribbonAction{id: "design-page-border"}) {
		t.Fatal("expected design-page-border to be available")
	}

	// 2. Insert Table Dialog & Execution
	app.showWordInsertTableDialog()
	if !app.modal.Visible || app.modal.Title != "插入表格" {
		t.Fatalf("expected 插入表格 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("3x3")
	if !strings.Contains(app.status, "已成功插入 3行3列 表格") {
		t.Fatalf("unexpected status after insert table: %q", app.status)
	}

	// Verify table in document
	blocks := doc.WordBlocks()
	foundTable := false
	for _, block := range blocks {
		if block.Kind == document.WordTableBlock && block.Table != nil {
			foundTable = true
			if len(block.Table.Rows) != 3 {
				t.Fatalf("expected 3 rows, got %d", len(block.Table.Rows))
			}
		}
	}
	if !foundTable {
		t.Fatal("inserted table not found in document blocks")
	}

	// Verify table is immediately visible in the active session without closing/reopening
	sessionBlocks := wordBlocksForState(state)
	foundInSession := false
	for _, block := range sessionBlocks {
		if block.Kind == document.WordTableBlock && block.Table != nil {
			foundInSession = true
			if len(block.Table.Rows) != 3 {
				t.Fatalf("expected 3 rows in session blocks, got %d", len(block.Table.Rows))
			}
		}
	}
	if !foundInSession {
		t.Fatal("inserted table not immediately visible in session state word blocks")
	}

	// 3. Page Border Dialog & Execution
	app.showWordPageBorderDialog()
	if !app.modal.Visible || app.modal.Title != "页面边框" {
		t.Fatalf("expected 页面边框 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("double")
	if !strings.Contains(app.status, "已设置页面边框") {
		t.Fatalf("unexpected status after set page border: %q", app.status)
	}

	// 4. Clear Page Border
	app.showWordPageBorderDialog()
	app.modal.OnAction("none")
	if !strings.Contains(app.status, "已清除页面边框") {
		t.Fatalf("unexpected status after clear page border: %q", app.status)
	}
}

func TestWordToolbar_CoverPageAndLanguage(t *testing.T) {
	doc, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()

	state := newDocumentSessionState("test.docx")
	state.word = doc
	state.wordBlocks = doc.WordBlocks()
	state.selectedWordParagraphID = state.wordBlocks[0].Paragraph.ID
	app := &App{documentSessionState: state}

	// 1. Verify availability
	if !app.wordToolbarActionEnabled(ribbonAction{id: "cover"}) {
		t.Fatal("expected cover to be enabled")
	}
	if !app.wordToolbarActionEnabled(ribbonAction{id: "review-language"}) {
		t.Fatal("expected review-language to be enabled")
	}

	// 2. Insert Cover Page Dialog & Execution
	app.showWordCoverPageDialog()
	if !app.modal.Visible || app.modal.Title != "插入封面" {
		t.Fatalf("expected 插入封面 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("项目年度规划")
	app.modal.OnAction("confirm")
	if !strings.Contains(app.status, "已成功在文档首页插入标准封面页") {
		t.Fatalf("unexpected status after insert cover page: %q", app.status)
	}

	// 3. Set Language Dialog & Execution
	app.showWordLanguageDialog()
	if !app.modal.Visible || app.modal.Title != "语言设置" {
		t.Fatalf("expected 语言设置 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("en-US")
	if !strings.Contains(app.status, "已成功将全文档语言设置为 en-US") {
		t.Fatalf("unexpected status after set language: %q", app.status)
	}
}

func TestWordToolbar_BookmarkLinkAndTextBox(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()

	state := newDocumentSessionState("word-insert.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app := &App{documentSessionState: state}

	// 1. Verify actions are enabled
	for _, id := range []string{"insert-bookmark", "insert-link", "insert-textbox"} {
		if !app.wordToolbarActionEnabled(ribbonAction{id: id}) {
			t.Fatalf("expected action %q to be enabled", id)
		}
	}

	// 2. Test Insert Bookmark Dialog & Execution
	app.showWordBookmarkDialog()
	if !app.modal.Visible || app.modal.Title != "插入书签" {
		t.Fatalf("expected 插入书签 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("Test_Bookmark")
	app.modal.OnAction("confirm")
	if !strings.Contains(app.status, "已成功插入书签") {
		t.Fatalf("unexpected status after insert bookmark: %q", app.status)
	}

	// 3. Test Insert Hyperlink Dialog & Execution
	app.showWordHyperlinkDialog()
	if !app.modal.Visible || app.modal.Title != "插入超链接" {
		t.Fatalf("expected 插入超链接 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("https://golang.org")
	app.modal.SecondInput.SetText("Go官网")
	app.modal.OnAction("confirm")
	if !strings.Contains(app.status, "已成功插入超链接") {
		t.Fatalf("unexpected status after insert link: %q", app.status)
	}

	// 4. Test Insert Text Box Dialog & Execution
	app.showWordTextBoxDialog()
	if !app.modal.Visible || app.modal.Title != "插入文本框" {
		t.Fatalf("expected 插入文本框 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("侧边栏重要提示")
	app.modal.OnAction("confirm")
	if !strings.Contains(app.status, "已成功插入文本框") {
		t.Fatalf("unexpected status after insert textbox: %q", app.status)
	}
}

func TestWordToolbar_HeaderFooterPageNumberAndSideBySide(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()

	state := newDocumentSessionState("word-header-footer.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app := &App{documentSessionState: state}

	// 1. Verify availability
	for _, id := range []string{"insert-header", "insert-footer", "insert-page-number", "view-side-by-side"} {
		if !app.wordToolbarActionEnabled(ribbonAction{id: id}) {
			t.Fatalf("expected action %q to be enabled", id)
		}
	}

	// 2. Set Header
	app.showWordHeaderDialog()
	if !app.modal.Visible || app.modal.Title != "插入页眉" {
		t.Fatalf("expected 插入页眉 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("测试页眉内容")
	app.modal.OnAction("confirm")
	if !strings.Contains(app.status, "已成功设置页眉") {
		t.Fatalf("unexpected status after set header: %q", app.status)
	}

	// 3. Set Footer
	app.showWordFooterDialog()
	if !app.modal.Visible || app.modal.Title != "插入页脚" {
		t.Fatalf("expected 插入页脚 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("测试页脚内容")
	app.modal.OnAction("confirm")
	if !strings.Contains(app.status, "已成功设置页脚") {
		t.Fatalf("unexpected status after set footer: %q", app.status)
	}

	// 4. Set Page Number
	app.showWordPageNumberDialog()
	if !app.modal.Visible || app.modal.Title != "插入页码" {
		t.Fatalf("expected 插入页码 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("center")
	if !strings.Contains(app.status, "已成功在页脚插入页码") {
		t.Fatalf("unexpected status after set page number: %q", app.status)
	}

	// 5. Toggle Side by Side View
	app.toggleWordSideBySideView()
	if !strings.Contains(app.status, "并排查看") {
		t.Fatalf("unexpected status after toggle side by side: %q", app.status)
	}
}

func TestWordToolbar_WatermarkTrackRevisionsEditorAndLayout(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()

	state := newDocumentSessionState("word-review-design-layout.docx")
	state.word = word
	state.wordBlocks = cloneWordBlockSnapshot(word.WordBlocks())
	app := &App{documentSessionState: state}

	// 1. Verify availability of all new actions
	actions := []string{
		"design-watermark",
		"review-track", "review-accept", "review-reject",
		"review-editor", "editor",
		"review-translate", "review-compare",
		"layout-position", "layout-wrap", "layout-align",
	}
	for _, id := range actions {
		if !app.wordToolbarActionEnabled(ribbonAction{id: id}) {
			t.Fatalf("expected action %q to be enabled", id)
		}
	}

	// 2. Test Watermark Dialog
	app.showWordWatermarkDialog()
	if !app.modal.Visible || app.modal.Title != "文档水印" {
		t.Fatalf("expected 文档水印 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("绝密文档")
	app.modal.OnAction("set")
	// Give background goroutine a moment to complete
	time.Sleep(50 * time.Millisecond)

	// 3. Test Track Revisions Toggle
	if wordToolbarActionActive(state, ribbonAction{id: "review-track"}) {
		t.Fatal("expected track revisions to be inactive initially")
	}
	app.toggleWordTrackRevisions()
	time.Sleep(50 * time.Millisecond)
	if !state.wordTrackRevisions {
		t.Fatal("expected state.wordTrackRevisions to be true after toggle")
	}
	if !wordToolbarActionActive(state, ribbonAction{id: "review-track"}) {
		t.Fatal("expected track revisions action to be active after toggle")
	}

	// 4. Test Accept Revisions Dialog
	app.acceptWordRevisions()
	if !app.modal.Visible || app.modal.Title != "接受全部修订" {
		t.Fatalf("expected 接受全部修订 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("confirm")
	time.Sleep(50 * time.Millisecond)

	// 5. Test Reject Revisions Dialog
	app.rejectWordRevisions()
	if !app.modal.Visible || app.modal.Title != "拒绝全部修订" {
		t.Fatalf("expected 拒绝全部修订 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("confirm")
	time.Sleep(50 * time.Millisecond)

	// 6. Test Document Editor Dialog
	app.showWordEditorDialog()
	if !app.modal.Visible || app.modal.Title != "文档编辑器" {
		t.Fatalf("expected 文档编辑器 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	if !strings.Contains(app.modal.Message, "综合评分") {
		t.Fatalf("expected editor report to mention 综合评分, got: %q", app.modal.Message)
	}
	app.modal.OnAction("close")

	// 7. Test Document Translate Dialog
	app.showWordTranslateDialog()
	if !app.modal.Visible || app.modal.Title != "翻译" {
		t.Fatalf("expected 翻译 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("Translated Text")
	app.modal.OnAction("confirm")
	time.Sleep(50 * time.Millisecond)

	// 8. Test Document Compare Dialog
	app.showWordCompareDialog()
	if !app.modal.Visible || app.modal.Title != "文档比较" {
		t.Fatalf("expected 文档比较 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	if !strings.Contains(app.modal.Message, "版本") {
		t.Fatalf("expected compare dialog to mention 版本, got: %q", app.modal.Message)
	}
	app.modal.OnAction("close")

	// 9. Test Layout Align Dialog
	app.showWordLayoutAlignDialog()
	if !app.modal.Visible || app.modal.Title != "排列对齐" {
		t.Fatalf("expected 排列对齐 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("center")
	app.modal.OnAction("confirm")
	time.Sleep(50 * time.Millisecond)

	// 10. Test Layout Wrap Dialog
	app.showWordLayoutWrapDialog()
	if !app.modal.Visible || app.modal.Title != "文字环绕" {
		t.Fatalf("expected 文字环绕 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("close")

	// 11. Test Layout Position Dialog
	app.showWordLayoutPositionDialog()
	if !app.modal.Visible || app.modal.Title != "对象位置" {
		t.Fatalf("expected 对象位置 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("close")
}

func TestWordToolbar_RemainingActionsBatch(t *testing.T) {
	state := newDocumentSessionState("batch_test.docx")
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state.word = word
	state.setWordBlocks(word.WordBlocks())

	paragraphs := allWordParagraphs(state.wordBlocks)
	if len(paragraphs) == 0 {
		t.Fatal("expected at least one paragraph")
	}
	state.selectedWordParagraphID = paragraphs[0].ID
	if len(paragraphs[0].Inlines) > 0 && paragraphs[0].Inlines[0].Text != nil {
		state.selectedWordRunID = paragraphs[0].Inlines[0].Text.ID
	}

	app := &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 1),
	}

	actions := []string{
		"insert-wordart",
		"insert-equation",
		"insert-shapes",
		"insert-chart",
		"review-new-comment",
		"review-delete-comment",
		"design-themes",
		"design-colors",
		"design-fonts",
		"design-effects",
		"designer",
		"dictate",
		"add-ins",
		"view-macros",
	}

	for _, id := range actions {
		if !app.wordToolbarActionEnabled(ribbonAction{id: id}) {
			t.Fatalf("expected action %q to be enabled", id)
		}
	}

	// 1. insert-wordart
	app.executeWordToolbarAction("insert-wordart")
	if !app.modal.Visible || app.modal.Title != "插入艺术字" {
		t.Fatalf("expected 插入艺术字 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("测试艺术字")
	app.modal.OnAction("style_gold")
	if app.modal.Visible {
		t.Fatal("modal should be dismissed after style_gold")
	}

	// 2. insert-equation
	app.executeWordToolbarAction("insert-equation")
	if !app.modal.Visible || app.modal.Title != "插入公式" {
		t.Fatalf("expected 插入公式 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("pythagorean")
	select {
	case res := <-app.wordCommandResults:
		app.wordCommandResults <- res
		app.processAsyncResults()
	case <-time.After(time.Second):
	}

	// 3. insert-shapes
	app.executeWordToolbarAction("insert-shapes")
	if !app.modal.Visible || app.modal.Title != "插入形状" {
		t.Fatalf("expected 插入形状 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("流程框说明")
	app.modal.OnAction("flowchart")

	// 4. insert-chart
	app.executeWordToolbarAction("insert-chart")
	if !app.modal.Visible || app.modal.Title != "插入图表" {
		t.Fatalf("expected 插入图表 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("column")

	// 5. review-new-comment
	app.executeWordToolbarAction("review-new-comment")
	if !app.modal.Visible || app.modal.Title != "新建批注" {
		t.Fatalf("expected 新建批注 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("核对数据")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.wordCommandResults:
		app.wordCommandResults <- res
		app.processAsyncResults()
	case <-time.After(time.Second):
	}

	// 6. review-delete-comment
	app.executeWordToolbarAction("review-delete-comment")
	if !app.modal.Visible || app.modal.Title != "删除批注" {
		t.Fatalf("expected 删除批注 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("confirm")
	select {
	case res := <-app.wordCommandResults:
		app.wordCommandResults <- res
		app.processAsyncResults()
	case <-time.After(time.Second):
	}

	// 7. design-themes
	app.executeWordToolbarAction("design-themes")
	if !app.modal.Visible || app.modal.Title != "文档主题" {
		t.Fatalf("expected 文档主题 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("theme_business")
	select {
	case res := <-app.wordCommandResults:
		app.wordCommandResults <- res
		app.processAsyncResults()
	case <-time.After(time.Second):
	}

	// 8. design-colors
	app.executeWordToolbarAction("design-colors")
	if !app.modal.Visible || app.modal.Title != "主题配色" {
		t.Fatalf("expected 主题配色 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("scheme_mint")

	// 9. design-fonts
	app.executeWordToolbarAction("design-fonts")
	if !app.modal.Visible || app.modal.Title != "主题字体" {
		t.Fatalf("expected 主题字体 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("font_yahei")

	// 10. design-effects
	app.executeWordToolbarAction("design-effects")
	if !app.modal.Visible || app.modal.Title != "主题效果" {
		t.Fatalf("expected 主题效果 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("moderate")

	// 11. designer
	app.executeWordToolbarAction("designer")
	if !app.modal.Visible || app.modal.Title != "设计器" {
		t.Fatalf("expected 设计器 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("scheme_gov")
	select {
	case res := <-app.wordCommandResults:
		app.wordCommandResults <- res
		app.processAsyncResults()
	case <-time.After(time.Second):
	}

	// 12. dictate
	app.executeWordToolbarAction("dictate")
	if !app.modal.Visible || app.modal.Title != "语音听写" {
		t.Fatalf("expected 语音听写 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.Input.SetText("语音输入的一段话")
	app.modal.OnAction("confirm")
	select {
	case res := <-app.wordCommandResults:
		app.wordCommandResults <- res
		app.processAsyncResults()
	case <-time.After(time.Second):
	}

	// 13. add-ins
	app.executeWordToolbarAction("add-ins")
	if !app.modal.Visible || app.modal.Title != "加载项与扩展" {
		t.Fatalf("expected 加载项与扩展 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("close")

	// 14. view-macros
	app.executeWordToolbarAction("view-macros")
	if !app.modal.Visible || app.modal.Title != "宏与安全性" {
		t.Fatalf("expected 宏与安全性 modal, got visible=%v title=%q", app.modal.Visible, app.modal.Title)
	}
	app.modal.OnAction("enable_alert")
}

func TestWordEditing_SplitAfterFontColorAndHighlight(t *testing.T) {
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	state := newDocumentSessionState("test.docx")
	state.word = word
	state.setWordBlocks(word.WordBlocks())

	paragraphs := allWordParagraphs(state.wordBlocks)
	if len(paragraphs) == 0 || len(paragraphs[0].Inlines) == 0 || paragraphs[0].Inlines[0].Text == nil {
		t.Fatal("expected paragraph with text run")
	}
	run := paragraphs[0].Inlines[0].Text
	state.selectedWordParagraphID = paragraphs[0].ID
	state.selectedWordRunID = run.ID

	app := &App{
		documentSessionState: state,
		wordCommandResults:   make(chan wordCommandResult, 2),
	}

	// 1. Replace run text
	if err := word.ReplaceTextRun(run.ID, "换行测试文本"); err != nil {
		t.Fatal(err)
	}
	state.setWordBlocks(word.WordBlocks())

	// 2. Set font color and highlight
	colorVal := "FF0000"
	highlightVal := "yellow"
	if err := word.SetTextRunStyle(run.ID, document.WordTextFormatPatch{
		FontColor: &colorVal,
		Highlight: &highlightVal,
	}); err != nil {
		t.Fatal(err)
	}
	state.setWordBlocks(word.WordBlocks())

	// 3. Initiate split intent at caret index 2 ("换行" / "测试文本")
	state.wordParagraphSplitIntent = &wordParagraphSplitIntent{
		runID:          run.ID,
		value:          "换行测试文本",
		selectionStart: 2,
		selectionEnd:   2,
	}
	app.startWordParagraphSplit(state)

	select {
	case result := <-app.wordCommandResults:
		if result.err != nil {
			t.Fatalf("split paragraph failed: %v", result.err)
		}
		app.wordCommandResults <- result
		app.processAsyncResults()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for paragraph split")
	}

	// Verify that blocks now have 2 paragraphs and no error status
	blocks := state.wordBlocks
	if len(blocks) < 2 {
		t.Fatalf("expected at least 2 paragraphs after split, got %d", len(blocks))
	}
	if strings.Contains(app.status, "失败") {
		t.Fatalf("status reported failure: %s", app.status)
	}
}
