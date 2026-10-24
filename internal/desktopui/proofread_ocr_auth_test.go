package desktopui

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestProofreadingOCRAuthPreferenceOnlyReusesCredentialForSameEndpoint(t *testing.T) {
	const raw = `{"endpoint":"https://ocr.example/api/v2/jobs","credential_ref":"proofreading-ocr-0123456789abcdef"}`
	auth, err := decodeProofreadingOCRAuthentication(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !auth.matchesEndpoint("https://ocr.example/api/v2/jobs/") {
		t.Fatal("equivalent endpoint with trailing slash should match saved credential")
	}
	if auth.matchesEndpoint("https://other.example/api/v2/jobs") {
		t.Fatal("credential for one endpoint must not be sent to another endpoint")
	}
	if auth.matchesEndpoint("https://ocr.example/api/v2/jobs") && auth.CredentialRef == "" {
		t.Fatal("a matching credential preference must contain an opaque reference")
	}
}

func TestProofreadingOCRAuthPreferenceRejectsMissingReference(t *testing.T) {
	if _, err := decodeProofreadingOCRAuthentication(`{"endpoint":"https://ocr.example/api/v2/jobs"}`); err == nil {
		t.Fatal("preference without a credential reference must not be accepted")
	}
}

func TestProofreadingOCRAuthPreferenceRejectsInlineToken(t *testing.T) {
	if _, err := decodeProofreadingOCRAuthentication(`{"endpoint":"https://ocr.example/api/v2/jobs","credential_ref":"ref","token":"synthetic-secret"}`); err == nil {
		t.Fatal("OCR authentication preference must never accept an inline token")
	}
}

func TestProofreadingOCRCredentialReferenceIsRandomAndOpaque(t *testing.T) {
	first, err := newProofreadingOCRCredentialReference()
	if err != nil {
		t.Fatal(err)
	}
	second, err := newProofreadingOCRCredentialReference()
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || second == "" || first == second {
		t.Fatalf("credential references must be nonempty and independently generated: %q %q", first, second)
	}
	if strings.Contains(first, "token") || strings.Contains(first, "secret") {
		t.Fatalf("credential reference must not embed a secret: %q", first)
	}
}

func TestProofreadingOCRDraftSnapshotHashIsStableAndContentSensitive(t *testing.T) {
	first := proofreadingOCRDraftSnapshotHash("甲乙\n丙")
	if first == "" || first != proofreadingOCRDraftSnapshotHash("甲乙\n丙") {
		t.Fatal("same B draft snapshot must produce a stable nonempty hash")
	}
	if first == proofreadingOCRDraftSnapshotHash("甲乙丙") {
		t.Fatal("draft hash must preserve content changes such as line breaks")
	}
}

func TestProofreadingOCRResumeRejectsAnyChangedSourceOrDraftSnapshot(t *testing.T) {
	record := &proofreadingOCRJobRecord{
		JobID: "job-17", Endpoint: "https://ocr.example/api/v2/jobs",
		SourceAHash: "pdf-hash", SourceBHash: "markdown-file-hash",
		DraftHash: proofreadingOCRDraftSnapshotHash("校对稿快照"),
	}
	resumed, ok := proofreadingOCRResumeJob(record, "pdf-hash", "markdown-file-hash", "校对稿快照")
	if !ok || resumed == record || resumed.JobID != "job-17" {
		t.Fatal("unchanged source and draft should return a safe copy of the saved OCR job")
	}
	for _, test := range []struct {
		name  string
		aHash string
		bHash string
		draft string
	}{
		{name: "PDF changed", aHash: "changed-pdf", bHash: "markdown-file-hash", draft: "校对稿快照"},
		{name: "B file changed", aHash: "pdf-hash", bHash: "changed-markdown", draft: "校对稿快照"},
		{name: "working draft changed", aHash: "pdf-hash", bHash: "markdown-file-hash", draft: "修改后的校对稿"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got, ok := proofreadingOCRResumeJob(record, test.aHash, test.bHash, test.draft); ok || got != nil {
				t.Fatalf("stale OCR job must not be resumed: got=%#v ok=%v", got, ok)
			}
		})
	}
}

func TestProofreadingOCRResumeRejectsTerminalRemoteJobs(t *testing.T) {
	for _, state := range []string{"failed", "expired"} {
		t.Run(state, func(t *testing.T) {
			record := proofreadingOCRJobRecord{
				JobID: "job", Endpoint: "https://ocr.example/jobs", State: state,
				SourceAHash: "A", SourceBHash: "B", DraftHash: proofreadingOCRDraftSnapshotHash("draft"),
			}
			if _, ok := proofreadingOCRResumeJob(&record, "A", "B", "draft"); ok {
				t.Fatalf("terminal remote job state %q must offer a fresh scan, not resume", state)
			}
		})
	}
}

