package desktopui

import (
	"hash/fnv"
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (a *App) layoutAgentSidebarMessages(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min = gtx.Constraints.Max
	originalConstraints := gtx.Constraints
	toggled := a.processAgentSidebarProcessClicks(gtx)
	groups := agentSidebarMessageGroups(a.agentUI.chatLines)
	list := &a.agentUI.chatList.List
	list.Axis = layout.Vertical
	if len(groups) == 0 {
		list.Position.Length = 0
	}
	prevLength := list.Position.Length
	// Follow the end while the user has not scrolled back, but let short
	// conversations use the normal top alignment instead of bottom padding.
	follow := list.ScrollToEnd
	if toggled {
		if len(groups) > 0 {
			list.Position.First = max(0, min(len(groups)-1, list.Position.First))
			list.Position.Offset = max(0, list.Position.Offset)
			list.Position.BeforeEnd = true
			list.ScrollToEnd = false
		}
		gtx.Execute(op.InvalidateCmd{})
	} else {
		// Only disable Gio's end alignment when all content fits. Long
		// conversations retain its native scroll handling, including a scroll
		// arriving in the same frame as streamed text or newly visible actions.
		short := list.Position.Length < gtx.Constraints.Max.Y && !list.Position.BeforeEnd
		if follow && short {
			list.Position.First = len(groups)
			list.Position.Offset = 0
			list.ScrollToEnd = false
		}
	}
	defer func() { list.ScrollToEnd = follow }()
	lastAssistant := lastAgentAssistantLine(a.agentUI.chatLines)
	now := gtx.Now
	if now.IsZero() {
		now = time.Now()
	}
	stamp := a.agentSidebarConversationStamp(now)

	if len(a.agentUI.chatGroupHeights) < len(groups) {
		grown := make([][2]agentChatGroupHeightEntry, len(groups))
		copy(grown, a.agentUI.chatGroupHeights)
		a.agentUI.chatGroupHeights = grown
	} else {
		a.agentUI.chatGroupHeights = a.agentUI.chatGroupHeights[:len(groups)]
	}
	heights := make([]int, len(groups))
	laidOut := make([]bool, len(groups))
	sigs := make([]uint64, len(groups))
	for i, grp := range groups {
		sigs[i] = a.agentSidebarGroupSignature(i, grp, lastAssistant, stamp, now)
	}

	listLen := 0
	if len(groups) > 0 {
		listLen = len(groups) + 1
		list.Position.First++
		list.Position.OffsetLast = -1_000_000
	}
	listDims := list.Layout(gtx, listLen, func(gtx layout.Context, rawIndex int) layout.Dimensions {
		if rawIndex == 0 {
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
		}
		index := rawIndex - 1
		rowWidth := gtx.Constraints.Max.X
		dims := a.layoutAgentSidebarMessageGroupRow(gtx, index, groups[index], lastAssistant, stamp)
		heights[index] = dims.Size.Y
		laidOut[index] = true
		a.storeAgentSidebarGroupHeight(index, agentChatGroupHeightEntry{
			width:     rowWidth,
			pxPerDp:   gtx.Metric.PxPerDp,
			pxPerSp:   gtx.Metric.PxPerSp,
			signature: sigs[index],
			height:    dims.Size.Y,
		})
		return dims
	})
	if list.Position.First > 0 {
		list.Position.First--
	}

	rowWidth := listDims.Size.X
	for i, grp := range groups {
		if laidOut[i] {
			continue
		}
		if h, ok := a.lookupAgentSidebarGroupHeight(i, rowWidth, gtx.Metric.PxPerDp, gtx.Metric.PxPerSp, sigs[i]); ok {
			heights[i] = h
			continue
		}
		h := a.measureAgentSidebarMessageGroupOffscreen(gtx, rowWidth, i, grp, lastAssistant, stamp)
		heights[i] = h
		a.storeAgentSidebarGroupHeight(i, agentChatGroupHeightEntry{
			width:     rowWidth,
			pxPerDp:   gtx.Metric.PxPerDp,
			pxPerSp:   gtx.Metric.PxPerSp,
			signature: sigs[i],
			height:    h,
		})
	}

	totalHeight := 0
	for _, h := range heights {
		totalHeight += h
	}
	list.Position.Length = totalHeight
	if prevLength < listDims.Size.Y && totalHeight > listDims.Size.Y {
		gtx.Execute(op.InvalidateCmd{})
	}

	scrollbarStyle := material.Scrollbar(a.theme, &a.agentUI.chatList.Scrollbar)
	start, end := float32(0), float32(1)
	maxScroll := totalHeight - listDims.Size.Y
	if len(groups) > 0 && maxScroll > 0 && listDims.Size.Y > 0 {
		scrollTop := maxScroll
		if list.Position.BeforeEnd {
			scrollTop = list.Position.Offset
			first := max(0, min(len(groups), list.Position.First))
			for i := 0; i < first; i++ {
				scrollTop += heights[i]
			}
			scrollTop = max(0, min(maxScroll, scrollTop))
		}
		majorPaddingPx := gtx.Dp(scrollbarStyle.Track.MajorPadding)
		trackLen := listDims.Size.Y - 2*majorPaddingPx
		if trackLen > 1 {
			minThumbPx := min(trackLen-1, max(1, gtx.Dp(scrollbarStyle.Indicator.MajorMinLen)))
			rawThumbPx := int(math.Round(float64(listDims.Size.Y) * float64(trackLen) / float64(totalHeight)))
			thumbPx := max(minThumbPx, min(trackLen-1, rawThumbPx))
			travelPx := trackLen - thumbPx
			startPx := int(math.Round(float64(scrollTop) * float64(travelPx) / float64(maxScroll)))
			startPx = max(0, min(travelPx, startPx))
			start = float32(startPx) / float32(trackLen)
			end = float32(startPx+thumbPx) / float32(trackLen)
		}
	}

	gtx.Constraints = originalConstraints
	gtx.Constraints.Min = listDims.Size
	gtx.Constraints.Max = listDims.Size
	layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return scrollbarStyle.Layout(gtx, list.Axis, start, end)
	})

	if delta := a.agentUI.chatList.ScrollDistance(); delta != 0 && maxScroll > 0 {
		travelFrac := 1 - (end - start)
		if travelFrac > 0 {
			progress := (start + delta) / travelFrac
			if progress < 0 {
				progress = 0
			} else if progress > 1 {
				progress = 1
			}
			targetScrollTop := int(math.Round(float64(progress) * float64(maxScroll)))
			applyAgentSidebarScrollTop(list, heights, targetScrollTop, maxScroll)
			gtx.Execute(op.InvalidateCmd{})
		}
	}

	return listDims
}

