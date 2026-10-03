package desktopui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
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

func TestWordRibbonActionSlotsMatchPenNodeClpgm(t *testing.T) {
	want := map[wordToolbarTab][]int{
		wordTabFile:    {34, 76, 118, 179, 221, 263, 324, 366, 408, 469, 511, 553},
		wordTabHome:    {34, 76, 118, 160, 221, 263, 305, 347, 389, 431, 473, 515, 576, 618, 660, 702, 744, 786, 828, 870, 931, 973, 1015, 1076, 1118, 1160, 1202},
		wordTabInsert:  {34, 76, 118, 179, 240, 282, 324, 385, 427, 488, 530, 572, 633, 675, 736, 778},
		wordTabDesign:  {34, 76, 118, 160, 221, 263, 324, 366, 408},
		wordTabLayout:  {34, 76, 118, 160, 221, 263, 324, 366, 408, 469, 511, 553},
		wordTabReview:  {34, 76, 137, 179, 240, 282, 343, 385, 427, 488, 530},
		wordTabView:    {34, 76, 118, 179, 221, 263, 324, 366, 408, 469, 511, 572},
		wordTabPicture: {34, 76, 118, 179, 221},
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
		wordTabFile:   {166, 311, 456},
		wordTabHome:   {208, 563, 918, 1063},
		wordTabInsert: {166, 227, 372, 475, 620, 723},
		wordTabDesign: {208, 311},
		wordTabLayout: {208, 311, 456},
		wordTabReview: {124, 227, 330, 475},
		wordTabView:   {166, 311, 456, 559},
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
