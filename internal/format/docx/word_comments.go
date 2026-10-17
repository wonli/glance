package docx

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"glance/internal/document"
)

const (
	wordCommentsPartName     = "word/comments.xml"
	wordCommentsContentType  = "application/vnd.openxmlformats-officedocument.wordprocessingml.comments+xml"
	wordCommentsRelationship = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/comments"
	wordMainNamespace        = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
)

var _ document.WordCommentReader = (*Document)(nil)
var _ document.WordCommentEditor = (*Document)(nil)
var _ document.WordTextSelectionCommentEditor = (*Document)(nil)

func (d *Document) WordComments() ([]document.WordComment, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return nil, err
	}
	return d.wordCommentsLocked()
}

func (d *Document) AddWordComment(runID, author, text string) (string, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return "", false, err
	}
	target, ok := d.wordTextTargets[runID]
	if !ok || target.run == nil || target.text == nil || !wordTextRunEditable(d.wordBlocks, runID) {
		return "", false, errors.New("Word 批注目标必须是可编辑文本运行")
	}
	text = normalizeWordCommentText(text)
	if length := utf8.RuneCountInString(text); length == 0 || length > 32768 {
		return "", false, errors.New("Word 批注文本必须为 1 至 32768 个 Unicode 字符")
	}
	author = strings.TrimSpace(author)
	if author == "" {
		author = "Glance"
	}
	if utf8.RuneCountInString(author) > 256 {
		return "", false, errors.New("Word 批注作者不能超过 256 个 Unicode 字符")
	}

	comments, err := d.wordCommentsLocked()
	if err != nil {
		return "", false, err
	}
	for _, existing := range comments {
		if existing.RunID == runID && existing.Author == author && existing.Text == text {
			return existing.ID, false, nil
		}
	}
	nextID := 0
	for _, comment := range comments {
		id, parseErr := strconv.Atoi(comment.ID)
		if parseErr != nil || id < 0 {
			return "", false, fmt.Errorf("DOCX 批注 ID %q 无效", comment.ID)
		}
		if id >= nextID {
			if id == int(^uint(0)>>1) {
				return "", false, errors.New("DOCX 批注 ID 空间已耗尽")
			}
			nextID = id + 1
		}
	}
	anchorIDs, err := wordCommentAnchorIDs(d.parts[d.documentIdx].data)
	if err != nil {
		return "", false, err
	}
	for rawID := range anchorIDs {
		id, parseErr := strconv.Atoi(rawID)
		if parseErr != nil || id < 0 {
			return "", false, fmt.Errorf("DOCX 正文批注锚点 ID %q 无效", rawID)
		}
		if id >= nextID {
			if id == int(^uint(0)>>1) {
				return "", false, errors.New("DOCX 批注锚点 ID 空间已耗尽")
			}
			nextID = id + 1
		}
	}
	commentID := strconv.Itoa(nextID)

	commentsPartName, commentsData, relationshipPart, err := d.prepareWordCommentsPart()
	if err != nil {
		return "", false, err
	}
	updatedComments, err := appendWordComment(commentsData, commentID, author, text, time.Now().UTC())
	if err != nil {
		return "", false, err
	}
	prefix := elementPrefix(target.run.rawName)
	if prefix == "" {
		prefix = "w"
	}
	startMarker := []byte(fmt.Sprintf(`<%s:commentRangeStart %s:id="%s"/>`, prefix, prefix, commentID))
	endAndReference := []byte(fmt.Sprintf(`<%s:commentRangeEnd %s:id="%s"/><%s:r><%s:commentReference %s:id="%s"/></%s:r>`, prefix, prefix, commentID, prefix, prefix, prefix, commentID, prefix))
	documentData, err := applyXMLReplacements(d.parts[d.documentIdx].data, []xmlReplacement{
		{start: target.run.start, end: target.run.start, value: startMarker},
		{start: target.run.end, end: target.run.end, value: endAndReference},
	})
	if err != nil {
		return "", false, fmt.Errorf("锚定 Word 批注失败: %w", err)
	}

	contentTypesPart, err := ensureWordCommentsContentType(d.parts, commentsPartName)
	if err != nil {
		return "", false, err
	}
	changes := map[string]*part{
		"word/document.xml":            d.partWithData("word/document.xml", documentData),
		commentsPartName:               d.partWithData(commentsPartName, updatedComments),
		"word/_rels/document.xml.rels": relationshipPart,
	}
	if contentTypesPart != nil {
		changes["[Content_Types].xml"] = contentTypesPart
	}
	if err := d.commitWordPackageChange(changes); err != nil {
		return "", false, err
	}
	return commentID, true, nil
}

