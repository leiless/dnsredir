// +build linux

package route

// RtMsg see: https://elixir.bootlin.com/linux/v5.10.46/source/include/uapi/linux/rtnetlink.h#L221
type RtMsg struct {
	Family RtmFamily
	DstLen uint8
	SrcLen uint8
	Tos    uint8

	Table    RtmTable
	Protocol RtmProtocol
	Scope    RtmScope
	Type     RtmType

	Flags RtmFlag
}

type (
	RtmFamily   = uint8
	RtmTable    = uint8
	RtmProtocol = uint8
	RtmScope    = uint8
	RtmType     = uint8
	RtmFlag     = uint32
)

// RtMsg.Family
// https://elixir.bootlin.com/linux/v5.10.46/source/include/linux/socket.h#L179
const (
	AF_INET  = RtmFamily(2)  /* Internet IP Protocol */
	AF_INET6 = RtmFamily(10) /* IP version 6 */
)

// RtMsg.Table
// https://elixir.bootlin.com/linux/v5.10.46/source/include/uapi/linux/rtnetlink.h#L325
const (
	RT_TABLE_MAIN = RtmTable(254)
)
