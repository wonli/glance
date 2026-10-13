package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"glance/internal/document"
)

const (
	maxPresentationAudioFileBytes = 64 << 20
	maxPresentationVideoFileBytes = 128 << 20
)

type PresentationAudioExecutor interface {
	AddPresentationAudio(context.Context, uint32, document.PresentationRect, document.PresentationAudioData) (int64, error)
}

type PresentationVideoExecutor interface {
	AddPresentationVideo(context.Context, uint32, document.PresentationRect, document.PresentationVideoData) (int64, error)
}

type presentationMediaInput struct {
	SlideID    uint64  `json:"slide_id"`
	Path       string  `json:"source_path"`
	MediaType  string  `json:"media_type"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
	Role       string  `json:"role"`
	AltText    string  `json:"alt_text"`
	Decorative bool    `json:"decorative"`
}

type mediaSchemaField struct {
	Type        string   `json:"type"`
	Minimum     *float64 `json:"minimum,omitempty"`
	Maximum     *float64 `json:"maximum,omitempty"`
	MinLength   *int     `json:"minLength,omitempty"`
	MaxLength   *int     `json:"maxLength,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Description string   `json:"description,omitempty"`
}

type mediaInputPropertiesSchema struct {
	SlideID    mediaSchemaField  `json:"slide_id"`
	SourcePath mediaSchemaField  `json:"source_path"`
	MediaType  mediaSchemaField  `json:"media_type"`
	X          mediaSchemaField  `json:"x"`
	Y          mediaSchemaField  `json:"y"`
	Width      mediaSchemaField  `json:"width"`
	Height     mediaSchemaField  `json:"height"`
	Role       *mediaSchemaField `json:"role,omitempty"`
	AltText    *mediaSchemaField `json:"alt_text,omitempty"`
	Decorative *mediaSchemaField `json:"decorative,omitempty"`
}

type mediaBoundsSchema struct {
	Type                 string                `json:"type"`
	Properties           mediaBoundsProperties `json:"properties"`
	Required             []string              `json:"required"`
	AdditionalProperties bool                  `json:"additionalProperties"`
}

type mediaBoundsProperties struct {
	X      mediaSchemaField `json:"x"`
	Y      mediaSchemaField `json:"y"`
	Width  mediaSchemaField `json:"width"`
	Height mediaSchemaField `json:"height"`
}

type mediaOutputPropertiesSchema struct {
	SlideID       mediaSchemaField  `json:"slide_id"`
	ElementID     mediaSchemaField  `json:"element_id"`
	MediaType     mediaSchemaField  `json:"media_type"`
	MediaSizeByte mediaSchemaField  `json:"media_size_bytes"`
	Bounds        mediaBoundsSchema `json:"bounds"`
	Changed       mediaSchemaField  `json:"changed"`
}

type mediaInputSchema struct {
	Type                 string                     `json:"type"`
	Properties           mediaInputPropertiesSchema `json:"properties"`
	Required             []string                   `json:"required"`
	AdditionalProperties bool                       `json:"additionalProperties"`
}

type mediaOutputSchema struct {
	Type                 string                      `json:"type"`
	Properties           mediaOutputPropertiesSchema `json:"properties"`
	Required             []string                    `json:"required"`
	AdditionalProperties bool                        `json:"additionalProperties"`
}

func registerPresentationMediaCapabilities(catalog *Catalog) error {
	if err := registerPresentationMediaCapability(catalog, false); err != nil {
		return err
	}
	return registerPresentationMediaCapability(catalog, true)
}

