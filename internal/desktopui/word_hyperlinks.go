package desktopui

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"

	"glance/internal/document"
	"glance/internal/i18n"
)

type wordHyperlinkClickTag struct{}

func wordHyperlinkClickTagForRun(state *documentSessionState, runID, target string) event.Tag {
	if state == nil || runID == "" || target == "" {
		return nil
	}
	if state.wordHyperlinkClickTags == nil {
		state.wordHyperlinkClickTags = make(map[string]*wordHyperlinkClickTag)
	}
	tag := state.wordHyperlinkClickTags[runID]
	if tag == nil {
		tag = new(wordHyperlinkClickTag)
		state.wordHyperlinkClickTags[runID] = tag
	}
	return tag
}

func wordHyperlinkActivationShortcut(modifiers key.Modifiers) bool {
	return modifiers.Contain(key.ModShortcut)
}

func observeWordHyperlinkShortcutKeys(gtx layout.Context, state *documentSessionState) {
	if state == nil {
		return
	}
	name := key.NameCtrl
	if key.ModShortcut == key.ModCommand {
		name = key.NameCommand
	}
	filter := key.Filter{
		Name:     name,
		Optional: key.ModCtrl | key.ModCommand | key.ModShift | key.ModAlt | key.ModSuper,
	}
	for value, ok := gtx.Event(filter); ok; value, ok = gtx.Event(filter) {
		event, ok := value.(key.Event)
		if !ok {
			continue
		}
		setWordHyperlinkShortcutDown(gtx, state, event.State == key.Press)
	}
}

func observeWordHyperlinkPointer(gtx layout.Context, state *documentSessionState, event pointer.Event) {
	if state == nil || event.Source != pointer.Mouse {
		return
	}
	setWordHyperlinkShortcutDown(gtx, state, wordHyperlinkActivationShortcut(event.Modifiers))
}

func setWordHyperlinkShortcutDown(gtx layout.Context, state *documentSessionState, down bool) {
	if state == nil || state.wordHyperlinkShortcutDown == down {
		return
	}
	state.wordHyperlinkShortcutDown = down
	gtx.Execute(op.InvalidateCmd{})
}

func wordExternalHyperlinkURL(target string) (string, error) {
	target = strings.TrimSpace(target)
	parsed, err := url.Parse(target)
	if err != nil {
		return "", fmt.Errorf("invalid hyperlink target: %w", err)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		if parsed.Host == "" {
			return "", errors.New("hyperlink URL has no host")
		}
	case "mailto":
		if parsed.Opaque == "" && parsed.Path == "" {
			return "", errors.New("mailto hyperlink has no address")
		}
	default:
		return "", fmt.Errorf("unsupported hyperlink protocol %q", parsed.Scheme)
	}
	return parsed.String(), nil
}

func wordHyperlinkTargetForRun(state *documentSessionState, runID string) string {
	if state == nil || runID == "" {
		return ""
	}
	run, _, ok := findEditableWordRunInState(state, runID)
	if !ok {
		return ""
	}
	return run.HyperlinkTarget
}

func wordHyperlinkRunTargetsForState(state *documentSessionState) map[string]string {
	if state == nil {
		return nil
	}
	revision := state.wordRevisionAtomic.Load()
	if state.wordHyperlinkRunTargets != nil && state.wordHyperlinkRunTargetsRevision == revision {
		return state.wordHyperlinkRunTargets
	}
	targets := make(map[string]string)
	addParagraph := func(paragraph document.WordParagraph) {
		for _, inline := range paragraph.Inlines {
			if inline.Text == nil || !inline.Text.Editable || inline.Text.HyperlinkTarget == "" {
				continue
			}
			targets[inline.Text.ID] = inline.Text.HyperlinkTarget
		}
	}
	for _, block := range wordBlocksForState(state) {
		if block.Paragraph != nil {
			addParagraph(*block.Paragraph)
		}
		if block.Table == nil {
			continue
		}
		for _, row := range block.Table.Rows {
			for _, cell := range row.Cells {
				for _, paragraph := range cell.Paragraphs {
					addParagraph(paragraph)
				}
			}
		}
	}
	state.wordHyperlinkRunTargets = targets
	state.wordHyperlinkRunTargetsRevision = revision
	return targets
}

func (a *App) activateWordHyperlink(gtx layout.Context, state *documentSessionState, target string) {
	if strings.HasPrefix(target, "#") {
		if a.scrollToWordBookmark(gtx, state, strings.TrimPrefix(target, "#")) {
			return
		}
		a.setStatusMessage(i18n.Message{ID: "common.feedback.operation_failed"}, feedbackWarning, notificationToast)
		return
	}
	url, err := wordExternalHyperlinkURL(target)
	if err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, err, notificationToast)
		return
	}
	launcher := a.wordHyperlinkLauncher
	if launcher == nil {
		launcher = launchWordExternalHyperlink
	}
	if !a.launchManagedBackground(func() {
		if err := launcher(url); err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, err, notificationToast)
		}
	}) {
		a.setStatusMessage(i18n.Message{ID: "common.feedback.operation_failed"}, feedbackError, notificationToast)
	}
}

func launchWordExternalHyperlink(target string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", "--", target)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	if err := command.Run(); err != nil {
		return fmt.Errorf("open Word hyperlink: %w", err)
	}
	return nil
}

func (a *App) scrollToWordBookmark(gtx layout.Context, state *documentSessionState, name string) bool {
	if state == nil || name == "" {
		return false
	}
	blocks := wordBlocksForState(state)
	for blockIndex, block := range blocks {
		paragraphs := make([]document.WordParagraph, 0, 1)
		if block.Paragraph != nil {
			paragraphs = append(paragraphs, *block.Paragraph)
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					paragraphs = append(paragraphs, cell.Paragraphs...)
				}
			}
		}
		for _, paragraph := range paragraphs {
			if !containsWordBookmark(paragraph.BookmarkNames, name) {
				continue
			}
			var runID string
			for _, inline := range paragraph.Inlines {
				if inline.Text != nil && inline.Text.Editable {
					runID = inline.Text.ID
					break
				}
			}
			if runID == "" {
				if state.wordViewMode != wordViewWebLayout {
					return false
				}
				state.wordListState.List.ScrollTo(blockIndex)
			} else if state.wordViewMode != wordViewWebLayout {
				state.wordPagePendingScrollRunID = runID
			} else {
				state.wordListState.List.ScrollTo(blockIndex)
			}
			gtx.Execute(op.InvalidateCmd{})
			return true
		}
	}
	return false
}

func containsWordBookmark(bookmarks []string, name string) bool {
	for _, bookmark := range bookmarks {
		if bookmark == name {
			return true
		}
	}
	return false
}
