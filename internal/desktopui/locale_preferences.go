package desktopui

import (
	"log"

	"glance/internal/i18n"
	"glance/internal/systemlocale"
)

const desktopLocalePreferenceKey = "ui.language"

func (a *App) loadLocalePreference() {
	runtime := a.ensureDesktopRuntime()
	runtime.localeStateOnce.Do(runtime.localeState.initialize)
	runtime.localePreferenceMu.Lock()
	defer runtime.localePreferenceMu.Unlock()
	if runtime.localePreferenceLoaded {
		return
	}
	if runtime.workspaceStore == nil {
		return
	}
	value, found, err := runtime.workspaceStore.Preference(desktopLocalePreferenceKey)
	if err != nil {
		a.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, err, notificationToast)
		return
	}
	selection, repair := i18n.NormalizePreference(value, found)
	a.applyLocaleSelection(selection)
	runtime.localePreferenceLoaded = true
	if repair {
		runtime.queueWorkspacePreference(desktopLocalePreferenceKey, string(i18n.SelectionEnglish))
	}
}

func (a *App) selectLocale(selection i18n.Selection) {
	selection, _ = i18n.NormalizePreference(string(selection), true)
	a.applyLocaleSelection(selection)
	a.ensureDesktopRuntime().queueWorkspacePreference(desktopLocalePreferenceKey, string(selection))
	a.ensureDesktopRuntime().invalidateWindows()
}

func (a *App) applyLocaleSelection(selection i18n.Selection) {
	runtime := a.ensureDesktopRuntime()
	runtime.localeStateOnce.Do(runtime.localeState.initialize)
	resolved := i18n.Resolve(selection, systemlocale.PreferredLanguages())
	catalog, err := i18n.NewCatalog(resolved, nil)
	if err != nil {
		log.Printf("load %s UI catalog: %v", resolved, err)
		resolved = i18n.LocaleEnglish
		catalog, err = i18n.NewCatalog(resolved, nil)
		if err != nil {
			a.setStatusErrorMessage(i18n.Message{ID: "common.feedback.operation_failed"}, err, notificationToast)
			return
		}
	}
	runtime.localeState.update(selection, resolved, catalog)
}
