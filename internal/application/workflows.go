package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"glance/internal/document"
)

const (
	maxDocumentDiffPaths = 200
	maxDocumentDiffNodes = 20000
	maxDocumentDiffDepth = 64
)

var ErrDocumentCompareFormatMismatch = errors.New("documents use different formats and cannot be compared as the same neutral view")

type documentCompareResult struct {
	LeftDocumentID     string   `json:"left_document_id"`
	RightDocumentID    string   `json:"right_document_id"`
	Format             Format   `json:"format"`
	LeftRevision       uint64   `json:"left_revision"`
	RightRevision      uint64   `json:"right_revision"`
	Scope              any      `json:"scope"`
	ChangedPaths       []string `json:"changed_paths"`
	HasMoreDifferences bool     `json:"has_more_differences"`
}

type documentDiffState struct {
	visited int
	paths   []string
	more    bool
}

func registerAgentWorkflowCapabilities(catalog *Catalog) error {
	if err := catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "document.compare", Name: "比较文档内容",
			Description: "只读比较两个相同格式的实时文档在指定工作表/区域或分页范围内的中立内容视图，返回变化路径和双方修订号。默认只比较文档开头的有界分页；指定表格区域时比较该区域。此操作不修改文档，也不声称覆盖未读取的内容。",
			Category:    CapabilityWorkflow, Risk: RiskLow,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"left_document_id":{"type":"string","minLength":1},"right_document_id":{"type":"string","minLength":1},"left_expected_revision":{"type":"integer","minimum":0},"right_expected_revision":{"type":"integer","minimum":0},"sheet":{"type":"string"},"range":{"type":"object","properties":{"start_row":{"type":"integer","minimum":1},"start_column":{"type":"integer","minimum":1},"end_row":{"type":"integer","minimum":1},"end_column":{"type":"integer","minimum":1}},"required":["start_row","start_column","end_row","end_column"],"additionalProperties":false},"start":{"type":"integer","minimum":0,"maximum":1000000},"limit":{"type":"integer","minimum":0,"maximum":1000}},"required":["left_document_id","right_document_id"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"left_document_id":{"type":"string"},"right_document_id":{"type":"string"},"format":{"type":"string"},"left_revision":{"type":"integer"},"right_revision":{"type":"integer"},"scope":{"type":"object"},"changed_paths":{"type":"array","maxItems":200,"items":{"type":"string"}},"has_more_differences":{"type":"boolean"}},"required":["left_document_id","right_document_id","format","left_revision","right_revision","scope","changed_paths","has_more_differences"],"additionalProperties":false}`),
		},
		Handler: handleDocumentCompare,
	}); err != nil {
		return err
	}
	return registerDocumentPlanCapabilities(catalog)
}

func handleDocumentCompare(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
	var input struct {
		LeftDocumentID        string              `json:"left_document_id"`
		RightDocumentID       string              `json:"right_document_id"`
		LeftExpectedRevision  *uint64             `json:"left_expected_revision"`
		RightExpectedRevision *uint64             `json:"right_expected_revision"`
		Sheet                 string              `json:"sheet"`
		Range                 *document.CellRange `json:"range"`
		Start                 int                 `json:"start"`
		Limit                 int                 `json:"limit"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return CapabilityResult{}, ErrInvalidCommandArguments
	}
	input.LeftDocumentID = strings.TrimSpace(input.LeftDocumentID)
	input.RightDocumentID = strings.TrimSpace(input.RightDocumentID)
	if input.LeftDocumentID == "" || input.RightDocumentID == "" || input.LeftDocumentID == input.RightDocumentID {
		return CapabilityResult{}, ErrInvalidCommandArguments
	}
	leftInfo, err := capability.Runtime.GetDocument(ctx, input.LeftDocumentID)
	if err != nil {
		return CapabilityResult{}, err
	}
	rightInfo, err := capability.Runtime.GetDocument(ctx, input.RightDocumentID)
	if err != nil {
		return CapabilityResult{}, err
	}
	if leftInfo.Format != rightInfo.Format {
		return CapabilityResult{}, ErrDocumentCompareFormatMismatch
	}
	if input.LeftExpectedRevision != nil && *input.LeftExpectedRevision != leftInfo.Revision {
		return CapabilityResult{}, fmt.Errorf("%w: left current=%d expected=%d", ErrRevisionConflict, leftInfo.Revision, *input.LeftExpectedRevision)
	}
	if input.RightExpectedRevision != nil && *input.RightExpectedRevision != rightInfo.Revision {
		return CapabilityResult{}, fmt.Errorf("%w: right current=%d expected=%d", ErrRevisionConflict, rightInfo.Revision, *input.RightExpectedRevision)
	}
	inspect := InspectRequest{
		Sheet: input.Sheet, Range: input.Range, Start: input.Start, Limit: input.Limit,
		MaxBytes: defaultQueryResultBytes,
	}
	leftRequest, rightRequest := inspect, inspect
	leftRequest.DocumentID, rightRequest.DocumentID = input.LeftDocumentID, input.RightDocumentID
	left, err := capability.Runtime.Queries().InspectDocument(ctx, leftRequest)
	if err != nil {
		return CapabilityResult{}, err
	}
	right, err := capability.Runtime.Queries().InspectDocument(ctx, rightRequest)
	if err != nil {
		return CapabilityResult{}, err
	}
	leftCurrent, err := capability.Runtime.GetDocument(ctx, input.LeftDocumentID)
	if err != nil {
		return CapabilityResult{}, err
	}
	rightCurrent, err := capability.Runtime.GetDocument(ctx, input.RightDocumentID)
	if err != nil {
		return CapabilityResult{}, err
	}
	if left.Info.Revision != leftCurrent.Revision || right.Info.Revision != rightCurrent.Revision {
		return CapabilityResult{}, ErrRevisionConflict
	}
	leftValue, err := decodeJSONValue(left.Content)
	if err != nil {
		return CapabilityResult{}, fmt.Errorf("decode left document view: %w", ErrInvalidQuery)
	}
	rightValue, err := decodeJSONValue(right.Content)
	if err != nil {
		return CapabilityResult{}, fmt.Errorf("decode right document view: %w", ErrInvalidQuery)
	}
	diff := documentDiffState{paths: make([]string, 0, 16)}
	compareJSONValues(leftValue, rightValue, "", 0, &diff)
	if diff.paths == nil {
		diff.paths = []string{}
	}
	result := documentCompareResult{
		LeftDocumentID: input.LeftDocumentID, RightDocumentID: input.RightDocumentID,
		Format: left.Info.Format, LeftRevision: left.Info.Revision, RightRevision: right.Info.Revision,
		Scope: struct {
			Sheet string              `json:"sheet,omitempty"`
			Range *document.CellRange `json:"range,omitempty"`
			Start int                 `json:"start"`
			Limit int                 `json:"limit"`
		}{input.Sheet, input.Range, input.Start, normalizedQueryLimit(input.Limit)},
		ChangedPaths: diff.paths, HasMoreDifferences: diff.more,
	}
	data, err := json.Marshal(result)
	return CapabilityResult{Data: data}, err
}

