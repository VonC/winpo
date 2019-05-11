package main

import (
	"fmt"
	"image"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
)

var (
	libUser32, _               = syscall.LoadLibrary("user32.dll")
	funcEnumDisplayMonitors, _ = syscall.GetProcAddress(syscall.Handle(libUser32), "EnumDisplayMonitors")
)

func main() {
	ndisplays := numActiveDisplays()
	fmt.Printf("Number of active displays: '%d'\n", ndisplays)

	for i := 0; i < ndisplays; i++ {
		fmt.Printf("Display #'%d': bounds '%+v'\n", i+1, getDisplayBounds(i))
	}
}

// https://github.com/kbinani/screenshot/blob/9ef8b9209e372fbb0c126cc2648e33bece0c9660/screenshot_windows.go
func numActiveDisplays() int {
	var count int
	enumDisplayMonitors(win.HDC(0), nil, syscall.NewCallback(countupMonitorCallback), uintptr(unsafe.Pointer(&count)))
	return count
}

func getDisplayBounds(displayIndex int) image.Rectangle {
	var ctx getMonitorBoundsContext
	ctx.Index = displayIndex
	ctx.Count = 0
	enumDisplayMonitors(win.HDC(0), nil, syscall.NewCallback(getMonitorBoundsCallback), uintptr(unsafe.Pointer(&ctx)))
	return image.Rect(
		int(ctx.Rect.Left), int(ctx.Rect.Top),
		int(ctx.Rect.Right), int(ctx.Rect.Bottom))
}

func enumDisplayMonitors(hdc win.HDC, lprcClip *win.RECT, lpfnEnum uintptr, dwData uintptr) bool {
	ret, _, _ := syscall.Syscall6(funcEnumDisplayMonitors, 4,
		uintptr(hdc),
		uintptr(unsafe.Pointer(lprcClip)),
		lpfnEnum,
		dwData,
		0,
		0)
	return int(ret) != 0
}

func countupMonitorCallback(hMonitor win.HMONITOR, hdcMonitor win.HDC, lprcMonitor *win.RECT, dwData uintptr) uintptr {
	var count *int
	count = (*int)(unsafe.Pointer(dwData))
	*count = *count + 1
	return uintptr(1)
}

type getMonitorBoundsContext struct {
	Index int
	Rect  win.RECT
	Count int
}

func getMonitorBoundsCallback(hMonitor win.HMONITOR, hdcMonitor win.HDC, lprcMonitor *win.RECT, dwData uintptr) uintptr {
	var ctx *getMonitorBoundsContext
	ctx = (*getMonitorBoundsContext)(unsafe.Pointer(dwData))
	if ctx.Count == ctx.Index {
		ctx.Rect = *lprcMonitor
		return uintptr(0)
	}
	ctx.Count = ctx.Count + 1
	return uintptr(1)

}
