//go:build darwin && cgo

package nativewebview

/*
#cgo LDFLAGS: -framework WebKit -framework AppKit -framework Foundation
#include <stdlib.h>

int glance_nativeweb_setup(const char *html_cstr);
int glance_nativeweb_eval_and_snapshot(
    const char *script_cstr,
    const char *args_json_cstr,
    int density,
    char **out_data,
    int *out_w,
    int *out_h,
    void **out_png,
    size_t *out_len,
    char **out_err
);
int glance_nativeweb_session_open(const char *html_cstr, unsigned long long *out_id, char **out_err);
int glance_nativeweb_session_eval_snapshot(
    unsigned long long session_id,
    const char *script_cstr,
    int density,
    int *out_w,
    int *out_h,
    void **out_png,
    size_t *out_len,
    char **out_err
);
int glance_nativeweb_session_snapshot(
    unsigned long long session_id,
    int width,
    int height,
    int density,
    int *out_w,
    int *out_h,
    void **out_png,
    size_t *out_len,
    char **out_err
);
void glance_nativeweb_session_close(unsigned long long session_id);
void glance_nativeweb_close(void);
void glance_nativeweb_pump(double seconds);
*/
import "C"

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"sync"
	"unsafe"
)

type darwinWebKitEngine struct {
	mu       sync.Mutex
	closed   bool
	sessions map[uint64]*darwinWebKitSession
}

type darwinWebKitSession struct {
	mu      sync.Mutex
	engine  *darwinWebKitEngine
	id      uint64
	density int
	width   int
	height  int
	closed  bool
}

func newPlatformEngine() (Engine, error) {
	return &darwinWebKitEngine{}, nil
}

func (e *darwinWebKitEngine) EvaluateAndSnapshot(ctx context.Context, req RenderRequest) (RenderResult, error) {
	if err := ctx.Err(); err != nil {
		return RenderResult{}, err
	}
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return RenderResult{}, ErrEngineClosed
	}
	e.mu.Unlock()

	html := req.HTML
	if html == "" {
		html = "<!doctype html><html><head><meta charset='utf-8'><style>html,body{margin:0;background:transparent;overflow:hidden;}</style></head><body><div id='output'></div></body></html>"
	}

	cHTML := C.CString(html)
	defer C.free(unsafe.Pointer(cHTML))

	if C.glance_nativeweb_setup(cHTML) == 0 {
		return RenderResult{}, errors.New("failed to setup native WebKit container")
	}
	if err := ctx.Err(); err != nil {
		return RenderResult{}, err
	}

	argsJSON, err := json.Marshal(req.Args)
	if err != nil {
		return RenderResult{}, fmt.Errorf("marshal script args: %w", err)
	}

	cScript := C.CString(req.Script)
	defer C.free(unsafe.Pointer(cScript))
	cArgs := C.CString(string(argsJSON))
	defer C.free(unsafe.Pointer(cArgs))

	var outW, outH C.int
	var outPng unsafe.Pointer
	var outLen C.size_t
	var outErr *C.char
	var outData *C.char

	type callResult struct {
		ok  C.int
		res RenderResult
		err error
	}
	done := make(chan callResult, 1)

	go func() {
		ok := C.glance_nativeweb_eval_and_snapshot(cScript, cArgs, C.int(req.Density), &outData, &outW, &outH, &outPng, &outLen, &outErr)
		var data any
		if outData != nil {
			_ = json.Unmarshal([]byte(C.GoString(outData)), &data)
			C.free(unsafe.Pointer(outData))
		}
		if ok == 0 {
			msg := "render failed"
			if outErr != nil {
				msg = C.GoString(outErr)
				C.free(unsafe.Pointer(outErr))
			}
			done <- callResult{ok: 0, err: errors.New(msg)}
			return
		}

		pngBytes := C.GoBytes(outPng, C.int(outLen))
		C.free(outPng)

		img, err := png.Decode(bytes.NewReader(pngBytes))
		if err != nil {
			done <- callResult{ok: 0, err: fmt.Errorf("decode rendered PNG: %w", err)}
			return
		}

		done <- callResult{
			ok: 1,
			res: RenderResult{
				Image:       img,
				LogicalSize: image.Pt(int(outW), int(outH)),
				Data:        data,
			},
		}
	}()

	select {
	case <-ctx.Done():
		// The native WebKit call owns C buffers and may still be using the shared
		// page. Wait for its completion before allowing another render to enter.
		<-done
		return RenderResult{}, ctx.Err()
	case r := <-done:
		return r.res, r.err
	}
}

