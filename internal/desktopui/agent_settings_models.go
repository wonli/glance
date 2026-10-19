package desktopui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image/color"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"glance/internal/agent"
	"glance/internal/agent/provider"
	"glance/internal/agentsettings"
	"glance/internal/i18n"
)

const agentModelLookupDebounce = 450 * time.Millisecond

type agentModelLookupResult struct {
	generation  uint64
	fingerprint string
	models      []provider.ModelInfo
	err         error
}

func agentModelLookupFingerprint(providerID, baseURL, apiKey string) string {
	input := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(providerID)),
		normalizeAgentBaseURL(providerID, baseURL),
		strings.TrimSpace(apiKey),
	}, "\x00")
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

func agentModelLookupResultIsCurrent(resultGeneration uint64, resultFingerprint string, currentGeneration uint64, currentFingerprint string) bool {
	return resultGeneration == currentGeneration && resultFingerprint != "" && resultFingerprint == currentFingerprint
}

func shouldLookupAgentModels(providerID, apiKey string, savedKeyMatchesDraft bool) bool {
	return strings.EqualFold(strings.TrimSpace(providerID), "ollama") || strings.TrimSpace(apiKey) != "" || savedKeyMatchesDraft
}

func (a *App) cancelAgentModelLookup(reset bool) {
	lookup := &a.agentUI.modelLookup
	lookup.generation++
	if lookup.cancel != nil {
		lookup.cancel()
		lookup.cancel = nil
	}
	lookup.loading = false
	if reset {
		lookup.fingerprint = ""
		lookup.models = nil
		lookup.err = ""
		lookup.errRef = i18n.Message{}
	}
}

func (a *App) observeAgentModelLookup() {
	if !a.agentSettingsVisible() || a.agentUI.saving || a.window == nil {
		return
	}
	settings := agentsettings.ModelSettings{
		ProviderID: strings.ToLower(strings.TrimSpace(a.agentUI.providerInput.Text())),
		BaseURL:    strings.TrimSpace(a.agentUI.baseURLInput.Text()),
		Model:      strings.TrimSpace(a.agentUI.modelInput.Text()),
	}
	apiKey := strings.TrimSpace(a.agentUI.apiKeyInput.Text())
	fingerprint := agentModelLookupFingerprint(settings.ProviderID, settings.BaseURL, apiKey)
	lookup := &a.agentUI.modelLookup
	if lookup.fingerprint == fingerprint {
		return
	}
	a.cancelAgentModelLookup(false)
	lookup = &a.agentUI.modelLookup
	lookup.fingerprint = fingerprint
	lookup.models = nil
	lookup.err = ""
	lookup.errRef = i18n.Message{}
	if !shouldLookupAgentModels(settings.ProviderID, apiKey, a.settingsSavedKeyMatchesDraft()) {
		lookup.loading = false
		return
	}
	lookup.loading = true
	if lookup.results == nil {
		lookup.results = make(chan agentModelLookupResult, 1)
	}
	lookupCtx, cancel := context.WithCancel(context.Background())
	lookup.cancel = cancel
	generation, results := lookup.generation, lookup.results
	runtime := a.ensureDesktopRuntime()
	started := a.launchManagedBackground(func() {
		defer cancel()
		timer := time.NewTimer(agentModelLookupDebounce)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-lookupCtx.Done():
			return
		case <-a.shutdown:
			return
		}
		requestCtx, requestCancel := context.WithTimeout(lookupCtx, 25*time.Second)
		models, err := runtime.listAgentModels(requestCtx, settings, apiKey)
		requestCancel()
		select {
		case results <- agentModelLookupResult{generation: generation, fingerprint: fingerprint, models: models, err: err}:
		case <-lookupCtx.Done():
			return
		case <-a.shutdown:
			return
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	})
	if !started {
		cancel()
		lookup.cancel = nil
		lookup.loading = false
		lookup.errRef = i18n.Message{ID: "settings.provider.model_lookup_unavailable"}
		lookup.err = a.text(lookup.errRef)
	}
}

func (a *App) agentSettingsVisible() bool {
	if a.homeSection == homeSectionSettings && a.agentUI.settingsPageTab == "providers" && (a.activeTabID == "" || a.activeTabID == "recent") {
		return true
	}
	return a.modal.Visible && a.modal.DialogID == dialogIDAgentModelSettings
}

