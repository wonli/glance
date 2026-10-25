package fontfallback

import (
	"reflect"
	"testing"
)

func TestFamilyCandidatesWestern(t *testing.T) {
	tests := []struct {
		input    string
		contains []string
	}{
		{
			input:    "Calibri",
			contains: []string{"Calibri", "Carlito", "Arial"},
		},
		{
			input:    "Times New Roman",
			contains: []string{"Times New Roman", "Times", "Liberation Serif"},
		},
		{
			input:    "Courier New",
			contains: []string{"Courier New", "Courier", "Menlo"},
		},
		{
			input:    "Helvetica",
			contains: []string{"Helvetica", "Helvetica Neue", "Arial"},
		},
		{
			input:    "Aptos",
			contains: []string{"Aptos", "Calibri", "Arial"},
		},
	}

	for _, tt := range tests {
		got := FamilyCandidates(tt.input)
		if len(got) == 0 {
			t.Fatalf("FamilyCandidates(%q) returned empty", tt.input)
		}
		if got[0] != tt.input {
			t.Errorf("FamilyCandidates(%q)[0] = %q; want %q", tt.input, got[0], tt.input)
		}
		for _, want := range tt.contains {
			found := false
			for _, item := range got {
				if item == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("FamilyCandidates(%q) missing %q; got %+v", tt.input, want, got)
			}
		}
	}
}

func TestFamilyCandidatesCJK(t *testing.T) {
	tests := []struct {
		input string
		want0 string
		want1 string
	}{
		{"宋体", "宋体", "SimSun"},
		{"SimSun", "SimSun", "宋体"},
		{"黑体", "黑体", "SimHei"},
		{"SimHei", "SimHei", "黑体"},
		{"楷体", "楷体", "KaiTi"},
		{"仿宋", "仿宋", "FangSong"},
	}

	for _, tt := range tests {
		got := FamilyCandidates(tt.input)
		if len(got) < 2 {
			t.Fatalf("FamilyCandidates(%q) too short: %+v", tt.input, got)
		}
		if got[0] != tt.want0 || got[1] != tt.want1 {
			t.Errorf("FamilyCandidates(%q) = [%q, %q, ...]; want [%q, %q, ...]", tt.input, got[0], got[1], tt.want0, tt.want1)
		}
	}
}

func TestFamilyCandidatesUnknown(t *testing.T) {
	got := FamilyCandidates("CustomBrandFont")
	want := []string{"CustomBrandFont"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FamilyCandidates(CustomBrandFont) = %+v; want %+v", got, want)
	}
}
