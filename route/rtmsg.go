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
	RT_TABLE_UNSPEC  = RtmTable(0)
	RT_TABLE_COMPAT  = RtmTable(252)
	RT_TABLE_DEFAULT = RtmTable(253)
	RT_TABLE_MAIN    = RtmTable(254)
	RT_TABLE_LOCAL   = RtmTable(255)
)

// RtMsg.Protocol
// https://elixir.bootlin.com/linux/v5.10.46/source/include/uapi/linux/rtnetlink.h#L258
const (
	RTPROT_UNSPEC   = RtmProtocol(0)
	RTPROT_REDIRECT = RtmProtocol(1)  /* Route installed by ICMP redirects; not used by current IPv4 */
	RTPROT_KERNEL   = RtmProtocol(2)  /* Route installed by kernel */
	RTPROT_BOOT     = RtmProtocol(3)  /* Route installed during boot */
	RTPROT_STATIC   = RtmProtocol(4)  /* Route installed by administrator */
	RTPROT_RA       = RtmProtocol(9)  /* RDISC/ND router advertisements */
	RTPROT_DHCP     = RtmProtocol(16) /* DHCP client */
)

// RtMsg.Scope
// https://elixir.bootlin.com/linux/v5.10.46/source/include/uapi/linux/rtnetlink.h#L292
const (
	RT_SCOPE_UNIVERSE = RtmScope(0)
	RT_SCOPE_SITE     = RtmScope(200)
	RT_SCOPE_LINK     = RtmScope(253)
	RT_SCOPE_HOST     = RtmScope(254)
	RT_SCOPE_NOWHERE  = RtmScope(255)
)

// RtMsg.Type
// https://elixir.bootlin.com/linux/v5.10.46/source/include/uapi/linux/rtnetlink.h#L235
const (
	RTN_UNSPEC    = RtmType(iota)
	RTN_UNICAST   /* Gateway or direct route */
	RTN_LOCAL     /* Accept locally */
	RTN_BROADCAST /* Accept locally as broadcast, send as broadcast */

	RTN_ANYCAST /* Accept locally as broadcast, but send as unicast */

	RTN_MULTICAST   /* Multicast route */
	RTN_BLACKHOLE   /* Drop */
	RTN_UNREACHABLE /* Destination is unreachable */
	RTN_PROHIBIT    /* Administratively prohibited */
	RTN_THROW       /* Not in this table */
	RTN_NAT         /* Translate this address */
	RTN_XRESOLVE    /* Use external resolver */
	__RTN_MAX
)

// RtMsg.Flags
// https://elixir.bootlin.com/linux/v5.10.46/source/include/uapi/linux/rtnetlink.h#L312
const (
	RTM_F_NOTIFY       = RtmFlag(0x100 << iota) /* Notify user of route change */
	RTM_F_CLONED                                /* This route is cloned */
	RTM_F_EQUALIZE                              /* Multipath equalizer: NI */
	RTM_F_PREFIX                                /* Prefix addresses */
	RTM_F_LOOKUP_TABLE                          /* set rtm_table to FIB lookup result */
	RTM_F_FIB_MATCH                             /* return full fib lookup match */
	RTM_F_OFFLOAD                               /* route is offloaded */
	RTM_F_TRAP                                  /* route is trapping packets */
)