func (a *App) processAgentModelLookupResult() {
	lookup := &a.agentUI.modelLookup
	if lookup.results == nil {
		return
	}
	select {
	case result := <-lookup.results:
		currentFingerprint := agentModelLookupFingerprint(a.agentUI.providerInput.Text(), a.agentUI.baseURLInput.Text(), a.agentUI.apiKeyInput.Text())
		if !agentModelLookupResultIsCurrent(result.generation, result.fingerprint, lookup.generation, lookup.fingerprint) ||
			result.fingerprint != currentFingerprint || !a.agentSettingsVisible() {
			return
		}
		if lookup.cancel != nil {
			lookup.cancel()
			lookup.cancel = nil
		}
		lookup.loading = false
		lookup.models = result.models
		lookup.err = ""
		lookup.errRef = i18n.Message{}
		if result.err != nil {
			_, lookup.err = agent.DescribeTurnError(result.err)
		} else if len(result.models) == 0 {
			lookup.errRef = i18n.Message{ID: "settings.provider.no_models"}
			lookup.err = a.text(lookup.errRef)
		}
		if a.window != nil {
			a.window.Invalidate()
		}
	default:
	}
}

func (a *App) layoutAgentModelOptions(gtx layout.Context) layout.Dimensions {
	lookup := &a.agentUI.modelLookup
	if lookup.loading {
		return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			style := material.Caption(a.theme, a.text(i18n.Message{ID: "settings.provider.loading_models"}))
			style.Color = homeMuted
			return style.Layout(gtx)
		})
	}
	if lookup.err != "" {
		return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			style := material.Caption(a.theme, a.messageText(lookup.errRef, lookup.err))
			style.Color = colorSettingsError
			style.MaxLines = 2
			return style.Layout(gtx)
		})
	}
	if len(lookup.models) == 0 {
		return layout.Dimensions{}
	}
	optionMaxWidth := gtx.Constraints.Max.X
	if lookup.optionButtons == nil {
		lookup.optionButtons = make(map[string]*widget.Clickable)
	}
	lookup.modelIDs = lookup.modelIDs[:0]
	lookup.modelsByID = make(map[string]provider.ModelInfo, len(lookup.models))
	for _, model := range lookup.models {
		if model.ID == "" {
			continue
		}
		lookup.modelIDs = append(lookup.modelIDs, model.ID)
		lookup.modelsByID[model.ID] = model
		if lookup.optionButtons[model.ID] == nil {
			lookup.optionButtons[model.ID] = new(widget.Clickable)
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := a.text(i18n.Message{ID: "settings.provider.models_available", Args: i18n.Args{"Count": len(lookup.modelIDs)}})
			style := material.Caption(a.theme, label)
			style.Color = homeMuted
			return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(4)}.Layout(gtx, style.Layout)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return lookup.optionsList.Layout(gtx, len(lookup.modelIDs), func(gtx layout.Context, index int) layout.Dimensions {
				// A horizontal List gives children an unbounded main axis.
				// Material labels must not shape or register hits at that width.
				gtx.Constraints.Max.X = optionMaxWidth
				gtx.Constraints.Min.X = 0
				id := lookup.modelIDs[index]
				model := lookup.modelsByID[id]
				name := strings.TrimSpace(model.Name)
				if name == "" {
					name = id
				}
				button := lookup.optionButtons[id]
				return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					style := material.Button(a.theme, button, name)
					style.Background = color.NRGBA{R: 242, G: 245, B: 249, A: 255}
					style.Color = settingsBlack
					return style.Layout(gtx)
				})
			})
		}),
	)
}

func (a *App) processAgentModelOptionClicks(gtx layout.Context) {
	lookup := &a.agentUI.modelLookup
	if lookup.loading || lookup.err != "" || a.agentUI.saving {
		return
	}
	for _, model := range lookup.models {
		if button := lookup.optionButtons[model.ID]; button != nil && button.Clicked(gtx) {
			a.agentUI.modelInput.SetText(model.ID)
			if a.window != nil {
				a.window.Invalidate()
			}
		}
	}
}

var colorSettingsError = color.NRGBA{R: 178, G: 54, B: 54, A: 255}

var errAgentAPIKeyRequired = errors.New("请填写当前服务商的 API Key 后获取模型列表")
