/*
 * Created Feb 23, 2020
 */

package dnsredir

import (
	"crypto/tls"
	"fmt"
	"github.com/caddyserver/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/parse"
	pkgtls "github.com/coredns/coredns/plugin/pkg/tls"
	"github.com/coredns/coredns/plugin/pkg/transport"
	"github.com/miekg/dns"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type reloadableUpstream struct {
	// Flag indicate match any request, i.e. the root zone "."
	matchAny bool
	*Namelist
	ignored domainSet
	*HealthCheck
}

// reloadableUpstream implements Upstream interface

// Check if given name in upstream name list
func (u *reloadableUpstream) Match(name string) bool {
	child, ok := stringToDomain(name)
	if !ok {
		log.Warningf("Skip invalid domain %q, report to Github repo if it's an error.", name)
		return false
	}

	if u.matchAny {
		if !plugin.Name(".").Matches(child) {
			panic(fmt.Sprintf("Why %v doesn't match %q?!", child, "."))
		}

		ignored := u.ignored.Match(child)
		if ignored {
			log.Debugf("#0 Skip %q since it's ignored", child)
		}
		return !ignored
	}

	if !u.Namelist.Match(child) {
		return false
	}

	if u.ignored.Match(child) {
		log.Debugf("#1 Skip %q since it's ignored", child)
		return false
	}
	return true
}

func (u *reloadableUpstream) Start() error {
	u.periodicUpdate()
	u.HealthCheck.Start()
	return nil
}

func (u *reloadableUpstream) Stop() error {
	close(u.stopReload)
	u.HealthCheck.Stop()
	return nil
}

// Parses Caddy config input and return a list of reloadable upstream for this plugin
func NewReloadableUpstreams(c *caddy.Controller) ([]Upstream, error) {
	var ups []Upstream

	for c.Next() {
		u, err := newReloadableUpstream(c)
		if err != nil {
			return nil, err
		}
		ups = append(ups, u)
	}

	if ups == nil {
		panic("Why upstream hosts is nil? it shouldn't happen.")
	}
	return ups, nil
}

// see: healthcheck.go/UpstreamHost.Dial()
func transToProto(proto string, t *Transport) string {
	switch {
	case t.tlsConfig != nil:
		proto = "tcp-tls"
	case t.forceTcp:
		proto = "tcp"
	case t.preferUdp || proto == transport.DNS:
		proto = "udp"
	}
	return proto
}

// XXX: Currently have no support over DoH and gRPC
func isKnownTrans(trans string) bool {
	return trans == transport.DNS || trans == transport.TLS
}

func newReloadableUpstream(c *caddy.Controller) (Upstream, error) {
	u := &reloadableUpstream{
		Namelist: &Namelist{
			reload:     defaultReloadInterval,
			stopReload: make(chan struct{}),
		},
		ignored: make(domainSet),
		HealthCheck: &HealthCheck{
			stop:          make(chan struct{}),
			maxFails:      defaultMaxFails,
			checkInterval: defaultHcInterval,
			transport: &Transport{
				expire: defaultConnExpire,
				tlsConfig: new(tls.Config),
			},
		},
	}

	if err := parseFilePaths(c, u); err != nil {
		return nil, err
	}

	for c.NextBlock() {
		if err := parseBlock(c, u); err != nil {
			return nil, err
		}
	}

	if u.hosts == nil {
		return nil, c.Errf("missing mandatory property: %q", "to")
	}
	for _, host := range u.hosts {
		trans, addr := parse.Transport(host.addr)
		if !isKnownTrans(trans) {
			return nil, c.Errf("%q protocol isn't supported currently", trans)
		}
		host.addr = addr

		host.transport = newTransport()
		// Inherit from global transport settings
		host.transport.forceTcp = u.transport.forceTcp
		host.transport.preferUdp = u.transport.preferUdp
		host.transport.expire = u.transport.expire
		if trans == transport.TLS {
			host.transport.tlsConfig = u.transport.tlsConfig
		}

		host.c = &dns.Client{
			Net: transToProto(trans, host.transport),
			TLSConfig: host.transport.tlsConfig,
			Timeout: defaultHcTimeout,
		}
	}

	if u.matchAny && u.reload != 0 {
		log.Warningf("Reset reload %v since FILE is %q", u.reload, ".")
		u.reload = 0
	}

	return u, nil
}

func parseFilePaths(c *caddy.Controller, u *reloadableUpstream) error {
	paths := c.RemainingArgs()
	n := len(paths)
	if n == 0 {
		return c.ArgErr()
	}

	if n == 1 && paths[0] == "." {
		u.matchAny = true
		log.Infof("Match any")
		return nil
	}

	config := dnsserver.GetConfig(c)
	for _, path := range paths {
		if !filepath.IsAbs(path) && config.Root != "" {
			path = filepath.Join(config.Root, path)
		}

		st, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				log.Warningf("File %s doesn't exist", path)
			} else {
				return err
			}
		} else if st != nil && !st.Mode().IsRegular() {
			log.Warningf("File %s isn't a regular file", path)
		}
	}

	u.items = NewNameitemsWithPaths(paths)
	log.Infof("Files: %v", paths)

	return nil
}

