package textalign

import (
	"context"
	"reflect"
	"testing"
)

func TestAlignReportsCandidateProgressAndPreservesResults(t *testing.T) {
	pages := []Page{
		{Number: 2, Blocks: []Block{{ID: 1, Text: "天地玄黄宇宙洪荒"}}},
		{Number: 5, Blocks: []Block{{ID: 2, Text: "日月盈昃辰宿列张"}}},
		{Number: 9, Blocks: []Block{{ID: 3, Text: "寒来暑往秋收冬藏"}}},
	}
	markdown := "天地玄黄宇宙洪荒\n日月盈昃辰宿列张\n寒来暑往秋收冬藏"
	wantResults, err := Align(context.Background(), pages, markdown, Options{})
	if err != nil {
		t.Fatal(err)
	}
	var events []AlignmentProgress
	gotResults, err := Align(context.Background(), pages, markdown, Options{OnProgress: func(progress AlignmentProgress) {
		events = append(events, progress)
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotResults, wantResults) {
		t.Fatalf("progress callback changed alignment results\nwith callback: %#v\nwithout callback: %#v", gotResults, wantResults)
	}
	want := []AlignmentProgress{
		{Phase: AlignmentPhaseCandidates, Page: 2, TotalPages: 3},
		{Phase: AlignmentPhaseCandidates, Page: 2, CompletedPages: 1, TotalPages: 3},
		{Phase: AlignmentPhaseCandidates, Page: 5, CompletedPages: 1, TotalPages: 3},
		{Phase: AlignmentPhaseCandidates, Page: 5, CompletedPages: 2, TotalPages: 3},
		{Phase: AlignmentPhaseCandidates, Page: 9, CompletedPages: 2, TotalPages: 3},
		{Phase: AlignmentPhaseCandidates, Page: 9, CompletedPages: 3, TotalPages: 3},
		{Phase: AlignmentPhasePath, CompletedPages: 3, TotalPages: 3},
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("progress events = %#v, want %#v", events, want)
	}
}

func TestAlignProgressCancellationStopsSearch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pages := []Page{
		{Number: 1, Blocks: []Block{{Text: "天地玄黄宇宙洪荒"}}},
		{Number: 2, Blocks: []Block{{Text: "日月盈昃辰宿列张"}}},
		{Number: 3, Blocks: []Block{{Text: "寒来暑往秋收冬藏"}}},
	}
	var events []AlignmentProgress
	_, err := Align(ctx, pages, "天地玄黄宇宙洪荒日月盈昃辰宿列张寒来暑往秋收冬藏", Options{OnProgress: func(progress AlignmentProgress) {
		events = append(events, progress)
		if progress.Phase == AlignmentPhaseCandidates && progress.Page == 2 && progress.CompletedPages == 1 {
			cancel()
		}
	}})
	if err != context.Canceled {
		t.Fatalf("Align error = %v, want context.Canceled", err)
	}
	if len(events) != 3 || events[2].Page != 2 || events[2].CompletedPages != 1 {
		t.Fatalf("cancellation should stop at the second page start, got %#v", events)
	}
}
