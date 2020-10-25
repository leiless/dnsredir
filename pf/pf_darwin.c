/**
 * Created: Oct 25, 2020.
 * License: MIT.
 */

#include <strings.h>        // bzero(3)
#include <errno.h>          // errno
#include <fcntl.h>          // open(2)
#include <sys/ioctl.h>      // ioctl(2)
#include <arpa/inet.h>      // inet_net_pton(3)

#define PRIVATE
#include <net/pfvar.h>      // pf*
#undef PRIVATE

#define ASSERTF_DEF_ONCE
#include "assertf.h"        // assert*

/**
 * Add IP/IP-CIDR addresses to a given table.
 *
 * @param dev       File descriptor to the /dev/pf
 * @param tbl       Which table to operates with
 * @param addr      An array of `struct pfr_addr`
 * @param size      Array size
 * @param nadd      [OUT] Effectively added address count
 * @param flags     Flags to effect add behaviour:
 *                      PFR_FLAG_ATOMIC         (this flag have no effect on macOS pf implementation)
 *                      PFR_FLAG_DUMMY          It's a dummy operation(won't take effect)
 *                      PFR_FLAG_FEEDBACK       `addr->pfra_fback` will be updated upon return
 * @return          -1 if an error has occurred(errno will be set accordingly), 0 otherwise.
 *                      EINVAL if flags/addresses is invalid
 *                      ESRCH if given table name not present or not active in pf
 *                      EPERM if given table name is immutable
 *                      EFAULT if `addr` outside the process's allocated address space
 *                      ENOMEM if kernel out of memory temporarily
 *                      And all return values of ioctl(2)
 *
 * see: xnu/bsd/net/pf_table.c#pfr_add_addrs()
 */
static int pfr_add_addrs(
        int dev,
        struct pfr_table *tbl,
        struct pfr_addr *addr,
        int size,
        int *nadd,
        int flags)
{
    struct pfioc_table io;

    if (dev < 0 || tbl == NULL || size < 0 || (size && addr == NULL)) {
        errno = EINVAL;
        return -1;
    }

    bzero(&io, sizeof(io));
    io.pfrio_flags = flags;
    io.pfrio_table = *tbl;
    io.pfrio_buffer = addr;
    // esize stands for element size
    io.pfrio_esize = sizeof(*addr);
    io.pfrio_size = size;

    if (ioctl(dev, DIOCRADDADDRS, &io) < 0) return -1;

    if (nadd != NULL) *nadd = io.pfrio_nadd;
    return 0;
}

/**
 * Create a pf table.
 *
 * @param dev       File descriptor to the /dev/pf
 * @param tbl       Table array
 * @param size      Array size
 * @param nadd      [OUT] Effectively added table count
 * @param flags     Flags to effect add behaviour:
 *                      PFR_FLAG_ATOMIC         (this flag have no effect on macOS pf implementation)
 *                      PFR_FLAG_DUMMY          It's a dummy operation(won't take effect)
 * @return          -1 if an error has occurred(errno will be set accordingly), 0 otherwise.
 *                      EINVAL if any parameter is invalid
 *                      EFAULT if `tbl` outside the process's allocated address space
 *                      ENOMEM if kernel out of memory temporarily
 *                      And all return values of ioctl(2)
 *
 * NOTE:
 *      if a table already exists, this function won't be fail, and *nadd(if not NULL) will be zero.
 *      if a table already exists(it have no anchor name), and if add table operation specified an anchor.
 *      the anchor name will be attached to the existing table.
 *      in such case, if we continue to add the same table(without anchor), it'll succeed.
 *      yet the table still with the anchor name.
 * see:
 *  xnu/bsd/net/pf_table.c#pfr_add_tables()
 */
static int pfr_add_tables(
        int dev,
        struct pfr_table *tbl,
        int size,
        int *nadd,
        int flags)
{
    struct pfioc_table io;

    if (dev < 0 || size < 0 || (size && tbl == NULL)) {
        errno = EINVAL;
        return -1;
    }

    bzero(&io, sizeof io);
    io.pfrio_flags = flags;
    io.pfrio_buffer = tbl;
    io.pfrio_esize = sizeof(*tbl);
    io.pfrio_size = size;

    if (ioctl(dev, DIOCRADDTABLES, &io) < 0) return -1;

    if (nadd != NULL) *nadd = io.pfrio_nadd;
    return 0;
}

