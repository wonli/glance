package desktopui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"glance/internal/application"
	"glance/internal/document"
)

func (a *App) showWordNewCommentDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	run, hasRun := selectedWordRun(wordBlocksForState(state), state.selectedWordRunID)
	if !hasRun || !run.Editable || !wordBodyRunEditable(wordBlocksForState(state), run.ID) {
		a.setStatus("请先选择可编辑的正文文本，再添加批注")
		return
	}
	expectedRevision := state.wordRevisionAtomic.Load()
	if _, ok := any(state.word).(document.WordCommentEditor); !ok {
		a.setStatus("当前文档不支持原生 Word 批注")
		return
	}

	a.modal.ShowInput(
		"新建批注",
		"批注将附着到当前选中的正文文本运行。",
		"请核对此处表述是否准确",
		"输入批注内容",
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			commentText := strings.TrimSpace(a.modal.Input.Text())
			if commentText == "" {
				a.setStatus("批注内容不能为空")
				return
			}
			if !a.runWordApplicationCommandExpectedRevision("添加批注", "word.comment.add", map[string]any{"run_id": run.ID, "text": commentText}, expectedRevision) {
				a.setStatus("文档已变化或应用命令运行时不可用，请重新选择文本后添加批注")
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "添加批注"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func (a *App) showWordDeleteCommentDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	if !a.applicationSessionRegistered(state.documentID) {
		a.setStatus("当前文档的应用命令运行时不可用，无法读取批注")
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		a.setStatus("文档正在处理其他操作，请稍后重试")
		return
	}
	generation := a.documentGeneration
	a.launchManagedBackground(func() {
		result, err := a.executeRibbonApplicationCommandWithContext(context.Background(), token, "word.comments.list", "word_document", token.documentID, "", map[string]any{"start": 0, "limit": 100})
		comments := make([]document.WordComment, 0)
		total := 0
		if err == nil {
			var payload struct {
				Total    int `json:"total"`
				Comments []struct {
					ID     string `json:"id"`
					Author string `json:"author"`
					Text   string `json:"text"`
					RunID  string `json:"run_id"`
				} `json:"comments"`
			}
			if json.Unmarshal(result.Data, &payload) != nil {
				err = application.ErrInvalidCommandResult
			} else {
				total = payload.Total
				for _, item := range payload.Comments {
					comments = append(comments, document.WordComment{ID: item.ID, Author: item.Author, Text: item.Text, RunID: item.RunID})
				}
			}
		}
		revision := uint64(0)
		if err == nil {
			revision = result.Revision
		}
		a.publishWordCommand(wordCommandResult{
			state: state, operationToken: token, revision: revision, generation: generation,
			operation: "读取 Word 批注", applicationCommand: true, wordCommentList: true,
			wordComments: comments, wordCommentTotal: total, err: err,
		})
	})
	a.setStatus("正在读取 Word 批注…")
}

func (a *App) showWordDeleteCommentPrompt(comments []document.WordComment, total int, expectedRevision uint64) {
	var listing strings.Builder
	visible := len(comments)
	if visible > 30 {
		visible = 30
	}
	for _, comment := range comments[:visible] {
		text := strings.Join(strings.Fields(comment.Text), " ")
		text = truncateWordCommentPreview(text, 96)
		fmt.Fprintf(&listing, "\n%s · %s：%s", comment.ID, comment.Author, text)
	}
	if total > visible {
		fmt.Fprintf(&listing, "\n当前列出 %d/%d 条；其余批注可通过 MCP word.comments.list 分页查询。", visible, total)
	}
	initial := ""
	if total == 1 && len(comments) == 1 {
		initial = comments[0].ID
	}
	a.modal.ShowInput(
		"删除批注",
		"输入要删除的批注 ID："+listing.String(),
		initial,
		"批注 ID",
		func(action string) {
			a.modal.Hide()
			if action != "confirm" {
				return
			}
			commentID := strings.TrimSpace(a.modal.Input.Text())
			found := false
			for _, comment := range comments {
				if comment.ID == commentID {
					found = true
					break
				}
			}
			if !found {
				a.setStatus("批注 ID 不在当前列表中，请重新打开删除批注")
				return
			}
			if !a.runWordApplicationCommandExpectedRevision("删除批注", "word.comment.delete", map[string]any{"comment_id": commentID}, expectedRevision) {
				a.setStatus("文档已变化或应用命令运行时不可用，请重新打开删除批注")
			}
		},
		modalDialogActionSpec{ID: "confirm", Label: "删除"},
		modalDialogActionSpec{ID: "cancel", Label: "取消"},
	)
}

func truncateWordCommentPreview(value string, maxRunes int) string {
	if !utf8.ValidString(value) {
		value = strings.ToValidUTF8(value, "�")
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "…"
}