// AddWordCommentForTextSelection anchors a comment to an exact text range. The
// selection is split into new run fragments so existing formatting and text
// outside the range remain unchanged.
func (d *Document) AddWordCommentForTextSelection(startRunID string, startOffset int, endRunID string, endOffset int, author, text string) (string, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return "", false, err
	}
	if d.documentIdx < 0 || d.documentIdx >= len(d.parts) {
		return "", false, errors.New("DOCX document part is unavailable")
	}
	start, startOK := d.wordTextTargets[startRunID]
	end, endOK := d.wordTextTargets[endRunID]
	if !startOK || !endOK || start.text == nil || end.text == nil || start.run == nil || end.run == nil || start.paragraph == nil || end.paragraph == nil || !wordTextRunEditable(d.wordBlocks, startRunID) || !wordTextRunEditable(d.wordBlocks, endRunID) {
		return "", false, errors.New("Word 批注范围必须是可编辑正文文本")
	}
	startRunes, endRunes := []rune(start.text.text), []rune(end.text.text)
	if startOffset < 0 || startOffset > len(startRunes) || endOffset < 0 || endOffset > len(endRunes) || start.text.start > end.text.start || start.paragraph != end.paragraph || start.run.parent != start.paragraph || end.run.parent != end.paragraph || (start.text == end.text && startOffset >= endOffset) {
		return "", false, errors.New("Word 批注范围端点无效或跨越段落")
	}
	text = normalizeWordCommentText(text)
	if length := utf8.RuneCountInString(text); length == 0 || length > 32768 {
		return "", false, errors.New("Word 批注文本必须为 1 至 32768 个 Unicode 字符")
	}
	author = strings.TrimSpace(author)
	if author == "" {
		author = "Glance"
	}
	if utf8.RuneCountInString(author) > 256 {
		return "", false, errors.New("Word 批注作者不能超过 256 个 Unicode 字符")
	}

	data := d.parts[d.documentIdx].data
	if err := validateWordSelectionTextOnly(data, start.paragraph, start.text.start, end.text.end); err != nil {
		return "", false, fmt.Errorf("Word 批注范围包含暂不支持的内容: %w", err)
	}

	type selectedTarget struct {
		target wordTextTarget
		range_ wordSelectedTextRange
	}
	selected := make([]selectedTarget, 0)
	for targetID, target := range d.wordTextTargets {
		if target.text == nil || target.run == nil || target.paragraph != start.paragraph || target.text.start < start.text.start || target.text.start > end.text.start {
			continue
		}
		if target.run.parent != start.paragraph || !wordTextRunEditable(d.wordBlocks, targetID) {
			return "", false, errors.New("Word 批注范围包含不可编辑的正文运行")
		}
		from, to := 0, len([]rune(target.text.text))
		if target.text == start.text {
			from = startOffset
		}
		if target.text == end.text {
			to = endOffset
		}
		if from < 0 || to > len([]rune(target.text.text)) || from >= to {
			continue
		}
		selected = append(selected, selectedTarget{target: target, range_: wordSelectedTextRange{start: from, end: to}})
	}
	if len(selected) == 0 {
		return "", false, errors.New("Word 批注范围不包含可编辑文本")
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].target.text.start < selected[j].target.text.start })
	rangesByRun := make(map[*wordXMLNode]map[*wordXMLNode]wordSelectedTextRange)
	runsByNode := make(map[*wordXMLNode]bool)
	for _, item := range selected {
		if rangesByRun[item.target.run] == nil {
			rangesByRun[item.target.run] = make(map[*wordXMLNode]wordSelectedTextRange)
		}
		rangesByRun[item.target.run][item.target.text] = item.range_
		runsByNode[item.target.run] = true
	}
	runs := make([]*wordXMLNode, 0, len(runsByNode))
	for run := range runsByNode {
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].start < runs[j].start })
	for i := 1; i < len(runs); i++ {
		if !onlyXMLWhitespace(data[runs[i-1].end:runs[i].start]) {
			return "", false, errors.New("Word 批注范围跨越暂不支持的行内内容")
		}
	}

	comments, err := d.wordCommentsLocked()
	if err != nil {
		return "", false, err
	}
	nextID := 0
	for _, comment := range comments {
		id, parseErr := strconv.Atoi(comment.ID)
		if parseErr != nil || id < 0 {
			return "", false, fmt.Errorf("DOCX 批注 ID %q 无效", comment.ID)
		}
		if id >= nextID {
			if id == int(^uint(0)>>1) {
				return "", false, errors.New("DOCX 批注 ID 空间已耗尽")
			}
			nextID = id + 1
		}
	}
	anchorIDs, err := wordCommentAnchorIDs(data)
	if err != nil {
		return "", false, err
	}
	for rawID := range anchorIDs {
		id, parseErr := strconv.Atoi(rawID)
		if parseErr != nil || id < 0 {
			return "", false, fmt.Errorf("DOCX 正文批注锚点 ID %q 无效", rawID)
		}
		if id >= nextID {
			if id == int(^uint(0)>>1) {
				return "", false, errors.New("DOCX 批注锚点 ID 空间已耗尽")
			}
			nextID = id + 1
		}
	}
	commentID := strconv.Itoa(nextID)
	commentsPartName, commentsData, relationshipPart, err := d.prepareWordCommentsPart()
	if err != nil {
		return "", false, err
	}
	updatedComments, err := appendWordComment(commentsData, commentID, author, text, time.Now().UTC())
	if err != nil {
		return "", false, err
	}

	prefix := elementPrefix(start.run.rawName)
	if prefix == "" {
		prefix = "w"
	}
	startMarker := []byte(fmt.Sprintf(`<%s:commentRangeStart %s:id="%s"/>`, prefix, prefix, commentID))
	endAndReference := []byte(fmt.Sprintf(`<%s:commentRangeEnd %s:id="%s"/><%s:r><%s:commentReference %s:id="%s"/></%s:r>`, prefix, prefix, commentID, prefix, prefix, prefix, commentID, prefix))
	var before, selectedXML, after bytes.Buffer
	for index, run := range runs {
		fragments, err := splitWordRunForSelection(data, run, rangesByRun[run], start.text.start, end.text.end)
		if err != nil {
			return "", false, fmt.Errorf("拆分 Word 批注范围: %w", err)
		}
		if index > 0 {
			selectedXML.Write(data[runs[index-1].end:run.start])
		}
		if fragments.prefix.Len() > 0 {
			before.Write(wrapWordRunFragment(data, run, fragments.props, fragments.prefix.Bytes()))
		}
		selectedXML.Write(wrapWordRunFragment(data, run, fragments.props, fragments.selected.Bytes()))
		if fragments.suffix.Len() > 0 {
			after.Write(wrapWordRunFragment(data, run, fragments.props, fragments.suffix.Bytes()))
		}
	}
	var replacement bytes.Buffer
	replacement.Write(before.Bytes())
	replacement.Write(startMarker)
	replacement.Write(selectedXML.Bytes())
	replacement.Write(endAndReference)
	replacement.Write(after.Bytes())
	documentData, err := applyXMLReplacements(data, []xmlReplacement{{start: runs[0].start, end: runs[len(runs)-1].end, value: replacement.Bytes()}})
	if err != nil {
		return "", false, fmt.Errorf("锚定 Word 批注失败: %w", err)
	}
	contentTypesPart, err := ensureWordCommentsContentType(d.parts, commentsPartName)
	if err != nil {
		return "", false, err
	}
	changes := map[string]*part{
		"word/document.xml":            d.partWithData("word/document.xml", documentData),
		commentsPartName:               d.partWithData(commentsPartName, updatedComments),
		"word/_rels/document.xml.rels": relationshipPart,
	}
	if contentTypesPart != nil {
		changes["[Content_Types].xml"] = contentTypesPart
	}
	if err := d.commitWordPackageChange(changes); err != nil {
		return "", false, err
	}
	return commentID, true, nil
}

