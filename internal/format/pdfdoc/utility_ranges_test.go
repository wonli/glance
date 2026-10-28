package pdfdoc

import (
	"reflect"
	"testing"
)

func TestParsePageRangesAcceptsInclusiveRanges(t *testing.T) {
	tests := []struct {
		name  string
		input string
		pages int
		want  []PageRange
	}{
		{
			name:  "multiple ranges and single page",
			input: "1-12, 18, 23-48",
			pages: 48,
			want:  []PageRange{{Start: 1, End: 12}, {Start: 18, End: 18}, {Start: 23, End: 48}},
		},
		{
			name:  "whitespace around tokens and dash",
			input: " 2 - 4 , 7 ",
			pages: 7,
			want:  []PageRange{{Start: 2, End: 4}, {Start: 7, End: 7}},
		},
		{
			name:  "single page",
			input: "5",
			pages: 5,
			want:  []PageRange{{Start: 5, End: 5}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePageRanges(tt.input, tt.pages)
			if err != nil {
				t.Fatalf("ParsePageRanges() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParsePageRanges() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParsePageRangesRejectsInvalidAndOverlapping(t *testing.T) {
	tests := []struct {
		name  string
		input string
		pages int
	}{
		{name: "empty input", input: "", pages: 4},
		{name: "empty item", input: "1,,2", pages: 4},
		{name: "trailing empty item", input: "1,", pages: 4},
		{name: "non-digits", input: "a", pages: 4},
		{name: "signed page", input: "+1", pages: 4},
		{name: "zero page", input: "0", pages: 4},
		{name: "negative page", input: "-1", pages: 4},
		{name: "descending range", input: "4-2", pages: 4},
		{name: "page count zero", input: "1", pages: 0},
		{name: "page beyond count", input: "5", pages: 4},
		{name: "duplicate page", input: "2,2", pages: 4},
		{name: "overlapping ranges", input: "1-3,3-4", pages: 4},
		{name: "range owns prior single page", input: "2,1-3", pages: 4},
		{name: "repeated separator", input: "1--2", pages: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := ParsePageRanges(tt.input, tt.pages); err == nil {
				t.Fatalf("ParsePageRanges(%q, %d) = %#v, want error", tt.input, tt.pages, got)
			}
		})
	}
}
