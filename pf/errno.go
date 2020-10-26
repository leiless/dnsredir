// +build darwin

package pf

// #cgo CFLAGS: -Wall -Wextra -Wno-unused-parameter
// #include <string.h>		// strerror_r(3)
// #include <stdlib.h>		// malloc(3), free(3)
import "C"
import "strconv"

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

// see:
//	runtime: goroutine stack exceeds 1000000000-byte limit in simple task #34251
// 	https://github.com/golang/go/issues/34251#issuecomment-530672361
//	os.SyscallError#Error()
func (e ErrnoError) Error() string {
	return "errno: " + strconv.FormatInt(int64(e), 10) + " desc: " + e.strerror()
}
