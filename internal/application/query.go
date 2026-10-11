package application

import (
	"context"
	"encoding/json"
	"fmt"

	"glance/internal/document"
)

const (
	// MaxQueryItems caps paginated document results regardless of caller input.
	MaxQueryItems = 1000
	// MaxQueryCells caps spreadsheet cells returned by one range query.
	MaxQueryCells = 10000
	// MaxQueryOffset bounds how far a page can seek into a document.
	MaxQueryOffset = 1_000_000
	// MaxQueryResultBytes caps the complete serialized snapshot returned to a caller.
	MaxQueryResultBytes     = 1 << 20
	defaultQueryItems       = 100
	defaultQueryResultBytes = 256 << 10
)

type InspectRequest struct {
	DocumentID string              `json:"document_id"`
	Sheet      string              `json:"sheet,omitempty"`
	Range      *document.CellRange `json:"range,omitempty"`
	Start      int                 `json:"start,omitempty"`
	Limit      int                 `json:"limit,omitempty"`
	MaxBytes   int                 `json:"max_bytes,omitempty"`
}

type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

type DocumentSnapshot struct {
	Info        DocumentInfo    `json:"info"`
	Content     json.RawMessage `json:"content"`
	Diagnostics []Diagnostic    `json:"diagnostics,omitempty"`
}

// DocumentInspector exposes format data through neutral JSON DTOs. Adapters
// should apply the request's pagination and format-specific filters before
// returning so the Runtime never needs Office library types.
type DocumentInspector interface {
	Inspect(context.Context, InspectRequest) (json.RawMessage, []Diagnostic, error)
}

type QueryService struct{ runtime *Runtime }

func (q *QueryService) InspectDocument(ctx context.Context, request InspectRequest) (DocumentSnapshot, error) {
	if q == nil || q.runtime == nil {
		return DocumentSnapshot{}, ErrRuntimeClosed
	}
	if err := ctx.Err(); err != nil {
		return DocumentSnapshot{}, err
	}
	if request.DocumentID == "" {
		return DocumentSnapshot{}, ErrInvalidDocumentID
	}
	if err := validateInspectRequest(&request); err != nil {
		return DocumentSnapshot{}, err
	}
	session, err := q.runtime.session(request.DocumentID)
	if err != nil {
		return DocumentSnapshot{}, err
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return DocumentSnapshot{}, err
	}
	if session.closed {
		return DocumentSnapshot{}, ErrDocumentNotFound
	}
	inspector, ok := session.adapter.(DocumentInspector)
	if !ok {
		return DocumentSnapshot{}, ErrQueryNotSupported
	}
	content, diagnostics, err := inspector.Inspect(ctx, request)
	if err != nil {
		return DocumentSnapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return DocumentSnapshot{}, err
	}
	if !json.Valid(content) {
		return DocumentSnapshot{}, fmt.Errorf("%w: inspector returned invalid JSON", ErrInvalidQuery)
	}
	snapshot := DocumentSnapshot{
		Info:        session.infoLocked(),
		Content:     append(json.RawMessage(nil), content...),
		Diagnostics: append([]Diagnostic(nil), diagnostics...),
	}
	serialized, err := json.Marshal(snapshot)
	if err != nil {
		return DocumentSnapshot{}, fmt.Errorf("%w: serialize query result: %v", ErrInvalidQuery, err)
	}
	if len(serialized) > request.MaxBytes {
		return DocumentSnapshot{}, ErrQueryResultTooLarge
	}
	return snapshot, nil
}

func validateInspectRequest(request *InspectRequest) error {
	if request.Start < 0 || request.Start > MaxQueryOffset || request.Limit < 0 || request.MaxBytes < 0 {
		return ErrInvalidQuery
	}
	if request.Range != nil && request.Start != 0 {
		return ErrInvalidQuery
	}
	if request.Limit == 0 {
		request.Limit = defaultQueryItems
	} else if request.Limit > MaxQueryItems {
		return ErrInvalidQuery
	}
	if request.MaxBytes == 0 {
		request.MaxBytes = defaultQueryResultBytes
	} else if request.MaxBytes > MaxQueryResultBytes {
		return ErrInvalidQuery
	}
	if request.Range != nil {
		r := request.Range
		if r.StartRow < 1 || r.StartColumn < 1 || r.EndRow < r.StartRow || r.EndColumn < r.StartColumn {
			return ErrInvalidQuery
		}
		rows := r.EndRow - r.StartRow + 1
		columns := r.EndColumn - r.StartColumn + 1
		if rows <= 0 || columns <= 0 || rows > MaxQueryCells/columns {
			return ErrInvalidQuery
		}
	}
	return nil
}
