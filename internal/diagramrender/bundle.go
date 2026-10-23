// Package diagramrender rasterizes local diagram data using an app-owned runtime.
package diagramrender

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

//go:embed assets/mermaid.min.js assets/mermaid-LICENSE assets/manifest.json assets/katex.min.js assets/katex.min.css assets/katex-LICENSE assets/vega.min.js assets/vega-LICENSE assets/vega-lite.min.js assets/vega-lite-LICENSE assets/vega-embed.min.js assets/vega-embed-LICENSE assets/vega-interpreter.min.js assets/vega-interpreter-LICENSE assets/vega-util-LICENSE
var assets embed.FS

type Bundle struct {
	Executable, Version, MermaidVersion  string
	VegaVersion, VegaLiteVersion         string
	VegaEmbedVersion, InterpreterVersion string
}

type assetManifest struct {
	MermaidVersion string `json:"mermaidVersion"`
	Vega           struct {
		Version string `json:"version"`
	} `json:"vega"`
	VegaLite struct {
		Version string `json:"version"`
	} `json:"vegaLite"`
	VegaEmbed struct {
		Version string `json:"version"`
	} `json:"vegaEmbed"`
	VegaInterpreter struct {
		Version string `json:"version"`
	} `json:"vegaInterpreter"`
}

func bundledAssetVersions() (Bundle, error) {
	raw, err := assets.ReadFile("assets/manifest.json")
	if err != nil {
		return Bundle{}, err
	}
	var manifest assetManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Bundle{}, err
	}
	if manifest.MermaidVersion == "" || manifest.Vega.Version == "" || manifest.VegaLite.Version == "" || manifest.VegaEmbed.Version == "" || manifest.VegaInterpreter.Version == "" {
		return Bundle{}, fmt.Errorf("diagram asset manifest is incomplete")
	}
	return Bundle{
		MermaidVersion:     manifest.MermaidVersion,
		VegaVersion:        manifest.Vega.Version,
		VegaLiteVersion:    manifest.VegaLite.Version,
		VegaEmbedVersion:   manifest.VegaEmbed.Version,
		InterpreterVersion: manifest.VegaInterpreter.Version,
	}, nil
}

func ResolveBundle(executable string) (Bundle, error) {
	return resolveBundleForOS(executable, runtime.GOOS, runtime.GOARCH)
}
func resolveBundleForOS(executable, goos, arch string) (Bundle, error) {
	root := filepath.Join(filepath.Dir(executable), "render-runtime")
	if goos == "darwin" && filepath.Base(filepath.Dir(executable)) == "MacOS" {
		root = filepath.Join(filepath.Dir(executable), "..", "Resources", "render-runtime")
	}
	platform := map[string]string{"darwin-arm64": "mac-arm64", "darwin-amd64": "mac-x64", "windows-amd64": "win64"}[goos+"-"+arch]
	if platform == "" {
		return Bundle{}, fmt.Errorf("offline diagram runtime unavailable for %s/%s", goos, arch)
	}
	name := "chrome-headless-shell"
	if goos == "windows" {
		name += ".exe"
	}
	dir := filepath.Join(root, "chrome-headless-shell-"+platform)
	for _, file := range []string{name, "icudtl.dat", "headless_lib_data.pak", "headless_command_resources.pak", "LICENSE.headless_shell"} {
		i, e := os.Stat(filepath.Join(dir, file))
		if e != nil || !i.Mode().IsRegular() || i.Size() == 0 {
			return Bundle{}, fmt.Errorf("offline diagram runtime missing or incomplete: %s", filepath.Join(dir, file))
		}
	}
	var metadata struct{ Version, Target string }
	b, e := os.ReadFile(filepath.Join(root, "runtime.json"))
	if e != nil {
		return Bundle{}, fmt.Errorf("offline diagram runtime metadata: %w", e)
	}
	if e = json.Unmarshal(b, &metadata); e != nil || metadata.Version == "" || metadata.Target != goos+"-"+arch {
		return Bundle{}, fmt.Errorf("offline diagram runtime metadata invalid")
	}
	versions, e := bundledAssetVersions()
	if e != nil {
		return Bundle{}, e
	}
	versions.Executable = filepath.Join(dir, name)
	versions.Version = metadata.Version
	return versions, nil
}
