// +build !darwin

// pf is generally available in BSD-derived systems,
//	yet currently we do not have plan to support other BSD distributions than macOS.

package pf

import (
	"github.com/coredns/caddy"
	"github.com/leiless/dnsredir"
	"github.com/miekg/dns"
	"runtime"
)

var once dnsredir.Once

func Parse(c *caddy.Controller, u *dnsredir.ReloadableUpstream) error {
	_ = u
	once.Do(func() {
		dir := c.Val()
		dnsredir.Log.Warningf("%v is not available on %v", dir, runtime.GOOS)
	})
	return nil
}

func Setup(u *dnsredir.ReloadableUpstream) error {
	_ = u
	return nil
}

func Shutdown(u *dnsredir.ReloadableUpstream) error {
	_ = u
	return nil
}

func AddIP(r *dnsredir.ReloadableUpstream, reply *dns.Msg) {
	_, _ = r, reply
}