func (d *Document) DeleteWordComment(commentID string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.checkOpen(); err != nil {
		return false, err
	}
	parsedID, parseErr := strconv.Atoi(commentID)
	if parseErr != nil || parsedID < 0 || commentID == "" {
		return false, errors.New("Word 批注 ID 无效")
	}
	commentsPartName, commentsIndex, found, err := locateWordCommentsPart(d.parts)
	if err != nil {
		return false, err
	}
	if !found {
		return false, errors.New("DOCX 没有批注部件")
	}
	commentsRoot, err := parseWordXML(d.parts[commentsIndex].data)
	if err != nil {
		return false, fmt.Errorf("解析 DOCX 批注失败: %w", err)
	}
	var target *wordXMLNode
	for _, child := range commentsRoot.children {
		if child.name.Local == "comment" && wordCommentAttribute(child, "id") == commentID {
			if target != nil {
				return false, fmt.Errorf("DOCX 中批注 ID %q 重复", commentID)
			}
			target = child
		}
	}
	if target == nil {
		return false, fmt.Errorf("未找到 Word 批注 %q", commentID)
	}
	updatedComments, err := applyXMLReplacements(d.parts[commentsIndex].data, []xmlReplacement{{start: target.start, end: target.end}})
	if err != nil {
		return false, err
	}
	updatedDocument, err := removeWordCommentAnchors(d.parts[d.documentIdx].data, commentID)
	if err != nil {
		return false, err
	}
	if err := d.commitWordPackageChange(map[string]*part{
		"word/document.xml": d.partWithData("word/document.xml", updatedDocument),
		commentsPartName:    d.partWithData(commentsPartName, updatedComments),
	}); err != nil {
		return false, err
	}
	return true, nil
}

