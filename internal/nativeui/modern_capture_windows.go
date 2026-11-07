//go:build windows

package nativeui

import (
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"unsafe"

	v "github.com/Naenier/ED-Copilot/internal/value"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

var (
	createPreviewStream = syscall.NewLazyDLL("shlwapi.dll").NewProc("SHCreateStreamOnFileEx")
	previewIUnknown     = windows.GUID{Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	previewCompletedIID = windows.GUID{Data1: 0x697e05e9, Data2: 0x3d8f, Data3: 0x45fa, Data4: [8]byte{0x96, 0xf4, 0x8f, 0xfe, 0x1e, 0xde, 0xda, 0xf5}}
	previewCallbackVtbl = [4]uintptr{
		syscall.NewCallback(func(this *previewCallback, iid, out uintptr) uintptr { return this.capture.QueryInterface(iid, out) }),
		syscall.NewCallback(func(this *previewCallback) uintptr { return this.capture.AddRef() }),
		syscall.NewCallback(func(this *previewCallback) uintptr { return this.capture.Release() }),
		syscall.NewCallback(func(this *previewCallback, code uintptr) uintptr { return this.capture.CapturePreviewCompleted(code) }),
	}
)

// The stable Win32 COM ABI puts IUnknown in slots 0..2. CapturePreview is
// ICoreWebView2 slot 30; GetIsVisible/GetParentWindow are controller slots 3/21.
// Only these methods are needed here; importing the SDK's generated all-API
// package would initialize unrelated callbacks with invalid Go string ABI.
type previewCoreABI struct{ vtbl *[31]uintptr }
type previewControllerABI struct{ vtbl *[22]uintptr }
type previewStreamABI struct{ vtbl *[3]uintptr }
type previewCallback struct {
	vtbl    *[4]uintptr
	capture *modernPreviewCapture
}

// Captures are available only in --smoke and only under the report directory.
// No renderer-provided path, arbitrary CDP method or desktop image is accepted.
var modernPreviewNames = map[string]bool{
	"overview": true, "academy": true, "reader": true, "calculator": true,
	"calculator-squadron": true, "colonization": true, "settings": true, "overview-900": true,
	"construction": true, "form": true,
}

type modernPreviewCapture struct {
	owner   *modernShell
	id      string
	name    string
	path    string
	stream  *previewStreamABI
	handler *previewCallback
	refs    atomic.Int32
	done    bool
	result  M
	err     string
}

func (m *modernShell) capturePreview(id, name string) error {
	if !m.owner.options.Smoke || !modernPreviewNames[name] {
		return errors.New("Снимки интерфейса доступны только в изолированной проверке")
	}
	if m.closed || m.core == nil || !m.ready || len(m.captures) >= 12 {
		return errors.New("Интерфейс не готов к снимку")
	}
	if source, err := m.core.GetSource(); err != nil || !trustedWebSource(source) {
		return errors.New("Недопустимый источник снимка")
	}
	dir := filepath.Dir(m.owner.options.SmokeReport)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	capture := &modernPreviewCapture{owner: m, id: id, name: name, path: filepath.Join(dir, "modern-"+name+".png")}
	// STGM_CREATE | STGM_WRITE | STGM_SHARE_EXCLUSIVE. CapturePreview writes
	// PNG data into this native IStream and completes asynchronously on the UI
	// thread. Holding the handler/stream in the shell keeps them live throughout.
	hr, _, _ := createPreviewStream.Call(uintptr(unsafe.Pointer(wide(capture.path))), 0x1011, 0x80, 1, 0, uintptr(unsafe.Pointer(&capture.stream)))
	if int32(hr) < 0 || capture.stream == nil {
		return fmt.Errorf("Создание потока PNG: HRESULT %08x", uint32(hr))
	}
	capture.refs.Store(1)
	capture.handler = &previewCallback{vtbl: &previewCallbackVtbl, capture: capture}
	m.captures = append(m.captures, capture)
	core := (*previewCoreABI)(unsafe.Pointer(m.core))
	hr, _, _ = syscall.SyscallN(core.vtbl[30], uintptr(unsafe.Pointer(core)), 0 /* PNG */, uintptr(unsafe.Pointer(capture.stream)), uintptr(unsafe.Pointer(capture.handler)))
	if int32(hr) < 0 {
		capture.done = true
		capture.err = fmt.Sprintf("HRESULT %08x", uint32(hr))
		capture.closeStream()
		return fmt.Errorf("Снимок WebView2: %s", capture.err)
	}
	return nil
}

func (c *modernPreviewCapture) QueryInterface(refiid, object uintptr) uintptr {
	if refiid == 0 || object == 0 {
		return 0x80004003 // E_POINTER
	}
	var iid windows.GUID
	copyNativeFrameRect.Call(uintptr(unsafe.Pointer(&iid)), refiid, unsafe.Sizeof(iid))
	var result unsafe.Pointer
	if iid == previewIUnknown || iid == previewCompletedIID {
		result = unsafe.Pointer(c.handler)
	}
	copyNativeFrameRect.Call(object, uintptr(unsafe.Pointer(&result)), unsafe.Sizeof(result))
	if result == nil {
		return 0x80004002 // E_NOINTERFACE
	}
	c.AddRef()
	return 0
}
func (c *modernPreviewCapture) AddRef() uintptr { return uintptr(c.refs.Add(1)) }
func (c *modernPreviewCapture) Release() uintptr {
	return uintptr(c.refs.Add(-1))
}
func (c *modernPreviewCapture) closeStream() {
	if c.stream != nil {
		syscall.SyscallN(c.stream.vtbl[2], uintptr(unsafe.Pointer(c.stream)))
		c.stream = nil
	}
}
func (c *modernPreviewCapture) CapturePreviewCompleted(errorCode uintptr) uintptr {
	if c.done {
		return 0
	}
	c.done = true
	c.closeStream()
	if c.owner.closed || c.owner.owner.disposed.Load() {
		return 0
	}
	// Let the WebView callback return before opening the output file: WebView
	// may retain its own exclusive IStream reference until Invoke returns.
	c.owner.owner.main.Synchronize(func() {
		if c.owner.closed || c.owner.owner.disposed.Load() {
			return
		}
		var result M
		var err error
		if int32(errorCode) < 0 {
			err = fmt.Errorf("Снимок WebView2: HRESULT %08x", uint32(errorCode))
		} else {
			result, err = inspectModernPreview(c.path)
		}
		c.result = result
		if err != nil {
			c.err = err.Error()
		}
		c.owner.reply(c.id, result, err)
	})
	return 0
}

func (m *modernShell) smokeVisualState(report M) {
	geometry := M{"ok": false}
	if m.view != nil && m.view.GetController() != nil && m.core != nil {
		controller := (*previewControllerABI)(unsafe.Pointer(m.view.GetController()))
		var actualParent win.HWND
		var visibleValue int32
		parentHR, _, _ := syscall.SyscallN(controller.vtbl[21], uintptr(unsafe.Pointer(controller)), uintptr(unsafe.Pointer(&actualParent)))
		visibleHR, _, _ := syscall.SyscallN(controller.vtbl[3], uintptr(unsafe.Pointer(controller)), uintptr(unsafe.Pointer(&visibleValue)))
		visible := visibleValue != 0
		bounds, boundsErr := m.view.GetController().GetBounds()
		host := m.owner.main.AsContainerBase().Handle()
		var client win.RECT
		win.GetClientRect(host, &client)
		source, sourceErr := m.core.GetSource()
		ok := int32(parentHR) >= 0 && int32(visibleHR) >= 0 && boundsErr == nil && sourceErr == nil && bounds != nil &&
			actualParent == host && host != m.owner.main.Handle() && visible &&
			bounds.Right > bounds.Left && bounds.Bottom > bounds.Top &&
			win.IsWindowVisible(host) && win.IsWindowVisible(m.owner.main.Handle()) && trustedWebSource(source)
		geometry = M{"ok": ok, "actualParent": uintptr(actualParent), "expectedHost": uintptr(host), "mainWindow": uintptr(m.owner.main.Handle()), "controllerVisible": visible, "controllerBounds": bounds, "hostClient": client, "embeddedDocument": sourceErr == nil && trustedWebSource(source)}
	}
	report["webviewGeometry"] = geometry
	captures := []M{}
	completed := map[string]bool{}
	for _, capture := range m.captures {
		ok := capture.done && capture.err == "" && v.Bool(capture.result["ok"])
		item := v.Merge(v.Copy(capture.result), M{"name": capture.name, "completed": capture.done, "ok": ok})
		if capture.err != "" {
			item["error"] = capture.err
		}
		captures = append(captures, item)
		completed[capture.name] = completed[capture.name] || ok
	}
	complete := true
	for _, name := range []string{"overview", "academy", "reader", "calculator", "calculator-squadron", "colonization", "settings"} {
		complete = complete && completed[name]
	}
	report["screenshots"] = captures
	for _, check := range []M{{"name": "WebView расположен в видимой клиентской панели и имеет размер", "ok": geometry["ok"]}, {"name": "Все семь экранов подтверждены непустыми PNG WebView", "ok": complete}} {
		report["scenarios"] = append(v.Arr(report["scenarios"]), check)
		if !v.Bool(check["ok"]) {
			report["ok"] = false
			report["problems"] = append(v.Arr(report["problems"]), check["name"])
		}
	}
}

func inspectModernPreview(path string) (M, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	config, err := png.DecodeConfig(file)
	if err != nil || config.Width < 300 || config.Height < 300 || int64(config.Width)*int64(config.Height) > 50000000 {
		return nil, errors.New("Некорректный размер снимка интерфейса")
	}
	if _, err = file.Seek(0, 0); err != nil {
		return nil, err
	}
	img, err := png.Decode(file)
	if err != nil {
		return nil, err
	}
	colors := map[uint32]bool{}
	for y := 0; y < config.Height; y += 4 {
		for x := 0; x < config.Width; x += 4 {
			r, g, b, _ := img.At(x, y).RGBA()
			colors[r>>8<<16|g>>8<<8|b>>8] = true
			if len(colors) >= 4096 {
				break
			}
		}
		if len(colors) >= 4096 {
			break
		}
	}
	if len(colors) < 16 {
		return nil, errors.New("Снимок интерфейса пуст или не отрисован")
	}
	return M{"ok": true, "file": filepath.Base(path), "width": config.Width, "height": config.Height, "sampledColors": len(colors)}, nil
}