func (e *darwinWebKitEngine) OpenSession(ctx context.Context, req SessionRequest) (Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	e.mu.Lock()
	closed := e.closed
	e.mu.Unlock()
	if closed {
		return nil, ErrEngineClosed
	}
	if req.HTML == "" || req.Script == "" {
		return nil, errors.New("session HTML and initialization script are required")
	}
	density := req.Density
	if density <= 0 {
		density = 2
	}
	cHTML := C.CString(req.HTML)
	defer C.free(unsafe.Pointer(cHTML))
	var id C.ulonglong
	var outErr *C.char
	type openResult struct {
		ok  C.int
		id  C.ulonglong
		err error
	}
	done := make(chan openResult, 1)
	go func() {
		ok := C.glance_nativeweb_session_open(cHTML, &id, &outErr)
		if ok == 0 {
			msg := "failed to open native WebKit session"
			if outErr != nil {
				msg = C.GoString(outErr)
				C.free(unsafe.Pointer(outErr))
			}
			done <- openResult{err: errors.New(msg)}
			return
		}
		done <- openResult{ok: ok, id: id}
	}()
	var opened openResult
	select {
	case <-ctx.Done():
		opened = <-done
		if opened.ok != 0 {
			C.glance_nativeweb_session_close(opened.id)
		}
		return nil, ctx.Err()
	case opened = <-done:
	}
	if opened.err != nil {
		return nil, opened.err
	}
	s := &darwinWebKitSession{engine: e, id: uint64(opened.id), density: density}
	res, err := s.evalAndSnapshot(ctx, req.Script)
	if err != nil {
		_ = s.Close()
		return nil, err
	}
	s.width, s.height = res.LogicalSize.X, res.LogicalSize.Y
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		_ = s.Close()
		return nil, ErrEngineClosed
	}
	if e.sessions == nil {
		e.sessions = make(map[uint64]*darwinWebKitSession)
	}
	e.sessions[s.id] = s
	e.mu.Unlock()
	return s, nil
}

func (s *darwinWebKitSession) Dispatch(ctx context.Context, event PointerEvent) (RenderResult, error) {
	if err := validatePointerEvent(event); err != nil {
		return RenderResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return RenderResult{}, ErrSessionClosed
	}
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return RenderResult{}, err
	}
	script := `(async()=>{
		const e=` + string(eventJSON) + `;
		const target=document.elementFromPoint(e.x,e.y)||document.getElementById('output');
		const init={bubbles:true,cancelable:true,clientX:e.x,clientY:e.y,button:e.button,buttons:e.buttons,shiftKey:e.shift,ctrlKey:e.control,pointerType:'mouse'};
		if(e.kind==='leave'){
			target.dispatchEvent(new PointerEvent('pointerout',init));
			target.dispatchEvent(new MouseEvent('mouseout',init));
			target.dispatchEvent(new PointerEvent('pointerleave',init));
			target.dispatchEvent(new MouseEvent('mouseleave',init));
		}else if(e.kind==='wheel'){
			target.dispatchEvent(new WheelEvent('wheel',{...init,deltaX:e.deltaX,deltaY:e.deltaY}));
		}else{
			const names={move:['pointermove','mousemove'],down:['pointerdown','mousedown'],up:['pointerup','mouseup']};
			for(const name of names[e.kind]||[]){const Ctor=name.startsWith('pointer')?PointerEvent:MouseEvent;target.dispatchEvent(new Ctor(name,init));}
			if(e.kind==='down')window.__glancePointerDown={x:e.x,y:e.y};
			if(e.kind==='up'){
				const down=window.__glancePointerDown;
				if(down&&Math.hypot(e.x-down.x,e.y-down.y)<=4)target.dispatchEvent(new MouseEvent('click',init));
				window.__glancePointerDown=null;
			}
		}
		if(window.__glanceVegaView)await window.__glanceVegaView.runAsync();
		return {width:window.__glanceVegaWidth,height:window.__glanceVegaHeight};
	})()`
	return s.evalAndSnapshot(ctx, script)
}

