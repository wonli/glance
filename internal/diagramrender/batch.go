package diagramrender

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"strings"
)

// BatchResult corresponds to the request at the same index. A malformed item
// does not prevent the remaining resources from rendering.
type BatchResult struct {
	Result Result
	Err    error
}

type batchBounds struct {
	X, Y, Width, Height int
	Error               string
}

// RenderBatch mounts the document's resources under distinct element IDs and
// evaluates them in one WebView call, then splits the snapshot by element bounds.
func (r *Renderer) RenderBatch(ctx context.Context, requests []Request) []BatchResult {
	results := make([]BatchResult, len(requests))
	var valid []Request
	var indexes []int
	for i, q := range requests {
		if err := validate(q); err != nil {
			results[i].Err = err
			continue
		}
		valid = append(valid, q)
		indexes = append(indexes, i)
	}
	if len(valid) == 0 {
		return results
	}
	source, _ := json.Marshal(valid)
	out, err := r.Render(ctx, Request{Kind: "batch", Source: source, Density: 2, batch: valid})
	var bounds []batchBounds
	if err == nil {
		metadata, marshalErr := json.Marshal(out.data)
		err = marshalErr
		if err == nil {
			err = json.Unmarshal(metadata, &bounds)
		}
		if err == nil && (len(bounds) != len(valid) || out.Image == nil || out.LogicalSize.X <= 0 || out.LogicalSize.Y <= 0) {
			err = fmt.Errorf("incomplete WebView batch result")
		}
	}
	for j, i := range indexes {
		if err != nil {
			results[i].Err = err
			continue
		}
		b := bounds[j]
		if b.Error != "" {
			results[i].Err = fmt.Errorf("%s: %s", valid[j].Kind, b.Error)
			continue
		}
		if b.X < 0 || b.Y < 0 || b.Width <= 0 || b.Height <= 0 || b.X+b.Width > out.LogicalSize.X || b.Y+b.Height > out.LogicalSize.Y {
			results[i].Err = fmt.Errorf("invalid WebView resource bounds")
			continue
		}
		pixels := out.Image.Bounds()
		x := pixels.Min.X + b.X*pixels.Dx()/out.LogicalSize.X
		y := pixels.Min.Y + b.Y*pixels.Dy()/out.LogicalSize.Y
		w := b.Width * pixels.Dx() / out.LogicalSize.X
		h := b.Height * pixels.Dy() / out.LogicalSize.Y
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		draw.Draw(img, img.Bounds(), out.Image, image.Pt(x, y), draw.Src)
		results[i].Result = Result{Image: img, LogicalSize: image.Pt(b.Width, b.Height)}
	}
	return results
}

func renderBatchScript(requests []Request) string {
	var script strings.Builder
	script.WriteString(`(async()=>{const root=document.getElementById('output');root.replaceChildren();root.style.padding='0';const data=[];let top=0,width=1;`)
	for i, q := range requests {
		id := fmt.Sprintf("glance-resource-%d", i)
		item := strings.ReplaceAll(renderScript(q), "document.getElementById('output')", fmt.Sprintf("document.getElementById('%s')", id))
		item = strings.ReplaceAll(item, "mermaid.render('diagram',", fmt.Sprintf("mermaid.render('diagram-%d',", i))
		fmt.Fprintf(&script, `{
const node=document.createElement('div');node.id='%s';node.style.cssText='position:absolute;left:0;top:'+top+'px;display:inline-block;padding:4px;';root.append(node);
try{const size=await %s;
document.documentElement.style.width='';document.documentElement.style.height='';document.body.style.width='';document.body.style.height='';
const w=Math.max(size.width,Math.ceil(node.getBoundingClientRect().width)),h=Math.max(size.height,Math.ceil(node.getBoundingClientRect().height));
if(w>4096||top+h>16384||Math.max(width,w)*(top+h)>4194304)throw new Error('document render batch exceeds pixel limits');
data.push({x:0,y:top,width:w,height:h});top+=h;width=Math.max(width,w);
}catch(error){node.remove();data.push({error:String(error.message||error)});}}
`, id, item)
	}
	script.WriteString(`if(window.__glanceVegaFinalize){window.__glanceVegaFinalize();window.__glanceVegaFinalize=null;}return {width,height:Math.max(1,top),data};})()`)
	return script.String()
}
