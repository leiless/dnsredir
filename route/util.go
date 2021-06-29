// +build linux

package route

import (
	"github.com/mdlayher/netlink"
	"unsafe"
)

// see: <linux/netlink.h>
const (
	netlinkFamilyRoute   = 0
	rtnetlinkRtmGetRoute = 26
)

// #define NLMSG_ALIGNTO   4U
const nlmsgAlignTo = 4

// #define NLMSG_ALIGN(len) ( ((len)+NLMSG_ALIGNTO-1) & ~(NLMSG_ALIGNTO-1) )
func nlmsgAlign(len int64) int64 {
	return ((len) + nlmsgAlignTo - 1) & ^(nlmsgAlignTo - 1)
}

var (
	nlmsghdrSizeof = nlmsgAlign(int64(unsafe.Sizeof(netlink.Header{})))
	rtmsgSizeof    = nlmsgAlign(int64(unsafe.Sizeof(RtMsg{})))
)
