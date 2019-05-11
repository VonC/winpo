package main

import (
	"fmt"
	"image"
	"log"
	"syscall"
	"unsafe"

	"github.com/davecgh/go-spew/spew"
	"github.com/lxn/win"
)

var (
	libUser32, _                = syscall.LoadLibrary("user32.dll")
	funcEnumDisplayMonitors, _  = syscall.GetProcAddress(syscall.Handle(libUser32), "EnumDisplayMonitors")
	funcGetDesktopWindow, _     = syscall.GetProcAddress(syscall.Handle(libUser32), "GetDesktopWindow")
	funcGetWindowTextW, _       = syscall.GetProcAddress(syscall.Handle(libUser32), "GetWindowTextW")
	funcGetWindowTextLengthW, _ = syscall.GetProcAddress(syscall.Handle(libUser32), "GetWindowTextLengthW")
)

func main() {
	ndisplays := numActiveDisplays()
	fmt.Printf("Number of active displays: '%d'\n", ndisplays)

	for i := 0; i < ndisplays; i++ {
		fmt.Printf("Display #'%d': bounds '%+v'\n", i+1, getDisplayBounds(i))
	}

	hwnd := getDesktopWindow()
	hdc := win.GetDC(hwnd)
	if hdc == 0 {
		log.Fatalf("GetDC failed")
	}
	defer win.ReleaseDC(hwnd, hdc)
	// https://github.com/lxn/win/issues/19
	spew.Dump(hwnd)

	win.EnumChildWindows(hwnd, syscall.NewCallback(printme), 0)
}

func getDesktopWindow() win.HWND {
	ret, _, _ := syscall.Syscall(funcGetDesktopWindow, 0, 0, 0, 0)
	return win.HWND(ret)
}

func printme(hwnd uintptr, lParam uintptr) uintptr {
	spew.Dump(hwnd)
	fmt.Printf("getWindowText: '%s'\n", getWindowText(hwnd))
	return 1 // true to continue
}

func getWindowText(hwnd uintptr) string {
	iLen := getWindowTextLength(hwnd) + 1
	buf := make([]uint16, iLen)

	if _, _, err := syscall.Syscall(uintptr(funcGetWindowTextW), 3, uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(iLen)); err != 0 {
		log.Fatalf("Call GetWindowText failed:" + syscall.Errno(err).Error())
	}
	return syscall.UTF16ToString(buf)
}

func getWindowTextLength(hwnd uintptr) int {

	var ret uintptr
	var err syscall.Errno
	if ret, _, err = syscall.Syscall(uintptr(funcGetWindowTextLengthW), 1, uintptr(hwnd), 0, 0); err != 0 {
		log.Fatalf("Call GetWindowTextLengthW failed:" + syscall.Errno(err).Error())
	}

	return int(ret)
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
