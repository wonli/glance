package desktopui

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"glance/internal/i18n"
	"image"
	"image/color"
	"path/filepath"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/document"
	"glance/internal/format/docx"
	"glance/internal/workspace"
)

type wordVersionHistoryResult struct {
	state      *documentSessionState
	token      sessionOperationToken
	generation uint64
	pathKey    string
	revision   uint64
	versions   []workspace.WordVersion
	err        error
}

type wordVersionRestoreResult struct {
	state      *documentSessionState
	token      sessionOperationToken
	generation uint64
	pathKey    string
	version    workspace.WordVersion
	blocks     []document.WordBlock
	revision   uint64
	applied    bool
	err        error
}

func (a *App) openWordVersionHistory() {
	state := a.documentSessionState
	if state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return
	}
	path := sessionPath(state)
	pathKey := canonicalSessionPath(path)
	if pathKey == "" {
		a.wordVersionHistoryVisible = true
		a.wordVersionHistoryLoading = false
		a.wordVersionHistoryError = "pending"
		a.wordVersionHistoryErrorRef = i18n.Message{ID: "word.history.error.save_before_view"}
		a.setStatusMessage(a.wordVersionHistoryErrorRef, feedbackWarning, notificationToast)
		return
	}
	token, ok := state.beginOperation(sessionOperationRead)
	if !ok {
		return
	}
	a.showNewMenu = false
	a.wordVersionHistoryVisible = true
	a.wordVersionHistoryLoading = true
	a.wordVersionHistoryError = ""
	a.wordVersionHistoryErrorRef = i18n.Message{}
	a.wordVersionHistoryPathKey = pathKey
	a.wordVersionHistoryVersions = nil
	a.wordVersionHistorySelectedID = 0
	a.wordVersionHistoryRowButtons = make(map[int64]*widget.Clickable)
	runtime := a.ensureDesktopRuntime()
	generation := a.documentGeneration
	a.launchManagedBackground(func() {
		versions, err := runtime.listWordVersions(pathKey)
		result := wordVersionHistoryResult{
			state: state, token: token, generation: generation, pathKey: pathKey,
			revision: state.wordRevisionAtomic.Load(), versions: versions, err: err,
		}
		state.finishOperation(token)
		if a.wordVersionHistoryResults == nil {
			return
		}
		select {
		case a.wordVersionHistoryResults <- result:
			if a.window != nil {
				a.window.Invalidate()
			}
		case <-a.shutdown:
		}
	})
}

func (a *App) applyWordVersionHistoryResult(result wordVersionHistoryResult) {
	if result.state == nil || result.state != a.documentSessionState || result.generation != a.documentGeneration || !result.state.acceptsResult(result.token) {
		return
	}
	if result.pathKey == "" || result.pathKey != canonicalSessionPath(sessionPath(result.state)) || result.pathKey != a.wordVersionHistoryPathKey {
		return
	}
	if result.revision != result.state.wordRevisionAtomic.Load() && a.wordVersionHistoryVisible {
		a.openWordVersionHistory()
		return
	}
	a.wordVersionHistoryLoading = false
	if result.err != nil {
		a.wordVersionHistoryError = "unavailable"
		a.wordVersionHistoryErrorRef = i18n.Message{ID: "word.history.error.read"}
		a.setStatusDetailMessage(a.wordVersionHistoryErrorRef, feedbackWarning, notificationToast, result.err.Error())
	} else {
		a.wordVersionHistoryError = ""
		a.wordVersionHistoryErrorRef = i18n.Message{}
		a.wordVersionHistoryVersions = append([]workspace.WordVersion(nil), result.versions...)
		a.wordVersionHistoryRowButtons = make(map[int64]*widget.Clickable, len(result.versions))
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *App) closeWordVersionHistory() {
	a.wordVersionHistoryVisible = false
	a.wordVersionHistoryLoading = false
	a.wordVersionHistoryError = ""
	a.wordVersionHistoryErrorRef = i18n.Message{}
	a.wordVersionHistorySelectedID = 0
}

func (a *App) requestWordVersionRestore() {
	var selected *workspace.WordVersion
	for index := range a.wordVersionHistoryVersions {
		version := &a.wordVersionHistoryVersions[index]
		if version.ID == a.wordVersionHistorySelectedID {
			selected = version
			break
		}
	}
	if selected == nil || a.documentSessionState == nil || a.documentSessionState.formatRunning.Load() || a.isSaving() {
		return
	}
	version := *selected
	label := a.wordVersionHistoryRowText(version)
	a.modal.ShowMessage("word.version_history.restore", i18n.Message{ID: "word.version_history.restore.dialog.title"}, i18n.Message{ID: "word.version_history.restore.dialog.body", Args: i18n.Args{"Version": label}}, func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		if a.startRestoreWordVersion(a.documentSessionState, version.ID) {
			a.closeWordVersionHistory()
		}
	}, modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}}, modalDialogActionSpec{ID: "confirm", LabelRef: i18n.Message{ID: "word.version_history.restore.action"}})
}

