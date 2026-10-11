package pptx

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"glance/internal/document"

	gopptx "github.com/F31/go-pptx/v2/pptx"
)

const (
	pptxCommentRelationshipType        = pptxOfficeRelationshipNS + "/comments"
	pptxCommentAuthorsRelationshipType = pptxOfficeRelationshipNS + "/commentAuthors"
	pptxCommentContentType             = "application/vnd.openxmlformats-officedocument.presentationml.comments+xml"
	pptxCommentAuthorsContentType      = "application/vnd.openxmlformats-officedocument.presentationml.commentAuthors+xml"
	pptxPresentationMLNamespace        = "http://schemas.openxmlformats.org/presentationml/2006/main"
	pptxMaxCommentCharacters           = 8192
	pptxMaxCommentAuthorsBytes         = 8 << 20
	pptxMaxCommentPartBytes            = 16 << 20
	pptxMaxComments                    = 10000
)

type pptxCommentAuthor struct {
	ID       uint32
	Name     string
	Initials string
	LastIdx  uint32
	ColorIdx uint32
}

type pptxCommentRecord struct {
	Comment  document.PresentationComment
	AuthorID uint32
	Index    uint32
	PartName string
}

type pptxCommentState struct {
	Authors                  []pptxCommentAuthor
	Records                  []pptxCommentRecord
	AuthorsPart              string
	PresentationRelsPart     string
	ContentTypesPart         string
	SlideParts               map[uint32]string
	SlideCommentsParts       map[uint32]string
	PartData                 map[string][]byte
	PackageEdits             map[string][]byte
	PartNames                map[string]bool
	UnsupportedModernComment bool
	LoadErr                  error
}

func newPPTXCommentState() *pptxCommentState {
	return &pptxCommentState{
		PresentationRelsPart: relationshipPartPath("ppt/presentation.xml"),
		ContentTypesPart:     "[Content_Types].xml",
		SlideParts:           make(map[uint32]string),
		SlideCommentsParts:   make(map[uint32]string),
		PartData:             make(map[string][]byte),
		PackageEdits:         make(map[string][]byte),
		PartNames:            make(map[string]bool),
	}
}

func clonePPTXCommentState(source *pptxCommentState) *pptxCommentState {
	if source == nil {
		return newPPTXCommentState()
	}
	clone := *source
	clone.Authors = append([]pptxCommentAuthor(nil), source.Authors...)
	clone.Records = append([]pptxCommentRecord(nil), source.Records...)
	clone.SlideParts = make(map[uint32]string, len(source.SlideParts))
	for id, name := range source.SlideParts {
		clone.SlideParts[id] = name
	}
	clone.SlideCommentsParts = make(map[uint32]string, len(source.SlideCommentsParts))
	for id, name := range source.SlideCommentsParts {
		clone.SlideCommentsParts[id] = name
	}
	clone.PartData = clonePPTXCommentByteMap(source.PartData)
	clone.PackageEdits = clonePPTXCommentByteMap(source.PackageEdits)
	clone.PartNames = make(map[string]bool, len(source.PartNames))
	for name, exists := range source.PartNames {
		clone.PartNames[name] = exists
	}
	return &clone
}

func clonePPTXCommentByteMap(source map[string][]byte) map[string][]byte {
	clone := make(map[string][]byte, len(source))
	for name, data := range source {
		clone[name] = append([]byte(nil), data...)
	}
	return clone
}

type pptxCommentListXML struct {
	XMLName xml.Name         `xml:"http://schemas.openxmlformats.org/presentationml/2006/main cmLst"`
	Items   []pptxCommentXML `xml:"http://schemas.openxmlformats.org/presentationml/2006/main cm"`
}

type pptxCommentXML struct {
	AuthorID uint32 `xml:"authorId,attr"`
	Date     string `xml:"dt,attr"`
	Index    uint32 `xml:"idx,attr"`
	Position struct {
		X int64 `xml:"x,attr"`
		Y int64 `xml:"y,attr"`
	} `xml:"http://schemas.openxmlformats.org/presentationml/2006/main pos"`
	Text string `xml:"http://schemas.openxmlformats.org/presentationml/2006/main text"`
}

