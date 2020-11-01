// +build darwin

package pf

// #cgo CFLAGS: -Wall -Wextra -Wno-unused-parameter
// #include <stdlib.h>		// free(3)
// #include "pf.h"
import "C"
import (
	"fmt"
	"net"
	"os"
	"unsafe"
)

// User should reply on the error number instead of the description.
func translateNegatedErrno(errno int) error {
	if errno == 0 {
		return nil
	}
	if errno > 0 {
		panic(fmt.Sprintf("expected a negated errno value, got: %v", errno))
	}
	return ErrnoError(-errno)	// Rectify errno
}

func OpenDevPf(oflag int) (int, error) {
	fd := int(C.open_dev_pf(C.int(oflag)))
	if fd < 0 {
		return 0, translateNegatedErrno(fd)
	}
	return fd, nil
}

func CloseDevPf(dev int) error {
	return translateNegatedErrno(int(C.close_dev_pf(C.int(dev))))
}

// Return 	true, nil if added successfully
//			false, nil if given name[:anchor] already exists
func AddTable(dev int, name, anchor string) (bool, error) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	canchor := C.CString(anchor)
	defer C.free(unsafe.Pointer(canchor))

	rc := int(C.pf_add_table(C.int(dev), cname, canchor))
	if rc < 0 {
		return false, translateNegatedErrno(rc)
	}
	return rc != 0, nil
}

func AddAddr(dev int, name, anchor string, ip net.IP) (bool, error) {
	var addr net.IP
	if a := ip.To4(); a != nil {
		addr = a
	} else if a := ip.To16(); a != nil {
		addr = a
	} else {
		return false, os.ErrInvalid
	}

	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	canchor := C.CString(anchor)
	defer C.free(unsafe.Pointer(canchor))
	caddr := C.CBytes(addr)
	defer C.free(caddr)

	// see:
	//	https://golang.org/cmd/cgo/#hdr-Go_references_to_C
	//	https://stackoverflow.com/questions/35673161/convert-go-byte-to-a-c-char
	rc := int(C.pf_add_addr(C.int(dev), cname, canchor, caddr, C.ulong(len(addr))))
	if rc < 0 {
		return false, translateNegatedErrno(rc)
	}
	return rc != 0, nil
}