func (a *App) wordVersionSourceLabel(source workspace.WordVersionSource) string {
	switch source {
	case workspace.WordVersionOpened:
		return a.text(i18n.Message{ID: "word.version_history.source.opened"})
	case workspace.WordVersionManualSave:
		return a.text(i18n.Message{ID: "word.version_history.source.manual_save"})
	case workspace.WordVersionAutoSave:
		return a.text(i18n.Message{ID: "word.version_history.source.auto_save"})
	case workspace.WordVersionRestoreBefore:
		return a.text(i18n.Message{ID: "word.version_history.source.restore_protection"})
	default:
		return a.text(i18n.Message{ID: "word.version_history.source.snapshot"})
	}
}

func (a *App) wordVersionHistoryRowText(version workspace.WordVersion) string {
	locale := a.localeSnapshot().Resolved
	return a.text(i18n.Message{ID: "word.version_history.row", Args: i18n.Args{
		"Source": a.wordVersionSourceLabel(version.Source),
		"Date":   i18n.FormatDateTime(locale, version.CreatedAt.Local()),
		"Size":   i18n.FormatNumber(locale, float64(version.SizeBytes)),
	}})
}

func (a *App) layoutWordVersionHistory(gtx layout.Context) layout.Dimensions {
	if !a.wordVersionHistoryVisible {
		return layout.Dimensions{}
	}
	if a.wordVersionHistoryScrim.Clicked(gtx) {
		a.closeWordVersionHistory()
	}
	if a.wordVersionHistoryCloseButton.Clicked(gtx) {
		a.closeWordVersionHistory()
	}
	if a.wordVersionHistoryRestore.Clicked(gtx) {
		a.requestWordVersionRestore()
	}
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			return a.wordVersionHistoryScrim.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				bounds := image.Rectangle{Max: gtx.Constraints.Max}
				paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 52}, clip.Rect{Max: bounds.Max}.Op())
				semantic.Button.Add(gtx.Ops)
				semantic.DescriptionOp(a.textID("word.accessibility.close_history")).Add(gtx.Ops)
				return layout.Dimensions{Size: bounds.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			width := min(gtx.Dp(unit.Dp(620)), gtx.Constraints.Max.X)
			height := min(gtx.Dp(unit.Dp(500)), gtx.Constraints.Max.Y)
			if width <= 0 || height <= 0 {
				return layout.Dimensions{}
			}
			gtx.Constraints.Min = image.Pt(width, height)
			gtx.Constraints.Max = image.Pt(width, height)
			return layoutWordVersionHistoryPanel(gtx, a)
		}),
	)
}

func layoutWordVersionHistoryPanel(gtx layout.Context, a *App) layout.Dimensions {
	return layout.Background{}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			bounds := image.Rectangle{Max: gtx.Constraints.Max}
			shape := clip.UniformRRect(bounds, gtx.Dp(unit.Dp(9)))
			paint.FillShape(gtx.Ops, contentSurface, shape.Op(gtx.Ops))
			paint.FillShape(gtx.Ops, homeBorder, clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
			return layout.Dimensions{Size: bounds.Max}
		},
		func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(18), Bottom: unit.Dp(16), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceEnd}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutWordVersionHistoryHeader(gtx) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						caption := material.Caption(a.theme, filepath.Base(a.wordVersionHistoryPathKey))
						caption.Color = mutedText
						return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(10)}.Layout(gtx, caption.Layout)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						line := gtx.Dp(unit.Dp(1))
						paint.FillShape(gtx.Ops, gridLine, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, line)}.Op())
						return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, line)}
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return a.layoutWordVersionHistoryContents(gtx) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.layoutWordVersionHistoryFooter(gtx) }),
				)
			})
		},
	)
}

func (a *App) layoutWordVersionHistoryHeader(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			label := material.H6(a.theme, a.textID("word.ribbon.action.version_history"))
			label.Color = color.NRGBA{R: 39, G: 45, B: 54, A: 255}
			return label.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := gtx.Dp(unit.Dp(30))
			gtx.Constraints.Min = image.Pt(size, size)
			gtx.Constraints.Max = gtx.Constraints.Min
			semantic.Button.Add(gtx.Ops)
			semantic.DescriptionOp(a.textID("word.accessibility.close_history")).Add(gtx.Ops)
			return a.wordVersionHistoryCloseButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				label := material.Body1(a.theme, "×")
				label.Color = wordStatusText
				return layout.Center.Layout(gtx, label.Layout)
			})
		}),
	)
}

