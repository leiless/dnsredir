// +build darwin

package pf

// #cgo CFLAGS: -Wall -Wextra -I.
// #include <string.h>		// strerror(3)
// #include <unistd.h>		// close(2)
// #include <errno.h>		// error constants
// #include "pf_darwin.h"
import "C"
import (
	"errors"
	"fmt"
	"os"
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
	case C.ACCES:
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
	return translateNegatedErrno(C.close_dev_pf(C.int(dev)))
}
