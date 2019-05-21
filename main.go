package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/lxn/win"
)

var (
	libUser32, _               = syscall.LoadLibrary("user32.dll")
	funcEnumDisplayMonitors, _ = syscall.GetProcAddress(syscall.Handle(libUser32), "EnumDisplayMonitors")
	funcGetDesktopWindow, _    = syscall.GetProcAddress(syscall.Handle(libUser32), "GetDesktopWindow")
	// funcGetWindowTextW, _       = syscall.GetProcAddress(syscall.Handle(libUser32), "GetWindowTextW")
	// funcGetWindowTextLengthW, _ = syscall.GetProcAddress(syscall.Handle(libUser32), "GetWindowTextLengthW")
	user32         = syscall.NewLazyDLL("user32.dll")
	getWindowTextW = user32.NewProc("GetWindowTextW")
	getWindow      = user32.NewProc("GetWindow")

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
	l := dump()

	hwnd = getDesktopWindow()
	zorder(hwnd, l)
	for _, w := range l {
		if strings.Contains(w.Name, "senv2") {
			win.MoveWindow(w.Hwnd, 10, 20, 450, 300, true)
			win.SetForegroundWindow(w.Hwnd)
			win.BringWindowToTop(w.Hwnd)
			win.ShowWindow(w.Hwnd, win.SW_MAXIMIZE)
		}
	}
}

func dump() []*window {
	l := listWindows(win.HWND(0))
	for _, win := range l {
		if !win.Caption || !win.visible || win.Name == "" {
			continue
		}
		fmt.Printf("%d:%s ", win.pid, win.process)
		fmt.Printf("[%X]", win.Hwnd)
		fmt.Printf(" '%s' (%s) %d,%d %dx%d: style '%d'\n",
			win.Name, win.Class,
			win.R.Left, win.R.Top,
			win.R.Right-win.R.Left, win.R.Bottom-win.R.Top, win.Style)
	}

	if err := Save("./file.tmp", &l); err != nil {
		log.Fatalln(err)
	}
	// load it back
	var ll []*window
	if err := Load("./file.tmp", &ll); err != nil {
		log.Fatalln(err)
	}
	return l
}

type window struct {
	Hwnd        win.HWND
	pid         uint32
	Name, Class string
	process     string
	R           win.RECT
	visible     bool
	Maximize    bool
	hasChild    bool
	Style       int32
	Caption     bool
}
type cbData struct {
	list []*window
	pid  map[uint32]string
}

const (
	// gwHWNDFIRST = 0
	// gwHWNDLAST  = 1
	gwHWNDNEXT = 2
	// gwHWNDPREV  = 3
	// gwOWNER     = 4
	gwCHILD = 5
)

// https://stackoverflow.com/questions/825595/how-to-get-the-z-order-in-windows
// https://docs.microsoft.com/en-us/windows/desktop/api/winuser/nf-winuser-getwindow
func zorder(hwnd win.HWND, l []*window) {
	// See also https://github.com/GompaMindPeople/CNN/blob/9c8425cf314a4adf1c010b2a9c6068a7c2634610/src/utils/windowsProTest.go
	lowestHwndp, _, _ := getWindow.Call(uintptr(hwnd), uintptr(gwCHILD))
	z := 0
	lowestHwnd := win.HWND(lowestHwndp)
	fmt.Printf("lowestHwnd='%+v' from hwnd='%+v'\n", lowestHwnd, hwnd)
	hwndTmp := lowestHwnd
	for hwndTmp != 0 {
		elligible := false
		for _, win := range l {
			//fmt.Printf("[%X] vs. <%X>\n", hwndTmp, win.Hwnd)
			if win.Hwnd == hwndTmp {
				elligible = true
				break
			}
		}
		name := getName(hwndTmp, getWindowTextW)
		if elligible {
			fmt.Printf("hwndTmp='%+v' for zorder: '%d' (name '%s')\n", hwndTmp, z, name)
		}
		hwndTmpp, _, _ := getWindow.Call(uintptr(hwndTmp), uintptr(gwHWNDNEXT))
		hwndTmp = win.HWND(hwndTmpp)
		z = z + 1
	}
}