func (a *App) layoutWordVersionHistoryContents(gtx layout.Context) layout.Dimensions {
	if a.wordVersionHistoryLoading {
		label := material.Body1(a.theme, a.textID("word.version_history.loading"))
		label.Color = mutedText
		return layout.Center.Layout(gtx, label.Layout)
	}
	if a.wordVersionHistoryError != "" {
		label := material.Body1(a.theme, a.messageText(a.wordVersionHistoryErrorRef, a.wordVersionHistoryError))
		label.Color = color.NRGBA{R: 171, G: 56, B: 50, A: 255}
		return layout.Center.Layout(gtx, label.Layout)
	}
	if len(a.wordVersionHistoryVersions) == 0 {
		label := material.Body1(a.theme, a.textID("word.version_history.empty"))
		label.Color = mutedText
		return layout.Center.Layout(gtx, label.Layout)
	}
	a.wordVersionHistoryList.List.Axis = layout.Vertical
	return material.List(a.theme, &a.wordVersionHistoryList).Layout(gtx, len(a.wordVersionHistoryVersions), func(gtx layout.Context, index int) layout.Dimensions {
		version := a.wordVersionHistoryVersions[index]
		button := a.wordVersionHistoryRowButtons[version.ID]
		if button == nil {
			button = new(widget.Clickable)
			a.wordVersionHistoryRowButtons[version.ID] = button
		}
		if button.Clicked(gtx) {
			a.wordVersionHistorySelectedID = version.ID
		}
		locale := a.localeSnapshot().Resolved
		height := gtx.Dp(unit.Dp(62))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			selected := version.ID == a.wordVersionHistorySelectedID
			background := color.NRGBA{A: 0}
			if selected {
				background = selectedFill
			} else if button.Hovered() {
				background = chromeSurface
			}
			return layout.Background{}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					bounds := image.Rectangle{Max: gtx.Constraints.Max}
					paint.FillShape(gtx.Ops, background, clip.UniformRRect(bounds, gtx.Dp(unit.Dp(5))).Op(gtx.Ops))
					return layout.Dimensions{Size: bounds.Max}
				},
				func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(10), Top: unit.Dp(7), Bottom: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceBetween}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								label := material.Body1(a.theme, a.text(i18n.Message{ID: "word.version_history.row.source_date", Args: i18n.Args{"Source": a.wordVersionSourceLabel(version.Source), "Date": i18n.FormatDateTime(locale, version.CreatedAt.Local())}}))
								label.Color = wordStatusText
								return label.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								label := material.Caption(a.theme, a.text(i18n.Message{ID: "word.version_history.row.size", Args: i18n.Args{"Size": i18n.FormatNumber(locale, float64(version.SizeBytes))}}))
								label.Color = mutedText
								return label.Layout(gtx)
							}),
						)
					})
				},
			)
		})
	})
}

func (a *App) layoutWordVersionHistoryFooter(gtx layout.Context) layout.Dimensions {
	selected := a.wordVersionHistorySelectedID != 0
	return layout.Inset{Top: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				label := material.Caption(a.theme, a.textID("word.version_history.restore_hint"))
				label.Color = mutedText
				return label.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				button := material.Button(a.theme, &a.wordVersionHistoryRestore, a.textID("word.version_history.restore.action"))
				if !selected || a.documentSessionState == nil || a.documentSessionState.formatRunning.Load() || a.isSaving() {
					button.Background = color.NRGBA{R: 180, G: 185, B: 192, A: 255}
				}
				return button.Layout(gtx)
			}),
		)
	})
}

func (a *App) startRestoreWordVersion(state *documentSessionState, versionID int64) bool {
	return a.startRestoreWordVersionInternal(state, versionID, false)
}

func (a *App) startRestoreWordVersionAfterDrafts(state *documentSessionState, versionID int64) bool {
	return a.startRestoreWordVersionInternal(state, versionID, true)
}