func (s *darwinWebKitSession) Snapshot(ctx context.Context) (RenderResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return RenderResult{}, ErrSessionClosed
	}
	return s.snapshot(ctx)
}

func (s *darwinWebKitSession) evalAndSnapshot(ctx context.Context, script string) (RenderResult, error) {
	if err := ctx.Err(); err != nil {
		return RenderResult{}, err
	}
	cScript := C.CString(script)
	defer C.free(unsafe.Pointer(cScript))
	return s.callNative(ctx, func(w, h *C.int, pngPtr *unsafe.Pointer, pngLen *C.size_t, outErr **C.char) C.int {
		return C.glance_nativeweb_session_eval_snapshot(C.ulonglong(s.id), cScript, C.int(s.density), w, h, pngPtr, pngLen, outErr)
	})
}

func (s *darwinWebKitSession) snapshot(ctx context.Context) (RenderResult, error) {
	if err := ctx.Err(); err != nil {
		return RenderResult{}, err
	}
	return s.callNative(ctx, func(w, h *C.int, pngPtr *unsafe.Pointer, pngLen *C.size_t, outErr **C.char) C.int {
		return C.glance_nativeweb_session_snapshot(C.ulonglong(s.id), C.int(s.width), C.int(s.height), C.int(s.density), w, h, pngPtr, pngLen, outErr)
	})
}

func (s *darwinWebKitSession) callNative(ctx context.Context, call func(*C.int, *C.int, *unsafe.Pointer, *C.size_t, **C.char) C.int) (RenderResult, error) {
	var outW, outH C.int
	var outPNG unsafe.Pointer
	var outLen C.size_t
	var outErr *C.char
	type callResult struct {
		res RenderResult
		err error
	}
	done := make(chan callResult, 1)
	go func() {
		if call(&outW, &outH, &outPNG, &outLen, &outErr) == 0 {
			msg := "native WebKit session operation failed"
			if outErr != nil {
				msg = C.GoString(outErr)
				C.free(unsafe.Pointer(outErr))
			}
			done <- callResult{err: errors.New(msg)}
			return
		}
		pngBytes := C.GoBytes(outPNG, C.int(outLen))
		C.free(outPNG)
		img, err := png.Decode(bytes.NewReader(pngBytes))
		done <- callResult{res: RenderResult{Image: img, LogicalSize: image.Pt(int(outW), int(outH))}, err: err}
	}()
	select {
	case <-ctx.Done():
		// Native WebKit callbacks cannot be interrupted; wait until their C-owned
		// buffers are released before returning so cancellation cannot leak them.
		<-done
		return RenderResult{}, ctx.Err()
	case result := <-done:
		return result.res, result.err
	}
}

func (s *darwinWebKitSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	C.glance_nativeweb_session_close(C.ulonglong(s.id))
	s.engine.mu.Lock()
	delete(s.engine.sessions, s.id)
	s.engine.mu.Unlock()
	return nil
}

func (e *darwinWebKitEngine) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	sessions := make([]*darwinWebKitSession, 0, len(e.sessions))
	for _, session := range e.sessions {
		sessions = append(sessions, session)
	}
	e.sessions = nil
	e.mu.Unlock()
	for _, session := range sessions {
		_ = session.Close()
	}
	e.mu.Lock()
	C.glance_nativeweb_close()
	e.mu.Unlock()
	return nil
}

// PumpMainRunLoop allows tests to pump the Cocoa run loop on thread 0.
func PumpMainRunLoop(seconds float64) {
	C.glance_nativeweb_pump(C.double(seconds))
}