func (d *Document) wordCommentsLocked() ([]document.WordComment, error) {
	_, index, found, err := locateWordCommentsPart(d.parts)
	if err != nil {
		return nil, err
	}
	if !found {
		return []document.WordComment{}, nil
	}
	root, err := parseWordXML(d.parts[index].data)
	if err != nil {
		return nil, fmt.Errorf("解析 DOCX 批注失败: %w", err)
	}
	if root.name.Local != "comments" || root.name.Space != wordMainNamespace {
		return nil, errors.New("DOCX 批注部件根节点无效")
	}
	anchors, err := wordCommentRunAnchors(d.parts[d.documentIdx].data, d.wordTextTargets)
	if err != nil {
		return nil, err
	}
	comments := make([]document.WordComment, 0, len(root.children))
	seen := make(map[string]bool)
	for _, node := range root.children {
		if node.name.Local != "comment" || node.name.Space != wordMainNamespace {
			continue
		}
		id := wordCommentAttribute(node, "id")
		parsedID, parseErr := strconv.Atoi(id)
		if parseErr != nil || parsedID < 0 || id == "" {
			return nil, fmt.Errorf("DOCX 批注 ID %q 无效", id)
		}
		if seen[id] {
			return nil, fmt.Errorf("DOCX 批注 ID %q 重复", id)
		}
		seen[id] = true
		comments = append(comments, document.WordComment{
			ID: id, Author: wordCommentAttribute(node, "author"), Text: wordCommentText(node), RunID: anchors[id],
		})
	}
	sort.Slice(comments, func(i, j int) bool {
		left, _ := strconv.Atoi(comments[i].ID)
		right, _ := strconv.Atoi(comments[j].ID)
		return left < right
	})
	return comments, nil
}

