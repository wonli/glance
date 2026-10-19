package desktopui

import (
	"path/filepath"
	"testing"

	"glance/internal/i18n"
	"glance/internal/systemlocale"
	"glance/internal/workspace"
)

const localePreferenceKey = "ui.language"

func newLocalePreferenceTestApp(t *testing.T) (*App, *desktopRuntime, *workspace.Store) {
	t.Helper()
	store, err := workspace.Open(filepath.Join(t.TempDir(), "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	runtime := newDesktopRuntime()
	runtime.workspaceStore = store
	app := newAppWithRuntime(nil, runtime)
	t.Cleanup(func() { runtime.shutdown() })
	return app, runtime, store
}

func TestLoadLocaleDefaultsToSystem(t *testing.T) {
	app, _, store := newLocalePreferenceTestApp(t)
	app.loadLocalePreference()

	state := app.localeSnapshot()
	if state.Selection != i18n.SelectionSystem {
		t.Fatalf("selection = %q, want %q", state.Selection, i18n.SelectionSystem)
	}
	if want := i18n.Resolve(i18n.SelectionSystem, systemlocale.PreferredLanguages()); state.Resolved != want {
		t.Fatalf("resolved locale = %q, want system locale %q", state.Resolved, want)
	}
	if _, found, err := store.Preference(localePreferenceKey); err != nil || found {
		t.Fatalf("default selection was persisted: found=%t err=%v", found, err)
	}
}

func TestLoadLocaleRestoresExplicitSelection(t *testing.T) {
	app, _, store := newLocalePreferenceTestApp(t)
	if err := store.SetPreference(localePreferenceKey, string(i18n.SelectionChinese)); err != nil {
		t.Fatal(err)
	}
	app.loadLocalePreference()

	state := app.localeSnapshot()
	if state.Selection != i18n.SelectionChinese || state.Resolved != i18n.LocaleChinese {
		t.Fatalf("restored locale = (%q, %q), want (%q, %q)", state.Selection, state.Resolved, i18n.SelectionChinese, i18n.LocaleChinese)
	}
}

func TestLoadLocaleDoesNotResetSharedSelectionAcrossWindows(t *testing.T) {
	first, runtime, store := newLocalePreferenceTestApp(t)
	if err := store.SetPreference(localePreferenceKey, string(i18n.SelectionEnglish)); err != nil {
		t.Fatal(err)
	}
	first.loadLocalePreference()
	first.applyLocaleSelection(i18n.SelectionChinese)

	second := newAppWithRuntime(nil, runtime)
	second.loadLocalePreference()
	if state := second.localeSnapshot(); state.Selection != i18n.SelectionChinese || state.Resolved != i18n.LocaleChinese {
		t.Fatalf("second window reset shared locale state to (%q, %q)", state.Selection, state.Resolved)
	}
}

func TestLoadLocaleInvalidValueFallsBackAndQueuesCorrection(t *testing.T) {
	app, runtime, store := newLocalePreferenceTestApp(t)
	if err := store.SetPreference(localePreferenceKey, "fr-FR"); err != nil {
		t.Fatal(err)
	}
	app.loadLocalePreference()
	if state := app.localeSnapshot(); state.Selection != i18n.SelectionEnglish || state.Resolved != i18n.LocaleEnglish {
		t.Fatalf("invalid preference state = (%q, %q), want English", state.Selection, state.Resolved)
	}
	runtime.drainWorkspaceWriter()
	if value, found, err := store.Preference(localePreferenceKey); err != nil || !found || value != string(i18n.SelectionEnglish) {
		t.Fatalf("corrected preference = %q, found=%t, err=%v", value, found, err)
	}
}

func TestSelectLocaleUpdatesImmediatelyAndQueuesPreference(t *testing.T) {
	app, runtime, store := newLocalePreferenceTestApp(t)
	app.loadLocalePreference()
	app.selectLocale(i18n.SelectionChinese)
	if state := app.localeSnapshot(); state.Selection != i18n.SelectionChinese || state.Resolved != i18n.LocaleChinese {
		t.Fatalf("locale did not update immediately: (%q, %q)", state.Selection, state.Resolved)
	}
	runtime.drainWorkspaceWriter()
	if value, found, err := store.Preference(localePreferenceKey); err != nil || !found || value != string(i18n.SelectionChinese) {
		t.Fatalf("persisted preference = %q, found=%t, err=%v", value, found, err)
	}
}

func TestLocalePreferenceWriteFailureKeepsCurrentLocale(t *testing.T) {
	app, runtime, store := newLocalePreferenceTestApp(t)
	app.loadLocalePreference()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	app.selectLocale(i18n.SelectionChinese)
	runtime.drainWorkspaceWriter()
	if state := app.localeSnapshot(); state.Selection != i18n.SelectionChinese || state.Resolved != i18n.LocaleChinese {
		t.Fatalf("write failure reverted current locale: (%q, %q)", state.Selection, state.Resolved)
	}
	select {
	case err := <-runtime.workspaceWriteResults:
		if err == nil {
			t.Fatal("closed preference store produced no write error")
		}
	default:
		t.Fatal("closed preference store write did not report an error")
	}
}