func (a *App) processAgentSidebarProcessClicks(gtx layout.Context) bool {
	toggled := false
	lines := a.agentUI.chatLines
	for i := range lines {
		if !isAgentProcessLine(lines[i]) {
			continue
		}
		indices := collectSubTurnProcessLines(lines, i)
		if len(indices) == 0 || indices[0] != i {
			continue
		}
		collapsed := agentProcessGroupCollapsed(lines, indices)
		toggleOwner := -1
		for _, idx := range indices {
			if a.agentUI.chatLines[idx].toggle != nil {
				toggleOwner = idx
				break
			}
		}
		if toggleOwner < 0 {
			toggleOwner = indices[0]
			a.agentUI.chatLines[toggleOwner].toggle = new(widget.Clickable)
			a.agentUI.chatLines[toggleOwner].collapsed = collapsed
		}
		if a.agentUI.chatLines[toggleOwner].toggle.Clicked(gtx) {
			collapsed = !collapsed
			for _, idx := range indices {
				if a.agentUI.chatLines[idx].toggle == nil {
					a.agentUI.chatLines[idx].toggle = new(widget.Clickable)
				}
				a.agentUI.chatLines[idx].collapsed = collapsed
			}
			toggled = true
		}
	}
	return toggled
}

func (a *App) lookupAgentSidebarGroupHeight(index, width int, pxPerDp, pxPerSp float32, sig uint64) (int, bool) {
	for _, entry := range a.agentUI.chatGroupHeights[index] {
		if entry.width == width &&
			entry.pxPerDp == pxPerDp &&
			entry.pxPerSp == pxPerSp &&
			entry.signature == sig &&
			entry.height > 0 {
			return entry.height, true
		}
	}
	return 0, false
}