func registerPresentationMediaCapability(catalog *Catalog, video bool) error {
	capabilityID, name, description, maxBytes := "presentation.element.audio.create", "在幻灯片上插入音频", "在明确的 PPTX 幻灯片上嵌入本机文件系统中的 MP3 或 WAV 音频文件。source_path 是运行 Glance Runtime 的机器上的文件路径；插入位置使用 point。最多读取 64 MiB。先用 document.inspect 获取稳定 slide_id 和 expected_revision。Ribbon 音频入口选择文件后调用同一能力。", maxPresentationAudioFileBytes
	mediaTypes := []string{"audio/mpeg", "audio/wav"}
	if video {
		capabilityID, name, description, maxBytes = "presentation.element.video.create", "在幻灯片上插入视频", "在明确的 PPTX 幻灯片上嵌入本机文件系统中的 MP4 或 WebM 视频文件。source_path 是运行 Glance Runtime 的机器上的文件路径；位置和尺寸使用 point。最多读取 128 MiB。先用 document.inspect 获取稳定 slide_id 和 expected_revision。Ribbon 视频入口选择文件后调用同一能力。", maxPresentationVideoFileBytes
		mediaTypes = []string{"video/mp4", "video/webm"}
	}
	one, maxSlideID := float64(1), float64(^uint32(0))
	eight, maxDimension := float64(8), float64(10000)
	oneChar, maxPath, maxAltText := 1, 4096, 1000
	inputSchemaValue := mediaInputSchema{
		Type:                 "object",
		Required:             []string{"slide_id", "source_path", "media_type", "x", "y", "width", "height"},
		AdditionalProperties: false,
	}
	inputSchemaValue.Properties.SlideID = mediaSchemaField{Type: "integer", Minimum: &one, Maximum: &maxSlideID, Description: "document.inspect 返回的稳定幻灯片 ID。"}
	inputSchemaValue.Properties.SourcePath = mediaSchemaField{Type: "string", MinLength: &oneChar, MaxLength: &maxPath, Description: "运行 Glance Runtime 的机器上可读取的媒体文件路径。"}
	inputSchemaValue.Properties.MediaType = mediaSchemaField{Type: "string", Enum: mediaTypes, Description: "必须与文件扩展名及媒体内容匹配。"}
	inputSchemaValue.Properties.X = mediaSchemaField{Type: "number", Description: "媒体左上角 X 坐标，单位 point。"}
	inputSchemaValue.Properties.Y = mediaSchemaField{Type: "number", Description: "媒体左上角 Y 坐标，单位 point。"}
	inputSchemaValue.Properties.Width = mediaSchemaField{Type: "number", Minimum: &eight, Maximum: &maxDimension, Description: "媒体宽度，单位 point。"}
	inputSchemaValue.Properties.Height = mediaSchemaField{Type: "number", Minimum: &eight, Maximum: &maxDimension, Description: "媒体高度，单位 point。"}
	if video {
		inputSchemaValue.Properties.AltText = &mediaSchemaField{Type: "string", MaxLength: &maxAltText, Description: "视频的无障碍替代文本；装饰性视频留空。"}
		inputSchemaValue.Properties.Decorative = &mediaSchemaField{Type: "boolean", Description: "视频是否仅作装饰。"}
	} else {
		inputSchemaValue.Properties.Role = &mediaSchemaField{Type: "string", Enum: []string{"narration", "background", "effect"}, Description: "音频角色，省略时为 effect。"}
	}
	inputSchema, err := json.Marshal(inputSchemaValue)
	if err != nil {
		return err
	}
	outputSchemaValue := mediaOutputSchema{
		Type:                 "object",
		Required:             []string{"slide_id", "element_id", "media_type", "media_size_bytes", "bounds", "changed"},
		AdditionalProperties: false,
	}
	outputSchemaValue.Properties.SlideID = mediaSchemaField{Type: "integer", Minimum: &one, Maximum: &maxSlideID}
	outputSchemaValue.Properties.ElementID = mediaSchemaField{Type: "integer", Minimum: &one, Maximum: &maxSlideID}
	outputSchemaValue.Properties.MediaType = mediaSchemaField{Type: "string", Enum: mediaTypes}
	maxBytesNumber := float64(maxBytes)
	outputSchemaValue.Properties.MediaSizeByte = mediaSchemaField{Type: "integer", Minimum: &one, Maximum: &maxBytesNumber}
	outputSchemaValue.Properties.Bounds.Type = "object"
	outputSchemaValue.Properties.Bounds.Required = []string{"x", "y", "width", "height"}
	outputSchemaValue.Properties.Bounds.AdditionalProperties = false
	outputSchemaValue.Properties.Bounds.Properties.X = mediaSchemaField{Type: "number"}
	outputSchemaValue.Properties.Bounds.Properties.Y = mediaSchemaField{Type: "number"}
	outputSchemaValue.Properties.Bounds.Properties.Width = mediaSchemaField{Type: "number"}
	outputSchemaValue.Properties.Bounds.Properties.Height = mediaSchemaField{Type: "number"}
	outputSchemaValue.Properties.Changed = mediaSchemaField{Type: "boolean"}
	outputSchema, err := json.Marshal(outputSchemaValue)
	if err != nil {
		return err
	}
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: capabilityID, Name: name, Description: description,
			Category: CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema: inputSchema, OutputSchema: outputSchema,
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			if len(raw) == 0 || len(raw) > 16<<10 {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			var input presentationMediaInput
			if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) ||
				strings.TrimSpace(input.Path) == "" || len([]rune(input.Path)) > 4096 ||
				!validPresentationObjectBounds(input.X, input.Y, input.Width, input.Height) || len([]rune(input.AltText)) > 1000 ||
				(video && input.Decorative && input.AltText != "") {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if !containsString(mediaTypes, input.MediaType) {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if video {
				if input.Role != "" {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
			} else if input.Role != "" && input.Role != "narration" && input.Role != "background" && input.Role != "effect" {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			if pendingPresentationDraft(capability) {
				return CapabilityResult{}, ErrPendingEditorDraft
			}
			data, mediaType, err := readPresentationMediaFile(ctx, input.Path, maxBytes)
			if err != nil {
				return CapabilityResult{}, err
			}
			if mediaType != input.MediaType {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			trackKey := presentationMediaTrackKey(capability.Document.DocumentID, capability.RequestID, video)
			bounds := document.PresentationRect{X: input.X, Y: input.Y, W: input.Width, H: input.Height}
			var elementID int64
			if video {
				executor, ok := capability.Adapter.(PresentationVideoExecutor)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				elementID, err = executor.AddPresentationVideo(ctx, uint32(input.SlideID), bounds, document.PresentationVideoData{
					TrackKey: trackKey, MediaType: mediaType, AltText: input.AltText, Decorative: input.Decorative, Bytes: data,
				})
			} else {
				executor, ok := capability.Adapter.(PresentationAudioExecutor)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				role := input.Role
				if role == "" {
					role = "effect"
				}
				elementID, err = executor.AddPresentationAudio(ctx, uint32(input.SlideID), bounds, document.PresentationAudioData{
					TrackKey: trackKey, Role: role, MediaType: mediaType, Bytes: data,
				})
			}
			if err != nil {
				return CapabilityResult{}, err
			}
			if elementID <= 0 {
				return CapabilityResult{}, ErrInvalidCommandResult
			}
			return capabilityJSONResult(struct {
				SlideID        uint32 `json:"slide_id"`
				ElementID      int64  `json:"element_id"`
				MediaType      string `json:"media_type"`
				MediaSizeBytes int    `json:"media_size_bytes"`
				Bounds         struct {
					X      float64 `json:"x"`
					Y      float64 `json:"y"`
					Width  float64 `json:"width"`
					Height float64 `json:"height"`
				} `json:"bounds"`
				Changed bool `json:"changed"`
			}{SlideID: uint32(input.SlideID), ElementID: elementID, MediaType: mediaType, MediaSizeBytes: len(data),
				Bounds: struct {
					X      float64 `json:"x"`
					Y      float64 `json:"y"`
					Width  float64 `json:"width"`
					Height float64 `json:"height"`
				}{X: bounds.X, Y: bounds.Y, Width: bounds.W, Height: bounds.H}, Changed: true,
			}, true)
		},
	})
}

func readPresentationMediaFile(ctx context.Context, path string, maxBytes int) ([]byte, string, error) {
	mediaType := ""
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		mediaType = "audio/mpeg"
	case ".wav":
		mediaType = "audio/wav"
	case ".mp4":
		mediaType = "video/mp4"
	case ".webm":
		mediaType = "video/webm"
	default:
		return nil, "", ErrInvalidCommandArguments
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > int64(maxBytes) {
		return nil, "", ErrInvalidCommandArguments
	}
	limited := io.LimitReader(file, int64(maxBytes)+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", err
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if len(data) == 0 || len(data) > maxBytes {
		return nil, "", ErrInvalidCommandArguments
	}
	return data, mediaType, nil
}

func presentationMediaTrackKey(documentID, requestID string, video bool) string {
	kind := "audio"
	if video {
		kind = "video"
	}
	sum := sha256.Sum256([]byte(documentID + "\x00" + requestID))
	return "glance-" + kind + "-" + hex.EncodeToString(sum[:16])
}