type pptxCommentAuthorsXML struct {
	XMLName xml.Name               `xml:"http://schemas.openxmlformats.org/presentationml/2006/main cmAuthorLst"`
	Authors []pptxCommentAuthorXML `xml:"http://schemas.openxmlformats.org/presentationml/2006/main cmAuthor"`
}

type pptxCommentAuthorXML struct {
	ID       uint32 `xml:"id,attr"`
	Name     string `xml:"name,attr"`
	Initials string `xml:"initials,attr"`
	LastIdx  uint32 `xml:"lastIdx,attr"`
	ColorIdx uint32 `xml:"clrIdx,attr"`
}

func (d *Document) loadPPTXCommentState(zr *zip.Reader) {
	state := newPPTXCommentState()
	d.presentationComments = state
	if d.pres == nil || zr == nil {
		return
	}
	pkg := newPPTXPackage(zr)
	for name := range pkg.parts {
		state.PartNames[name] = true
	}
	state.PresentationRelsPart = relationshipPartPath("ppt/presentation.xml")
	state.ContentTypesPart = "[Content_Types].xml"
	state.PartData[state.PresentationRelsPart] = packagePartBytes(pkg, state.PresentationRelsPart)
	state.PartData[state.ContentTypesPart] = packagePartBytes(pkg, state.ContentTypesPart)

	if authorsPart, ok := pkg.relatedPart("ppt/presentation.xml", pptxCommentAuthorsRelationshipType); ok {
		state.AuthorsPart = authorsPart
		data, err := packagePartBytesLimited(pkg, authorsPart, pptxMaxCommentAuthorsBytes)
		if err != nil {
			state.LoadErr = fmt.Errorf("读取 PPTX 批注作者部件失败: %w", err)
			return
		}
		state.PartData[authorsPart] = data
		var authors pptxCommentAuthorsXML
		if len(data) == 0 || xml.Unmarshal(data, &authors) != nil || authors.XMLName.Space != pptxPresentationMLNamespace || authors.XMLName.Local != "cmAuthorLst" {
			state.LoadErr = errors.New("PPTX 批注作者部件无效")
			return
		}
		if len(authors.Authors) > pptxMaxComments {
			state.LoadErr = fmt.Errorf("PPTX 批注作者数量超过上限 %d", pptxMaxComments)
			return
		}
		seen := make(map[uint32]bool, len(authors.Authors))
		for _, author := range authors.Authors {
			if seen[author.ID] || author.Name == "" {
				state.LoadErr = fmt.Errorf("PPTX 批注作者 ID %d 重复或名称为空", author.ID)
				return
			}
			seen[author.ID] = true
			state.Authors = append(state.Authors, pptxCommentAuthor{
				ID: author.ID, Name: author.Name, Initials: author.Initials,
				LastIdx: author.LastIdx, ColorIdx: author.ColorIdx,
			})
		}
	}

	slides, err := d.pres.Slides()
	if err != nil {
		state.LoadErr = fmt.Errorf("读取幻灯片批注失败: %w", err)
		return
	}
	for index, slide := range slides {
		slideID := uint32(slide.ID())
		slidePart := strings.TrimPrefix(path.Clean(slide.PartName()), "/")
		state.SlideParts[slideID] = slidePart
		for _, rel := range pkg.rels[slidePart] {
			if rel.External || !strings.HasSuffix(rel.Type, "/comments") || !strings.Contains(rel.Type, "2018/10/relationships") {
				continue
			}
			state.UnsupportedModernComment = true
		}
		commentsPart, ok := pkg.relatedPart(slidePart, pptxCommentRelationshipType)
		if !ok {
			continue
		}
		data, err := packagePartBytesLimited(pkg, commentsPart, pptxMaxCommentPartBytes)
		if err != nil {
			state.LoadErr = fmt.Errorf("读取 PPTX 幻灯片 %d 的批注部件失败: %w", index+1, err)
			return
		}
		if len(data) == 0 {
			state.LoadErr = fmt.Errorf("PPTX 幻灯片 %d 的批注部件缺失", index+1)
			return
		}
		state.SlideCommentsParts[slideID] = commentsPart
		state.PartData[commentsPart] = data
		var list pptxCommentListXML
		if err := xml.Unmarshal(data, &list); err != nil || list.XMLName.Space != pptxPresentationMLNamespace || list.XMLName.Local != "cmLst" {
			state.LoadErr = fmt.Errorf("PPTX 幻灯片 %d 的经典批注部件无效", index+1)
			return
		}
		seen := make(map[string]bool, len(list.Items))
		for _, item := range list.Items {
			if len(state.Records) >= pptxMaxComments {
				state.LoadErr = fmt.Errorf("PPTX 批注数量超过上限 %d", pptxMaxComments)
				return
			}
			if item.Index == 0 || item.AuthorID == 0 && len(state.Authors) == 0 {
				state.LoadErr = fmt.Errorf("PPTX 幻灯片 %d 包含无效批注索引或缺少作者部件", index+1)
				return
			}
			author, exists := state.authorByID(item.AuthorID)
			if !exists {
				state.LoadErr = fmt.Errorf("PPTX 批注引用了不存在的作者 ID %d", item.AuthorID)
				return
			}
			id := pptxCommentID(item.AuthorID, item.Index)
			if seen[id] {
				state.LoadErr = fmt.Errorf("PPTX 幻灯片 %d 的批注 ID %s 重复", index+1, id)
				return
			}
			seen[id] = true
			state.Records = append(state.Records, pptxCommentRecord{
				Comment: document.PresentationComment{
					ID: id, SlideID: slideID, SlideNumber: index + 1,
					Author: author.Name, Text: item.Text,
					XPT: float64(item.Position.X) / emuPerPoint, YPT: float64(item.Position.Y) / emuPerPoint,
				},
				AuthorID: item.AuthorID, Index: item.Index, PartName: commentsPart,
			})
			if item.Index > author.LastIdx {
				state.setAuthorLastIndex(author.ID, item.Index)
			}
		}
	}
	sort.SliceStable(state.Records, func(i, j int) bool {
		if state.Records[i].Comment.SlideNumber != state.Records[j].Comment.SlideNumber {
			return state.Records[i].Comment.SlideNumber < state.Records[j].Comment.SlideNumber
		}
		if state.Records[i].AuthorID != state.Records[j].AuthorID {
			return state.Records[i].AuthorID < state.Records[j].AuthorID
		}
		return state.Records[i].Index < state.Records[j].Index
	})
}

