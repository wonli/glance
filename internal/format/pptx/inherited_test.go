package pptx

const (
	testOfficeRelNS  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	testPackageRelNS = "http://schemas.openxmlformats.org/package/2006/relationships"
)

type testPictureRef struct {
	id  int
	rid string
}

type testImageRel struct {
	id       string
	target   string
	external bool
}
