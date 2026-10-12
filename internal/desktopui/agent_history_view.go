package desktopui

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"glance/internal/agent"
)

type agentHistoryRow struct {
	item  agent.ConversationSummary
	group string
}

func (a *App) agentHistoryRows(now time.Time) []agentHistoryRow {
	query := strings.ToLower(strings.TrimSpace(a.agentUI.historySearch.Text()))
	rows := []agentHistoryRow{}
	previous := ""
	for _, item := range a.agentUI.conversationHistory {
		if query != "" && !strings.Contains(strings.ToLower(item.Title), query) {
			continue
		}
		group := agentHistoryDateGroup(item.UpdatedAt, now)
		row := agentHistoryRow{item: item}
		if group != previous {
			row.group = group
			previous = group
		}
		rows = append(rows, row)
	}
	return rows
}

func agentHistoryDateGroup(at, now time.Time) string {
	at = at.In(now.Location())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if !at.Before(today) {
		return "今天"
	}
	if !at.Before(today.AddDate(0, 0, -1)) {
		return "昨天"
	}
	if !at.Before(today.AddDate(0, 0, -7)) {
		return "过去 7 天"
	}
	return "更早"
}

func (a *App) layoutAgentHistorySidebar(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
	paint.FillShape(gtx.Ops, agentSidebarFill, clip.Rect{Max: gtx.Constraints.Max}.Op())
	paint.FillShape(gtx.Ops, agentSidebarBorder, clip.Rect{Min: image.Pt(gtx.Constraints.Max.X-gtx.Dp(1), 0), Max: gtx.Constraints.Max}.Op())
	return layout.Inset{Top: unit.Dp(13), Bottom: unit.Dp(16), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(a.layoutAgentHistoryHeader),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(17), Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = gtx.Dp(36)
					gtx.Constraints.Max.Y = gtx.Dp(36)
					return homeRoundedPanel(gtx, agentSidebarInputFill, agentSidebarInputLine, 6, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return homeIcon(gtx, "search", 14, agentSidebarMuted) }),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										settingsFieldFocus(gtx, &a.agentUI.historySearch)
										gtx.Constraints.Min.Y = 0
										e := material.Editor(a.theme, &a.agentUI.historySearch, "搜索对话标题")
										e.TextSize = 12
										e.Color = agentSidebarText
										e.HintColor = agentSidebarMuted
										return layout.Inset{Top: 3}.Layout(gtx, e.Layout)
									})
								}),
							)
						})
					})
				})
			}),
			layout.Flexed(1, a.layoutAgentConversationHistory),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: 16, Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.agentHistoryLabel(gtx, fmt.Sprintf("当前文档 · %d 个会话", len(a.agentUI.conversationHistory)), 10, agentSidebarMuted)
				})
			}),
		)
	})
}

func (a *App) layoutAgentHistoryHeader(gtx layout.Context) layout.Dimensions {
	if !a.modal.Visible && a.agentUI.historyBackButton.Clicked(gtx) {
		a.agentUI.conversationHistoryVisible = false
		if a.agentUI.conversationID == "" && !a.agentUI.conversationHistoryLoading {
			a.createAgentConversation()
		}
	}
	if !a.modal.Visible && a.agentUI.newConversationButton.Clicked(gtx) {
		a.createAgentConversation()
	}
	return layout.Inset{Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		height := gtx.Dp(unit.Dp(30))
		gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutAgentSidebarIconButton(gtx, a.theme, &a.agentUI.historyBackButton, "arrow-left", "返回聊天", false)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				return layout.Inset{Top: homeTextOpticalCenterTopInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.agentHistoryLabel(gtx, "历史对话", 13, agentSidebarText)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layoutAgentSidebarIconButton(gtx, a.theme, &a.agentUI.newConversationButton, "square-pen", "新建会话", false)
			}),
		)
	})
}

func (a *App) agentHistoryLabel(gtx layout.Context, text string, size unit.Sp, c color.NRGBA) layout.Dimensions {
	label := material.Label(a.theme, size, text)
	label.Color = c
	label.MaxLines = 1
	return label.Layout(gtx)
}