func packagePartBytes(pkg *pptxPackage, name string) []byte {
	part := pkg.parts[name]
	if part == nil {
		return nil
	}
	data, err := readOPCPart(part)
	if err != nil {
		return nil
	}
	return data
}

func packagePartBytesLimited(pkg *pptxPackage, name string, maxBytes int64) ([]byte, error) {
	part := pkg.parts[name]
	if part == nil {
		return nil, fmt.Errorf("PPTX 缺少部件 %q", name)
	}
	return readOPCPartLimited(part, maxBytes)
}

func (s *pptxCommentState) authorByID(id uint32) (pptxCommentAuthor, bool) {
	for _, author := range s.Authors {
		if author.ID == id {
			return author, true
		}
	}
	return pptxCommentAuthor{}, false
}

func (s *pptxCommentState) authorIndexByID(id uint32) int {
	for i := range s.Authors {
		if s.Authors[i].ID == id {
			return i
		}
	}
	return -1
}

func (s *pptxCommentState) setAuthorLastIndex(id, index uint32) {
	if position := s.authorIndexByID(id); position >= 0 && index > s.Authors[position].LastIdx {
		s.Authors[position].LastIdx = index
	}
}

func (d *Document) ListPresentationComments(slideID uint32, start, limit int) ([]document.PresentationComment, int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.pres == nil {
		return nil, 0, errors.New("演示文稿已关闭")
	}
	state := d.presentationComments
	if state == nil {
		return []document.PresentationComment{}, 0, nil
	}
	if state.LoadErr != nil {
		return nil, 0, state.LoadErr
	}
	if start < 0 || limit < 1 || limit > 100 {
		return nil, 0, errors.New("PPTX 批注分页参数无效")
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return nil, 0, fmt.Errorf("读取幻灯片批注所属页失败: %w", err)
	}
	slideNumbers := make(map[uint32]int, len(slides))
	for index, slide := range slides {
		slideNumbers[uint32(slide.ID())] = index + 1
	}
	filtered := make([]document.PresentationComment, 0)
	for _, record := range state.Records {
		slideNumber, exists := slideNumbers[record.Comment.SlideID]
		if exists && (slideID == 0 || record.Comment.SlideID == slideID) {
			comment := record.Comment
			comment.SlideNumber = slideNumber
			filtered = append(filtered, comment)
		}
	}
	total := len(filtered)
	if start >= total {
		return []document.PresentationComment{}, total, nil
	}
	end := min(total, start+limit)
	return append([]document.PresentationComment(nil), filtered[start:end]...), total, nil
}

