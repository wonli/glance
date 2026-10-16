package desktopui

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
		a.wordVersionHistoryError = "请先保存文档，再查看版本历史"
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
		a.wordVersionHistoryError = "读取版本历史失败：" + result.err.Error()
		a.setStatus(a.wordVersionHistoryError)
	} else {
		a.wordVersionHistoryError = ""
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
	label := wordVersionHistoryRowText(version)
	a.modal.Show("恢复历史版本？", "将文档恢复到“"+label+"”。恢复前会保留当前完整 DOCX；恢复后可使用撤销回到当前内容。", func(action string) {
		a.modal.Hide()
		if action != "confirm" {
			return
		}
		if a.startRestoreWordVersion(a.documentSessionState, version.ID) {
			a.closeWordVersionHistory()
		}
	}, modalDialogActionSpec{ID: "cancel", Label: "取消"}, modalDialogActionSpec{ID: "confirm", Label: "恢复"})
}

func wordVersionSourceLabel(source workspace.WordVersionSource) string {
	switch source {
	case workspace.WordVersionOpened:
		return "打开文档"
	case workspace.WordVersionManualSave:
		return "手动保存"
	case workspace.WordVersionAutoSave:
		return "自动保存"
	case workspace.WordVersionRestoreBefore:
		return "恢复前保护"
	default:
		return "版本快照"
	}
}

func wordVersionHistoryRowText(version workspace.WordVersion) string {
	return fmt.Sprintf("%s · %s · %d 字节", wordVersionSourceLabel(version.Source), version.CreatedAt.Local().Format("2006-01-02 15:04:05"), version.SizeBytes)
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
				semantic.DescriptionOp("关闭版本历史").Add(gtx.Ops)
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
			label := material.H6(a.theme, "版本历史")
			label.Color = color.NRGBA{R: 39, G: 45, B: 54, A: 255}
			return label.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := gtx.Dp(unit.Dp(30))
			gtx.Constraints.Min = image.Pt(size, size)
			gtx.Constraints.Max = gtx.Constraints.Min
			semantic.Button.Add(gtx.Ops)
			semantic.DescriptionOp("关闭版本历史").Add(gtx.Ops)
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
		label := material.Body1(a.theme, "正在读取版本历史…")
		label.Color = mutedText
		return layout.Center.Layout(gtx, label.Layout)
	}
	if a.wordVersionHistoryError != "" {
		label := material.Body1(a.theme, a.wordVersionHistoryError)
		label.Color = color.NRGBA{R: 171, G: 56, B: 50, A: 255}
		return layout.Center.Layout(gtx, label.Layout)
	}
	if len(a.wordVersionHistoryVersions) == 0 {
		label := material.Body1(a.theme, "当前文档还没有历史版本")
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
								label := material.Body1(a.theme, wordVersionSourceLabel(version.Source)+" · "+version.CreatedAt.Local().Format("2006-01-02 15:04:05"))
								label.Color = wordStatusText
								return label.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								label := material.Caption(a.theme, fmt.Sprintf("完整 DOCX 快照 · %d 字节", version.SizeBytes))
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
				label := material.Caption(a.theme, "恢复前会先保存当前版本；恢复后可撤销。")
				label.Color = mutedText
				return label.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				button := material.Button(a.theme, &a.wordVersionHistoryRestore, "恢复此版本")
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
			a.setStatus("请先完成当前 Word 操作，再恢复历史版本")
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
			a.setStatus("正在提交正文修改并恢复历史版本")
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
		a.setStatus("正在恢复历史版本")
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
			recordWordRestoreNotice(runtime, state, "恢复历史版本失败："+err.Error())
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
