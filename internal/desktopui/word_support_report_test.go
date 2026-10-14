package desktopui

import (
	"strings"
	"testing"

	"glance/internal/wordlayout"
)

func TestWordPageExportGateUsesSupportImpact(t *testing.T) {
	approximation := wordlayout.Result{
		CanPreview: true,
		SupportReport: wordlayout.SupportReport{Findings: []wordlayout.SupportFinding{{
			SourceID: "b4.p", Code: "unsupported-list", Detail: "Unsupported list format.",
			Impact: wordlayout.SupportImpactApproximation, Fallback: "The marker is omitted.",
		}}},
	}
	if wordLayoutBlocksPageExport(approximation) {
		t.Fatal("an acknowledged approximation should allow page export")
	}
	warnings := wordLayoutWarnings(approximation)
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "b4.p") || !strings.Contains(joined, "编号标记会省略") {
		t.Fatalf("warning details = %q; want source location and fallback", joined)
	}

	blocked := wordlayout.Result{
		CanPreview: true,
		SupportReport: wordlayout.SupportReport{Findings: []wordlayout.SupportFinding{{
			SourceID: "section-2", Code: "unsupported-columns", Detail: "Columns are unsupported.",
			Impact: wordlayout.SupportImpactBlocksExport,
		}}},
	}
	if !wordLayoutBlocksPageExport(blocked) {
		t.Fatal("a finding that blocks export must stop page export even when preview remains available")
	}
}
