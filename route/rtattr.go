// +build linux

package route

import (
	"fmt"
	"unsafe"
)

type RtAttr struct {
	Len  uint16
	Type RtAttrType
}

type RtAttrType = uint16

var RtAttrSizeof = nlmsgAlign(int64(unsafe.Sizeof(RtAttr{})))

// https://elixir.bootlin.com/linux/v5.10.46/source/include/uapi/linux/rtnetlink.h#L336
const (
	RTA_OIF     = RtAttrType(4)
	RTA_GATEWAY = RtAttrType(5)
	__RTA_MAX   = RtAttrType(31)
)

func (a *RtAttr) valid() error {
	if int64(a.Len) < RtAttrSizeof {
		return fmt.Errorf("rtattr len should at least %v, but got %v", RtAttrSizeof, a.Len)
	}
	if a.Type >= __RTA_MAX {
		return fmt.Errorf("unknown rtattr type id %v", a.Type)
	}
	return nil
}

func (a *RtAttr) dataLen() int {
	return int(int64(a.Len) - RtAttrSizeof)
}

func (a *RtAttr) dataPtr() unsafe.Pointer {
	return unsafe.Pointer(uintptr(unsafe.Pointer(a)) + uintptr(RtAttrSizeof))
}

func (a *RtAttr) data() []byte {
	return (*(*[]byte)(a.dataPtr()))[:a.dataLen()]
}
