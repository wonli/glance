package textalign

import (
	"context"
	"strings"
	"testing"
)

func uniqueText(start, n int) string {
	q := make([]rune, n)
	for i := range q {
		q[i] = rune(start + i)
	}
	return string(q)
}
func textPage(number int, text string) Page {
	return Page{Number: number, Blocks: []Block{{ID: 1, Text: text}}}
}

func TestJointAnchorsRecoverFragmentedMiddlePage(t *testing.T) {
	left, middle, right := uniqueText(0x5100, 40), uniqueText(0x5200, 24), uniqueText(0x5300, 40)
	p := Page{Number: 2}
	for i, c := range middle {
		p.Blocks = append(p.Blocks, Block{ID: i, Text: string(c)})
	}
	got, err := Align(context.Background(), []Page{textPage(1, left), p, textPage(3, right)}, left+"\n"+middle+"\n"+right, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Anchors) == 0 || len(got[2].Anchors) == 0 {
		t.Fatal("no reliable interior anchors")
	}
	if got[1].Selected == nil || !got[1].Selected.Joint || got[1].Selected.Coverage != 1 {
		t.Fatalf("middle page not recovered: %+v", got[1])
	}
	if got[1].Selected.Range != (Range{41, 65}) {
		t.Fatalf("bad source range %+v", got[1].Selected.Range)
	}
	if got[0].BoundaryAfter == nil || got[0].BoundaryAfter.Min != 40 || got[0].BoundaryAfter.Max != 41 {
		t.Fatalf("newline swallowed: %+v", got[0].BoundaryAfter)
	}
}

func TestJointMissingPageIsNotFabricated(t *testing.T) {
	left, right := uniqueText(0x5400, 40), uniqueText(0x5500, 40)
	got, err := Align(context.Background(), []Page{textPage(1, left), textPage(2, uniqueText(0x5600, 24)), textPage(3, right)}, left+right, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[1].Selected != nil {
		t.Fatal("missing page assigned B text")
	}
	if got[0].BoundaryAfter == nil || got[0].BoundaryAfter.Status != "unresolved" {
		t.Fatalf("missing page got exact boundary %+v", got[0].BoundaryAfter)
	}
}

func TestJointBoundaryIncludesAllOptimalCuts(t *testing.T) {
	left, right := uniqueText(0x5700, 40), uniqueText(0x5800, 40)
	got, err := Align(context.Background(), []Page{textPage(1, left+"甲甲"), textPage(2, "甲甲"+right)}, left+"甲甲甲"+right, Options{})
	if err != nil {
		t.Fatal(err)
	}
	b := got[0].BoundaryAfter
	if b == nil || b.Status != "range" || b.Min != 41 || b.Max != 42 {
		t.Fatalf("optimal cuts lost: %+v results=%+v", b, got)
	}
	if got[0].ExactBoundary {
		t.Fatal("text cut promoted to verified page ownership")
	}
}

func TestJointBudgetAndPhysicalPageGap(t *testing.T) {
	a, b := uniqueText(0x5900, 40), uniqueText(0x5a00, 40)
	pages := []Page{textPage(1, a+"甲乙"), textPage(2, "丙丁"+b)}
	got, err := Align(context.Background(), pages, a+"甲乙丙丁"+b, Options{MaxJointCells: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Context != "limited" || got[0].BoundaryAfter.Status != "unresolved" {
		t.Fatalf("ignored joint budget %+v", got[0])
	}
	pages[1].Number = 3
	got, err = Align(context.Background(), pages, a+"甲乙丙丁"+b, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].BoundaryAfter != nil {
		t.Fatal("linked nonadjacent PDF pages")
	}
}

func TestInternalAnchorsBoundRepeatedCatalogSearch(t *testing.T) {
	body := uniqueText(0x5c00, 80)
	repeated := string([]rune(body)[20:60])
	got, err := Align(context.Background(), []Page{textPage(1, body)}, body+strings.Repeat(repeated, 100), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Status == "limited" || got[0].Selected == nil || got[0].Search.InternalPins != 2 {
		t.Fatalf("unique body anchors not used: %+v", got[0])
	}
}

func TestUnreadImageDoesNotProvePageBoundary(t *testing.T) {
	a, b := "天地玄黄宇宙洪荒日月盈昃辰宿列张", "寒来暑往秋收冬藏闰余成岁律吕调阳"
	p := textPage(1, a)
	p.Blocks = append(p.Blocks, Block{Label: "image"})
	got, err := Align(context.Background(), []Page{p, textPage(2, b)}, a+b, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].BoundaryAfter.Status != "unresolved" {
		t.Fatalf("unread image ignored %+v", got[0].BoundaryAfter)
	}
}

func TestLaterJointBudgetDoesNotEraseEarlierBoundary(t *testing.T) {
	a, b, c := uniqueText(0x5d00, 40), uniqueText(0x5e00, 40), uniqueText(0x5f00, 40)
	pages := []Page{textPage(1, a+"甲乙"), textPage(2, "丙丁"+b+"戊己"), textPage(3, "庚辛"+c)}
	md := a + "甲乙丙丁" + b + "戊己庚辛" + c
	all, err := Align(context.Background(), pages, md, Options{})
	if err != nil {
		t.Fatal(err)
	}
	limit := all[0].Search.JointCells
	if limit <= 0 {
		t.Fatal("no first joint span")
	}
	got, err := Align(context.Background(), pages, md, Options{MaxJointCells: limit})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].BoundaryAfter.Status == "unresolved" || got[1].BoundaryAfter.Status != "unresolved" {
		t.Fatalf("later budget contaminated prior span: %+v / %+v", got[0].BoundaryAfter, got[1].BoundaryAfter)
	}
}
