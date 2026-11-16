//go:build windows

package app

import "syscall"

// hiddenProcAttr keeps a program the app starts from opening a console
// window.
func hiddenProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true}
}