func (a *App) layoutAgentConversationHistory(gtx layout.Context) layout.Dimensions {
	state := &a.agentUI
	if !state.conversationHistoryVisible {
		return layout.Dimensions{}
	}
	now := gtx.Now
	if now.IsZero() {
		now = time.Now()
	}
	rows := a.agentHistoryRows(now)
	message := ""
	if state.conversationHistoryLoading && len(state.conversationHistory) == 0 {
		message = "正在加载会话历史…"
	} else if state.conversationHistoryError != "" && len(rows) == 0 {
		message = state.conversationHistoryError
	} else if len(rows) == 0 {
		message = "此文档还没有历史会话。"
		if strings.TrimSpace(state.historySearch.Text()) != "" {
			message = "没有匹配的会话。"
		}
	}
	if message != "" {
		return layout.NW.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			label := material.Label(a.theme, 12, message)
			label.Color = agentSidebarMuted
			return label.Layout(gtx)
		})
	}
	if state.conversationHistoryError != "" {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Label(a.theme, 12, state.conversationHistoryError)
				label.Color = color.NRGBA{R: 174, G: 85, B: 85, A: 255}
				return layout.Inset{Bottom: 8}.Layout(gtx, label.Layout)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return a.layoutAgentHistoryListRows(gtx, rows, now) }),
		)
	}
	return a.layoutAgentHistoryListRows(gtx, rows, now)
}

func (a *App) layoutAgentHistoryListRows(gtx layout.Context, rows []agentHistoryRow, now time.Time) layout.Dimensions {
	state := &a.agentUI
	state.conversationHistoryList.Axis = layout.Vertical
	style := material.List(a.theme, &state.conversationHistoryList)
	style.AnchorStrategy = material.Overlay
	return style.Layout(gtx, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
		row := rows[index]
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if row.group == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: 4, Bottom: 6, Left: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.agentHistoryLabel(gtx, row.group, 10, agentSidebarMuted)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return a.layoutAgentHistoryRow(gtx, row.item, now) })
			}),
		)
	})
}

func (a *App) layoutAgentHistoryRow(gtx layout.Context, item agent.ConversationSummary, now time.Time) layout.Dimensions {
	state := &a.agentUI
	if state.conversationHistoryButtons == nil {
		state.conversationHistoryButtons = make(map[agent.ConversationID]*widget.Clickable)
	}
	if state.conversationHistoryDeleteButtons == nil {
		state.conversationHistoryDeleteButtons = make(map[agent.ConversationID]*widget.Clickable)
	}
	selectButton := state.conversationHistoryButtons[item.ID]
	if selectButton == nil {
		selectButton = new(widget.Clickable)
		state.conversationHistoryButtons[item.ID] = selectButton
	}
	deleteButton := state.conversationHistoryDeleteButtons[item.ID]
	if deleteButton == nil {
		deleteButton = new(widget.Clickable)
		state.conversationHistoryDeleteButtons[item.ID] = deleteButton
	}
	if !a.modal.Visible && deleteButton.Clicked(gtx) {
		a.confirmAgentConversationDelete(item)
	} else if !a.modal.Visible && selectButton.Clicked(gtx) && !state.conversationHistoryLoading && state.conversationDeletes[item.ID] == "" {
		a.loadAgentConversation(item.ID)
	}
	hovered := selectButton.Hovered() || deleteButton.Hovered()
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(56))
	gtx.Constraints = layout.Exact(size)
	fill := agentSidebarFill
	if item.ID == state.conversationID {
		fill = color.NRGBA{R: 221, G: 229, B: 239, A: 255}
	} else if hovered {
		fill = color.NRGBA{R: 227, G: 231, B: 237, A: 255}
	}
	return homeRoundedPanel(gtx, fill, fill, 6, func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{Alignment: layout.E}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(size)
				return selectButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: 10, Right: 42}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = 0
							label := strings.TrimSpace(item.Title)
							if label == "" {
								label = "新会话"
							}
							stamp := item.UpdatedAt.In(now.Location()).Format("15:04")
							switch agentHistoryDateGroup(item.UpdatedAt, now) {
							case "昨天":
								stamp = "昨天 " + stamp
							case "过去 7 天", "更早":
								stamp = item.UpdatedAt.In(now.Location()).Format("01/02 15:04")
							}
							if item.ID == state.conversationID {
								stamp += " · 当前会话"
							}
							if state.conversationDeletes[item.ID] != "" {
								stamp = "正在删除…"
							}
							return layout.Inset{Top: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return a.agentHistoryLabel(gtx, label, 12, agentSidebarText)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return layout.Inset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return a.agentHistoryLabel(gtx, stamp, 10, agentSidebarMuted)
										})
									}),
								)
							})
						})
					})
				})
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				if !hovered || state.conversationDeletes[item.ID] != "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(26), gtx.Dp(26)))
					return deleteButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return homeRoundedPanel(gtx, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, 4, func(gtx layout.Context) layout.Dimensions {
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return homeIcon(gtx, "trash-2", 14, color.NRGBA{R: 166, G: 109, B: 109, A: 255})
							})
						})
					})
				})
			}),
		)
	})
}
