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
	user32                      = syscall.NewLazyDLL("user32.dll")
	getWindowTextW              = user32.NewProc("GetWindowTextW")

	enumWindows = user32.NewProc("EnumWindows")
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

	//win.EnumChildWindows(hwnd, syscall.NewCallback(printme), 0)
	fmt.Println("--------------")
	dump()
}

func dump() {
	l := listWindows(win.HWND(0))
	for _, win := range l {
		if !win.caption || !win.visible || win.name == "" {
			continue
		}
		fmt.Printf("%d:%s ", win.pid, win.process)
		fmt.Printf("[%X]", win.hwnd)
		fmt.Printf(" '%s' (%s) %d,%d %dx%d: style '%d'\n",
			win.name, win.class,
			win.r.Left, win.r.Top,
			win.r.Right-win.r.Left, win.r.Bottom-win.r.Top, win.style)
	}
}

type window struct {
	hwnd        win.HWND
	pid         uint32
	name, class string
	process     string
	r           win.RECT
	visible     bool
	hasChild    bool
	style       int32
	caption     bool
}
type cbData struct {
	list []window
	pid  map[uint32]string
}

func listWindows(hwnd win.HWND) []window {
	var d cbData
	d.list = make([]window, 0)
	d.pid = make(map[uint32]string)
	//win.EnumChildWindows(hwnd, syscall.NewCallback(perWindow), uintptr(unsafe.Pointer(&d)))
	// https://docs.microsoft.com/en-us/windows/desktop/api/winuser/nf-winuser-enumwindows
	syscall.Syscall(enumWindows.Addr(), 2,
		//uintptr(hwnd),
		syscall.NewCallback(perWindow),
		uintptr(unsafe.Pointer(&d)), 0)
	return d.list
}

func perWindow(hwnd win.HWND, param uintptr) uintptr {
	// https://go101.org/article/unsafe.html
	d := (*cbData)(unsafe.Pointer(param))
	w := window{hwnd: hwnd}
	w.visible = win.IsWindowVisible(hwnd)
	win.GetWindowRect(hwnd, &w.r)
	w.name = getName(hwnd, getWindowTextW)
	w.hasChild = win.GetWindow(hwnd, win.GW_CHILD) != 0
	w.style = win.GetWindowLong(hwnd, win.GWL_STYLE)
	// https://stackoverflow.com/questions/21503109/how-to-use-enumwindows-to-get-only-actual-application-windows
	w.caption = ((w.style & 0x10C00000) == 0x10C00000)
	d.list = append(d.list, w)
	return 1
}

func getDesktopWindow() win.HWND {
	ret, _, _ := syscall.Syscall(funcGetDesktopWindow, 0, 0, 0, 0)
	return win.HWND(ret)
}

const bufSiz = 128 // Max length I want to see
func getName(hwnd win.HWND, get *syscall.LazyProc) string {
	var buf [bufSiz]uint16
	siz, _, _ := get.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if siz == 0 {
		return ""
	}
	name := syscall.UTF16ToString(buf[:siz])
	if siz == bufSiz-1 {
		name = name + "\u22EF"
	}
	return name
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