func (a *App) storeAgentSidebarGroupHeight(index int, entry agentChatGroupHeightEntry) {
	slots := &a.agentUI.chatGroupHeights[index]
	if slots[0].signature == entry.signature &&
		slots[0].width == entry.width &&
		slots[0].pxPerDp == entry.pxPerDp &&
		slots[0].pxPerSp == entry.pxPerSp {
		slots[0] = entry
		return
	}
	slots[1] = slots[0]
	slots[0] = entry
}

func (a *App) agentSidebarConversationStamp(now time.Time) string {
	if a.agentUI.conversationStartedAt.IsZero() {
		return ""
	}
	at := a.agentUI.conversationStartedAt.In(now.Location())
	stamp := at.Format("01/02 15:04")
	if group := agentHistoryDateGroup(at, now); group == "今天" || group == "昨天" {
		stamp = group + " " + at.Format("15:04")
	}
	return stamp
}

func (a *App) layoutAgentSidebarMessageGroupRow(gtx layout.Context, index int, group [2]int, lastAssistant int, stamp string) layout.Dimensions {
	return layout.Inset{Left: 16, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if index != 0 || stamp == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: 12, Bottom: 15}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return layout.N.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = 0
						label := material.Label(a.theme, 10, stamp)
						label.Color = color.NRGBA{R: 147, G: 157, B: 169, A: 255}
						return label.Layout(gtx)
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutAgentSidebarMessageGroup(gtx, group, lastAssistant)
			}),
		)
	})
}

func (a *App) measureAgentSidebarMessageGroupOffscreen(gtx layout.Context, width, index int, group [2]int, lastAssistant int, stamp string) int {
	savedCopy := a.agentUI.copyResponseButton
	savedRegen := a.agentUI.regenerateResponseButton
	savedInsert := a.agentUI.insertResponseButton
	type savedToggleState struct {
		row int
		val widget.Clickable
	}
	type savedCardState struct {
		card        *agentConfirmationCard
		approve     widget.Clickable
		reject      widget.Clickable
		alwaysAllow widget.Clickable
		allowOnce   widget.Clickable
	}
	var savedToggles []savedToggleState
	var savedCards []savedCardState
	for row := group[0]; row < group[1]; row++ {
		line := a.agentUI.chatLines[row]
		if line.toggle != nil {
			savedToggles = append(savedToggles, savedToggleState{row: row, val: *line.toggle})
		}
		if line.confirmationID != "" && a.agentUI.confirmations != nil {
			if card := a.agentUI.confirmations[line.confirmationID]; card != nil {
				savedCards = append(savedCards, savedCardState{
					card:        card,
					approve:     card.approve,
					reject:      card.reject,
					alwaysAllow: card.alwaysAllow,
					allowOnce:   card.allowOnce,
				})
			}
		}
	}
	defer func() {
		a.agentUI.copyResponseButton = savedCopy
		a.agentUI.regenerateResponseButton = savedRegen
		a.agentUI.insertResponseButton = savedInsert
		for _, st := range savedToggles {
			if a.agentUI.chatLines[st.row].toggle != nil {
				*a.agentUI.chatLines[st.row].toggle = st.val
			}
		}
		for _, sc := range savedCards {
			sc.card.approve = sc.approve
			sc.card.reject = sc.reject
			sc.card.alwaysAllow = sc.alwaysAllow
			sc.card.allowOnce = sc.allowOnce
		}
	}()

	measureGtx := gtx
	measureGtx.Source = input.Source{}
	measureGtx.Constraints.Min = image.Point{X: width, Y: 0}
	measureGtx.Constraints.Max = image.Point{X: width, Y: 1_000_000}
	rec := op.Record(measureGtx.Ops)
	dims := a.layoutAgentSidebarMessageGroupRow(measureGtx, index, group, lastAssistant, stamp)
	_ = rec.Stop()
	return dims.Size.Y
}

