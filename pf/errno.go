package pf

// #cgo CFLAGS: -Wall -Wextra -Wno-unused-parameter
// #include <string.h>		// strerror_r(3)
// #include <stdlib.h>		// malloc(3), free(3)
import "C"
import "fmt"

type ErrnoError int

const errorBufSize = uint(256)

func (e ErrnoError) strerror() string {
	size := C.ulong(C.sizeof_char * errorBufSize)
	buf := C.malloc(size)
	defer C.free(buf)
	// We don't care the return value, since the buf will always be filled.
	_ = C.strerror_r(C.int(e), (*C.char)(buf), size)
	return C.GoString((*C.char)(buf))
}

func (e ErrnoError) Errno() int {
	return int(e)
}

func (e ErrnoError) Error() string {
	return fmt.Sprintf("errno: %v desc: %v", e, e.strerror())
}
