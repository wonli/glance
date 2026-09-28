package xls

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyXlsSampleOpensReadOnly(t *testing.T) {
	dir := os.Getenv("GLANCE_OFFICE_TEST_DIR")
	if dir == "" {
		t.Skip("set GLANCE_OFFICE_TEST_DIR to the local office sample directory")
	}
	path := filepath.Join(dir, "测试工作表.xls")
	reader, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	names := reader.SheetNames()
	if len(names) != 1 || names[0] == "" {
		t.Fatalf("legacy sheet names = %#v", names)
	}
	_, err = reader.Range(names[0], 100, 16)
	if err != nil {
		t.Fatal(err)
	}
}
