package desktopui

import (
	"strings"
	"testing"

	"glance/internal/i18n"
)

func TestProofreadingStatusBarIncludesCharacterCoverageWithoutPlaceholders(t *testing.T) {
	for _, limited := range []bool{false, true} {
		a, _, c := proofreadingConnectorFixture(1)
		c.HasEvidence, c.Complete, c.AlignmentLimited = true, true, limited
		c.CharacterLocated, c.CharacterTotal = 137, 183
		step := a.proofreading.ocrStep
		step.comparisons[proofreadingOCRStepPair{APage: c.APage, BPage: c.BPage}] = c
		step.diffRunning = false
		message, _, ok := a.proofreadingStatusBarOCRText()
		if !ok || !strings.Contains(message, "137/183") || strings.ContainsAny(message, "{}") {
			t.Fatalf("status bar lost coverage or exposed placeholders: %q", message)
		}
		step.diffRunning = true
		step.diffCursor = 5
		message, _, _ = a.proofreadingStatusBarOCRText()
		if strings.Contains(message, "137/183") || !strings.Contains(message, "5") {
			t.Fatalf("coverage displaced active playback: %q", message)
		}
	}
}

func TestProofreadingCharacterCoverageCatalogArguments(t *testing.T) {
	for _, locale := range []i18n.Locale{i18n.LocaleChinese, i18n.LocaleEnglish} {
		catalog, err := i18n.NewCatalog(locale, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{"proofread.ai.step.character_geometry_coverage", "proofread.ai.step.character_geometry_coverage_unconfirmed", "proofread.ai.step.character_geometry_coverage_partial"} {
			message := catalog.Text(i18n.Message{ID: id, Args: i18n.Args{"Located": "137", "SourceChars": "183"}})
			if strings.ContainsAny(message, "{}") || !strings.Contains(message, "137/183") {
				t.Fatalf("%s/%s: %s", locale, id, message)
			}
		}
	}
}
