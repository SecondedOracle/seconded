//go:build windows

package client

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func createPrivateDirectory(p string) error {
	if _, e := os.Lstat(p); e == nil {
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	u, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		return e
	}
	sd, e := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;" + u.User.Sid.String() + ")")
	if e != nil {
		return e
	}
	sa := windows.SecurityAttributes{SecurityDescriptor: sd}
	sa.Length = uint32(unsafe.Sizeof(sa))
	name, e := windows.UTF16PtrFromString(p)
	if e != nil {
		return e
	}
	return windows.CreateDirectory(name, &sa)
}

func securePath(p string, dir bool) error {
	s, e := os.Lstat(p)
	if os.IsNotExist(e) {
		return ErrNotFound
	}
	if e != nil || s.Mode()&os.ModeSymlink != 0 || s.IsDir() != dir || (!dir && !s.Mode().IsRegular()) {
		return ErrStorage
	}
	// Refuse broad inherited ACLs. Setup uses a private directory owned by this user.
	sd, e := windows.GetNamedSecurityInfo(p, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		return ErrStorage
	}
	owner, _, e := sd.Owner()
	if e != nil {
		return ErrStorage
	}
	u, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil || !owner.Equals(u.User.Sid) {
		return ErrStorage
	}
	dacl, _, e := sd.DACL()
	if e != nil || dacl == nil {
		return ErrStorage
	}
	for j := uint32(0); j < uint32(dacl.AceCount); j++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(dacl, j, &ace) != nil {
			return ErrStorage
		}
		if ace.Header.AceType == windows.ACCESS_ALLOWED_ACE_TYPE {
			sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
			if !sid.Equals(u.User.Sid) && !sid.IsWellKnown(windows.WinLocalSystemSid) && !sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
				return ErrStorage
			}
		}
	}
	return nil
}
func lockFile(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &windows.Overlapped{})
}
func unlockFile(f *os.File) {
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &windows.Overlapped{})
}
func replaceFile(a, b string) error {
	x, e := windows.UTF16PtrFromString(a)
	if e != nil {
		return e
	}
	y, e := windows.UTF16PtrFromString(b)
	if e != nil {
		return e
	}
	return windows.MoveFileEx(x, y, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
func syncDir(string) error { return nil }