func (d *Document) AddPresentationComment(slideID uint32, xPT, yPT float64, authorName, text string) (string, bool, error) {
	text = normalizePPTXCommentText(text)
	authorName = strings.Join(strings.Fields(authorName), " ")
	if authorName == "" {
		authorName = "Glance"
	}
	if utf8.RuneCountInString(text) == 0 || utf8.RuneCountInString(text) > pptxMaxCommentCharacters || utf8.RuneCountInString(authorName) > 256 || !validXMLText(text) || !validXMLText(authorName) {
		return "", false, errors.New("PPTX 批注需要有效 UTF-8 文本：正文最多 8192 个 Unicode 字符，作者最多 256 个 Unicode 字符")
	}
	if !finite(xPT) || !finite(yPT) || xPT < 0 || yPT < 0 {
		return "", false, errors.New("PPTX 批注位置必须是非负有限点坐标")
	}
	xPT = math.Round(xPT*emuPerPoint) / emuPerPoint
	yPT = math.Round(yPT*emuPerPoint) / emuPerPoint

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return "", false, errors.New("演示文稿已关闭")
	}
	state := d.presentationComments
	if state == nil {
		state = newPPTXCommentState()
	}
	originalState := state
	state = clonePPTXCommentState(state)
	d.presentationComments = state
	committed := false
	defer func() {
		if !committed {
			d.presentationComments = originalState
		}
	}()
	if state.LoadErr != nil {
		return "", false, state.LoadErr
	}
	if state.UnsupportedModernComment {
		return "", false, errors.New("此演示文稿包含新版 PowerPoint 批注；当前仅支持经典幻灯片批注的编辑")
	}
	if len(state.Records) >= pptxMaxComments {
		return "", false, fmt.Errorf("PPTX 批注数量超过上限 %d", pptxMaxComments)
	}
	if xPT > d.widthPT || yPT > d.heightPT {
		return "", false, errors.New("PPTX 批注位置超出演示文稿画布")
	}
	for _, partName := range []string{state.PresentationRelsPart, state.ContentTypesPart} {
		if partName == "" {
			return "", false, errors.New("PPTX 批注包元数据路径无效")
		}
		if len(state.PartData[partName]) == 0 {
			if _, err := d.packagePartLocked(partName); err != nil {
				return "", false, fmt.Errorf("读取 PPTX 包元数据失败: %w", err)
			}
		}
	}
	slides, err := d.pres.Slides()
	if err != nil {
		return "", false, fmt.Errorf("读取幻灯片失败: %w", err)
	}
	var slidePart string
	slideIndex := -1
	for index, slide := range slides {
		if uint32(slide.ID()) == slideID {
			slidePart = strings.TrimPrefix(path.Clean(slide.PartName()), "/")
			slideIndex = index
			break
		}
	}
	if slideIndex < 0 {
		return "", false, fmt.Errorf("未找到幻灯片 ID %d", slideID)
	}
	state.SlideParts[slideID] = slidePart

	if _, err := d.packagePartLocked(slidePart); err != nil {
		return "", false, err
	}
	slideRelsPart := relationshipPartPath(slidePart)
	slideRelsData, err := d.packagePartLocked(slideRelsPart)
	if err != nil {
		return "", false, err
	}
	commentsPart := state.SlideCommentsParts[slideID]
	newCommentsPart := commentsPart == ""
	if newCommentsPart {
		commentsPart = d.nextPPTXCommentPartLocked()
	}
	commentsData := state.PartData[commentsPart]
	if len(commentsData) == 0 && !newCommentsPart {
		commentsData, err = d.packagePartLocked(commentsPart)
		if err != nil {
			return "", false, err
		}
	}
	if newCommentsPart {
		commentsData = []byte(`<?xml version="1.0" encoding="UTF-8"?><p:cmLst xmlns:p="` + pptxPresentationMLNamespace + `"></p:cmLst>`)
	}

	authorIndex := -1
	for i, existing := range state.Authors {
		if strings.EqualFold(existing.Name, authorName) {
			authorIndex = i
			break
		}
	}
	newAuthor := authorIndex < 0
	var author pptxCommentAuthor
	if newAuthor {
		id, err := nextPPTXCommentAuthorID(state.Authors)
		if err != nil {
			return "", false, err
		}
		author = pptxCommentAuthor{ID: id, Name: authorName, Initials: initialsForPPTXCommentAuthor(authorName), ColorIdx: uint32(id % 8)}
	} else {
		author = state.Authors[authorIndex]
	}
	if author.LastIdx == math.MaxUint32 {
		return "", false, errors.New("PPTX 批注索引空间已耗尽")
	}
	commentIndex := author.LastIdx + 1
	commentID := pptxCommentID(author.ID, commentIndex)
	for _, existing := range state.Records {
		if existing.Comment.SlideID == slideID && existing.AuthorID == author.ID && existing.Index == commentIndex {
			return "", false, errors.New("PPTX 批注索引冲突")
		}
		if existing.Comment.SlideID == slideID && strings.EqualFold(existing.Comment.Author, authorName) && existing.Comment.Text == text && existing.Comment.XPT == xPT && existing.Comment.YPT == yPT {
			return existing.Comment.ID, false, nil
		}
	}
	commentXML := fmt.Sprintf(`<p:cm authorId="%d" dt="%s" idx="%d"><p:pos x="%d" y="%d"/><p:text>%s</p:text></p:cm>`,
		author.ID, time.Now().UTC().Format(time.RFC3339), commentIndex,
		int64(math.Round(xPT*emuPerPoint)), int64(math.Round(yPT*emuPerPoint)), escapePPTXXMLText(text))
	commentsData, err = appendPPTXXMLChild(commentsData, "cmLst", commentXML)
	if err != nil {
		return "", false, fmt.Errorf("追加 PPTX 批注失败: %w", err)
	}
	if len(commentsData) > pptxMaxCommentPartBytes {
		return "", false, fmt.Errorf("PPTX 单张幻灯片的批注部件超过 %d 字节上限", pptxMaxCommentPartBytes)
	}

	if newAuthor {
		authorPart, err := d.ensurePPTXCommentAuthorsPartLocked()
		if err != nil {
			return "", false, err
		}
		state.AuthorsPart = authorPart
		authorData := state.PartData[authorPart]
		authorXML := fmt.Sprintf(`<p:cmAuthor id="%d" name="%s" initials="%s" lastIdx="%d" clrIdx="%d"/>`,
			author.ID, escapePPTXXMLAttribute(author.Name), escapePPTXXMLAttribute(author.Initials), commentIndex, author.ColorIdx)
		updated, err := appendPPTXXMLChild(authorData, "cmAuthorLst", authorXML)
		if err != nil {
			return "", false, fmt.Errorf("更新 PPTX 批注作者失败: %w", err)
		}
		if len(updated) > pptxMaxCommentAuthorsBytes {
			return "", false, fmt.Errorf("PPTX 批注作者部件超过 %d 字节上限", pptxMaxCommentAuthorsBytes)
		}
		state.PartData[authorPart] = updated
		state.PackageEdits[authorPart] = updated
		state.Authors = append(state.Authors, author)
		state.Authors[len(state.Authors)-1].LastIdx = commentIndex
	} else {
		author.LastIdx = commentIndex
		state.Authors[authorIndex].LastIdx = commentIndex
		authorData := state.PartData[state.AuthorsPart]
		updated, err := setPPTXCommentAuthorLastIndex(authorData, author.ID, commentIndex)
		if err != nil {
			return "", false, fmt.Errorf("更新 PPTX 批注作者索引失败: %w", err)
		}
		state.PartData[state.AuthorsPart] = updated
		state.PackageEdits[state.AuthorsPart] = updated
	}

	state.PartData[commentsPart] = commentsData
	state.PackageEdits[commentsPart] = commentsData
	state.PartNames[commentsPart] = true
	if newCommentsPart {
		relID := nextPPTXRelationshipID(slideRelsData)
		relTarget := relativeOPCTarget(slidePart, commentsPart)
		relXML := fmt.Sprintf(`<Relationship Id="%s" Type="%s" Target="%s"/>`, relID, pptxCommentRelationshipType, escapePPTXXMLAttribute(relTarget))
		slideRelsData, err = appendPPTXOPCRelationship(slideRelsData, relXML)
		if err != nil {
			return "", false, fmt.Errorf("添加 PPTX 批注关系失败: %w", err)
		}
		state.PartData[slideRelsPart] = slideRelsData
		state.PackageEdits[slideRelsPart] = slideRelsData
		state.SlideCommentsParts[slideID] = commentsPart
		if err := d.ensurePPTXContentTypeLocked(commentsPart, pptxCommentContentType); err != nil {
			return "", false, err
		}
	}
	state.Records = append(state.Records, pptxCommentRecord{
		Comment: document.PresentationComment{
			ID: commentID, SlideID: slideID, SlideNumber: slideIndex + 1,
			Author: authorName, Text: text, XPT: xPT, YPT: yPT,
		},
		AuthorID: author.ID, Index: commentIndex, PartName: commentsPart,
	})
	d.dirty = true
	d.revision++
	committed = true
	return commentID, true, nil
}

