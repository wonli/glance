package application

import (
	"context"
	"encoding/json"

	"glance/internal/document"
)

type PresentationTextExecutor interface {
	SetPresentationElementText(context.Context, uint32, int64, string) (bool, error)
}

type PresentationTextReader interface {
	ReadPresentationElement(context.Context, uint32, int64) (document.PresentationElement, error)
}

type PresentationStyleExecutor interface {
	SetPresentationElementStyle(context.Context, uint32, int64, document.PresentationElementStylePatch) (bool, error)
}

type PresentationTransformExecutor interface {
	SetPresentationElementTransform(context.Context, uint32, int64, document.PresentationRect, float64) (bool, error)
}

type PresentationNotesExecutor interface {
	SetPresentationSlideNotes(context.Context, uint32, string, string) (bool, error)
}

type PresentationDraftGuard interface {
	HasPendingPresentationDraft() bool
}

type PresentationEditorGuard interface {
	HasOpenPresentationTextEditor() bool
}

type PresentationCropDraftGuard interface {
	HasPendingPresentationCropDraft() bool
}

func registerPresentationEditCapabilities(catalog *Catalog) error {
	registrations := []CapabilityRegistration{
		{
			Descriptor: CapabilityDescriptor{
				ID: "presentation.element.text.copy", Name: "读取幻灯片元素文本",
				Description: "按 document.inspect 返回的稳定 slide_id 和 element_id 读取可编辑文本元素的纯文本。支持文本框和含可编辑文本的形状；返回内容可以包含段落换行，但不保留字体、段落样式或其他对象结构。若桌面任一窗口存在未提交文本草稿或打开的文本编辑器，Agent 读取会返回 pending_editor_draft。",
				Category:    CapabilityQuery, Formats: []Format{FormatPPTX}, Risk: RiskLow,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"来自 document.inspect 的稳定幻灯片 ID。"},"element_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"来自 document.inspect 的可编辑文本元素 ID。"}},"required":["slide_id","element_id"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"element_id":{"type":"integer","minimum":1,"maximum":4294967295},"text":{"type":"string","maxLength":32767}},"required":["slide_id","element_id","text"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					SlideID   uint64 `json:"slide_id"`
					ElementID int64  `json:"element_id"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) || input.ElementID <= 0 || uint64(input.ElementID) > uint64(^uint32(0)) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				if pendingPresentationDraft(capability) {
					return CapabilityResult{}, ErrPendingEditorDraft
				}
				reader, ok := capability.Adapter.(PresentationTextReader)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				element, err := reader.ReadPresentationElement(ctx, uint32(input.SlideID), input.ElementID)
				if err != nil {
					return CapabilityResult{}, err
				}
				text := element.PlainText()
				if len([]rune(text)) > 32767 {
					return CapabilityResult{}, ErrQueryResultTooLarge
				}
				return capabilityJSONResult(struct {
					SlideID   uint32 `json:"slide_id"`
					ElementID int64  `json:"element_id"`
					Text      string `json:"text"`
				}{uint32(input.SlideID), input.ElementID, text}, false)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "presentation.element.text.set", Name: "设置幻灯片元素文本",
				Description: "先用 document.inspect 查询目标幻灯片和元素，再将返回的稳定 slide_id、element_id 与当前 expected_revision 传入。设置可编辑文本元素；text 最多 32767 个字符，目前只允许单段落文本，换行、字段或特殊格式结构会被拒绝。若桌面任一窗口的文本编辑器处于打开状态或存在未提交文本草稿，Agent 写入返回 pending_editor_draft。",
				Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"来自 document.inspect 的稳定幻灯片 ID。"},"element_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"来自 document.inspect 的稳定可编辑文本元素 ID。"},"text":{"type":"string","maxLength":32767,"description":"要替换的单段落文本；最多 32767 个字符，不得包含换行。"}},"required":["slide_id","element_id","text"],"additionalProperties":false}`),
				OutputSchema: presentationEditOutputSchema(),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					SlideID   uint64 `json:"slide_id"`
					ElementID int64  `json:"element_id"`
					Text      string `json:"text"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) || input.ElementID <= 0 || uint64(input.ElementID) > uint64(^uint32(0)) || len([]rune(input.Text)) > 32767 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				if pendingPresentationDraft(capability) {
					return CapabilityResult{}, ErrPendingEditorDraft
				}
				executor, ok := capability.Adapter.(PresentationTextExecutor)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := executor.SetPresentationElementText(ctx, uint32(input.SlideID), input.ElementID, input.Text)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					SlideID   uint32 `json:"slide_id"`
					ElementID int64  `json:"element_id"`
					Changed   bool   `json:"changed"`
				}{uint32(input.SlideID), input.ElementID, changed}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "presentation.element.style.set", Name: "设置幻灯片元素样式",
				Description: "先用 document.inspect 查询目标对象的稳定 slide_id、element_id 和当前样式，再传入当前 expected_revision。一次可设置多个 PPTX 文本/形状样式字段：字体、字号、粗体、斜体、下划线、文字颜色、段落项目符号/对齐/间距、填充和轮廓；bullet 为 true/false，图片只支持轮廓。Agent 写入遇到桌面任一窗口打开文本编辑器或存在未提交草稿时会返回 pending_editor_draft。",
				Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  presentationStyleInputSchema(),
				OutputSchema: presentationEditOutputSchema(),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input presentationStyleInput
				if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) || input.ElementID <= 0 || uint64(input.ElementID) > uint64(^uint32(0)) || input.patchEmpty() {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				if pendingPresentationDraft(capability) {
					return CapabilityResult{}, ErrPendingEditorDraft
				}
				executor, ok := capability.Adapter.(PresentationStyleExecutor)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := executor.SetPresentationElementStyle(ctx, uint32(input.SlideID), input.ElementID, input.patch())
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					SlideID   uint32 `json:"slide_id"`
					ElementID int64  `json:"element_id"`
					Changed   bool   `json:"changed"`
				}{uint32(input.SlideID), input.ElementID, changed}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "presentation.element.transform.set", Name: "设置幻灯片元素几何变换",
				Description: "先用 document.inspect 查询目标对象的稳定 slide_id、element_id 和几何位置。按明确文档坐标设置 x、y、width、height（单位 point）与 rotation_deg（单位度）；宽高至少 8 point。适用于 Agent 精确布局，也供桌面拖动、缩放和旋转手势复用。调用必须提供当前 expected_revision。",
				Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"来自 document.inspect 的稳定幻灯片 ID。"},"element_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"来自 document.inspect 的稳定形状或图片 ID。"},"x":{"type":"number","description":"对象左上角 X 坐标，单位 point；允许对象位于页面外。"},"y":{"type":"number","description":"对象左上角 Y 坐标，单位 point；允许对象位于页面外。"},"width":{"type":"number","minimum":8,"description":"对象宽度，单位 point，至少 8。"},"height":{"type":"number","minimum":8,"description":"对象高度，单位 point，至少 8。"},"rotation_deg":{"type":"number","description":"顺时针旋转角度，单位度。"}},"required":["slide_id","element_id","x","y","width","height","rotation_deg"],"additionalProperties":false}`),
				OutputSchema: presentationEditOutputSchema(),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					SlideID     uint64  `json:"slide_id"`
					ElementID   int64   `json:"element_id"`
					X           float64 `json:"x"`
					Y           float64 `json:"y"`
					Width       float64 `json:"width"`
					Height      float64 `json:"height"`
					RotationDeg float64 `json:"rotation_deg"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) || input.ElementID <= 0 || uint64(input.ElementID) > uint64(^uint32(0)) {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				executor, ok := capability.Adapter.(PresentationTransformExecutor)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				bounds := document.PresentationRect{X: input.X, Y: input.Y, W: input.Width, H: input.Height}
				changed, err := executor.SetPresentationElementTransform(ctx, uint32(input.SlideID), input.ElementID, bounds, input.RotationDeg)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					SlideID   uint32 `json:"slide_id"`
					ElementID int64  `json:"element_id"`
					Changed   bool   `json:"changed"`
				}{uint32(input.SlideID), input.ElementID, changed}, changed)
			},
		},
		{
			Descriptor: CapabilityDescriptor{
				ID: "presentation.slide.notes.set", Name: "设置幻灯片演讲者备注",
				Description: "先用 document.inspect 查询目标幻灯片的稳定 slide_id 和当前 notes，再传入 expected_notes 与新 notes。只有当目标备注仍等于 expected_notes 时才会替换；这与 expected_revision 一起避免覆盖并发修改。最多 32767 个字符；桌面备注编辑器和 Agent 共用该命令。若桌面任一窗口打开文本编辑器或存在未提交草稿，Agent 写入返回 pending_editor_draft。",
				Category:    CapabilityCommand, Formats: []Format{FormatPPTX}, Risk: RiskMedium,
				RequiresDocument: true, RequiresRevision: true,
				InputSchema:  json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"来自 document.inspect 的稳定幻灯片 ID。"},"expected_notes":{"type":"string","maxLength":32767,"description":"document.inspect 返回的当前备注；仅在它仍匹配时执行写入。没有备注时传空字符串。"},"notes":{"type":"string","maxLength":32767,"description":"替换该页完整演讲者备注；最多 32767 个字符，可包含换行。"}},"required":["slide_id","expected_notes","notes"],"additionalProperties":false}`),
				OutputSchema: json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"changed":{"type":"boolean"}},"required":["slide_id","changed"],"additionalProperties":false}`),
			},
			Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
				var input struct {
					SlideID       uint64 `json:"slide_id"`
					ExpectedNotes string `json:"expected_notes"`
					Notes         string `json:"notes"`
				}
				if err := json.Unmarshal(raw, &input); err != nil || input.SlideID == 0 || input.SlideID > uint64(^uint32(0)) || len([]rune(input.ExpectedNotes)) > 32767 || len([]rune(input.Notes)) > 32767 {
					return CapabilityResult{}, ErrInvalidCommandArguments
				}
				if pendingPresentationDraft(capability) {
					return CapabilityResult{}, ErrPendingEditorDraft
				}
				executor, ok := capability.Adapter.(PresentationNotesExecutor)
				if !ok {
					return CapabilityResult{}, ErrCapabilityUnsupported
				}
				changed, err := executor.SetPresentationSlideNotes(ctx, uint32(input.SlideID), input.ExpectedNotes, input.Notes)
				if err != nil {
					return CapabilityResult{}, err
				}
				return capabilityJSONResult(struct {
					SlideID uint32 `json:"slide_id"`
					Changed bool   `json:"changed"`
				}{uint32(input.SlideID), changed}, changed)
			},
		},
	}
	for _, registration := range registrations {
		if err := catalog.Register(registration); err != nil {
			return err
		}
	}
	return nil
}

type presentationStyleInput struct {
	SlideID            uint64                        `json:"slide_id"`
	ElementID          int64                         `json:"element_id"`
	FontFamily         *string                       `json:"font_family"`
	FontSizePT         *float64                      `json:"font_size_pt"`
	Bold               *bool                         `json:"bold"`
	Italic             *bool                         `json:"italic"`
	Underline          *bool                         `json:"underline"`
	TextColorHex       *string                       `json:"text_color_hex"`
	ParagraphAlignment *string                       `json:"paragraph_alignment"`
	Bullet             *bool                         `json:"bullet"`
	LineSpacing        *document.PresentationSpacing `json:"line_spacing"`
	SpaceBefore        *document.PresentationSpacing `json:"space_before"`
	SpaceAfter         *document.PresentationSpacing `json:"space_after"`
	FillColorHex       *string                       `json:"fill_color_hex"`
	BorderColor        *string                       `json:"border_color"`
	BorderWidthPT      *float64                      `json:"border_width_pt"`
}

func (i presentationStyleInput) patchEmpty() bool {
	return i.FontFamily == nil && i.FontSizePT == nil && i.Bold == nil && i.Italic == nil && i.Underline == nil &&
		i.TextColorHex == nil && i.ParagraphAlignment == nil && i.Bullet == nil && i.LineSpacing == nil && i.SpaceBefore == nil &&
		i.SpaceAfter == nil && i.FillColorHex == nil && i.BorderColor == nil && i.BorderWidthPT == nil
}

func (i presentationStyleInput) patch() document.PresentationElementStylePatch {
	return document.PresentationElementStylePatch{
		FontFamily: i.FontFamily, FontSizePT: i.FontSizePT, Bold: i.Bold, Italic: i.Italic,
		Underline: i.Underline, TextColorHex: i.TextColorHex, ParagraphAlignment: i.ParagraphAlignment, Bullet: i.Bullet,
		LineSpacing: i.LineSpacing, SpaceBefore: i.SpaceBefore, SpaceAfter: i.SpaceAfter,
		FillColorHex: i.FillColorHex, BorderColor: i.BorderColor, BorderWidthPT: i.BorderWidthPT,
	}
}

func pendingPresentationDraft(capability CapabilityContext) bool {
	if capability.Operator.Kind == OperatorHuman {
		crop, ok := capability.Adapter.(PresentationCropDraftGuard)
		return ok && crop.HasPendingPresentationCropDraft()
	}
	guard, ok := capability.Adapter.(PresentationDraftGuard)
	if ok && guard.HasPendingPresentationDraft() {
		return true
	}
	editor, ok := capability.Adapter.(PresentationEditorGuard)
	return ok && editor.HasOpenPresentationTextEditor()
}

func presentationStyleInputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"properties":{
			"slide_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"来自 document.inspect 的稳定幻灯片 ID。"},
			"element_id":{"type":"integer","minimum":1,"maximum":4294967295,"description":"来自 document.inspect 的稳定文本、形状或图片 ID。"},
			"font_family":{"type":"string","minLength":1,"maxLength":128,"description":"文本字体名称；仅适用于可编辑文本。"},
			"font_size_pt":{"type":"number","minimum":1,"maximum":400,"description":"文本字号，单位 point；仅适用于可编辑文本。"},
			"bold":{"type":"boolean","description":"设置文本粗体状态；仅适用于可编辑文本。"},
			"italic":{"type":"boolean","description":"设置文本斜体状态；仅适用于可编辑文本。"},
			"underline":{"type":"boolean","description":"设置文本下划线状态；仅适用于可编辑文本。"},
			"text_color_hex":{"type":"string","maxLength":7,"description":"文本颜色，使用 #RRGGBB；空字符串清除显式颜色。"},
			"paragraph_alignment":{"type":"string","enum":["left","center","right","justify"],"description":"段落对齐方式；仅适用于可编辑文本。"},
			"bullet":{"type":"boolean","description":"设置段落项目符号状态；true 使用实心圆点，false 显式关闭项目符号。"},
			"line_spacing":{"type":"object","description":"行距；pct 为比例，pts 为 point。","properties":{"kind":{"type":"string","enum":["pct","pts"],"description":"间距单位。"},"value":{"type":"number","minimum":0,"maximum":1000,"description":"间距值；比例 1.5 表示 150%。"}},"required":["kind","value"],"additionalProperties":false},
			"space_before":{"type":"object","description":"段前间距；通常使用 pts。","properties":{"kind":{"type":"string","enum":["pct","pts"],"description":"间距单位。"},"value":{"type":"number","minimum":0,"maximum":1000,"description":"间距值；比例 1.5 表示 150%。"}},"required":["kind","value"],"additionalProperties":false},
			"space_after":{"type":"object","description":"段后间距；通常使用 pts。","properties":{"kind":{"type":"string","enum":["pct","pts"],"description":"间距单位。"},"value":{"type":"number","minimum":0,"maximum":1000,"description":"间距值；比例 1.5 表示 150%。"}},"required":["kind","value"],"additionalProperties":false},
			"fill_color_hex":{"type":"string","maxLength":7,"description":"形状填充颜色，使用 #RRGGBB；空字符串清除显式填充。"},
			"border_color":{"type":"string","maxLength":7,"description":"形状或图片轮廓颜色，使用 #RRGGBB；空字符串移除显式轮廓。"},
			"border_width_pt":{"type":"number","minimum":0,"maximum":120,"description":"形状或图片轮廓宽度，单位 point。"}
		},
		"required":["slide_id","element_id"],
		"additionalProperties":false
	}`)
}

func presentationEditOutputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"slide_id":{"type":"integer","minimum":1,"maximum":4294967295},"element_id":{"type":"integer","minimum":1,"maximum":4294967295},"changed":{"type":"boolean"}},"required":["slide_id","element_id","changed"],"additionalProperties":false}`)
}
