package fontcatalog

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func privateDeclaration(path string) (string, error) {
	digest := filepath.Base(path)
	decoded, err := hex.DecodeString(digest)
	if err != nil || len(decoded) != 32 || strings.ToLower(digest) != digest {
		return "", errors.New("not a committed font object")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(path)), "licenses", digest+".json"))
	if err != nil {
		return "", err
	}
	var record struct{ SHA256, Family, License, Version string }
	if err = json.Unmarshal(data, &record); err != nil {
		return "", err
	}
	if record.SHA256 != digest || record.Family == "" || record.License == "" || record.Version == "" {
		return "", errors.New("font object lacks verified declaration")
	}
	return record.Family, nil
}
