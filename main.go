package main

import (
	"fmt"

	"github.com/lxn/win"
)

// https://stackoverflow.com/questions/47256354/windows-how-to-get-screen-resolution-in-golang
func main() {
	hDC := win.GetDC(0)
	defer win.ReleaseDC(0, hDC)
	width := int(win.GetDeviceCaps(hDC, win.HORZRES))
	height := int(win.GetDeviceCaps(hDC, win.VERTRES))
	fmt.Printf("%dx%d\n", width, height)

	width = int(win.GetSystemMetrics(win.SM_CXSCREEN))
	height = int(win.GetSystemMetrics(win.SM_CYSCREEN))
	fmt.Printf("%dx%d\n", width, height)

	nmonitors := int(win.GetSystemMetrics(win.SM_CMONITORS))
	fmt.Printf("nmonitors '%d'\n", nmonitors)
}
