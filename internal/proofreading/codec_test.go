package proofreading

import (
	"reflect"
	"testing"
)

func TestCodecPreservesIdentityAndRejectsCorruption(t *testing.T) {
	m := model(t)
	must(t, m.Lock(1, true))
	must(t, m.Adjust(1, 2))
	must(t, m.ReplacePage(1, "甲\n乙丙丁戊己"))
	data, e := Encode(m.Snapshot())
	must(t, e)
	s, e := Decode(data)
	must(t, e)
	if !reflect.DeepEqual(s, m.Snapshot()) {
		t.Fatal("snapshot changed")
	}
	s.Runs = append(s.Runs, s.Runs[0])
	if _, e = Restore(s); e == nil {
		t.Fatal("accepted duplicate identities")
	}
}
func TestReflowStopsAtIndependentPages(t *testing.T) {
	m := model(t)
	must(t, m.Review(2, true))
	must(t, m.ReplacePage(1, "甲乙\n丙丁"))
	must(t, m.Reflow(map[PageID]int{1: 1, 3: 9}))
	if m.Text(1) != "甲乙\n丙丁" || m.Text(2) != "戊己" || m.Text(3) != "庚辛" {
		t.Fatal("crossed barrier")
	}
}