func parseBlock(c *caddy.Controller, u *reloadableUpstream) error {
	switch dir := c.Val(); dir {
	case "reload":
		dur, err := parseDuration(c)
		if err != nil {
			return err
		}
		if dur < minReloadInterval {
			return c.Errf("%v: minimal interval is %v", dir, minReloadInterval)
		}
		u.reload = dur
		log.Infof("%v: %v", dir, u.reload)
	case "except":
		// Multiple "except"s will be merged together
		args := c.RemainingArgs()
		if len(args) == 0 {
			return c.ArgErr()
		}
		for _, name := range args {
			if !u.ignored.Add(name) {
				log.Warningf("'%v' isn't a domain name", name)
			}
		}
		log.Infof("%v: %v", dir, u.ignored)
	case "spray":
		if len(c.RemainingArgs()) != 0 {
			return c.ArgErr()
		}
		u.spray = &Spray{}
		log.Infof("%v: enabled", dir)
	case "policy":
		arr := c.RemainingArgs()
		if len(arr) != 1 {
			return c.ArgErr()
		}
		policy, ok := SupportedPolicies[arr[0]]
		if !ok {
			return c.Errf("unknown policy: %q", arr[0])
		}
		u.policy = policy
		log.Infof("%v: %v", dir, arr[0])
	case "max_fails":
		n, err := parseInt32(c)
		if err != nil {
			return err
		}
		u.maxFails = uint32(n)
		log.Infof("%v: %v", dir, n)
	case "health_check":
		dur, err := parseDuration(c)
		if err != nil {
			return err
		}
		if dur < minHcInterval {
			return c.Errf("%v: minimal interval is %v", dir, minHcInterval)
		}
		u.checkInterval = dur
		log.Infof("%v: %v", dir, u.checkInterval)
	case "to":
		// Multiple "to"s will be merged together
		if err := parseTo(c, u); err != nil {
			return err
		}
	case "force_tcp":
		if c.NextArg() {
			return c.ArgErr()
		}
		u.transport.forceTcp = true
		// Reset prefer_udp since force_tcp takes precedence
		if u.transport.preferUdp {
			u.transport.preferUdp = false
			log.Warningf("%v: prefer_udp invalidated", dir)
		}
		log.Infof("%v: enabled", dir)
	case "prefer_udp":
		if c.NextArg() {
			return c.ArgErr()
		}
		if u.transport.forceTcp == false {
			// Ditto.
			u.transport.preferUdp = true
			log.Infof("%v: enabled", dir)
		} else {
			log.Warningf("%v: force_tcp already turned on", dir)
		}
	case "expire":
		dur, err := parseDuration(c)
		if err != nil {
			return err
		}
		if dur < minExpireInterval {
			return c.Errf("%v: minimal interval is %v", dir, minExpireInterval)
		}
		u.transport.expire = dur
		log.Infof("%v: %v", dir, dur)
	case "tls":
		args := c.RemainingArgs()
		if len(args) > 3 {
			return c.ArgErr()
		}
		tlsConfig, err := pkgtls.NewTLSConfigFromArgs(args...)
		if err != nil {
			return err
		}
		// Merge server name if tls_servername set previously
		tlsConfig.ServerName = u.transport.tlsConfig.ServerName
		u.transport.tlsConfig = tlsConfig
		log.Infof("%v: %v", dir, args)
	case "tls_servername":
		args := c.RemainingArgs()
		if len(args) != 1 {
			return c.ArgErr()
		}
		domain, ok := stringToDomain(args[0])
		if !ok {
			return c.Errf("%v: %q isn't a valid domain name", dir, args[0])
		}
		u.transport.tlsConfig.ServerName = domain
		log.Infof("%v: %v", dir, domain)
	default:
		return c.Errf("unknown property: %q", dir)
	}
	return nil
}

// Return a non-negative int32
// see: https://golang.org/pkg/builtin/#int
func parseInt32(c *caddy.Controller) (int32, error) {
	dir := c.Val()
	args := c.RemainingArgs()
	if len(args) != 1 {
		return 0, c.ArgErr()
	}

	n, err := strconv.Atoi(args[0])
	if err != nil {
		return 0, err
	}

	// In case of n is 64-bit
	if n < 0 || n > 0x7fffffff {
		return 0, c.Errf("%v: value %v of out of non-negative int32 range", dir, n)
	}

	return int32(n), nil
}

// Return a non-negative time.Duration and an error(if any)
func parseDuration(c *caddy.Controller) (time.Duration, error) {
	dir := c.Val()
	args := c.RemainingArgs()
	if len(args) != 1 {
		return 0, c.ArgErr()
	}

	arg := args[0]
	if _, err := strconv.Atoi(arg); err == nil {
		log.Warningf("%v: %s missing time unit, assume it's second", dir, arg)
		arg += "s"
	}

	duration, err := time.ParseDuration(arg)
	if err != nil {
		return 0, err
	}

	if duration < 0 {
		return 0, c.Errf("%v: negative time duration %s", arg)
	}
	return duration, nil
}

func parseTo(c *caddy.Controller, u *reloadableUpstream) error {
	dir := c.Val()
	args := c.RemainingArgs()
	if len(args) == 0 {
		return c.ArgErr()
	}

	toHosts, err := parse.HostPortOrFile(args...)
	if err != nil {
		return err
	}
	if len(toHosts) == 0 {
		return c.Errf("%q parsed from file(s), yet no valid entry was found", dir)
	}

	for i, host := range toHosts {
		trans, addr := parse.Transport(host)
		log.Infof("#%v: Transport: %v \t Address: %v", i, trans, addr)

		uh := &UpstreamHost{
			addr: host,		// Not an error, addr will be separated later
			downFunc: checkDownFunc(u),
		}
		u.hosts = append(u.hosts, uh)

		log.Infof("Upstream: %v", uh)
	}

	return nil
}

const (
	defaultMaxFails       = 3
	defaultReloadInterval = 2 * time.Second
	defaultHcInterval     = 2000 * time.Millisecond
	defaultHcTimeout      = 5000 * time.Millisecond
)

const (
	minReloadInterval = 1 * time.Second
	minHcInterval     = 500 * time.Millisecond
	minExpireInterval = 1 * time.Second
)

