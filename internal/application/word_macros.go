package application

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
)

const maxWordContentTypesBytes = 1 << 20

type wordMacroInspection struct {
	MacroPayloadDetected bool   `json:"macro_payload_detected"`
	ExecutionSupported   bool   `json:"execution_supported"`
	Summary              string `json:"summary"`
}

func registerWordMacroInspectionCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "word.macros.inspect", Name: "检查 Word 宏部件",
			Description: "当需要确认 DOCX 是否携带 VBA 宏部件时调用。目标必须是明确的 DOCX 实时文档与修订版本；只检查 ZIP 部件名和宏启用内容类型，不读取或执行宏代码，也不扫描或判断代码是否恶意。",
			Category:    CapabilityQuery, Formats: []Format{FormatDocx}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"macro_payload_detected":{"type":"boolean"},"execution_supported":{"type":"boolean","const":false},"summary":{"type":"string"}},"required":["macro_payload_detected","execution_supported","summary"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, _ json.RawMessage) (CapabilityResult, error) {
			reader, ok := capability.Adapter.(DocumentSnapshotReader)
			if !ok {
				return CapabilityResult{}, ErrCapabilityUnsupported
			}
			snapshot, err := reader.Snapshot(ctx)
			if err != nil {
				return CapabilityResult{}, err
			}
			detected, err := detectWordMacroPayload(snapshot)
			if err != nil {
				return CapabilityResult{}, err
			}
			inspection := wordMacroInspection{MacroPayloadDetected: detected}
			if detected {
				inspection.Summary = "检测到 VBA 宏部件；Glance 不会执行宏，也不判断宏代码是否安全。"
			} else {
				inspection.Summary = "未发现宏部件；Glance 不会执行宏，也不提供恶意宏扫描。"
			}
			return capabilityJSONResult(inspection, false)
		},
	})
}

func detectWordMacroPayload(snapshot []byte) (bool, error) {
	archive, err := zip.NewReader(bytes.NewReader(snapshot), int64(len(snapshot)))
	if err != nil {
		return false, err
	}
	macroPayload := false
	macroEnabledContentType := false
	for _, file := range archive.File {
		if strings.EqualFold(file.Name, "word/vbaProject.bin") {
			macroPayload = true
		}
		if file.Name != "[Content_Types].xml" {
			continue
		}
		part, err := file.Open()
		if err != nil {
			return false, err
		}
		data, readErr := io.ReadAll(io.LimitReader(part, maxWordContentTypesBytes+1))
		closeErr := part.Close()
		if readErr != nil {
			return false, readErr
		}
		if closeErr != nil {
			return false, closeErr
		}
		if len(data) > maxWordContentTypesBytes {
			return false, ErrQueryResultTooLarge
		}
		macroEnabledContentType = bytes.Contains(bytes.ToLower(data), []byte("macroenabled"))
	}
	return macroPayload || macroEnabledContentType, nil
}
