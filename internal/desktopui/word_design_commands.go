package desktopui

import (
	"glance/internal/document"
	"glance/internal/i18n"
)

func (a *App) showWordThemesDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	a.modal.ShowMessage("word.design.themes", i18n.Message{ID: "word.design.themes.dialog.title"}, i18n.Message{ID: "word.design.themes.dialog.body"},
		func(action string) {
			a.modal.Hide()
			var before, after int64
			var themeName i18n.Message
			switch action {
			case "theme_office":
				before, after, themeName = 0, 120, i18n.Message{ID: "word.design.themes.option.office"}
			case "theme_business":
				before, after, themeName = 40, 200, i18n.Message{ID: "word.design.themes.option.business"}
			case "theme_academic":
				before, after, themeName = 0, 60, i18n.Message{ID: "word.design.themes.option.academic"}
			case "theme_modern":
				before, after, themeName = 60, 240, i18n.Message{ID: "word.design.themes.option.modern"}
			default:
				return
			}
			if !a.runWordApplicationCommand(a.text(i18n.Message{ID: "word.design.themes.operation", Args: i18n.Args{"Theme": a.text(themeName)}}), "word.paragraph.spacing_all.set", worddesignBeforePointsAfterPointsArguments{BeforePoints: float64(before) / 20, AfterPoints: float64(after) / 20}) {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_start_the_word_084e93"}, feedbackWarning, notificationToast)
			}
		},
		modalDialogActionSpec{ID: "theme_office", LabelRef: i18n.Message{ID: "word.design.themes.option.office"}},
		modalDialogActionSpec{ID: "theme_business", LabelRef: i18n.Message{ID: "word.design.themes.option.business"}},
		modalDialogActionSpec{ID: "theme_academic", LabelRef: i18n.Message{ID: "word.design.themes.option.academic"}},
		modalDialogActionSpec{ID: "theme_modern", LabelRef: i18n.Message{ID: "word.design.themes.option.modern"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

type wordPageBackgroundSetArguments struct {
	Color string `json:"color"`
}

func (a *App) showWordColorsDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	_, ok := any(state.word).(document.WordPageBackgroundEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_cdd5a6"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowMessage("word.design.color_schemes", i18n.Message{ID: "word.design.color_schemes.dialog.title"}, i18n.Message{ID: "word.design.color_schemes.dialog.body"},
		func(action string) {
			a.modal.Hide()
			var colorHex string
			var schemeName i18n.Message
			switch action {
			case "scheme_white":
				colorHex, schemeName = "", i18n.Message{ID: "word.design.color_schemes.white"}
			case "scheme_parchment":
				colorHex, schemeName = "FAF8F5", i18n.Message{ID: "word.design.color_schemes.parchment"}
			case "scheme_mint":
				colorHex, schemeName = "E8F5E9", i18n.Message{ID: "word.design.color_schemes.mint"}
			case "scheme_blue":
				colorHex, schemeName = "E3F2FD", i18n.Message{ID: "word.design.color_schemes.blue"}
			case "scheme_dark":
				colorHex, schemeName = "263238", i18n.Message{ID: "word.design.color_schemes.dark"}
			default:
				return
			}
			if !a.runWordApplicationCommand(a.text(i18n.Message{ID: "word.design.color_schemes.operation", Args: i18n.Args{"Scheme": a.text(schemeName)}}), "word.page.background.set", wordPageBackgroundSetArguments{Color: colorHex}) {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_start_the_word_c38b8a"}, feedbackWarning, notificationToast)
			}
		},
		modalDialogActionSpec{ID: "scheme_white", LabelRef: i18n.Message{ID: "word.design.color_schemes.white"}},
		modalDialogActionSpec{ID: "scheme_parchment", LabelRef: i18n.Message{ID: "word.design.color_schemes.parchment"}},
		modalDialogActionSpec{ID: "scheme_mint", LabelRef: i18n.Message{ID: "word.design.color_schemes.mint"}},
		modalDialogActionSpec{ID: "scheme_blue", LabelRef: i18n.Message{ID: "word.design.color_schemes.blue"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

type wordTextRunFontFamilySetArguments struct {
	RunId      string `json:"run_id"`
	FontFamily string `json:"font_family"`
}

func (a *App) showWordFontsDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	_, ok := any(state.word).(document.WordTextFontFamilyEditor)
	if !ok {
		a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_fc65cd"}, feedbackWarning, notificationToast)
		return
	}
	a.modal.ShowMessage("word.design.fonts", i18n.Message{ID: "word.design.fonts.dialog.title"}, i18n.Message{ID: "word.design.fonts.dialog.body"},
		func(action string) {
			a.modal.Hide()
			var fontName string
			var label i18n.Message
			switch action {
			case "font_yahei":
				fontName, label = "Microsoft YaHei", i18n.Message{ID: "word.design.fonts.option.yahei"}
			case "font_song":
				fontName, label = "SimSun", i18n.Message{ID: "word.design.fonts.option.song"}
			case "font_fangsong":
				fontName, label = "FangSong", i18n.Message{ID: "word.design.fonts.option.fangsong"}
			case "font_calibri":
				fontName, label = "Calibri", i18n.Message{ID: "word.design.fonts.option.calibri"}
			default:
				return
			}
			runID := state.selectedWordRunID
			if runID == "" {
				blocks := wordBlocksForState(state)
				for _, b := range blocks {
					if b.Paragraph != nil {
						for _, in := range b.Paragraph.Inlines {
							if in.Text != nil && in.Text.Editable {
								runID = in.Text.ID
								break
							}
						}
					}
					if runID != "" {
						break
					}
				}
			}
			if runID == "" {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.no_body_text_is_available_cb1d2a"}, feedbackWarning, notificationToast)
				return
			}
			if !a.runWordApplicationCommand(a.text(i18n.Message{ID: "word.design.fonts.operation", Args: i18n.Args{"Font": a.text(label)}}), "word.text_run.font_family.set", wordTextRunFontFamilySetArguments{RunId: runID, FontFamily: fontName}) {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_start_the_word_6be8d2"}, feedbackWarning, notificationToast)
			}
		},
		modalDialogActionSpec{ID: "font_yahei", LabelRef: i18n.Message{ID: "word.design.fonts.option.yahei"}},
		modalDialogActionSpec{ID: "font_song", LabelRef: i18n.Message{ID: "word.design.fonts.option.song"}},
		modalDialogActionSpec{ID: "font_fangsong", LabelRef: i18n.Message{ID: "word.design.fonts.option.fangsong"}},
		modalDialogActionSpec{ID: "font_calibri", LabelRef: i18n.Message{ID: "word.design.fonts.option.calibri"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

type wordThemeEffectsSetArguments struct {
	EffectProfile string `json:"effect_profile"`
}

func (a *App) showWordEffectsDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	a.modal.ShowMessage("word.design.effects", i18n.Message{ID: "word.design.effects.dialog.title"}, i18n.Message{ID: "word.design.effects.dialog.body"},
		func(action string) {
			a.modal.Hide()
			if action == "cancel" {
				return
			}
			if a.applicationSessionRegistered(state.documentID) {
				if !a.runWordApplicationCommand("设置主题效果", "word.theme.effects.set", wordThemeEffectsSetArguments{EffectProfile: action}) {
					a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_start_the_theme_ac965b"}, feedbackWarning, notificationToast)
				}
				return
			}
			if !a.runWordThemeEffectsCommand("设置主题效果", action) {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.the_current_document_does_not_4c7688"}, feedbackWarning, notificationToast)
			}
		},
		modalDialogActionSpec{ID: "subtle", LabelRef: i18n.Message{ID: "word.design.effects.subtle"}},
		modalDialogActionSpec{ID: "moderate", LabelRef: i18n.Message{ID: "word.design.effects.moderate"}},
		modalDialogActionSpec{ID: "intense", LabelRef: i18n.Message{ID: "word.design.effects.intense"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}

type worddesignBeforePointsAfterPointsArguments struct {
	BeforePoints float64 `json:"before_points"`
	AfterPoints  float64 `json:"after_points"`
}

func (a *App) showWordDesignerDialog() {
	state := a.documentSessionState
	if state == nil || state.word == nil {
		return
	}
	stats := wordTextStatisticsForState(state)
	a.modal.ShowMessage("word.design.designer", i18n.Message{ID: "word.design.designer.dialog.title"}, i18n.Message{ID: "word.design.designer.dialog.body", Args: i18n.Args{"Paragraphs": stats.paragraphs, "Characters": stats.charactersWithWhitespace}},
		func(action string) {
			a.modal.Hide()
			var before, after int64
			var schemeName i18n.Message
			switch action {
			case "scheme_gov":
				before, after, schemeName = 0, 160, i18n.Message{ID: "word.design.designer.scheme.government"}
			case "scheme_biz":
				before, after, schemeName = 40, 160, i18n.Message{ID: "word.design.designer.scheme.business"}
			case "scheme_academic":
				before, after, schemeName = 0, 60, i18n.Message{ID: "word.design.designer.scheme.academic"}
			default:
				return
			}
			if !a.runWordApplicationCommand(a.text(i18n.Message{ID: "word.design.designer.operation", Args: i18n.Args{"Scheme": a.text(schemeName)}}), "word.paragraph.spacing_all.set", worddesignBeforePointsAfterPointsArguments{BeforePoints: float64(before) / 20, AfterPoints: float64(after) / 20}) {
				a.setStatusMessage(i18n.Message{ID: "word.status.inline.could_not_start_the_word_08c2a1"}, feedbackWarning, notificationToast)
			}
		},
		modalDialogActionSpec{ID: "scheme_gov", LabelRef: i18n.Message{ID: "word.design.designer.action.government"}},
		modalDialogActionSpec{ID: "scheme_biz", LabelRef: i18n.Message{ID: "word.design.designer.action.business"}},
		modalDialogActionSpec{ID: "scheme_academic", LabelRef: i18n.Message{ID: "word.design.designer.action.academic"}},
		modalDialogActionSpec{ID: "cancel", LabelRef: i18n.Message{ID: "common.action.cancel"}},
	)
}