func (d *Document) DeletePresentationComment(slideID uint32, commentID string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pres == nil {
		return false, errors.New("演示文稿已关闭")
	}
	state := d.presentationComments
	if state == nil {
		return false, errors.New("PPTX 批注状态不可用")
	}
	originalState := state
	state = clonePPTXCommentState(state)
	d.presentationComments = state
	committed := false
	defer func() {
		if !committed {
			d.presentationComments = originalState
		}
	}()
	if state.LoadErr != nil {
		return false, state.LoadErr
	}
	if state.UnsupportedModernComment {
		return false, errors.New("此演示文稿包含新版 PowerPoint 批注；当前仅支持经典幻灯片批注的编辑")
	}
	index := -1
	for i, record := range state.Records {
		if record.Comment.SlideID == slideID && record.Comment.ID == commentID {
			index = i
			break
		}
	}
	if index < 0 {
		return false, fmt.Errorf("未找到幻灯片 %d 上的批注 %q", slideID, commentID)
	}
	record := state.Records[index]
	data, ok := state.PartData[record.PartName]
	if !ok {
		var err error
		data, err = d.packagePartLocked(record.PartName)
		if err != nil {
			return false, err
		}
	}
	updated, err := removePPTXComment(data, record.AuthorID, record.Index)
	if err != nil {
		return false, err
	}
	state.PartData[record.PartName] = updated
	state.PackageEdits[record.PartName] = updated
	state.Records = append(state.Records[:index], state.Records[index+1:]...)
	d.dirty = true
	d.revision++
	committed = true
	return true, nil
}

