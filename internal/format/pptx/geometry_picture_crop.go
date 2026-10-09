package pptx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"glance/internal/document"
	"io"
	"strconv"
	"strings"
)

type pictureCropElementSpan struct {
	start, startTagEnd, end int
	qName                   string
	selfClose               bool
	parent                  string
}

type pictureCropXMLTags struct {
	srcRect       *pictureCropElementSpan
	blipQName     string
	insertAt      int
	blipFillClose int
}

func inspectPictureCropXMLTags(fragment []byte) (pictureCropXMLTags, error) {
	tags := pictureCropXMLTags{insertAt: -1, blipFillClose: -1}
	type frame struct {
		name        xml.Name
		parent      string
		start       int
		startTagEnd int
		selfClose   bool
	}
	decoder := xml.NewDecoder(bytes.NewReader(fragment))
	var stack []frame
	for {
		before := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return pictureCropXMLTags{}, fmt.Errorf("解析图片裁剪 XML 失败: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			start := findNextXMLStart(fragment, before)
			end, err := xmlStartTagEnd(fragment, start)
			if err != nil {
				return pictureCropXMLTags{}, err
			}
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1].name.Local
			}
			selfClose := bytes.HasSuffix(bytes.TrimSpace(fragment[start:end-1]), []byte("/"))
			current := frame{name: value.Name, parent: parent, start: start, startTagEnd: end, selfClose: selfClose}
			if value.Name.Local == "blip" && parent == "blipFill" && tags.blipQName == "" {
				tags.blipQName, err = xmlElementQName(fragment[start:end])
				if err != nil {
					return pictureCropXMLTags{}, err
				}
			}
			if parent == "blipFill" {
				switch value.Name.Local {
				case "srcRect":
					span := &pictureCropElementSpan{start: start, startTagEnd: end, qName: "", selfClose: selfClose, parent: parent}
					span.qName, err = xmlElementQName(fragment[start:end])
					if err != nil {
						return pictureCropXMLTags{}, err
					}
					// The end offset is filled by the matching EndElement below.
					if tags.srcRect == nil {
						tags.srcRect = span
					}
				case "stretch", "tile":
					if tags.insertAt < 0 {
						tags.insertAt = start
					}
				}
			}
			stack = append(stack, current)
		case xml.EndElement:
			if len(stack) == 0 || stack[len(stack)-1].name.Local != value.Name.Local {
				return pictureCropXMLTags{}, fmt.Errorf("图片裁剪 XML 元素层级不匹配")
			}
			current := stack[len(stack)-1]
			if current.name.Local == "srcRect" && current.parent == "blipFill" && tags.srcRect != nil && tags.srcRect.start == current.start {
				tags.srcRect.end = int(decoder.InputOffset())
			}
			if current.name.Local == "blipFill" && tags.blipFillClose < 0 {
				tags.blipFillClose = findNextXMLStart(fragment, before)
			}
			stack = stack[:len(stack)-1]
		}
	}
	if tags.blipQName == "" || tags.blipFillClose < 0 {
		return pictureCropXMLTags{}, fmt.Errorf("图片对象缺少有效的 p:blipFill/a:blip")
	}
	if tags.insertAt < 0 {
		tags.insertAt = tags.blipFillClose
	}
	if tags.srcRect != nil && tags.srcRect.end <= tags.srcRect.start {
		return pictureCropXMLTags{}, fmt.Errorf("a:srcRect 元素不完整")
	}
	return tags, nil
}

func patchShapePictureCropFragment(fragment []byte, crop document.PresentationCrop) ([]byte, error) {
	tags, err := inspectPictureCropXMLTags(fragment)
	if err != nil {
		return nil, err
	}
	allZero := crop == (document.PresentationCrop{})
	if tags.srcRect != nil {
		if allZero {
			result := append([]byte(nil), fragment...)
			return append(result[:tags.srcRect.start], result[tags.srcRect.end:]...), nil
		}
		startTag := fragment[tags.srcRect.start:tags.srcRect.startTagEnd]
		updated, err := setXMLStartTagAttributes(startTag, map[string]string{
			"l": strconv.Itoa(crop.Left), "t": strconv.Itoa(crop.Top),
			"r": strconv.Itoa(crop.Right), "b": strconv.Itoa(crop.Bottom),
		})
		if err != nil {
			return nil, err
		}
		result := append([]byte(nil), fragment...)
		return append(result[:tags.srcRect.start], append(updated, result[tags.srcRect.startTagEnd:]...)...), nil
	}
	if allZero {
		return append([]byte(nil), fragment...), nil
	}
	prefix := ""
	if separator := strings.IndexByte(tags.blipQName, ':'); separator >= 0 {
		prefix = tags.blipQName[:separator+1]
	}
	insert := []byte(fmt.Sprintf(`<%ssrcRect l="%d" t="%d" r="%d" b="%d"/>`, prefix, crop.Left, crop.Top, crop.Right, crop.Bottom))
	result := make([]byte, 0, len(fragment)+len(insert))
	result = append(result, fragment[:tags.insertAt]...)
	result = append(result, insert...)
	result = append(result, fragment[tags.insertAt:]...)
	return result, nil
}
