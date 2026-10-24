package desktopui

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"glance/internal/agentsettings"
	"glance/internal/ocr"
)

const proofreadingOCRAuthenticationPreferenceKey = "proofreading.ocr.authentication"

type proofreadingOCRCredentialStore interface {
	Get(context.Context, string) (string, error)
	Put(context.Context, string, string) error
	Delete(context.Context, string) error
}

type proofreadingOCRPreferenceStore interface {
	Preference(string) (string, bool, error)
	SetPreference(string, string) error
}

type proofreadingOCRAuthentication struct {
	Endpoint      string `json:"endpoint"`
	CredentialRef string `json:"credential_ref"`
}

func (a proofreadingOCRAuthentication) matchesEndpoint(endpoint string) bool {
	left, leftErr := normalizeProofreadingOCREndpoint(a.Endpoint)
	right, rightErr := normalizeProofreadingOCREndpoint(endpoint)
	return leftErr == nil && rightErr == nil && left == right && a.CredentialRef != ""
}

func decodeProofreadingOCRAuthentication(raw string) (proofreadingOCRAuthentication, error) {
	var auth proofreadingOCRAuthentication
	if strings.TrimSpace(raw) == "" {
		return auth, errors.New("OCR credential preference is empty")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&auth); err != nil {
		return auth, fmt.Errorf("decode OCR credential preference: %w", err)
	}
	endpoint, err := normalizeProofreadingOCREndpoint(auth.Endpoint)
	if err != nil {
		return proofreadingOCRAuthentication{}, err
	}
	if strings.TrimSpace(auth.CredentialRef) == "" {
		return proofreadingOCRAuthentication{}, errors.New("OCR credential reference is missing")
	}
	auth.Endpoint = endpoint
	return auth, nil
}

func encodeProofreadingOCRAuthentication(auth proofreadingOCRAuthentication) (string, error) {
	endpoint, err := normalizeProofreadingOCREndpoint(auth.Endpoint)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(auth.CredentialRef) == "" {
		return "", errors.New("OCR credential reference is missing")
	}
	auth.Endpoint = endpoint
	encoded, err := json.Marshal(auth)
	return string(encoded), err
}

func normalizeProofreadingOCREndpoint(endpoint string) (string, error) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		endpoint = strings.TrimRight(ocr.DefaultEndpoint, "/")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return "", errors.New("OCR endpoint must be an HTTP or HTTPS URL")
	}
	return endpoint, nil
}

func newProofreadingOCRCredentialReference() (string, error) {
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate OCR credential reference: %w", err)
	}
	return "proofreading-ocr-" + hex.EncodeToString(random[:]), nil
}

func proofreadingOCRDraftSnapshotHash(markdown string) string {
	digest := sha256.Sum256([]byte(markdown))
	return hex.EncodeToString(digest[:])
}

func loadProofreadingOCRAuthentication(ctx context.Context, credentials proofreadingOCRCredentialStore, preferences proofreadingOCRPreferenceStore, endpoint string) (proofreadingOCRAuthentication, string, bool, error) {
	if credentials == nil || preferences == nil {
		return proofreadingOCRAuthentication{}, "", false, errors.New("OCR credential storage is unavailable")
	}
	raw, found, err := preferences.Preference(proofreadingOCRAuthenticationPreferenceKey)
	if err != nil || !found {
		return proofreadingOCRAuthentication{}, "", false, err
	}
	auth, err := decodeProofreadingOCRAuthentication(raw)
	if err != nil || !auth.matchesEndpoint(endpoint) {
		return auth, "", false, err
	}
	secret, err := credentials.Get(ctx, auth.CredentialRef)
	if err != nil {
		return auth, "", false, err
	}
	if strings.TrimSpace(secret) == "" {
		return auth, "", false, errors.New("OCR credential is empty")
	}
	return auth, secret, true, nil
}

func saveProofreadingOCRAuthentication(ctx context.Context, credentials proofreadingOCRCredentialStore, preferences proofreadingOCRPreferenceStore, endpoint, secret string) (proofreadingOCRAuthentication, error) {
	if credentials == nil || preferences == nil {
		return proofreadingOCRAuthentication{}, errors.New("OCR credential storage is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return proofreadingOCRAuthentication{}, err
	}
	if secret == "" {
		return proofreadingOCRAuthentication{}, errors.New("OCR credential is empty")
	}
	endpoint, err := normalizeProofreadingOCREndpoint(endpoint)
	if err != nil {
		return proofreadingOCRAuthentication{}, err
	}
	oldRaw, oldFound, err := preferences.Preference(proofreadingOCRAuthenticationPreferenceKey)
	if err != nil {
		return proofreadingOCRAuthentication{}, err
	}
	var old proofreadingOCRAuthentication
	if oldFound {
		if decoded, decodeErr := decodeProofreadingOCRAuthentication(oldRaw); decodeErr == nil {
			old = decoded
		}
	}
	ref, err := newProofreadingOCRCredentialReference()
	if err != nil {
		return proofreadingOCRAuthentication{}, err
	}
	if err = credentials.Put(ctx, ref, secret); err != nil {
		return proofreadingOCRAuthentication{}, err
	}
	created := proofreadingOCRAuthentication{Endpoint: endpoint, CredentialRef: ref}
	encoded, err := encodeProofreadingOCRAuthentication(created)
	if err == nil {
		err = preferences.SetPreference(proofreadingOCRAuthenticationPreferenceKey, encoded)
	}
	if err != nil {
		_ = credentials.Delete(context.Background(), ref)
		return proofreadingOCRAuthentication{}, err
	}
	if old.CredentialRef != "" && old.CredentialRef != ref {
		if err = credentials.Delete(ctx, old.CredentialRef); err != nil {
			return created, fmt.Errorf("saved OCR credential but could not remove previous credential: %w", err)
		}
	}
	return created, nil
}

type proofreadingOCRJobRecord struct {
	JobID       string    `json:"job_id"`
	Endpoint    string    `json:"endpoint"`
	Model       string    `json:"model"`
	SourceAHash string    `json:"source_a_hash"`
	SourceBHash string    `json:"source_b_hash"`
	DraftHash   string    `json:"draft_hash"`
	CreatedAt   time.Time `json:"created_at"`
	State       string    `json:"state"`
}

func (r proofreadingOCRJobRecord) validFor(sourceAHash, sourceBHash, draftHash string) bool {
	endpoint, err := normalizeProofreadingOCREndpoint(r.Endpoint)
	return err == nil && r.JobID != "" && endpoint == r.Endpoint && r.SourceAHash == sourceAHash && r.SourceBHash == sourceBHash && r.DraftHash == draftHash
}

func (r proofreadingOCRJobRecord) terminal() bool {
	switch strings.ToLower(strings.TrimSpace(r.State)) {
	case "failed", "expired":
		return true
	default:
		return false
	}
}

func proofreadingOCRResumeJob(record *proofreadingOCRJobRecord, sourceAHash, sourceBHash, draft string) (*proofreadingOCRJobRecord, bool) {
	if record == nil || record.terminal() || !record.validFor(sourceAHash, sourceBHash, proofreadingOCRDraftSnapshotHash(draft)) {
		return nil, false
	}
	copy := *record
	return &copy, true
}

var _ proofreadingOCRCredentialStore = agentsettings.SystemCredentialStore{}