func normalizePPTXCommentText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.TrimSpace(text)
}

func pptxCommentID(authorID, index uint32) string {
	return fmt.Sprintf("%d:%d", authorID, index)
}

func nextPPTXCommentAuthorID(authors []pptxCommentAuthor) (uint32, error) {
	var next uint32
	for _, author := range authors {
		if author.ID == math.MaxUint32 {
			return 0, errors.New("PPTX 批注作者 ID 空间已耗尽")
		}
		if author.ID >= next {
			next = author.ID + 1
		}
	}
	return next, nil
}

func initialsForPPTXCommentAuthor(name string) string {
	fields := strings.Fields(name)
	var initials strings.Builder
	for _, field := range fields {
		for _, r := range field {
			initials.WriteRune(r)
			break
		}
		if utf8.RuneCountInString(initials.String()) == 3 {
			break
		}
	}
	if initials.Len() == 0 {
		return "G"
	}
	return initials.String()
}

func validXMLText(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r == '\t' || r == '\n' || r == '\r' || r >= 0x20 && r <= 0xD7FF || r >= 0xE000 && r <= 0xFFFD || r >= 0x10000 && r <= 0x10FFFF {
			continue
		}
		return false
	}
	return true
}

func (d *Document) nextPPTXCommentPartLocked() string {
	for index := 1; ; index++ {
		name := fmt.Sprintf("ppt/comments/comment%d.xml", index)
		if !d.presentationComments.PartNames[name] {
			d.presentationComments.PartNames[name] = true
			return name
		}
	}
}

