package diagramrender

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveBundleNeverUsesPATH(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "chrome"), []byte("fake"), 0700)
	t.Setenv("PATH", d)
	_, err := ResolveBundle(filepath.Join(d, "Glance"))
	if err == nil {
		t.Fatal("missing sidecar accepted")
	}
}

func TestMermaidBundleProvenance(t *testing.T) {
	raw, e := assets.ReadFile("assets/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	var m struct{ JSSHA256 string }
	if e = json.Unmarshal(raw, &m); e != nil {
		t.Fatal(e)
	}
	js, e := assets.ReadFile("assets/mermaid.min.js")
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(js)) != m.JSSHA256 {
		t.Fatal("embedded Mermaid changed from pinned upstream bytes")
	}
	license, e := assets.ReadFile("assets/mermaid-LICENSE")
	if e != nil || len(license) < 100 {
		t.Fatal("missing Mermaid license")
	}
}

func TestVegaBundleProvenance(t *testing.T) {
	raw, err := assets.ReadFile("assets/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	type assetEntry struct {
		Version             string `json:"version"`
		URL                 string `json:"url"`
		SHA256              string `json:"sha256"`
		SourcePackageSHA256 string `json:"sourcePackageSHA256"`
	}
	var manifest struct {
		Vega            assetEntry `json:"vega"`
		VegaLite        assetEntry `json:"vegaLite"`
		VegaEmbed       assetEntry `json:"vegaEmbed"`
		VegaInterpreter struct {
			assetEntry
			Dependency struct {
				Version             string `json:"version"`
				URL                 string `json:"url"`
				SourcePackageSHA256 string `json:"sourcePackageSHA256"`
			} `json:"dependency"`
		} `json:"vegaInterpreter"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name, license string
		metadata      assetEntry
	}{
		{"vega.min.js", "vega-LICENSE", manifest.Vega},
		{"vega-lite.min.js", "vega-lite-LICENSE", manifest.VegaLite},
		{"vega-embed.min.js", "vega-embed-LICENSE", manifest.VegaEmbed},
		{"vega-interpreter.min.js", "vega-interpreter-LICENSE", manifest.VegaInterpreter.assetEntry},
	} {
		if item.metadata.Version == "" || item.metadata.URL == "" || item.metadata.SHA256 == "" || item.metadata.SourcePackageSHA256 == "" {
			t.Errorf("missing provenance for %s: %+v", item.name, item.metadata)
			continue
		}
		js, err := assets.ReadFile("assets/" + item.name)
		if err != nil {
			t.Errorf("read embedded %s: %v", item.name, err)
			continue
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(js)); got != item.metadata.SHA256 {
			t.Errorf("%s digest=%s, manifest=%s", item.name, got, item.metadata.SHA256)
		}
		license, err := assets.ReadFile("assets/" + item.license)
		if err != nil || len(license) < 100 {
			t.Errorf("missing upstream license %s: %v", item.license, err)
		}
	}
	if manifest.VegaInterpreter.Dependency.Version != "2.1.3" || manifest.VegaInterpreter.Dependency.URL == "" || manifest.VegaInterpreter.Dependency.SourcePackageSHA256 == "" {
		t.Fatalf("missing bundled interpreter dependency provenance: %+v", manifest.VegaInterpreter.Dependency)
	}
	vegaUtilLicense, err := assets.ReadFile("assets/vega-util-LICENSE")
	if err != nil || len(vegaUtilLicense) < 100 {
		t.Fatalf("missing vega-util license: %v", err)
	}
}
