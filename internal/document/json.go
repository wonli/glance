package document

// JSONTree is an immutable view of the original source. Byte ranges allow an
// adapter to edit a value without rewriting unrelated keys or number lexemes.
type JSONTree struct {
	Source   string
	Nodes    []JSONNode
	KeyCount int
	MaxDepth int
}

type JSONNode struct {
	Key, Kind, Raw, Display    string
	Pointer, JSONPath, DotPath string
	Parent, Depth, Start, End  int
	Children                   []int
}

func (n JSONNode) Container() bool { return n.Kind == "object" || n.Kind == "array" }