func (d *Document) ensurePPTXCommentAuthorsPartLocked() (string, error) {
	state := d.presentationComments
	if state.AuthorsPart != "" {
		return state.AuthorsPart, nil
	}
	if len(state.PartData[state.PresentationRelsPart]) == 0 {
		if _, err := d.packagePartLocked(state.PresentationRelsPart); err != nil {
			return "", err
		}
	}
	if len(state.PartData[state.ContentTypesPart]) == 0 {
		if _, err := d.packagePartLocked(state.ContentTypesPart); err != nil {
			return "", err
		}
	}
	partName := "ppt/commentAuthors.xml"
	if state.PartNames[partName] {
		return "", errors.New("PPTX 存在未关联的 commentAuthors.xml")
	}
	data := []byte(`<?xml version="1.0" encoding="UTF-8"?><p:cmAuthorLst xmlns:p="` + pptxPresentationMLNamespace + `"></p:cmAuthorLst>`)
	if err := d.ensurePPTXContentTypeLocked(partName, pptxCommentAuthorsContentType); err != nil {
		return "", err
	}
	relsPart := state.PresentationRelsPart
	relsData := state.PartData[relsPart]
	if len(relsData) == 0 {
		return "", errors.New("PPTX 缺少演示文稿关系部件")
	}
	relID := nextPPTXRelationshipID(relsData)
	relXML := fmt.Sprintf(`<Relationship Id="%s" Type="%s" Target="commentAuthors.xml"/>`, relID, pptxCommentAuthorsRelationshipType)
	updatedRels, err := appendPPTXOPCRelationship(relsData, relXML)
	if err != nil {
		return "", fmt.Errorf("添加 PPTX 批注作者关系失败: %w", err)
	}
	state.PartData[relsPart] = updatedRels
	state.PackageEdits[relsPart] = updatedRels
	state.PartData[partName] = data
	state.PackageEdits[partName] = data
	state.PartNames[partName] = true
	state.AuthorsPart = partName
	return partName, nil
}