func (a *App) agentSidebarGroupSignature(index int, group [2]int, lastAssistant int, stamp string, now time.Time) uint64 {
	h := fnv.New64a()
	writeStr := func(s string) {
		_, _ = ioWriteString(h, s)
		_, _ = h.Write([]byte{0})
	}
	writeBool := func(b bool) {
		if b {
			_, _ = h.Write([]byte{1})
		} else {
			_, _ = h.Write([]byte{0})
		}
	}
	if index == 0 {
		writeStr(stamp)
	}
	writeBool(a.agentUI.conversationBusy)
	writeBool(lastAssistant >= group[0] && lastAssistant < group[1])
	for i := group[0]; i < group[1]; i++ {
		line := a.agentUI.chatLines[i]
		writeStr(line.role)
		writeStr(line.text)
		writeStr(line.toolName)
		writeStr(line.toolSummary)
		writeStr(line.confirmationID)
		writeBool(line.collapsed)
		writeBool(line.inProgress)
		writeBool(line.isError)
		writeBool(line.needsAuth)
		if isAgentProcessLine(line) {
			indices := collectSubTurnProcessLines(a.agentUI.chatLines, i)
			if len(indices) > 0 && indices[0] == i {
				writeBool(agentProcessGroupCollapsed(a.agentUI.chatLines, indices))
				writeStr(agentProcessGroupHeaderTitle(a.agentUI.chatLines, indices, now))
			}
		}
	}
	return h.Sum64()
}

func ioWriteString(h interface{ Write([]byte) (int, error) }, s string) (int, error) {
	return h.Write([]byte(s))
}

func applyAgentSidebarScrollTop(list *layout.List, heights []int, targetScrollTop, maxScroll int) {
	if len(heights) == 0 {
		return
	}
	targetScrollTop = max(0, min(maxScroll, targetScrollTop))
	rem := targetScrollTop
	for i, h := range heights {
		if rem < h || i == len(heights)-1 {
			list.Position.First = i
			list.Position.Offset = rem
			list.Position.BeforeEnd = true
			return
		}
		rem -= h
	}
}

func (a *App) layoutAgentSidebarMessageGroup(gtx layout.Context, group [2]int, lastAssistant int) layout.Dimensions {
	if a.agentUI.chatLines[group[0]].role == "user" {
		return a.layoutAgentSidebarMessage(gtx, group[0], false)
	}
	return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, group[1]-group[0]+1)
		hasAssistant := false
		for i := group[0]; i < group[1]; i++ {
			role := a.agentUI.chatLines[i].role
			if role == "assistant" || role == "reasoning" || role == "tool" {
				hasAssistant = true
				break
			}
		}
		if hasAssistant {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				label := material.Label(a.theme, 10, "AI 助手")
				label.Color = color.NRGBA{R: 122, G: 135, B: 151, A: 255}
				label.Font.Weight = 600
				return layout.Inset{Bottom: 4}.Layout(gtx, label.Layout)
			}))
		}
		for i := group[0]; i < group[1]; i++ {
			row := i
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return a.layoutAgentSidebarMessageContent(gtx, row, row == lastAssistant, false)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

// Tool and reasoning events split streamed replies into separate stored lines.
// Display the assistant title only on the first nonempty reply in each user turn.
func agentSidebarStartsAssistantTurn(lines []agentChatLine, index int) bool {
	for i := index - 1; i >= 0; i-- {
		if lines[i].role == "user" {
			return true
		}
		if lines[i].role == "assistant" && strings.TrimSpace(lines[i].text) != "" {
			return false
		}
	}
	return true
}

func (a *App) layoutAgentSidebarMessage(gtx layout.Context, index int, latest bool) layout.Dimensions {
	return a.layoutAgentSidebarMessageContent(gtx, index, latest, true)
}

func (a *App) layoutAgentSidebarMessageContent(gtx layout.Context, index int, latest, showAssistantTitle bool) layout.Dimensions {
	line := a.agentUI.chatLines[index]
	if line.role == "assistant" && strings.TrimSpace(line.text) == "" {
		return layout.Dimensions{}
	}
	if isAgentProcessLine(line) {
		indices := collectSubTurnProcessLines(a.agentUI.chatLines, index)
		if len(indices) == 0 || indices[0] != index {
			return layout.Dimensions{}
		}
		return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return a.layoutAgentSidebarProcessGroup(gtx, indices)
		})
	}
	user := line.role == "user"
	left, right := unit.Dp(0), unit.Dp(0)
	if user {
		left, right = unit.Dp(26), unit.Dp(0)
	}
	return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: left, Right: right}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		layoutLine := func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if user || (line.role == "assistant" && (!showAssistantTitle || !agentSidebarStartsAssistantTurn(a.agentUI.chatLines, index))) {
						return layout.Dimensions{}
					}
					role := "AI 助手"
					roleColor := color.NRGBA{R: 67, G: 90, B: 115, A: 255}
					switch line.role {
					case "user":
						role, roleColor = "你", color.NRGBA{R: 59, G: 71, B: 85, A: 255}
					case "error":
						role, roleColor = "错误", agentProcessErrorColor
					case "confirmation":
						role, roleColor = "人工确认", agentProcessAuthColor
					}
					style := material.Caption(a.theme, role)
					style.TextSize = unit.Sp(11)
					style.Font.Weight = 600
					style.Color = roleColor
					return style.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if line.role == "assistant" {
						top := unit.Dp(4)
						if showAssistantTitle && agentSidebarStartsAssistantTurn(a.agentUI.chatLines, index) {
							top = unit.Dp(9)
						}
						return layout.Inset{Top: top}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return agentMessageBubble(gtx, false, func(gtx layout.Context) layout.Dimensions {
								return layout.UniformInset(12).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return a.layoutAgentAssistantMarkdown(gtx, &a.agentUI.chatLines[index])
								})
							})
						})
					}
					body := material.Body1(a.theme, line.text)
					body.TextSize = unit.Sp(12)
					body.Color = agentSidebarText
					body.MaxLines = 0
					return layout.Inset{Top: unit.Dp(3)}.Layout(gtx, body.Layout)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !latest || line.role != "assistant" || a.agentUI.conversationBusy {
						return layout.Dimensions{}
					}
					return layout.Inset{Top: unit.Dp(7)}.Layout(gtx, a.layoutAgentSidebarResponseActions)
				}),
			)
		}
		if !user {
			return layoutLine(gtx)
		}
		return agentMessageBubble(gtx, true, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 12, Bottom: 12, Left: 14, Right: 14}.Layout(gtx, layoutLine)
		})
	})
}