func (a *App) startRestoreWordVersionInternal(state *documentSessionState, versionID int64, fromDraftFlush bool) bool {
	if a == nil || state == nil || state.word == nil || state.formatRunning.Load() || a.isSaving() {
		return false
	}
	if !fromDraftFlush && state.wordRestoreLaunchInProgress() {
		return false
	}
	if state.wordParagraphSplitIntent != nil || state.wordParagraphJoinIntent != nil || state.wordPageBreakIntent != nil ||
		state.wordSaveAfterDrafts || state.wordPDFExportAfterDrafts || state.wordPrintAfterDrafts ||
		state.wordInsertAfterDraftFlush || state.wordPendingPasteSelection != nil ||
		state.wordClipboardSelection != nil || state.wordClipboardRunID != "" || state.wordDeferredToolbarAction != "" {
		if state == a.documentSessionState {
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.finish_the_current_word_operation_a24d6f"}, feedbackWarning, notificationToast)
		}
		return false
	}
	if state.pendingWordRestoreAfterDrafts() != 0 {
		return false
	}
	if wordHasPendingTextDrafts(state) {
		if !state.queueWordRestoreAfterDrafts(versionID) {
			return false
		}
		state.wordFlushRequested.Store(true)
		state.wordFlushAllDrafts.Store(true)
		if state == a.documentSessionState {
			a.setStatusMessage(i18n.Message{ID: "word.status.inline.committing_text_edits_and_restoring_f4bd88"}, feedbackInfo, notificationToast)
		}
		a.startNextWordTextCommit()
		return true
	}
	token, ok := state.beginOperation(sessionOperationMutation)
	if !ok {
		return false
	}
	generation := a.documentGeneration
	path := sessionPath(state)
	pathKey := canonicalSessionPath(path)
	runtime := a.ensureDesktopRuntime()
	if state == a.documentSessionState {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.restoring_a_previous_version_68fe33"}, feedbackInfo, notificationToast)
	}
	if !a.launchManagedBackground(func() {
		result := wordVersionRestoreResult{
			state: state, token: token, generation: generation, pathKey: pathKey,
		}
		finish := func() {
			result.revision = state.wordRevisionAtomic.Load()
			state.finishOperation(token)
			state.releaseWordRestoreClaim()
			if a.wordVersionRestoreResults == nil {
				return
			}
			select {
			case a.wordVersionRestoreResults <- result:
				if a.window != nil {
					a.window.Invalidate()
				}
			case <-a.shutdown:
			}
		}
		fail := func(err error) {
			result.err = err
			recordWordRestoreMessage(runtime, state, i18n.Message{ID: "word.history.error.restore"})
			finish()
		}
		if pathKey == "" {
			fail(fmt.Errorf("Word 文档没有可用的历史路径"))
			return
		}
		version, snapshot, err := runtime.loadWordVersion(pathKey, versionID)
		if err != nil {
			fail(err)
			return
		}
		result.version = version
		digest := sha256.Sum256(snapshot)
		if version.PathKey != pathKey || hex.EncodeToString(digest[:]) != version.SHA256 {
			fail(fmt.Errorf("历史版本校验失败，文档没有更改"))
			return
		}
		candidate, err := docx.OpenSnapshot(snapshot)
		if err != nil {
			fail(fmt.Errorf("历史版本 DOCX 无法解析，文档没有更改：%w", err))
			return
		}
		_ = candidate.Close()

		var currentSnapshot []byte
		unlock := lockSessionAdapterRead(state)
		snapshotter, ok := any(state.word).(document.WordPackageSnapshotter)
		if !ok {
			err = fmt.Errorf("DOCX 适配器不支持完整包快照")
		} else {
			currentSnapshot, err = snapshotter.Snapshot()
		}
		unlock()
		if err != nil {
			fail(fmt.Errorf("无法保护当前 DOCX 版本：%w", err))
			return
		}
		if err := runtime.addWordSnapshot(path, workspace.WordVersionRestoreBefore, currentSnapshot); err != nil {
			fail(fmt.Errorf("无法保存恢复前保护版本，已取消恢复：%w", err))
			return
		}

		unlock = lockSessionAdapterWrite(state)
		restorer, ok := any(state.word).(document.WordPackageSnapshotRestorer)
		if !ok {
			err = fmt.Errorf("DOCX 适配器不支持历史版本恢复")
		} else {
			err = restorer.RestoreSnapshot(snapshot)
		}
		if err == nil {
			result.blocks = cloneWordBlockSnapshot(state.word.WordBlocks())
		}
		unlock()
		if err != nil {
			fail(fmt.Errorf("恢复历史版本失败：%w", err))
			return
		}
		state.setWordBlocksAndTextState(result.blocks, true)
		runtime.publishSessionRevision(token.documentID, max(token.revision+1, state.wordRevisionAtomic.Load()))
		result.applied = true
		finish()
	}) {
		state.finishOperation(token)
		return false
	}
	return true
}
