// +build linux

package route

import (
	"errors"
	"fmt"
	"github.com/mdlayher/netlink"
	"net"
	"unsafe"
)

type Info struct {
	Gateway net.IP
	IfIndex int
	IfName  string
}

func (info *Info) clear() {
	info.Gateway = nil
	// Valid interface index must be positive
	// see: https://github.com/golang/go/blob/master/src/net/interface.go#L130-L132
	info.IfIndex = 0
	info.IfName = ""
}

func (info *Info) valid() bool {
	return info.Gateway != nil && info.IfIndex > 0 && info.IfName != ""
}

var ErrDefaultRouteNotFound = errors.New("default route not found")

func GetDefaultRoute() (*Info, error) {
	c, err := netlink.Dial(netlinkFamilyRoute, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to dial netlink: %w", err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			panic(fmt.Errorf("cannot close netlink socket: %w", err))
		}
	}()

	req := netlink.Message{
		// Package netlink will automatically set header fields which are set to zero
		Header: netlink.Header{
			Length: uint32(nlmsghdrSizeof + rtmsgSizeof),
			Type:   rtnetlinkRtmGetRoute,
			Flags:  netlink.Request | netlink.Dump,
		},
	}

	msgList, err := c.Execute(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	info := Info{}
	for _, msg := range msgList {
		// Verify message total length matches
		if int64(msg.Header.Length) != nlmsghdrSizeof+int64(len(msg.Data)) {
			return nil, fmt.Errorf("bad header len: %v != %v + %v",
				msg.Header.Length, nlmsghdrSizeof, len(msg.Data))
		}

		routeEntry := (*RtMsg)(unsafe.Pointer(&msg.Data[0]))
		if routeEntry.Family != AF_INET && routeEntry.Family != AF_INET6 {
			continue
		}
		if routeEntry.Table != RT_TABLE_MAIN {
			continue
		}

		attr := (*RtAttr)(unsafe.Pointer(uintptr(unsafe.Pointer(routeEntry)) + uintptr(rtmsgSizeof)))
		attrTotalLen := int64(msg.Header.Length) - nlmsgAlign(nlmsghdrSizeof+rtmsgSizeof)

		info.clear()
		cursor := int64(0)
		for cursor < attrTotalLen {
			if err := attr.valid(); err != nil {
				return nil, fmt.Errorf("route attribute is invalid: %w", err)
			}

			if attr.Type == RTA_OIF {
				if attr.dataLen() != int(unsafe.Sizeof(int32(0))) {
					return nil, fmt.Errorf("bad RTA_OTF data len %v", attr.dataLen())
				}
				ifIndex := *(*int32)(attr.dataPtr())
				dev, err := net.InterfaceByIndex(int(ifIndex))
				if err != nil {
					return nil, fmt.Errorf("cannot get network interface by index %v: %w", ifIndex, err)
				}
				info.IfIndex = dev.Index
				info.IfName = dev.Name
			} else if attr.Type == RTA_GATEWAY {
				if attr.dataLen() == net.IPv4len {
					gateway := net.IP((*(*[net.IPv4len]byte)(attr.dataPtr()))[:])
					info.Gateway = gateway
				} else if attr.dataLen() == net.IPv6len {
					gateway := net.IP((*(*[net.IPv6len]byte)(attr.dataPtr()))[:])
					info.Gateway = gateway
				} else {
					return nil, fmt.Errorf("bad RTA_GATEWAY data len: %v", attr.dataLen())
				}
			}

			rtaLen := nlmsgAlign(int64(attr.Len))
			cursor += rtaLen
			attr = (*RtAttr)(unsafe.Pointer(uintptr(unsafe.Pointer(attr)) + uintptr(rtaLen)))
		}

		if cursor != attrTotalLen {
			return nil, fmt.Errorf("expected cursor position %v, got %v", attrTotalLen, cursor)
		}

		if info.valid() {
			return &info, nil
		}
	}

	return nil, ErrDefaultRouteNotFound
}
