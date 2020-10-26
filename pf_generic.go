// +build !darwin

// pf is generally available in BSD-derived systems,
//	yet currently we do not have plan to support other BSD distributions than macOS.

package dnsredir

import (
	"github.com/coredns/caddy"
	"github.com/miekg/dns"
	"runtime"
)

var once Once

func pfParse(c *caddy.Controller, u *ReloadableUpstream) error {
	_ = u
	once.Do(func() {
		dir := c.Val()
		log.Warningf("%v is not available on %v", dir, runtime.GOOS)
	})
	return nil
}

func pfSetup(u *ReloadableUpstream) error {
	_ = u
	return nil
}

func pfShutdown(u *ReloadableUpstream) error {
	_ = u
	return nil
}

func pfAddIP(r *ReloadableUpstream, reply *dns.Msg) {
	_, _ = r, reply
}