func locateWordCommentsPart(parts []part) (string, int, bool, error) {
	indexes := make(map[string]int, len(parts))
	for i := range parts {
		indexes[parts[i].header.Name] = i
	}
	relationships := readDocumentRelationships(parts, indexes)
	partName := ""
	for _, relationship := range relationships {
		if !relationship.comments || relationship.external {
			continue
		}
		if partName != "" && partName != relationship.target {
			return "", -1, false, errors.New("DOCX 存在多个批注部件关系")
		}
		partName = relationship.target
	}
	if partName == "" {
		if _, orphaned := findPackagePart(parts, wordCommentsPartName); orphaned {
			return "", -1, false, errors.New("DOCX 存在未关联的 word/comments.xml")
		}
		return wordCommentsPartName, -1, false, nil
	}
	index, exists := indexes[path.Clean(partName)]
	if !exists {
		return "", -1, false, fmt.Errorf("DOCX 批注关系目标 %q 缺失", partName)
	}
	return path.Clean(partName), index, true, nil
}

func (d *Document) prepareWordCommentsPart() (string, []byte, *part, error) {
	partName, index, exists, err := locateWordCommentsPart(d.parts)
	if err != nil {
		return "", nil, nil, err
	}
	if !exists {
		relationshipID := d.nextWordRelationshipID()
		relationshipPart, err := d.addWordRelationshipType(relationshipID, "comments.xml", wordCommentsRelationship)
		if err != nil {
			return "", nil, nil, err
		}
		return partName, []byte(`<?xml version="1.0" encoding="UTF-8"?><w:comments xmlns:w="` + wordMainNamespace + `"></w:comments>`), relationshipPart, nil
	}
	relationshipPart, ok := findPackagePart(d.parts, "word/_rels/document.xml.rels")
	if !ok {
		return "", nil, nil, errors.New("DOCX 批注部件存在但关系部件缺失")
	}
	return partName, append([]byte(nil), d.parts[index].data...), &relationshipPart, nil
}

func appendWordComment(data []byte, id, author, text string, created time.Time) ([]byte, error) {
	root, err := parseWordXML(data)
	if err != nil {
		return nil, fmt.Errorf("解析 DOCX 批注部件失败: %w", err)
	}
	if root.name.Local != "comments" || root.name.Space != wordMainNamespace {
		return nil, errors.New("DOCX 批注部件根节点无效")
	}
	for _, child := range root.children {
		if child.name.Local == "comment" && wordCommentAttribute(child, "id") == id {
			return nil, fmt.Errorf("DOCX 批注 ID %q 已存在", id)
		}
	}
	prefix := elementPrefix(root.rawName)
	if prefix == "" {
		prefix = "w"
	}
	initials := author
	if runes := []rune(author); len(runes) > 2 {
		initials = string(runes[:2])
	}
	child := fmt.Sprintf(`<%s:comment %s:id="%s" %s:author="%s" %s:initials="%s" %s:date="%s"><%s:p><%s:r>%s</%s:r></%s:p></%s:comment>`,
		prefix, prefix, id, prefix, string(escapeXMLAttribute(author)), prefix, string(escapeXMLAttribute(initials)), prefix, created.Format(time.RFC3339),
		prefix, prefix, wordCommentTextXML(prefix, text), prefix, prefix, prefix)
	if root.selfClosing {
		open := data[root.start:root.openEnd]
		marker := strings.LastIndex(string(open), "/>")
		if marker < 0 {
			return nil, errors.New("DOCX 批注部件根节点无效")
		}
		replacement := append([]byte(nil), open[:marker]...)
		replacement = append(replacement, '>', '\n')
		replacement = append(replacement, child...)
		replacement = append(replacement, []byte("</"+root.rawName+">")...)
		return applyXMLReplacements(data, []xmlReplacement{{start: root.start, end: root.end, value: replacement}})
	}
	return insertElementContent(data, root, []byte(child))
}

