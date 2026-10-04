package printservice

import "testing"

func TestNormalizePrintOptions(t *testing.T) {
	for _, test := range []struct {
		name string
		in   Options
		want Options
	}{
		{name: "defaults", in: Options{}, want: Options{Copies: 1}},
		{name: "copies", in: Options{Copies: 3}, want: Options{Copies: 3}},
		{name: "range", in: Options{Copies: 2, FirstPage: 2, LastPage: 4}, want: Options{Copies: 2, FirstPage: 2, LastPage: 4}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeOptions(test.in, 5)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("normalized options = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestNormalizePrintOptionsRejectsInvalidRanges(t *testing.T) {
	for _, options := range []Options{
		{Copies: -1},
		{FirstPage: 1},
		{FirstPage: 3, LastPage: 2},
		{FirstPage: 0, LastPage: 2},
		{FirstPage: 1, LastPage: 6},
	} {
		if _, err := normalizeOptions(options, 5); err == nil {
			t.Errorf("normalizeOptions(%+v) unexpectedly succeeded", options)
		}
	}
}
