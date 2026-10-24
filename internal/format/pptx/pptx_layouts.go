package pptx

import (
	"archive/zip"
	"context"
	"fmt"
	"os"

	"glance/internal/document"
	"glance/internal/fileutil"

	gopptx "github.com/F31/go-pptx/v2/pptx"
)

const maxPresentationLayoutOptions = 500

// PresentationLayouts returns the document-local layout catalog. The index is
// revision-bound because the dependency does not expose public layout IDs.
func (d *Document) PresentationLayouts(ctx context.Context) (document.PresentationLayoutSummary, error) {
	if err := ctx.Err(); err != nil {
		return document.PresentationLayoutSummary{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.pres == nil {
		return document.PresentationLayoutSummary{}, fmt.Errorf("演示文稿已关闭")
	}
	layouts, err := d.pres.Layouts()
	if err != nil {
		return document.PresentationLayoutSummary{}, fmt.Errorf("读取演示文稿版式失败: %w", err)
	}
	result := document.PresentationLayoutSummary{
		Revision:  d.revision,
		Layouts:   make([]document.PresentationLayoutOption, 0, min(len(layouts), maxPresentationLayoutOptions)),
		Total:     len(layouts),
		Truncated: len(layouts) > maxPresentationLayoutOptions,
	}
	limit := min(len(layouts), maxPresentationLayoutOptions)
	for index := 0; index < limit; index++ {
		name := layouts[index].Name()
		if name == "" {
			name = fmt.Sprintf("版式 %d", index+1)
		}
		name, nameTruncated := boundedPresentationLayoutName(name)
		result.Layouts = append(result.Layouts, document.PresentationLayoutOption{Index: index, Name: name, NameTruncated: nameTruncated})
	}
	return result, nil
}

// SetSlideLayoutByID changes the layout relationship for one slide. Layout
// indexes are scoped to the document revision that was used to read the
// catalog. The override stays adapter-owned until Save or SnapshotBytes writes
// the package relationship.
func (d *Document) SetSlideLayoutByID(ctx context.Context, slideID uint32, layoutIndex int, expectedRevision uint64) (document.PresentationLayoutOption, bool, error) {
	if err := ctx.Err(); err != nil {
		return document.PresentationLayoutOption{}, false, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return document.PresentationLayoutOption{}, false, fmt.Errorf("演示文稿已关闭")
	}
	if expectedRevision == 0 || d.revision != expectedRevision {
		return document.PresentationLayoutOption{}, false, fmt.Errorf("演示文稿修订已变化，请重新读取版式目录")
	}

	slides, err := d.pres.Slides()
	if err != nil {
		return document.PresentationLayoutOption{}, false, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	slideIndex := -1
	for index, slide := range slides {
		if uint32(slide.ID()) == slideID {
			slideIndex = index
			break
		}
	}
	if slideIndex < 0 {
		return document.PresentationLayoutOption{}, false, fmt.Errorf("未找到幻灯片 ID %d", slideID)
	}

	layouts, err := d.pres.Layouts()
	if err != nil {
		return document.PresentationLayoutOption{}, false, fmt.Errorf("读取演示文稿版式失败: %w", err)
	}
	if layoutIndex < 0 || layoutIndex >= len(layouts) || layoutIndex >= maxPresentationLayoutOptions {
		return document.PresentationLayoutOption{}, false, fmt.Errorf("版式索引超出当前目录范围: %d", layoutIndex)
	}
	pkg, cleanup, err := d.serializedPackageLocked(ctx)
	if err != nil {
		return document.PresentationLayoutOption{}, false, err
	}
	defer cleanup()
	if err := ctx.Err(); err != nil {
		return document.PresentationLayoutOption{}, false, err
	}
	if len(pkg.layoutParts) != len(layouts) || slideIndex >= len(pkg.slideParts) {
		return document.PresentationLayoutOption{}, false, fmt.Errorf("版式目录与当前 PPTX 部件顺序不一致")
	}
	layoutPart := pkg.layoutParts[layoutIndex]
	masterPart := pkg.layoutMasterParts[layoutPart]
	if pkg.parts[layoutPart] == nil || masterPart == "" || pkg.parts[masterPart] == nil {
		return document.PresentationLayoutOption{}, false, fmt.Errorf("目标版式缺少有效的版式或母版部件")
	}
	slidePart := pkg.slideParts[slideIndex]
	currentLayout, ok := pkg.relatedPart(slidePart, pptxSlideLayoutRelationshipType)
	if !ok {
		return document.PresentationLayoutOption{}, false, fmt.Errorf("幻灯片 %d 缺少有效的版式关系", slideID)
	}
	if override := d.slideLayoutChanges[slideID]; override != "" {
		currentLayout = override
	}
	optionName := layouts[layoutIndex].Name()
	if optionName == "" {
		optionName = fmt.Sprintf("版式 %d", layoutIndex+1)
	}
	optionName, nameTruncated := boundedPresentationLayoutName(optionName)
	option := document.PresentationLayoutOption{Index: layoutIndex, Name: optionName, NameTruncated: nameTruncated}
	if currentLayout == layoutPart {
		return option, false, nil
	}

	d.ensureSlideProjectionCacheCount(len(slides))
	if d.slideLayoutChanges == nil {
		d.slideLayoutChanges = make(map[uint32]string)
	}
	d.slideLayoutChanges[slideID] = layoutPart
	d.slideInherited[slideIndex] = pkg.inheritedElements(slidePart, layoutPart, masterPart)
	d.slidePlaceholderBounds[slideIndex] = pkg.placeholderBounds(slidePart, layoutPart, masterPart)
	d.slidePlaceholderXfrms[slideIndex] = pkg.placeholderTransforms(slidePart, layoutPart, masterPart)
	if !d.slideBackgroundChanges[slideIndex].Set {
		d.slideBackgrounds[slideIndex] = pkg.resolveBackground(slidePart, layoutPart, masterPart)
	}
	d.dirty = true
	d.revision++
	return option, true, nil
}

func (d *Document) serializedPackageLocked(ctx context.Context) (*pptxPackage, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	tmp, err := fileutil.CreateTemp("glance-pptx-layout-*.pptx")
	if err != nil {
		return nil, nil, fmt.Errorf("创建版式校验文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, nil, fmt.Errorf("关闭版式校验文件失败: %w", err)
	}
	cleanupPath := func() { _ = os.Remove(tmpPath) }
	if _, err := d.pres.Save(ctx, tmpPath, gopptx.WithSaveOverwrite(true)); err != nil {
		cleanupPath()
		return nil, nil, fmt.Errorf("序列化演示文稿版式失败: %w", err)
	}
	file, err := os.Open(tmpPath)
	if err != nil {
		cleanupPath()
		return nil, nil, fmt.Errorf("打开版式校验文件失败: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		cleanupPath()
		return nil, nil, fmt.Errorf("读取版式校验文件信息失败: %w", err)
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		_ = file.Close()
		cleanupPath()
		return nil, nil, fmt.Errorf("解析版式校验文件失败: %w", err)
	}
	pkg := newPPTXPackage(reader)
	cleanup := func() {
		_ = file.Close()
		cleanupPath()
	}
	return pkg, cleanup, nil
}

func boundedPresentationLayoutName(name string) (string, bool) {
	count := 0
	for offset := range name {
		count++
		if count > 256 {
			return name[:offset], true
		}
	}
	return name, false
}