func ensureWordCommentsContentType(parts []part, partName string) (*part, error) {
	contentTypes, exists := findPackagePart(parts, "[Content_Types].xml")
	if !exists {
		return nil, errors.New("DOCX 缺少 [Content_Types].xml")
	}
	root, err := parseWordXML(contentTypes.data)
	if err != nil {
		return nil, fmt.Errorf("解析 DOCX 内容类型失败: %w", err)
	}
	cleanPartName := "/" + strings.TrimPrefix(path.Clean(partName), "/")
	for _, child := range root.children {
		if child.name.Local != "Override" || !strings.EqualFold(wordPackageAttribute(child, "PartName"), cleanPartName) {
			continue
		}
		if wordPackageAttribute(child, "ContentType") != wordCommentsContentType {
			return nil, fmt.Errorf("DOCX 批注部件 %q 的内容类型冲突", cleanPartName)
		}
		return nil, nil
	}
	entry := []byte(fmt.Sprintf(`<Override PartName="%s" ContentType="%s"/>`, cleanPartName, wordCommentsContentType))
	patched, err := insertElementContent(contentTypes.data, root, entry)
	if err != nil {
		return nil, err
	}
	contentTypes.data = patched
	return &contentTypes, nil
}

func removeWordCommentAnchors(data []byte, commentID string) ([]byte, error) {
	root, err := parseWordXML(data)
	if err != nil {
		return nil, fmt.Errorf("解析 Word 正文批注锚点失败: %w", err)
	}
	var replacements []xmlReplacement
	var visit func(*wordXMLNode)
	visit = func(node *wordXMLNode) {
		for _, child := range node.children {
			if (child.name.Local == "commentRangeStart" || child.name.Local == "commentRangeEnd") && wordCommentAttribute(child, "id") == commentID {
				replacements = append(replacements, xmlReplacement{start: child.start, end: child.end})
				continue
			}
			if child.name.Local == "commentReference" && wordCommentAttribute(child, "id") == commentID {
				parent := child.parent
				if parent != nil && parent.name.Local == "r" && wordRunContainsOnlyCommentReference(parent) {
					replacements = append(replacements, xmlReplacement{start: parent.start, end: parent.end})
					continue
				}
				replacements = append(replacements, xmlReplacement{start: child.start, end: child.end})
				continue
			}
			visit(child)
		}
	}
	visit(root)
	return applyXMLReplacements(data, replacements)
}

func wordRunContainsOnlyCommentReference(run *wordXMLNode) bool {
	for _, child := range run.children {
		if child.name.Local != "rPr" && child.name.Local != "commentReference" {
			return false
		}
	}
	return true
}

func wordCommentRunAnchors(documentData []byte, targets map[string]wordTextTarget) (map[string]string, error) {
	root, err := parseWordXML(documentData)
	if err != nil {
		return nil, fmt.Errorf("解析 Word 批注范围失败: %w", err)
	}
	type rangeStart struct {
		id  string
		end int
	}
	starts := make(map[string][]rangeStart)
	ends := make(map[string][]int)
	var visit func(*wordXMLNode)
	visit = func(node *wordXMLNode) {
		for _, child := range node.children {
			switch child.name.Local {
			case "commentRangeStart":
				id := wordCommentAttribute(child, "id")
				starts[id] = append(starts[id], rangeStart{id: id, end: child.end})
			case "commentRangeEnd":
				id := wordCommentAttribute(child, "id")
				ends[id] = append(ends[id], child.start)
			}
			visit(child)
		}
	}
	visit(root)
	anchors := make(map[string]string)
	for id, idStarts := range starts {
		if len(idStarts) != 1 || len(ends[id]) != 1 {
			continue
		}
		for _, start := range idStarts {
			endOffset := -1
			for _, end := range ends[id] {
				if end >= start.end && (endOffset < 0 || end < endOffset) {
					endOffset = end
				}
			}
			if endOffset < 0 {
				continue
			}
			runID := ""
			for candidate, target := range targets {
				if target.run != nil && target.run.start >= start.end && target.run.end <= endOffset {
					if runID != "" {
						runID = ""
						break
					}
					runID = candidate
				}
			}
			if runID != "" {
				anchors[id] = runID
			}
		}
	}
	return anchors, nil
}