func listWindows(hwnd win.HWND) []*window {
	var d cbData
	d.list = make([]*window, 0)
	d.pid = make(map[uint32]string)
	//win.EnumChildWindows(hwnd, syscall.NewCallback(perWindow), uintptr(unsafe.Pointer(&d)))
	// https://docs.microsoft.com/en-us/windows/desktop/api/winuser/nf-winuser-enumwindows
	_, _, _ = syscall.Syscall(enumWindows.Addr(), 2,
		//uintptr(hwnd),
		syscall.NewCallback(perWindow),
		uintptr(unsafe.Pointer(&d)), 0)
	return d.list
}

func perWindow(hwnd win.HWND, param uintptr) uintptr {
	// https://go101.org/article/unsafe.html
	d := (*cbData)(unsafe.Pointer(param))
	w := window{Hwnd: hwnd}
	w.visible = win.IsWindowVisible(hwnd)
	win.GetWindowRect(hwnd, &w.R)
	w.Name = getName(hwnd, getWindowTextW)
	w.hasChild = win.GetWindow(hwnd, win.GW_CHILD) != 0
	w.Style = win.GetWindowLong(hwnd, win.GWL_STYLE)
	// https://stackoverflow.com/questions/21503109/how-to-use-enumwindows-to-get-only-actual-application-windows
	w.Maximize = ((w.Style & 0x01000000) == 0x01000000)
	w.Caption = ((w.Style & 0x10C00000) == 0x10C00000)
	if w.Caption && w.visible && w.Name != "" {
		d.list = append(d.list, &w)
	}
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

// https://github.com/kbinani/screenshot/blob/9ef8b9209e372fbb0c126cc2648e33bece0c9660/screenshot_windows.go
func numActiveDisplays() int {
	// see also
	// https://github.com/search?q=EnumDisplayMonitors+extension%3Ago&type=Code
	// https://github.com/austinhyde/wallpaper-go/blob/60745ad3fd10b18d832cdb6685b3cc2ed4d7eee4/winutils/winutils.go#L36-L57
	// https://github.com/kbinani/screenshot/blob/9ef8b9209e372fbb0c126cc2648e33bece0c9660/screenshot_windows.go#L95-L109
	// https://github.com/redgoat650/window-scale/blob/5974d75f1f529a0b630adbeb49603781275391bf/windowScale.go
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
	count := (*int)(unsafe.Pointer(dwData))
	*count = *count + 1
	return uintptr(1)
}

type getMonitorBoundsContext struct {
	Index int
	Rect  win.RECT
	Count int
}

func getMonitorBoundsCallback(hMonitor win.HMONITOR, hdcMonitor win.HDC, lprcMonitor *win.RECT, dwData uintptr) uintptr {
	ctx := (*getMonitorBoundsContext)(unsafe.Pointer(dwData))
	if ctx.Count == ctx.Index {
		ctx.Rect = *lprcMonitor
		return uintptr(0)
	}
	ctx.Count = ctx.Count + 1
	return uintptr(1)

}

// https://medium.com/@matryer/golang-advent-calendar-day-eleven-persisting-go-objects-to-disk-7caf1ee3d11d

// Marshal is a function that marshals the object into an
// io.Reader.
// By default, it uses the JSON marshaller.
var Marshal = func(v interface{}) (io.Reader, error) {
	b, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}

var lock sync.Mutex

// Save saves a representation of v to the file at path.
func Save(path string, v interface{}) error {
	lock.Lock()
	defer lock.Unlock()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r, err := Marshal(v)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, r)
	return err
}

// Unmarshal is a function that unmarshals the data from the
// reader into the specified value.
// By default, it uses the JSON unmarshaller.
var Unmarshal = func(r io.Reader, v interface{}) error {
	return json.NewDecoder(r).Decode(v)
}

// Load loads the file at path into v.
// Use os.IsNotExist() to see if the returned error is due
// to the file being missing.
func Load(path string, v interface{}) error {
	lock.Lock()
	defer lock.Unlock()
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return Unmarshal(f, v)
}
