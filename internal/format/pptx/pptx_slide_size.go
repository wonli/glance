package pptx

import (
	"fmt"
	"math"
	"strconv"
)

const (
	minPresentationSlideSizePT = 36.0
	maxPresentationSlideSizePT = 56 * 72.0
)

type presentationSlideSizeOverride struct {
	Set       bool
	WidthEMU  int64
	HeightEMU int64
}

// SetSlideSize updates the presentation canvas dimensions. Existing elements
// retain their absolute point coordinates; they are not scaled or repositioned.
func (d *Document) SetSlideSize(widthPT, heightPT float64) (bool, error) {
	change, err := presentationSlideSize(widthPT, heightPT)
	if err != nil {
		return false, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return false, fmt.Errorf("演示文稿已关闭")
	}
	currentWidthEMU := int64(math.Round(d.widthPT * emuPerPoint))
	currentHeightEMU := int64(math.Round(d.heightPT * emuPerPoint))
	if currentWidthEMU == change.WidthEMU && currentHeightEMU == change.HeightEMU {
		return false, nil
	}
	d.widthPT = float64(change.WidthEMU) / emuPerPoint
	d.heightPT = float64(change.HeightEMU) / emuPerPoint
	d.slideSizeChange = change
	d.dirty = true
	d.revision++
	return true, nil
}

func presentationSlideSize(widthPT, heightPT float64) (presentationSlideSizeOverride, error) {
	if math.IsNaN(widthPT) || math.IsInf(widthPT, 0) || math.IsNaN(heightPT) || math.IsInf(heightPT, 0) ||
		widthPT < minPresentationSlideSizePT || widthPT > maxPresentationSlideSizePT ||
		heightPT < minPresentationSlideSizePT || heightPT > maxPresentationSlideSizePT {
		return presentationSlideSizeOverride{}, fmt.Errorf("幻灯片宽高必须在 %.0f 至 %.0f point 之间", minPresentationSlideSizePT, maxPresentationSlideSizePT)
	}
	return presentationSlideSizeOverride{
		Set:       true,
		WidthEMU:  int64(math.Round(widthPT * emuPerPoint)),
		HeightEMU: int64(math.Round(heightPT * emuPerPoint)),
	}, nil
}

func patchPresentationSlideSize(data []byte, change presentationSlideSizeOverride) ([]byte, error) {
	if !change.Set || change.WidthEMU <= 0 || change.HeightEMU <= 0 {
		return nil, fmt.Errorf("幻灯片尺寸补丁无效")
	}
	nodes, err := scanStyleXML(data)
	if err != nil {
		return nil, err
	}
	index := firstStyleNode(nodes, "sldSz")
	if index < 0 {
		return nil, fmt.Errorf("演示文稿缺少 p:sldSz")
	}
	node := nodes[index]
	startTag, err := setXMLStartTagAttributes(data[node.start:node.startTagEnd], map[string]string{
		"cx":   strconv.FormatInt(change.WidthEMU, 10),
		"cy":   strconv.FormatInt(change.HeightEMU, 10),
		"type": "custom",
	})
	if err != nil {
		return nil, err
	}
	return applyXMLReplacements(data, []shapeXMLReplacement{{start: node.start, end: node.startTagEnd, data: startTag}}), nil
}
