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
	"strings"
	"time"
)

type reloadableUpstream struct {
	// Flag indicate match any request, i.e. the root zone "."
	matchAny bool
	*Namelist
	inline domainSet
	ignored domainSet
	*HealthCheck
}

// reloadableUpstream implements Upstream interface

// Check if given name in upstream name list
// `name' is lower cased and without trailing dot(except for root zone)
func (u *reloadableUpstream) Match(name string) bool {
	if u.matchAny {
		if !plugin.Name(".").Matches(name) {
			panic(fmt.Sprintf("Why %q doesn't match %q?!", name, "."))
		}

		ignored := u.ignored.Match(name)
		if ignored {
			log.Debugf("#0 Skip %q since it's ignored", name)
		}
		return !ignored
	}

	if !u.Namelist.Match(name) && !u.inline.Match(name) {
		return false
	}

	if u.ignored.Match(name) {
		log.Debugf("#1 Skip %q since it's ignored", name)
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
		inline: make(domainSet),
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
		addr, tlsServerName := SplitByByte(host.addr, '@')
		trans, addr := parse.Transport(addr)
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
			// Deep copy
			host.transport.tlsConfig = new(tls.Config)
			*host.transport.tlsConfig = *u.transport.tlsConfig

			// TLS server name in tls:// takes precedence over the global one(if any)
			if len(tlsServerName) != 0 {
				serverName, ok := stringToDomain(tlsServerName)
				if !ok {
					return nil, c.Errf("invalid TLS server name %q", tlsServerName)
				}
				host.transport.tlsConfig.ServerName = serverName
			}
		}

		host.c = &dns.Client{
			Net: transToProto(trans, host.transport),
			TLSConfig: host.transport.tlsConfig,
			Timeout: defaultHcTimeout,
		}
	}

	if err := u.inline.ForEachDomain(func(name string) error {
		if u.ignored.Match(name) {
			return c.Errf("conflict domain %q in both %q and %q", name, "except", "INLINE")
		}
		return nil
	}); err != nil {
		return nil, err
	}

	if u.matchAny {
		if u.inline.Len() != 0 {
			return nil, c.Errf("INLINE %q is forbidden since %q will match all requests", u.inline, ".")
		}
		if u.reload != 0 {
			log.Warningf("Reset reload %v to zero since %q is matched", u.reload, ".")
			u.reload = 0
		}
	}

	if u.inline.Len() != 0 {
		log.Infof("inline: %v", u.inline)
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
				log.Warningf("File %q doesn't exist", path)
			} else {
				return err
			}
		} else if st != nil && !st.Mode().IsRegular() {
			log.Warningf("File %q isn't a regular file", path)
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
				log.Warningf("%q isn't a domain name", name)
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
		serverName, ok := stringToDomain(args[0])
		if !ok {
			return c.Errf("%v: %q isn't a valid domain name", dir, args[0])
		}
		u.transport.tlsConfig.ServerName = serverName
		log.Infof("%v: %v", dir, serverName)
	default:
		if len(c.RemainingArgs()) != 0 ||!u.inline.Add(dir) {
			return c.Errf("unknown property: %q", dir)
		}
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

// tls://ip[[:port]|[@tls_server_name]]
// If you combine ':' and '@', ':' must comes first
// TODO: lame implementation, rewrite this someday
func splitTlsServerNames(args []string) ([]string, []string) {
	var tos []string
	var tlsServerNames []string
	for _, to := range args {
		i := strings.IndexByte(to, '@')
		if i >= 0 {
			tos = append(tos, to[:i])
			// '@' will be reserved in place
			tlsServerNames = append(tlsServerNames, to[i:])
		} else {
			tos = append(tos, to)
			tlsServerNames = append(tlsServerNames, "")
		}
	}
	if len(tos) != len(tlsServerNames) {
		panic(fmt.Sprintf("Size mismatch  %v vs %v", len(tos), len(tlsServerNames)))
	}
	return tos, tlsServerNames
}

func parseTo(c *caddy.Controller, u *reloadableUpstream) error {
	dir := c.Val()
	args := c.RemainingArgs()
	if len(args) == 0 {
		return c.ArgErr()
	}

	args, tlsServerNames := splitTlsServerNames(args)
	toHosts, err := parse.HostPortOrFile(args...)
	if err != nil {
		return err
	}
	if len(toHosts) == 0 {
		return c.Errf("%q parsed from file(s), yet no valid entry was found", dir)
	}

	for i, host := range toHosts {
		trans, addr := parse.Transport(host)
		log.Infof("#%v: Transport: %v \t Address: %v%v", i, trans, addr, tlsServerNames[i])

		uh := &UpstreamHost{
			// Not an error, host and tls server name will be separated later
			addr: host + tlsServerNames[i],
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
	minHcInterval     = 1 * time.Second
	minExpireInterval = 1 * time.Second
)

