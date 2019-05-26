package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

var (
	libuser32               *windows.LazyDLL
	procGetWindowTextW      *windows.LazyProc
	procEnumDisplayMonitors *windows.LazyProc
	procEnumWindows         *windows.LazyProc
)

func init() {
	// Library
	libuser32 = windows.NewLazySystemDLL("user32.dll")
	procGetWindowTextW = libuser32.NewProc("GetWindowTextW")
	procEnumDisplayMonitors = libuser32.NewProc("EnumDisplayMonitors")
	procEnumWindows = libuser32.NewProc("EnumWindows")
}

func main() {
	ndisplays := numActiveDisplays()
	// fmt.Printf("numActiveDisplays='%d'\n", ndisplays)
	argsWithoutProg := os.Args[1:]

	if len(argsWithoutProg) != 1 {
		fmt.Printf("Usage: winpos [record|restore]\n")
		return
	}
	if argsWithoutProg[0] == "record" {
		if ndisplays <= 1 {
			fmt.Printf("Winpos record: only 1 screen, nothing to record\n")
			return
		}
		record()
	}
	if argsWithoutProg[0] == "restore" {
		if ndisplays <= 1 {
			fmt.Printf("Winpos restore: only 1 screen, nothing to restore\n")
			return
		}
		restore()
	}
}

func record() {
	hwnd := win.GetDesktopWindow()
	hdc := win.GetDC(hwnd)
	if hdc == 0 {
		log.Fatalf("GetDC failed")
	}
	defer win.ReleaseDC(hwnd, hdc)
	l := listWindows(win.HWND(0))
	if err := Save("./file.tmp", &l); err != nil {
		log.Fatalln(err)
	}
}

func restore() {
	// load it back
	var ll []*window
	if err := Load("./file.tmp", &ll); err != nil {
		log.Fatalln(err)
	}
	for i := range ll {
		w := ll[len(ll)-i-1]
		win.MoveWindow(w.Hwnd, w.R.Left, w.R.Top, w.R.Right-w.R.Left, w.R.Bottom-w.R.Top, true)
		win.SetForegroundWindow(w.Hwnd)
		win.BringWindowToTop(w.Hwnd)
		if w.Maximize {
			win.ShowWindow(w.Hwnd, win.SW_MAXIMIZE)
		}
		win.SetFocus(w.Hwnd)
	}
}

type window struct {
	Hwnd        win.HWND
	Name, Class string
	R           win.RECT
	visible     bool
	Maximize    bool
	hasChild    bool
	Style       int32
	Caption     bool
}

func listWindows(hwnd win.HWND) []*window {
	l := make([]*window, 0)
	perWindow := func(hwnd win.HWND, param uintptr) uintptr {
		// https://go101.org/article/unsafe.html
		w := window{Hwnd: hwnd}
		w.visible = win.IsWindowVisible(hwnd)
		win.GetWindowRect(hwnd, &w.R)
		w.Name = getName(hwnd, procGetWindowTextW)
		w.hasChild = win.GetWindow(hwnd, win.GW_CHILD) != 0
		w.Style = win.GetWindowLong(hwnd, win.GWL_STYLE)
		// https://stackoverflow.com/questions/21503109/how-to-use-enumwindows-to-get-only-actual-application-windows
		w.Maximize = ((w.Style & 0x01000000) == 0x01000000)
		w.Caption = ((w.Style & 0x10C00000) == 0x10C00000)
		if w.Caption && w.visible && w.Name != "" {
			l = append(l, &w)
		}
		return 1
	}
	_, _, _ = syscall.Syscall(procEnumWindows.Addr(), 2,
		windows.NewCallback(perWindow), 0, 0)
	return l
}

const bufSiz = 128 // Max length I want to see
func getName(hwnd win.HWND, get *windows.LazyProc) string {
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
	var count int
	countupMonitorCallback := func(hMonitor win.HMONITOR, hdcMonitor win.HDC, lprcMonitor *win.RECT, dwData uintptr) uintptr {
		count = count + 1
		return uintptr(1)
	}
	enumDisplayMonitors(win.HDC(0), nil, syscall.NewCallback(countupMonitorCallback), uintptr(unsafe.Pointer(&count)))
	return count
}

func enumDisplayMonitors(hdc win.HDC, lprcClip *win.RECT, lpfnEnum uintptr, dwData uintptr) bool {
	ret, _, _ := syscall.Syscall6(procEnumDisplayMonitors.Addr(), 4,
		uintptr(hdc),
		uintptr(unsafe.Pointer(lprcClip)),
		lpfnEnum,
		dwData,
		0,
		0)
	return int(ret) != 0
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
