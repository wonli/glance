package desktopui

import "testing"

func TestAgentModelLookupFingerprintTracksDraftAndDoesNotExposeKey(t *testing.T) {
	key := "sk-secret-value"
	base := agentModelLookupFingerprint("openai-compatible", "https://api.example.test/v1", key)
	if base == "" || base == key {
		t.Fatalf("fingerprint must be opaque: %q", base)
	}
	if got := agentModelLookupFingerprint("openai-compatible", "https://api.example.test/v1", key); got != base {
		t.Fatalf("same model lookup settings produced different fingerprints: %q != %q", got, base)
	}
	if got := agentModelLookupFingerprint("openai-compatible", "https://api.example.test/v1", "sk-another-secret"); got == base {
		t.Fatal("changing API key did not change the model lookup fingerprint")
	}
	if got := agentModelLookupFingerprint("deepseek", "https://api.example.test/v1", key); got == base {
		t.Fatal("changing provider did not change the model lookup fingerprint")
	}
	if got := agentModelLookupFingerprint("openai-compatible", "https://other.example.test/v1", key); got == base {
		t.Fatal("changing base URL did not change the model lookup fingerprint")
	}
}

func TestAgentModelLookupRejectsStaleResults(t *testing.T) {
	if !agentModelLookupResultIsCurrent(4, "fingerprint-a", 4, "fingerprint-a") {
		t.Fatal("current model lookup result was rejected")
	}
	for _, tc := range []struct {
		name               string
		resultGeneration   uint64
		resultFingerprint  string
		currentGeneration  uint64
		currentFingerprint string
	}{
		{name: "old generation", resultGeneration: 3, resultFingerprint: "fingerprint-a", currentGeneration: 4, currentFingerprint: "fingerprint-a"},
		{name: "old provider or key", resultGeneration: 4, resultFingerprint: "fingerprint-a", currentGeneration: 4, currentFingerprint: "fingerprint-b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if agentModelLookupResultIsCurrent(tc.resultGeneration, tc.resultFingerprint, tc.currentGeneration, tc.currentFingerprint) {
				t.Fatal("stale model lookup result was accepted")
			}
		})
	}
}

func TestAgentModelLookupRequiresCurrentCredentials(t *testing.T) {
	for _, tc := range []struct {
		name       string
		providerID string
		apiKey     string
		savedMatch bool
		want       bool
	}{
		{name: "new key draft", providerID: "deepseek", apiKey: "draft-key", want: true},
		{name: "matching saved key", providerID: "deepseek", savedMatch: true, want: true},
		{name: "ollama needs no key", providerID: "ollama", want: true},
		{name: "no key for different provider", providerID: "deepseek", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldLookupAgentModels(tc.providerID, tc.apiKey, tc.savedMatch); got != tc.want {
				t.Fatalf("shouldLookupAgentModels()=%v, want %v", got, tc.want)
			}
		})
	}
}