func decodeJSONValue(data json.RawMessage) (json.RawMessage, error) {
	if !json.Valid(data) {
		return nil, errors.New("invalid JSON")
	}
	return append(json.RawMessage(nil), data...), nil
}

func normalizedQueryLimit(limit int) int {
	if limit <= 0 {
		return defaultQueryItems
	}
	return limit
}

func compareJSONValues(left, right json.RawMessage, path string, depth int, state *documentDiffState) {
	if state.more {
		return
	}
	state.visited++
	if state.visited > maxDocumentDiffNodes || depth > maxDocumentDiffDepth {
		state.more = true
		return
	}
	leftMap, leftIsMap := jsonObject(left)
	rightMap, rightIsMap := jsonObject(right)
	if leftIsMap && rightIsMap {
		keys := make([]string, 0, len(leftMap)+len(rightMap))
		seen := make(map[string]struct{}, len(leftMap)+len(rightMap))
		for key := range leftMap {
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
		for key := range rightMap {
			if _, ok := seen[key]; !ok {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			childPath := path + "/" + escapeJSONPointer(key)
			leftChild, leftExists := leftMap[key]
			rightChild, rightExists := rightMap[key]
			if !leftExists || !rightExists {
				appendDocumentDiff(childPath, state)
				if state.more {
					return
				}
				continue
			}
			compareJSONValues(leftChild, rightChild, childPath, depth+1, state)
			if state.more {
				return
			}
		}
		return
	}
	leftArray, leftIsArray := jsonArray(left)
	rightArray, rightIsArray := jsonArray(right)
	if leftIsArray && rightIsArray {
		maximum := max(len(leftArray), len(rightArray))
		for index := 0; index < maximum; index++ {
			childPath := fmt.Sprintf("%s/%d", path, index)
			if index >= len(leftArray) || index >= len(rightArray) {
				appendDocumentDiff(childPath, state)
				if state.more {
					return
				}
				continue
			}
			compareJSONValues(leftArray[index], rightArray[index], childPath, depth+1, state)
			if state.more {
				return
			}
		}
		return
	}
	if !equalJSONScalars(left, right) {
		appendDocumentDiff(path, state)
	}
}

func equalJSONScalars(left, right json.RawMessage) bool {
	if leftString, ok := jsonString(left); ok {
		rightString, rightOK := jsonString(right)
		return rightOK && leftString == rightString
	}
	if leftBool, ok := jsonBool(left); ok {
		rightBool, rightOK := jsonBool(right)
		return rightOK && leftBool == rightBool
	}
	return bytes.Equal(bytes.TrimSpace(left), bytes.TrimSpace(right))
}

func appendDocumentDiff(path string, state *documentDiffState) {
	if len(state.paths) >= maxDocumentDiffPaths {
		state.more = true
		return
	}
	if path == "" {
		path = "/"
	}
	state.paths = append(state.paths, path)
}

func escapeJSONPointer(value string) string {
	value = strings.ReplaceAll(value, "~", "~0")
	return strings.ReplaceAll(value, "/", "~1")
}