func (a *App) layoutAgentSidebarProcessGroup(gtx layout.Context, indices []int) layout.Dimensions {
	collapsed := agentProcessGroupCollapsed(a.agentUI.chatLines, indices)
	toggleOwner := -1
	for _, idx := range indices {
		if a.agentUI.chatLines[idx].toggle != nil {
			toggleOwner = idx
			break
		}
	}
	if toggleOwner < 0 {
		toggleOwner = indices[0]
		a.agentUI.chatLines[toggleOwner].toggle = new(widget.Clickable)
		a.agentUI.chatLines[toggleOwner].collapsed = collapsed
	}
	button := a.agentUI.chatLines[toggleOwner].toggle
	if button.Clicked(gtx) {
		collapsed = !collapsed
		for _, idx := range indices {
			if a.agentUI.chatLines[idx].toggle == nil {
				a.agentUI.chatLines[idx].toggle = new(widget.Clickable)
			}
			a.agentUI.chatLines[idx].collapsed = collapsed
		}
	}
	title := agentProcessGroupHeaderTitle(a.agentUI.chatLines, indices, gtx.Now)
	return layout.Inset{Top: unit.Dp(2), Right: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return agentSidebarProcessDisclosure(gtx, a.theme, button, title, collapsed)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !collapsed {
					return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return a.layoutAgentSidebarProcessItems(gtx, indices)
					})
				}
				return a.layoutAgentSidebarCollapsedProcessStatus(gtx, indices)
			}),
		)
	})
}

