package diagramrender

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

const pageHTML = `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'; script-src 'none'; connect-src 'none'; font-src 'none'; object-src 'none'; frame-src 'none'"><style>html,body{margin:0;background:transparent}#output{display:inline-block}svg{max-width:none!important}</style></head><body><div id="output"></div></body></html>`

func validate(q Request) error {
	if q.Density != 2 {
		return errors.New("diagram density must be 2")
	}
	if len(q.Source) == 0 {
		return errors.New("diagram source is empty")
	}
	switch q.Kind {
	case "batch":
		if len(q.batch) == 0 || len(q.batch) > 128 {
			return errors.New("invalid render batch size")
		}
		for _, item := range q.batch {
			if item.Kind == "batch" {
				return errors.New("nested render batch")
			}
			if err := validate(item); err != nil {
				return err
			}
		}
	case "mermaid":
		if len(q.Source) > 65536 {
			return errors.New("Mermaid source exceeds 64 KiB")
		}
		s := strings.TrimSpace(string(q.Source))
		if strings.Contains(s, "%%{") || strings.HasPrefix(s, "---") {
			return errors.New("document Mermaid configuration is unsupported")
		}
	case "vega", "vega-lite":
		return validateVega(q)
	case "svg":
		if len(q.Source) > 32<<20 {
			return errors.New("SVG exceeds 32 MiB")
		}
		d := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(q.Source, []byte{0xef, 0xbb, 0xbf})))
		depth := 0
		root := false
		tokens := 0
		for {
			t, e := d.Token()
			if e == io.EOF {
				break
			}
			if e != nil {
				return fmt.Errorf("invalid SVG: %w", e)
			}
			tokens++
			if tokens > 200000 {
				return errors.New("SVG too complex")
			}
			switch t := t.(type) {
			case xml.Directive:
				return errors.New("SVG DTD/entities are unsupported")
			case xml.StartElement:
				depth++
				if depth > 128 {
					return errors.New("SVG nesting exceeds 128")
				}
				if !root {
					if t.Name.Local != "svg" || (t.Name.Space != "" && t.Name.Space != "http://www.w3.org/2000/svg") {
						return errors.New("image root is not SVG")
					}
					root = true
				}
			case xml.EndElement:
				depth--
			}
		}
		if !root {
			return errors.New("missing SVG root")
		}
	case "math", "math-inline":
		if len(q.Source) > 65536 {
			return errors.New("math source exceeds 64 KiB")
		}
	default:
		return errors.New("unknown diagram kind")
	}
	return nil
}

const (
	maxVegaSourceBytes = 1 << 20
	maxVegaInlineRows  = 10000
	maxVegaDimension   = 4096
	maxVegaCSSPixels   = 4 << 20
)

