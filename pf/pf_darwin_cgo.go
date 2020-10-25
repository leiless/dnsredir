// +build darwin

package pf

// #cgo CFLAGS: -Wall -Wextra -I.
// #include <string.h>		// strerror(3)
// #include <unistd.h>		// close(2)
// #include <errno.h>		// error constants
// #include <stdlib.h>		// malloc(3), free(3)
// #include "pf_darwin.h"
import "C"
import (
	"errors"
	"fmt"
	"net"
	"os"
	"unsafe"
)

const errorBufSize = uint(256)

func strerror(errno int) string {
	size := C.ulong(C.sizeof_char * errorBufSize)
	buf := C.malloc(size)
	defer C.free(buf)
	// We don't care the return value, since the buf will always be filled.
	_ = C.strerror_r(C.int(errno), (*C.char)(buf), size)
	return C.GoString((*C.char)(buf))
}

func translateNegatedErrno(errno int) error {
	if errno == 0 {
		return nil
	}
	if errno > 0 {
		panic(fmt.Sprintf("expected a negated errno value, got: %v", errno))
	}
	switch errno {
	case C.EINVAL:
		return os.ErrInvalid
	case C.EACCES:
		return os.ErrPermission
	case C.EEXIST:
		return os.ErrExist
	case C.ENOENT:
		return os.ErrNotExist
	default:
		return errors.New(fmt.Sprintf("errno: %v desc: %v", errno, strerror(errno)))
	}
}

func openDevPf(oflag int) (int, error) {
	fd := int(C.open_dev_pf(C.int(oflag)))
	if fd < 0 {
		return 0, translateNegatedErrno(fd)
	}
	return fd, nil
}

func closeDevPf(dev int) error {
	return translateNegatedErrno(int(C.close_dev_pf(C.int(dev))))
}

// Return 	true, nil if added successfully
//			false, nil if given name[:anchor] already exists
func addTable(dev int, name, anchor string) (bool, error) {
	rc := int(C.pf_add_table(C.int(dev), C.CString(name), C.CString(anchor)))
	if rc < 0 {
		return false, translateNegatedErrno(rc)
	}
	return rc != 0, nil
}

func addAddr(dev int, name, anchor string, ip net.IP) (bool, error) {
	var addr net.IP
	if a := ip.To4(); a != nil {
		addr = a
	} else if a := ip.To16(); a != nil {
		addr = a
	} else {
		return false, os.ErrInvalid
	}
	// see: https://stackoverflow.com/questions/35673161/convert-go-byte-to-a-c-char
	rc := int(C.pf_add_addr(C.int(dev), C.CString(name), C.CString(anchor), (unsafe.Pointer)(&addr[0]), C.ulong(len(addr))))
	if rc < 0 {
		return false, translateNegatedErrno(rc)
	}
	return rc != 0, nil
}