func (d *Document) ensurePPTXContentTypeLocked(partName, contentType string) error {
	state := d.presentationComments
	data := state.PartData[state.ContentTypesPart]
	if len(data) == 0 {
		return errors.New("PPTX 缺少 [Content_Types].xml")
	}
	root, err := scanPPTXXML(data)
	if err != nil || root.Local != "Types" {
		return errors.New("PPTX [Content_Types].xml 无效")
	}
	for _, child := range root.Children {
		if child.Local == "Override" && strings.EqualFold(pptxXMLAttribute(child, "PartName"), "/"+strings.TrimPrefix(partName, "/")) {
			if pptxXMLAttribute(child, "ContentType") != contentType {
				return fmt.Errorf("PPTX 部件 %q 的内容类型冲突", partName)
			}
			return nil
		}
	}
	entry := fmt.Sprintf(`<Override PartName="/%s" ContentType="%s"/>`, escapePPTXXMLAttribute(strings.TrimPrefix(partName, "/")), contentType)
	updated, err := appendPPTXXMLChild(data, "Types", entry)
	if err != nil {
		return fmt.Errorf("添加 PPTX 内容类型失败: %w", err)
	}
	state.PartData[state.ContentTypesPart] = updated
	state.PackageEdits[state.ContentTypesPart] = updated
	return nil
}

func (d *Document) packagePartLocked(partName string) ([]byte, error) {
	state := d.presentationComments
	if data, ok := state.PartData[partName]; ok {
		return append([]byte(nil), data...), nil
	}
	if d.pres == nil {
		return nil, errors.New("演示文稿已关闭")
	}
	tmpFile, err := os.CreateTemp("", "glance-pptx-comment-read-*.pptx")
	if err != nil {
		return nil, fmt.Errorf("创建 PPTX 批注读取快照失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(tmpPath)
	if _, err := d.pres.Save(context.Background(), tmpPath, gopptx.WithSaveOverwrite(true)); err != nil {
		return nil, fmt.Errorf("序列化 PPTX 批注读取快照失败: %w", err)
	}
	file, err := os.Open(tmpPath)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	zr, err := zip.NewReader(file, info.Size())
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("读取 PPTX 批注快照失败: %w", err)
	}
	for _, candidate := range zr.File {
		state.PartNames[strings.TrimPrefix(candidate.Name, "/")] = true
	}
	var part *zip.File
	for _, candidate := range zr.File {
		if strings.TrimPrefix(candidate.Name, "/") == strings.TrimPrefix(partName, "/") {
			part = candidate
			break
		}
	}
	if part == nil {
		_ = file.Close()
		return nil, fmt.Errorf("PPTX 缺少部件 %q", partName)
	}
	reader, err := part.Open()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	data, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	fileErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if fileErr != nil {
		return nil, fileErr
	}
	state.PartNames[partName] = true
	state.PartData[partName] = append([]byte(nil), data...)
	return data, nil
}

func relativeOPCTarget(ownerPart, targetPart string) string {
	fromDir := filepath.FromSlash(path.Dir(strings.TrimPrefix(ownerPart, "/")))
	target := filepath.FromSlash(strings.TrimPrefix(targetPart, "/"))
	rel, err := filepath.Rel(fromDir, target)
	if err != nil {
		return "../" + path.Base(targetPart)
	}
	return filepath.ToSlash(rel)
}

func pptxCommentIDParts(value string) (uint32, uint32, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, 0, errors.New("PPTX 批注 ID 无效")
	}
	authorID, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil {
		return 0, 0, errors.New("PPTX 批注作者 ID 无效")
	}
	index, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil || index == 0 {
		return 0, 0, errors.New("PPTX 批注索引无效")
	}
	return uint32(authorID), uint32(index), nil
}