func validateVega(q Request) error {
	if len(q.Source) > maxVegaSourceBytes {
		return errors.New("Vega spec exceeds 1 MiB")
	}
	dec := json.NewDecoder(bytes.NewReader(q.Source))
	dec.UseNumber()
	var spec any
	if err := dec.Decode(&spec); err != nil {
		return fmt.Errorf("invalid Vega JSON: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return errors.New("invalid Vega JSON: trailing content")
	}
	root, ok := spec.(map[string]any)
	if !ok {
		return errors.New("Vega spec must be a JSON object")
	}
	imageMark := vegaSpecHasImageMark(spec)
	rows := 0
	if err := validateVegaValue(spec, false, false, imageMark, 0, &rows); err != nil {
		return err
	}
	if rows > maxVegaInlineRows {
		return fmt.Errorf("Vega inline data exceeds %d rows", maxVegaInlineRows)
	}
	width, hasWidth, err := vegaDimension(root, "width")
	if err != nil {
		return err
	}
	height, hasHeight, err := vegaDimension(root, "height")
	if err != nil {
		return err
	}
	if hasWidth && width > maxVegaDimension || hasHeight && height > maxVegaDimension {
		return fmt.Errorf("Vega chart dimensions exceed %d pixels", maxVegaDimension)
	}
	if hasWidth && hasHeight && width*height > maxVegaCSSPixels {
		return fmt.Errorf("Vega chart exceeds %d CSS pixels", maxVegaCSSPixels)
	}
	return nil
}

func vegaDimension(spec map[string]any, key string) (int, bool, error) {
	value, ok := spec[key]
	if !ok {
		return 0, false, nil
	}
	number, ok := value.(json.Number)
	if !ok {
		return 0, false, nil
	}
	parsed, err := number.Int64()
	if err != nil || parsed < 0 {
		return 0, false, fmt.Errorf("Vega %s must be a non-negative integer", key)
	}
	return int(parsed), true, nil
}

func validateVegaValue(value any, inData bool, inValues bool, imageMark bool, depth int, rows *int) error {
	if depth > 64 {
		return errors.New("Vega spec nesting exceeds 64 levels")
	}
	switch current := value.(type) {
	case map[string]any:
		if usermeta, ok := current["usermeta"].(map[string]any); ok {
			if options, exists := usermeta["embedOptions"].(map[string]any); exists {
				if _, hasPatch := options["patch"]; hasPatch {
					return errors.New("external Vega patch resources are disabled")
				}
				return errors.New("custom Vega Embed options are unsupported")
			}
		}
		for key, child := range current {
			switch key {
			case "patch":
				return errors.New("external Vega patch resources are disabled")
			case "href":
				if !inValues {
					return errors.New("Vega chart hyperlinks are disabled")
				}
			case "url":
				if text, ok := child.(string); ok && (inData || isExternalVegaURL(text) || (inValues && imageMark)) {
					return errors.New("external Vega data and image resources are disabled")
				}
			case "src":
				if text, ok := child.(string); ok && isExternalVegaURL(text) {
					return errors.New("external Vega image resources are disabled")
				}
			case "values":
				if values, ok := child.([]any); ok {
					*rows += len(values)
					if *rows > maxVegaInlineRows {
						return fmt.Errorf("Vega inline data exceeds %d rows", maxVegaInlineRows)
					}
				}
			}
			childInData := inData
			childInValues := inValues
			if key == "data" {
				childInData = true
			}
			if key == "values" {
				childInData = false
				childInValues = true
			}
			if err := validateVegaValue(child, childInData, childInValues, imageMark, depth+1, rows); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range current {
			if err := validateVegaValue(child, inData, inValues, imageMark, depth+1, rows); err != nil {
				return err
			}
		}
	case string:
		if inValues && imageMark && isExternalVegaURL(current) {
			return errors.New("external Vega image resources are disabled")
		}
	}
	return nil
}

func isExternalVegaURL(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme != "" || strings.Contains(value, "://") || strings.HasPrefix(value, "//") || strings.HasPrefix(value, "/")
}

func vegaSpecHasImageMark(value any) bool {
	switch current := value.(type) {
	case map[string]any:
		if current["type"] == "image" || current["mark"] == "image" {
			return true
		}
		for _, child := range current {
			if vegaSpecHasImageMark(child) {
				return true
			}
		}
	case []any:
		for _, child := range current {
			if vegaSpecHasImageMark(child) {
				return true
			}
		}
	}
	return false
}

func renderScript(q Request) string {
	if q.Kind == "batch" {
		return renderBatchScript(q.batch)
	}
	if q.Kind == "svg" {
		uri, _ := json.Marshal("data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(q.Source))
		return `(async()=>{const output=document.getElementById('output');output.replaceChildren();const img=new Image();img.src=` + string(uri) + `;await img.decode();const w=Math.ceil(img.naturalWidth),h=Math.ceil(img.naturalHeight);if(w<=0||h<=0||w*h*4>16777216)throw new Error('SVG dimensions invalid or exceed 16 megapixels');img.width=w;img.height=h;output.append(img);return {width:w,height:h};})()`
	}
	if q.Kind == "math" || q.Kind == "math-inline" {
		source, _ := json.Marshal(string(q.Source))
		displayMode := "true"
		fontSize := "22px"
		if q.Kind == "math-inline" {
			displayMode = "false"
			fontSize = "16px"
		}
		return `(async()=>{
			const output = document.getElementById('output');
			output.replaceChildren();
			output.style.fontSize = '` + fontSize + `';
			const tex = ` + string(source) + `;
			output.innerHTML = katex.renderToString(tex, {
				displayMode: ` + displayMode + `,
				throwOnError: true,
				output: 'mathml'
			});
			await document.fonts.ready;
			const rect = output.getBoundingClientRect();
			let width = Math.ceil(rect.width);
			let height = Math.ceil(rect.height);
			if (width <= 0 || height <= 0 || width * height * 4 > 16777216) {
				throw new Error('math layout dimensions invalid or exceed 16 megapixels');
			}
			return { width, height };
		})()`
	}
	if q.Kind == "vega" || q.Kind == "vega-lite" {
		return renderVegaScript(q)
	}
	source, _ := json.Marshal(string(q.Source))
	return `(async()=>{const output=document.getElementById('output');output.replaceChildren();mermaid.initialize({startOnLoad:false,securityLevel:'strict',maxTextSize:65536,maxEdges:500,suppressErrorRendering:true,theme:'default',fontFamily:'Arial,Microsoft YaHei,PingFang SC,sans-serif',secure:['secure','securityLevel','startOnLoad','maxTextSize','maxEdges','themeCSS','fontFamily','htmlLabels','flowchart'],flowchart:{htmlLabels:false,useMaxWidth:false}});const result=await mermaid.render('diagram',` + string(source) + `);output.innerHTML=result.svg;await document.fonts.ready;const svg=output.querySelector('svg');if(!svg)throw new Error('Mermaid produced no SVG');const view=svg.viewBox.baseVal;const width=Math.ceil(view.width||svg.getBoundingClientRect().width),height=Math.ceil(view.height||svg.getBoundingClientRect().height);if(width<=0||height<=0||width*height*4>16777216)throw new Error('diagram exceeds 16 megapixels');svg.setAttribute('width',width);svg.setAttribute('height',height);svg.style.maxWidth='none';return {width,height};})()`
}

func renderVegaScript(q Request) string {
	source, _ := json.Marshal(string(q.Source))
	return `(async()=>{
		const output=document.getElementById('output');
		output.replaceChildren();
		output.style.padding='0';
		const spec=JSON.parse(` + string(source) + `);
		const loader={load:async uri=>{throw new Error('external Vega resource disabled: '+String(uri));}};
		const usermeta=spec.usermeta;
		if(usermeta&&typeof usermeta==='object') delete usermeta.embedOptions;
		const result=await vegaEmbed(output,spec,{
			mode:` + fmt.Sprintf("%q", q.Kind) + `,
			actions:false,
			renderer:'svg',
			ast:true,
			expr:vegaExpressionInterpreter,
			loader,
			config:{background:'transparent',view:{stroke:'transparent'},axis:{labelFont:'Arial,Microsoft YaHei,PingFang SC,sans-serif',titleFont:'Arial,Microsoft YaHei,PingFang SC,sans-serif'}}
		});
		window.__glanceVegaView=result.view;
		window.__glanceVegaFinalize=result.finalize;
		await result.view.runAsync();
		await document.fonts.ready;
		const bounds=output.getBoundingClientRect();
		const width=Math.ceil(bounds.width),height=Math.ceil(bounds.height);
		if(width<=0||height<=0||width>4096||height>4096||width*height>4194304) throw new Error('Vega chart dimensions exceed render limits');
		window.__glanceVegaWidth=width;
		window.__glanceVegaHeight=height;
		document.documentElement.style.width=width+'px';
		document.documentElement.style.height=height+'px';
		document.body.style.width=width+'px';
		document.body.style.height=height+'px';
		return {width,height};
	})()`
}
