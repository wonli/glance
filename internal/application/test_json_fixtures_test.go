package application

type spreadsheetCellEditFixture struct {
	Row    int `json:"row"`
	Column int `json:"column"`
	Value  any `json:"value"`
}
