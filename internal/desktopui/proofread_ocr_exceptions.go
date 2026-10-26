package desktopui

import (
	"fmt"
	"strings"
	"time"
)

// proofreadingDiffException 表示用户添加到当前对比 Session 中的一个例外字规则。
type proofreadingDiffException struct {
	ID        string    `json:"id"`
	Char      rune      `json:"char"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
}

// Matches 检查差异帧是否涉及该例外字。
func (e proofreadingDiffException) Matches(frame proofreadingOCRDiffFrame) bool {
	if e.Char == 0 {
		return false
	}
	return frame.AChar == e.Char || frame.BChar == e.Char
}

// proofreadingDiffExceptionState 管理当前对比 Session 的所有用户自定义例外字。
// 绝无硬编码预设，用户添加什么才处理什么。
type proofreadingDiffExceptionState struct {
	Rules       []proofreadingDiffException `json:"rules"`
	rulesByChar map[rune]bool
}

func newProofreadingDiffExceptionState() proofreadingDiffExceptionState {
	return proofreadingDiffExceptionState{
		Rules:       make([]proofreadingDiffException, 0),
		rulesByChar: make(map[rune]bool),
	}
}

func (s *proofreadingDiffExceptionState) rebuildCache() {
	if s.rulesByChar == nil {
		s.rulesByChar = make(map[rune]bool)
	} else {
		for k := range s.rulesByChar {
			delete(s.rulesByChar, k)
		}
	}
	for _, r := range s.Rules {
		if r.Char != 0 {
			s.rulesByChar[r.Char] = true
		}
	}
}

// AddCharException 将用户指定的字添加到当前对比 Session 的例外列表中。
func (s *proofreadingDiffExceptionState) AddCharException(r rune) bool {
	if r == 0 {
		return false
	}
	s.rebuildCache()
	if s.rulesByChar[r] {
		return false
	}
	rule := proofreadingDiffException{
		ID:        fmt.Sprintf("char_%d_%d", r, time.Now().UnixNano()),
		Char:      r,
		Label:     fmt.Sprintf("'%c'", r),
		CreatedAt: time.Now(),
	}
	s.Rules = append(s.Rules, rule)
	s.rebuildCache()
	return true
}

// RemoveCharException 将指定字从当前对比 Session 的例外列表中移除。
func (s *proofreadingDiffExceptionState) RemoveCharException(r rune) bool {
	if r == 0 {
		return false
	}
	removed := false
	var remaining []proofreadingDiffException
	for _, rule := range s.Rules {
		if rule.Char == r {
			removed = true
			continue
		}
		remaining = append(remaining, rule)
	}
	if removed {
		s.Rules = remaining
		s.rebuildCache()
	}
	return removed
}

// RemoveRuleAt 移除指定索引处的规则。
func (s *proofreadingDiffExceptionState) RemoveRuleAt(index int) bool {
	if index < 0 || index >= len(s.Rules) {
		return false
	}
	s.Rules = append(s.Rules[:index], s.Rules[index+1:]...)
	s.rebuildCache()
	return true
}

// Contains 判断指定字是否在当前对比 Session 的例外列表中。
func (s *proofreadingDiffExceptionState) Contains(r rune) bool {
	if r == 0 {
		return false
	}
	if s.rulesByChar == nil {
		s.rebuildCache()
	}
	return s.rulesByChar[r]
}

// Matches 检查差异帧是否命中当前对比 Session 的任意例外字。
func (s *proofreadingDiffExceptionState) Matches(frame proofreadingOCRDiffFrame) bool {
	if len(s.Rules) == 0 {
		return false
	}
	if s.rulesByChar == nil {
		s.rebuildCache()
	}
	return (frame.AChar != 0 && s.rulesByChar[frame.AChar]) || (frame.BChar != 0 && s.rulesByChar[frame.BChar])
}

// Map 返回当前活动例外字符的只读副本，供扫描前直接排除。
func (s *proofreadingDiffExceptionState) Map() map[rune]bool {
	if s.rulesByChar == nil {
		s.rebuildCache()
	}
	res := make(map[rune]bool, len(s.rulesByChar))
	for k, v := range s.rulesByChar {
		res[k] = v
	}
	return res
}

// List 返回当前所有例外字符切片。
func (s *proofreadingDiffExceptionState) List() []rune {
	out := make([]rune, 0, len(s.Rules))
	for _, r := range s.Rules {
		if r.Char != 0 {
			out = append(out, r.Char)
		}
	}
	return out
}

// ToRuleStrings 导出为字符串切片供对比 Session 持久化保存。
func (s *proofreadingDiffExceptionState) ToRuleStrings() []string {
	out := make([]string, 0, len(s.Rules))
	for _, r := range s.Rules {
		if r.Char != 0 {
			out = append(out, string(r.Char))
		}
	}
	return out
}

// LoadFromRuleStrings 从持久化字符串切片恢复对比 Session 的例外列表。
func (s *proofreadingDiffExceptionState) LoadFromRuleStrings(rules []string) {
	s.Rules = make([]proofreadingDiffException, 0, len(rules))
	for _, raw := range rules {
		raw = strings.TrimSpace(raw)
		for _, r := range raw {
			s.AddCharException(r)
		}
	}
	s.rebuildCache()
}