func (a *App) layoutAgentSidebarProcessItems(gtx layout.Context, indices []int) layout.Dimensions {
	type processRun struct {
		role    string
		indices []int
	}
	runs := make([]processRun, 0, len(indices))
	for _, idx := range indices {
		role := a.agentUI.chatLines[idx].role
		if role == "reasoning" && strings.TrimSpace(a.agentUI.chatLines[idx].text) == "" {
			continue
		}
		if len(runs) > 0 && runs[len(runs)-1].role == role {
			runs[len(runs)-1].indices = append(runs[len(runs)-1].indices, idx)
		} else {
			runs = append(runs, processRun{role: role, indices: []int{idx}})
		}
	}
	if len(runs) == 0 {
		return layout.Dimensions{}
	}
	children := make([]layout.FlexChild, 0, len(runs))
	for runIndex, run := range runs {
		currentRun := run
		topInset := unit.Dp(0)
		if runIndex > 0 {
			topInset = unit.Dp(8)
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: topInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return agentProcessLeftBar(gtx, agentProcessLeftBarColor, func(gtx layout.Context) layout.Dimensions {
					if currentRun.role == "reasoning" {
						return a.layoutAgentSidebarReasoningRun(gtx, currentRun.indices)
					}
					return a.layoutAgentSidebarToolRun(gtx, currentRun.indices)
				})
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (a *App) layoutAgentSidebarReasoningRun(gtx layout.Context, indices []int) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(indices))
	for i, idx := range indices {
		text := strings.TrimSpace(a.agentUI.chatLines[idx].text)
		top := unit.Dp(1)
		if i > 0 {
			top = unit.Dp(6)
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: top, Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				body := material.Body1(a.theme, text)
				body.TextSize = unit.Sp(11)
				body.LineHeight = unit.Sp(18)
				body.Color = agentProcessPrimaryColor
				body.MaxLines = 0
				return body.Layout(gtx)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (a *App) layoutAgentSidebarToolRun(gtx layout.Context, indices []int) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(indices))
	for i, idx := range indices {
		line := a.agentUI.chatLines[idx]
		top := unit.Dp(0)
		if i > 0 {
			top = unit.Dp(10)
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: top}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutAgentSidebarToolRecord(gtx, line)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (a *App) layoutAgentSidebarToolRecord(gtx layout.Context, line agentChatLine) layout.Dimensions {
	title, summary := agentToolRecordDisplay(line)
	primaryColor := agentProcessPrimaryColor
	secondaryColor := agentProcessSecondaryColor
	if line.isError || strings.Contains(line.text, "失败") {
		primaryColor = agentProcessErrorColor
		secondaryColor = agentProcessErrorColor
	} else if line.needsAuth {
		primaryColor = agentProcessAuthColor
		secondaryColor = agentProcessAuthColor
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := material.Body1(a.theme, title)
			label.TextSize = unit.Sp(11)
			label.Color = primaryColor
			label.MaxLines = 0
			return label.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if summary == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				sub := material.Caption(a.theme, summary)
				sub.TextSize = unit.Sp(10)
				sub.Color = secondaryColor
				sub.MaxLines = 0
				return sub.Layout(gtx)
			})
		}),
	)
}

func (a *App) layoutAgentSidebarCollapsedProcessStatus(gtx layout.Context, indices []int) layout.Dimensions {
	hasConfirmationCard := len(indices) > 0 && subTurnHasConfirmationLine(a.agentUI.chatLines, indices[0])
	activeIndices := make([]int, 0, 2)
	alertIndices := make([]int, 0, 2)
	for _, idx := range indices {
		line := a.agentUI.chatLines[idx]
		if line.inProgress {
			activeIndices = append(activeIndices, idx)
			continue
		}
		if line.role == "tool" {
			failed := line.isError || strings.Contains(line.text, "失败")
			authNeeded := (line.needsAuth || strings.Contains(line.text, "等待批准") || strings.Contains(line.text, "需要授权")) && !hasConfirmationCard
			if failed || authNeeded {
				alertIndices = append(alertIndices, idx)
			}
		}
	}
	if len(activeIndices) == 0 && len(alertIndices) == 0 {
		return layout.Dimensions{}
	}
	children := make([]layout.FlexChild, 0, len(activeIndices)+len(alertIndices))
	if len(activeIndices) > 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return a.layoutAgentSidebarProcessItems(gtx, activeIndices)
			})
		}))
	}
	for _, idx := range alertIndices {
		line := a.agentUI.chatLines[idx]
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			barColor := agentProcessErrorBarColor
			if line.needsAuth && !line.isError && !strings.Contains(line.text, "失败") {
				barColor = agentProcessAuthBarColor
			}
			return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return agentProcessLeftBar(gtx, barColor, func(gtx layout.Context) layout.Dimensions {
					return a.layoutAgentSidebarToolRecord(gtx, line)
				})
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}
