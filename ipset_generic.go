// +build !linux

package dnsredir

import (
	"github.com/miekg/dns"
	"sync/atomic"
)

func ipsetSetup(u *reloadableUpstream) error {
	log.Infof("ipset option only available on Linux")
	return nil
}

func ipsetShutdown(u *reloadableUpstream) error {
	return nil
}

var warnedOnce int32

func ipsetAddIP(r *reloadableUpstream, reply *dns.Msg) {
	if atomic.CompareAndSwapInt32(&warnedOnce, 0, 1) {
		log.Warningf("Cannot add IP, ipset only available on Linux.")
	}
}

