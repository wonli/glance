package jsondoc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Validate checks whether raw is a valid single JSON value.
func Validate(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return errors.New("JSON 内容为空")
	}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return formatSyntaxError(raw, err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("JSON 包含多余的数据")
	}
	return nil
}

// Format returns indented JSON with 2 spaces and a trailing newline.
func Format(raw string) (string, error) {
	if err := Validate(raw); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(strings.TrimSpace(raw)), "", "  "); err != nil {
		return "", formatSyntaxError(raw, err)
	}
	buf.WriteByte('\n')
	return buf.String(), nil
}

// Minify returns compact JSON with unnecessary whitespace removed.
func Minify(raw string) (string, error) {
	if err := Validate(raw); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(strings.TrimSpace(raw))); err != nil {
		return "", formatSyntaxError(raw, err)
	}
	return buf.String(), nil
}

// SortKeys parses JSON and outputs indented JSON with map keys sorted alphabetically.
func SortKeys(raw string) (string, error) {
	if err := Validate(raw); err != nil {
		return "", err
	}
	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	dec.UseNumber()
	var val any
	if err := dec.Decode(&val); err != nil {
		return "", formatSyntaxError(raw, err)
	}
	data, err := json.MarshalIndent(val, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data) + "\n", nil
}

func (d *Document) Format() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errors.New("document is closed")
	}
	formatted, err := Format(d.text)
	if err != nil {
		return err
	}
	if formatted != d.text {
		d.text = formatted
		d.dirty = true
	}
	return nil
}

func (d *Document) Minify() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errors.New("document is closed")
	}
	minified, err := Minify(d.text)
	if err != nil {
		return err
	}
	if minified != d.text {
		d.text = minified
		d.dirty = true
	}
	return nil
}

func (d *Document) SortKeys() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return errors.New("document is closed")
	}
	sorted, err := SortKeys(d.text)
	if err != nil {
		return err
	}
	if sorted != d.text {
		d.text = sorted
		d.dirty = true
	}
	return nil
}

func (d *Document) Validate() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return Validate(d.text)
}

func formatSyntaxError(text string, err error) error {
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		line, col := lineColFromOffset(text, syntaxErr.Offset)
		return fmt.Errorf("第 %d 行第 %d 列语法错误：%s", line, col, syntaxErr.Error())
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		line, col := lineColFromOffset(text, int64(len(text)))
		return fmt.Errorf("第 %d 行第 %d 列语法错误：JSON 未闭合", line, col)
	}
	return err
}

func lineColFromOffset(text string, offset int64) (int, int) {
	line, col := 1, 1
	for i, r := range text {
		if int64(i) >= offset {
			break
		}
		if r == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}
