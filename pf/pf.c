/**
 * Created: Oct 25, 2020.
 * License: MIT.
 */

#include <assert.h>         // assert(3)
#include <strings.h>        // bzero(3)
#include <errno.h>          // errno
#include <fcntl.h>          // open(2)
#include <unistd.h>         // close(2)
#include <sys/ioctl.h>      // ioctl(2)
#include <arpa/inet.h>      // inet_net_pton(3)

#define PRIVATE
#include <net/pfvar.h>      // pf*
#undef PRIVATE

#include "pf.h"

/**
 * Compile-time assertion  see: linux/arch/x86/boot/boot.h
 *
 * [sic modified]
 * BUILD_BUG_ON - break compile if a condition is true.
 * @cond: the condition which the compiler should know is false.
 *
 * If you have some code which relies on certain constants being true, or
 * some other compile-time evaluated condition, you should use BUILD_BUG_ON() to
 * detect if someone changes it unexpectedly.
 */
#ifndef BUILD_BUG_ON
#ifdef DEBUG
#define BUILD_BUG_ON(cond)      ((void) sizeof(char[1 - 2 * !!(cond)]))
#else
#define BUILD_BUG_ON(cond)      ((void) (cond))
#endif
#endif

/**
 * @return      Negated errno if failed to open /dev/pf
 */
int pf_open(int oflag)
{
    int fd = open("/dev/pf", oflag);
    if (fd < 0) return -errno;
    return fd;
}

int pf_close(int dev)
{
    return close(dev) < 0 ? -errno : 0;
}

/**
 * @return      1 if pf is enabled, 0 if disabled.
 *              Negated errno if failed to get pf status.
 */
int pf_is_enabled(int dev)
{
    struct pf_status st;
    bzero(&st, sizeof(st));
    if (ioctl(dev, DIOCGETSTATUS, &st) < 0) return -errno;
    return st.running != 0;
}

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

int pf_add_addr(int dev, const char *table_name, const char *anchor, const void *addr_buf, size_t n)
{
    BUILD_BUG_ON(sizeof(struct in_addr) != 4u);
    BUILD_BUG_ON(sizeof(struct in6_addr) != 16u);

    if (table_name == NULL || addr_buf == NULL) return -EINVAL;
    if (n != 4 && n != 16) return -EINVAL;

    struct pfr_table tbl;
    bzero(&tbl, sizeof(tbl));
    size_t size = strlcpy(tbl.pfrt_name, table_name, sizeof(tbl.pfrt_name));
    if (size >= sizeof(tbl.pfrt_name)) return -ENAMETOOLONG;

    if (anchor != NULL) {
        size = strlcpy(tbl.pfrt_anchor, anchor, sizeof(tbl.pfrt_anchor));
        if (size >= sizeof(tbl.pfrt_anchor)) return -ENAMETOOLONG;
    }

    struct pfr_addr addr;
    bzero(&addr, sizeof(addr));
    memcpy(&addr.pfra_u, addr_buf, n);
    addr.pfra_af = (n == 4 ? PF_INET : PF_INET6);
    addr.pfra_net = n << 3u;

    int nadd = 0;
    if (pfr_add_addrs(dev, &tbl, &addr, 1, &nadd, PFR_FLAG_ATOMIC) < 0) return -errno;
    assert(nadd >= 0);
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
    assert(nadd >= 0);
    return nadd;
}
