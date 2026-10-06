package jsondoc

import (
	"strings"
	"testing"
)

func TestTreePreservesOrderNumbersAndEscapedPaths(t *testing.T) {
	source := ` {"z":9007199254740993,"a/b~c":{"点.键":["林墨",true,null]},"z":2} `
	tree, err := ParseTree(source)
	if err != nil {
		t.Fatal(err)
	}
	if tree.KeyCount != 4 || tree.MaxDepth != 3 {
		t.Fatalf("stats: keys=%d depth=%d", tree.KeyCount, tree.MaxDepth)
	}
	if len(tree.Nodes) != 8 {
		t.Fatalf("nodes=%d", len(tree.Nodes))
	}
	if tree.Nodes[1].Raw != "9007199254740993" || tree.Nodes[7].Key != "z" {
		t.Fatal("number precision or duplicate key/order lost")
	}
	n := tree.Nodes[4]
	if n.Pointer != "/a~1b~0c/点.键/0" || n.JSONPath != `$["a/b~c"]["点.键"][0]` || n.Display != "林墨" {
		t.Fatalf("node=%+v", n)
	}
	updated, err := ReplaceValue(tree, 4, `"中文输入"`)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(source, `"林墨"`, `"中文输入"`, 1)
	if updated != want {
		t.Fatalf("replacement changed untouched bytes: %q", updated)
	}
	if _, err := ReplaceValue(tree, 4, `true false`); err == nil {
		t.Fatal("multiple values accepted")
	}
}

func TestTreeRejectsInvalidAndTrailingJSON(t *testing.T) {
	for _, text := range []string{"", `{"name":`, `{} []`, `{"x":NaN}`} {
		if _, err := ParseTree(text); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
	for _, text := range []string{`null`, `[]`, `"中文"`, `123`} {
		if _, err := ParseTree(text); err != nil {
			t.Fatalf("rejected %s: %v", text, err)
		}
	}
}
