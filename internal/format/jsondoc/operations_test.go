package jsondoc

import (
	"strings"
	"testing"
)

func TestValidateJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid object", `{"a": 1, "b": "hello"}`, false},
		{"valid array", `[1, 2, "three", true, null]`, false},
		{"valid string", `"hello world"`, false},
		{"valid number", `12345.678`, false},
		{"empty string", ``, true},
		{"whitespace only", `   \n\t  `, true},
		{"trailing garbage", `{"a": 1} {"b": 2}`, true},
		{"unclosed brace", `{"a": 1`, true},
		{"bad comma", `{"a": 1,}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate(%q) err = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestFormatJSON(t *testing.T) {
	input := `{"b":2,"a":1,"nested":{"z":10,"y":20}}`
	formatted, err := Format(input)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}
	expected := "{\n  \"b\": 2,\n  \"a\": 1,\n  \"nested\": {\n    \"z\": 10,\n    \"y\": 20\n  }\n}\n"
	if formatted != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, formatted)
	}

	// Test invalid
	if _, err := Format(`{"bad":`); err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

func TestMinifyJSON(t *testing.T) {
	input := "{\n  \"b\": 2,\n  \"a\": 1\n}\n"
	minified, err := Minify(input)
	if err != nil {
		t.Fatalf("Minify failed: %v", err)
	}
	expected := `{"b":2,"a":1}`
	if minified != expected {
		t.Fatalf("expected %q, got %q", expected, minified)
	}

	if _, err := Minify(`{`); err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

func TestSortKeysJSON(t *testing.T) {
	input := `{"z": 1, "b": 2, "a": {"k": 9, "c": 8}, "d": 4}`
	sorted, err := SortKeys(input)
	if err != nil {
		t.Fatalf("SortKeys failed: %v", err)
	}
	expected := "{\n  \"a\": {\n    \"c\": 8,\n    \"k\": 9\n  },\n  \"b\": 2,\n  \"d\": 4,\n  \"z\": 1\n}\n"
	if sorted != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, sorted)
	}

	// Verify large numbers and floating precision are preserved
	numInput := `{"b": 12345678901234567890.123456789, "a": 9999999999999999999}`
	sortedNum, err := SortKeys(numInput)
	if err != nil {
		t.Fatalf("SortKeys failed on large numbers: %v", err)
	}
	if !strings.Contains(sortedNum, "12345678901234567890.123456789") || !strings.Contains(sortedNum, "9999999999999999999") {
		t.Fatalf("number precision lost: %s", sortedNum)
	}
}

func TestDocumentOperations(t *testing.T) {
	doc := NewBlank()
	_ = doc.SetText(`{"z": 1, "a": 2}`)
	doc.SetDirty(false)

	if err := doc.SortKeys(); err != nil {
		t.Fatal(err)
	}
	if !doc.Dirty() {
		t.Fatal("expected doc to be dirty after SortKeys")
	}
	if !strings.HasPrefix(doc.Text(), "{\n  \"a\": 2,\n  \"z\": 1") {
		t.Fatalf("unexpected sorted text: %s", doc.Text())
	}

	if err := doc.Minify(); err != nil {
		t.Fatal(err)
	}
	if doc.Text() != `{"a":2,"z":1}` {
		t.Fatalf("unexpected minified text: %s", doc.Text())
	}

	if err := doc.Validate(); err != nil {
		t.Fatal(err)
	}

	_ = doc.SetText(`{"invalid":`)
	if err := doc.Validate(); err == nil {
		t.Fatal("expected validate error")
	}
}
