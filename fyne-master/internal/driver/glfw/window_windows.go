package glfw

import (
	"runtime"
	"syscall"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/internal/scale"

	"golang.org/x/sys/windows/registry"
)

var (
	imm32DLL                    = syscall.NewLazyDLL("imm32.dll")
	procImmGetContext           = imm32DLL.NewProc("ImmGetContext")
	procImmReleaseContext       = imm32DLL.NewProc("ImmReleaseContext")
	procImmSetCandidateWindow   = imm32DLL.NewProc("ImmSetCandidateWindow")
	procImmSetCompositionWindow = imm32DLL.NewProc("ImmSetCompositionWindow")
)

type imePoint struct {
	x int32
	y int32
}

type imeCandidateForm struct {
	dwIndex   uint32
	dwStyle   uint32
	ptCurrent imePoint
	rcArea    [4]int32
}

type imeCompositionForm struct {
	dwStyle   uint32
	ptCurrent imePoint
	rcArea    [4]int32
}

const (
	imeCandidatePos   = 0x0040
	imeCompositionPos = 0x0002
)

func (w *window) setIMEPosition(position fyne.Position, size fyne.Size) {
	if runtime.GOOS != "windows" || w.view() == nil {
		return
	}
	hwnd := uintptr(unsafe.Pointer(w.view().GetWin32Window()))
	if hwnd == 0 {
		return
	}
	point := imePoint{
		x: int32(scale.ToScreenCoordinate(w.canvas, position.X)),
		y: int32(scale.ToScreenCoordinate(w.canvas, position.Y+size.Height)),
	}
	himc, _, _ := procImmGetContext.Call(hwnd)
	if himc == 0 {
		return
	}
	form := imeCandidateForm{dwIndex: 0, dwStyle: imeCandidatePos, ptCurrent: point}
	procImmSetCandidateWindow.Call(himc, uintptr(unsafe.Pointer(&form)))
	composition := imeCompositionForm{dwStyle: imeCompositionPos, ptCurrent: point}
	procImmSetCompositionWindow.Call(himc, uintptr(unsafe.Pointer(&composition)))
	procImmReleaseContext.Call(hwnd, himc)
}

func (w *window) setDarkMode() {
	if runtime.GOOS == "windows" {
		hwnd := w.view().GetWin32Window()
		dark := isDark()
		// cannot use a go bool.
		var winBool int32
		if dark {
			winBool = 1
		}
		dwm := syscall.NewLazyDLL("dwmapi.dll")
		setAtt := dwm.NewProc("DwmSetWindowAttribute")
		ret, _, err := setAtt.Call(uintptr(unsafe.Pointer(hwnd)), // window handle
			20,                                // DWMWA_USE_IMMERSIVE_DARK_MODE
			uintptr(unsafe.Pointer(&winBool)), // on or off
			4)                                 // sizeof(bool for windows)

		if ret != 0 && ret != 0x80070057 { // err is always non-nil, we check return value (except erroneous code)
			fyne.LogError("Failed to set dark mode", err)
		}
	}
}

func isDark() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil { // older version of Windows will not have this key
		return false
	}
	defer k.Close()

	useLight, _, err := k.GetIntegerValue("AppsUseLightTheme")
	if err != nil { // older version of Windows will not have this value
		return false
	}

	return useLight == 0
}

func (w *window) computeCanvasSize(width, height int) fyne.Size {
	if w.fixedSize {
		return fyne.NewSize(scale.ToFyneCoordinate(w.canvas, w.width), scale.ToFyneCoordinate(w.canvas, w.height))
	}
	return fyne.NewSize(scale.ToFyneCoordinate(w.canvas, width), scale.ToFyneCoordinate(w.canvas, height))
}

// assert we are implementing driver.NativeWindow
var _ driver.NativeWindow = (*window)(nil)

func (w *window) RunNative(f func(any)) {
	context := driver.WindowsWindowContext{}
	if v := w.view(); v != nil {
		context.HWND = uintptr(unsafe.Pointer(v.GetWin32Window()))
	}

	f(context)
}