typedef struct {
    union {
        struct in_addr v4;
        struct in6_addr v6;
    };
    uint8_t family;
    uint8_t mask;
} ip_cidr_t;

static int parse_ip_cidr(const char *ip_cidr, int family, ip_cidr_t *out)
{
    assert_nonnull(ip_cidr);
    assert_nonnull(out);

    struct in6_addr addr;
    // [ref] Note: the buffer pointed to by netp should be zeroed out before calling inet_net_pton()
    // see: https://man7.org/linux/man-pages/man3/inet_net_pton.3.html#DESCRIPTION
    bzero(&addr, sizeof(addr));
    int mask = inet_net_pton(family, ip_cidr, &addr, sizeof(addr));
    if (mask >= 0) {
        assert_le(mask, 255, %d);

        if (family == PF_INET) {
            assert_le(mask, 32, %d);
        } else {
            assert_le(mask, 128, %d);
            assert_eq(family, PF_INET6, %d);
        }

        if (family == PF_INET) {
            memcpy(&out->v4, &addr, sizeof(out->v4));
        } else {
            out->v6 = addr;
        }

        out->family = family;
        out->mask = mask;

        return 0;
    }

    return -1;
}

/**
 * NOTE: IP-CIDR is not supported in `addr_str` of current implementation.
 */
int pf_add_addr(int dev, const char *table_name, const char *anchor, const char *addr_str, int family)
{
    if (table_name == NULL || addr_str == NULL) return -EINVAL;

    struct pfr_table tbl;
    bzero(&tbl, sizeof(tbl));
    size_t size = strlcpy(tbl.pfrt_name, table_name, sizeof(tbl.pfrt_name));
    if (size >= sizeof(tbl.pfrt_name)) return -ENAMETOOLONG;

    if (anchor != NULL) {
        size = strlcpy(tbl.pfrt_anchor, anchor, sizeof(tbl.pfrt_anchor));
        if (size >= sizeof(tbl.pfrt_anchor)) return -ENAMETOOLONG;
    }

    ip_cidr_t ip;
    if (parse_ip_cidr(addr_str, family, &ip) < 0) return -errno;

    struct pfr_addr addr;
    bzero(&addr, sizeof(addr));
    if (ip.family == PF_INET) {
        addr.pfra_ip4addr = ip.v4;
    } else {
        addr.pfra_ip6addr = ip.v6;
        assert_eq(ip.family, PF_INET6, %d);
    }
    addr.pfra_af = ip.family;
    addr.pfra_net = ip.mask;

    int nadd = 0;
    if (pfr_add_addrs(dev, &tbl, &addr, 1, &nadd, PFR_FLAG_ATOMIC) < 0) return -errno;
    assert_ge(nadd, 0, %d);
    if (nadd == 0) return -EEXIST;
    return nadd;
}

int pf_add_table(int dev, const char *table_name, const char *anchor)
{
    if (table_name == NULL) return -EINVAL;

    struct pfr_table tbl;
    bzero(&tbl, sizeof(tbl));

    size_t size = strlcpy(tbl.pfrt_name, table_name, sizeof(tbl.pfrt_name));
    if (size >= sizeof(tbl.pfrt_name)) return -ENAMETOOLONG;

    if (anchor != NULL) {
        size = strlcpy(tbl.pfrt_anchor, anchor, sizeof(tbl.pfrt_anchor));
        if (size >= sizeof(tbl.pfrt_anchor)) return -ENAMETOOLONG;
    }

    int nadd = 0;
    if (pfr_add_tables(dev, &tbl, 1, &nadd, PFR_FLAG_ATOMIC) < 0) return -errno;
    assert_ge(nadd, 0, %d);
    if (nadd == 0) return -EEXIST;
    return nadd;
}

/**
 * @return      Negated errno if failed to open /dev/pf
 */
int open_dev_pf(int oflag)
{
    int fd = open("/dev/pf", oflag);
    if (fd < 0) return -errno;
    return fd;
}

int close_dev_pf(int dev)
{
    return close(dev) < 0 ? -errno : 0;
}
