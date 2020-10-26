// +build !linux

package dnsredir

import (
	"github.com/coredns/caddy"
	"github.com/miekg/dns"
	"runtime"
	"sync/atomic"
)

var warnedOnce int32

func parseIpset(c *caddy.Controller, u *reloadableUpstream) error {
	dir := c.Val()
	if atomic.CompareAndSwapInt32(&warnedOnce, 0, 1) {
		log.Warningf("%v: this option isn't available on %v", dir, runtime.GOOS)
	}
	return nil
}

func ipsetSetup(u *reloadableUpstream) error {
	return nil
}

func ipsetShutdown(u *reloadableUpstream) error {
	return nil
}

func ipsetAddIP(r *reloadableUpstream, reply *dns.Msg) {

}