func TestProofreadingOCRAuthenticationRotationStoresBeforeSwitchingAndDeletesOldKey(t *testing.T) {
	credentials := &fakeProofreadingOCRCredentials{values: map[string]string{"old-ref": "old-secret"}}
	preferences := &fakeProofreadingOCRPreferences{raw: `{"endpoint":"https://ocr.example/api/v2/jobs","credential_ref":"old-ref"}`, found: true, credentials: credentials}
	result, err := saveProofreadingOCRAuthentication(context.Background(), credentials, preferences, "https://ocr.example/api/v2/jobs/", "new-secret")
	if err != nil {
		t.Fatal(err)
	}
	if result.CredentialRef == "old-ref" || result.Endpoint != "https://ocr.example/api/v2/jobs" {
		t.Fatalf("replacement should use a new reference and normalized endpoint: %#v", result)
	}
	if credentials.values[result.CredentialRef] != "new-secret" {
		t.Fatal("new secret must be stored under the returned reference")
	}
	if _, exists := credentials.values["old-ref"]; exists {
		t.Fatal("old secret must be deleted after preference replacement")
	}
	if preferences.raw == "" || strings.Contains(preferences.raw, "new-secret") || strings.Contains(preferences.raw, "old-secret") {
		t.Fatalf("preference must hold only endpoint and opaque reference: %q", preferences.raw)
	}
	if strings.Join(credentials.events, ",") != "put,preference-set,delete" || strings.Join(preferences.events, ",") != "set" {
		t.Fatalf("unexpected credential/reference ordering: credential=%v preference=%v", credentials.events, preferences.events)
	}
}

func TestProofreadingOCRAuthenticationPreferenceFailureDeletesNewSecretAndAborts(t *testing.T) {
	credentials := &fakeProofreadingOCRCredentials{values: map[string]string{}}
	preferences := &fakeProofreadingOCRPreferences{setErr: errors.New("database unavailable"), credentials: credentials}
	if _, err := saveProofreadingOCRAuthentication(context.Background(), credentials, preferences, "https://ocr.example/api/v2/jobs", "synthetic-token"); err == nil {
		t.Fatal("preference failure must stop credential configuration")
	}
	if len(credentials.values) != 0 {
		t.Fatalf("unreferenced credential must be rolled back: %#v", credentials.values)
	}
	if strings.Join(credentials.events, ",") != "put,preference-set,delete" {
		t.Fatalf("expected new credential rollback, got %v", credentials.events)
	}
}

func TestProofreadingOCRAuthenticationCanReplaceMalformedSavedPreference(t *testing.T) {
	credentials := &fakeProofreadingOCRCredentials{values: map[string]string{}}
	preferences := &fakeProofreadingOCRPreferences{raw: `{"token":"legacy-inline-secret"}`, found: true, credentials: credentials}
	result, err := saveProofreadingOCRAuthentication(context.Background(), credentials, preferences, "https://ocr.example/api/v2/jobs", "new-secret")
	if err != nil {
		t.Fatal(err)
	}
	if result.CredentialRef == "" || credentials.values[result.CredentialRef] != "new-secret" || strings.Contains(preferences.raw, "new-secret") {
		t.Fatalf("replacement must recover from malformed metadata without putting a token in preferences: result=%#v value=%q", result, preferences.raw)
	}
}

type fakeProofreadingOCRCredentials struct {
	values map[string]string
	events []string
}

func (f *fakeProofreadingOCRCredentials) Get(_ context.Context, reference string) (string, error) {
	value, ok := f.values[reference]
	if !ok {
		return "", errors.New("missing credential")
	}
	return value, nil
}

func (f *fakeProofreadingOCRCredentials) Put(_ context.Context, reference, secret string) error {
	f.events = append(f.events, "put")
	f.values[reference] = secret
	return nil
}

func (f *fakeProofreadingOCRCredentials) Delete(_ context.Context, reference string) error {
	f.events = append(f.events, "delete")
	delete(f.values, reference)
	return nil
}

type fakeProofreadingOCRPreferences struct {
	raw         string
	found       bool
	setErr      error
	events      []string
	credentials *fakeProofreadingOCRCredentials
}

func (f *fakeProofreadingOCRPreferences) Preference(string) (string, bool, error) {
	return f.raw, f.found, nil
}

func (f *fakeProofreadingOCRPreferences) SetPreference(_ string, value string) error {
	f.events = append(f.events, "set")
	if f.credentials != nil {
		f.credentials.events = append(f.credentials.events, "preference-set")
	}
	if f.setErr != nil {
		return f.setErr
	}
	f.raw, f.found = value, true
	return nil
}
