//go:build !windows

package app

import "syscall"

// hiddenProcAttr has nothing to hide where a program started opens no
// console window.
func hiddenProcAttr() *syscall.SysProcAttr {
	return nil
}