func wordCommentAnchorIDs(documentData []byte) (map[string]struct{}, error) {
	root, err := parseWordXML(documentData)
	if err != nil {
		return nil, fmt.Errorf("解析 Word 批注锚点 ID 失败: %w", err)
	}
	ids := make(map[string]struct{})
	var visit func(*wordXMLNode)
	visit = func(node *wordXMLNode) {
		for _, child := range node.children {
			switch child.name.Local {
			case "commentRangeStart", "commentRangeEnd", "commentReference":
				ids[wordCommentAttribute(child, "id")] = struct{}{}
			}
			visit(child)
		}
	}
	visit(root)
	if _, missing := ids[""]; missing {
		return nil, errors.New("DOCX 正文包含缺少 ID 的批注锚点")
	}
	return ids, nil
}

func wordCommentAttribute(node *wordXMLNode, localName string) string {
	if node == nil {
		return ""
	}
	for _, attr := range node.attrs {
		if attr.Name.Space == wordMainNamespace && attr.Name.Local == localName {
			return attr.Value
		}
	}
	return ""
}

func wordCommentText(comment *wordXMLNode) string {
	var text strings.Builder
	var visit func(*wordXMLNode)
	visit = func(node *wordXMLNode) {
		if node.name.Local == "br" {
			text.WriteRune('\n')
			return
		}
		if node.name.Local == "tab" {
			text.WriteRune('\t')
			return
		}
		if node.name.Local == "t" {
			text.WriteString(node.text)
			return
		}
		for _, child := range node.children {
			visit(child)
		}
	}
	visit(comment)
	return text.String()
}

func wordCommentTextXML(prefix, value string) string {
	segments := strings.Split(value, "\n")
	var content strings.Builder
	for index, segment := range segments {
		if index > 0 {
			content.WriteString("<" + prefix + ":br/>")
		}
		content.WriteString("<" + prefix + ":t xml:space=\"preserve\">")
		content.WriteString(escapeXMLString(segment))
		content.WriteString("</" + prefix + ":t>")
	}
	return content.String()
}

func normalizeWordCommentText(value string) string {
	value = strings.ToValidUTF8(strings.TrimSpace(value), "�")
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	var normalized strings.Builder
	for _, r := range value {
		if r == '\t' || r == '\n' || r == '\r' || r >= 0x20 && r != 0x7f && (r < 0xD800 || r > 0xDFFF) {
			normalized.WriteRune(r)
		}
	}
	return normalized.String()
}

func wordTextRunEditable(blocks []document.WordBlock, runID string) bool {
	for _, block := range blocks {
		if block.Paragraph != nil {
			for _, inline := range block.Paragraph.Inlines {
				if inline.Text != nil && inline.Text.ID == runID {
					return inline.Text.Editable
				}
			}
		}
		if block.Table != nil {
			for _, row := range block.Table.Rows {
				for _, cell := range row.Cells {
					for _, paragraph := range cell.Paragraphs {
						for _, inline := range paragraph.Inlines {
							if inline.Text != nil && inline.Text.ID == runID {
								return inline.Text.Editable
							}
						}
					}
				}
			}
		}
	}
	return false
}
