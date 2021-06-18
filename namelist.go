package dnsredir

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/coredns/coredns/plugin"
	"golang.org/x/net/idna"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// uint16 used to store first two ASCII characters
type domainSet map[uint16]StringSet

func (d domainSet) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%T[", d))

	var i uint64
	n := d.Len()
	for _, s := range d {
		for name := range s {
			sb.WriteString(name)
			if i++; i != n {
				sb.WriteString(", ")
			}
		}
	}
	sb.WriteString("]")

	return sb.String()
}

// Return total number of domains in the domain set
func (d *domainSet) Len() uint64 {
	var n uint64
	for _, s := range *d {
		n += uint64(len(s))
	}
	return n
}

func domainToIndex(s string) uint16 {
	n := len(s)
	if n == 0 {
		panic(fmt.Sprintf("Unexpected empty string?!"))
	}
	// Since we use two ASCII characters to present index
	//	Insufficient length will padded with '-'
	//	Since a valid domain segment will never begin with '-'
	//	So it can maintain balance between buckets
	if n == 1 {
		return (uint16('-') << 8) | uint16(s[0])
	}
	// The index will be encoded in big endian
	return (uint16(s[0]) << 8) | uint16(s[1])
}

// Return true if name added successfully, false otherwise
func (d *domainSet) Add(str string) bool {
	// To reduce memory, we don't use full qualified name

	name, ok := stringToDomain(str)
	if !ok {
		var err error
		name, err = idna.ToASCII(str)
		// idna.ToASCII("") return no error
		if err != nil || len(name) == 0 {
			return false
		}
	}

	// To speed up name lookup, we utilized two-way hash
	// The first one is the first two ASCII characters of the domain name
	// The second one is the real domain set
	// Which works somewhat like ordinary English dictionary lookup
	s := (*d)[domainToIndex(name)]
	if s == nil {
		// MT-Unsafe: Initialize real domain set on demand
		s = make(StringSet)
		(*d)[domainToIndex(name)] = s
	}
	s.Add(name)
	return true
}

// for loop will exit in advance if f() return error
func (d *domainSet) ForEachDomain(f func(name string) error) error {
	for _, s := range *d {
		for name := range s {
			if err := f(name); err != nil {
				return err
			}
		}
	}
	return nil
}

// Assume `child' is lower cased and without trailing dot
func (d *domainSet) Match(child string) bool {
	if len(child) == 0 {
		panic(fmt.Sprintf("Why child is an empty string?!"))
	}

	for {
		s := (*d)[domainToIndex(child)]
		// Fast lookup for a full match
		if s.Contains(child) {
			return true
		}

		// Fallback to iterate the whole set
		for parent := range s {
			if plugin.Name(parent).Matches(child) {
				return true
			}
		}

		i := strings.Index(child, ".")
		if i <= 0 {
			break
		}
		child = child[i+1:]
	}

	return false
}

const (
	NameItemTypePath = iota
	NameItemTypeUrl
	NameItemTypeLast // Dummy
)

type NameItem struct {
	sync.RWMutex

	// Domain name set for lookups
	names domainSet

	whichType int

	path  string
	mtime time.Time
	size  int64

	url         string
	contentHash uint64
}

func NewNameItemsWithForms(forms []string) ([]*NameItem, error) {
	items := make([]*NameItem, len(forms))
	for i, from := range forms {
		if j := strings.Index(from, "://"); j > 0 {
			proto := strings.ToLower(from[:j])
			if proto == "http" {
				log.Warningf("Due to security reasons, URL %q is prohibited", from)
				continue
			}
			if proto != "https" {
				return nil, errors.New(fmt.Sprintf("Unsupport URL %q", from))
			}
			items[i] = &NameItem{
				whichType: NameItemTypeUrl,
				url:       from,
			}
		} else {
			items[i] = &NameItem{
				whichType: NameItemTypePath,
				path:      from,
			}
		}
	}
	return items, nil
}

type NameList struct {
	// List of name items
	items []*NameItem

	// All name items shared the same reload duration

	pathReload     time.Duration
	stopPathReload chan struct{}

	urlReload      time.Duration
	urlReadTimeout time.Duration
	stopUrlReload  chan struct{}
}

// Assume `child' is lower cased and without trailing dot
func (n *NameList) Match(child string) bool {
	for _, item := range n.items {
		item.RLock()
		if item.names.Match(child) {
			item.RUnlock()
			return true
		}
		item.RUnlock()
	}
	return false
}

// MT-Unsafe
func (n *NameList) periodicUpdate(bootstrap []string) {
	// Kick off initial name list content population
	n.updateList(NameItemTypeLast, bootstrap)

	if n.pathReload > 0 {
		go func() {
			ticker := time.NewTicker(n.pathReload)
			for {
				select {
				case <-n.stopPathReload:
					return
				case <-ticker.C:
					n.updateList(NameItemTypePath, bootstrap)
				}
			}
		}()
	}

	if n.urlReload > 0 {
		go func() {
			ticker := time.NewTicker(n.urlReload)
			for {
				select {
				case <-n.stopUrlReload:
					return
				case <-ticker.C:
					n.updateList(NameItemTypeUrl, bootstrap)
				}
			}
		}()
	}
}

func (n *NameList) updateList(whichType int, bootstrap []string) {
	for _, item := range n.items {
		if whichType == NameItemTypeLast || whichType == item.whichType {
			switch item.whichType {
			case NameItemTypePath:
				n.updateItemFromPath(item)
			case NameItemTypeUrl:
				if whichType == NameItemTypeLast {
					n.initialUpdateFromUrl(item, bootstrap)
				} else {
					_ = n.updateItemFromUrl(item, bootstrap)
				}
			default:
				panic(fmt.Sprintf("Unexpected NameItem type %v", whichType))
			}
		}
	}
}

func (n *NameList) updateItemFromPath(item *NameItem) {
	file, err := os.Open(item.path)
	if err != nil {
		if os.IsNotExist(err) {
			// File not exist already reported at setup stage
			log.Debugf("%v", err)
		} else {
			log.Warningf("%v", err)
		}
		return
	}
	defer Close(file)

	stat, err := file.Stat()
	if err == nil {
		item.RLock()
		mtime := item.mtime
		size := item.size
		item.RUnlock()

		if stat.ModTime() == mtime && stat.Size() == size {
			return
		}
	} else {
		// Proceed parsing anyway
		log.Warningf("%v", err)
	}

	t1 := time.Now()
	names, totalLines := n.parse(file)
	t2 := time.Since(t1)
	log.Debugf("Parsed %v  time spent: %v name added: %v / %v",
		file.Name(), t2, names.Len(), totalLines)

	item.Lock()
	item.names = names
	item.mtime = stat.ModTime()
	item.size = stat.Size()
	item.Unlock()
}

func (n *NameList) parse(r io.Reader) (domainSet, uint64) {
	names := make(domainSet)

	var totalLines uint64
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		totalLines++

		line := scanner.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}

		f := strings.Split(line, "/")
		if len(f) != 3 {
			// Treat the whole line as a domain name
			_ = names.Add(line)
			continue
		}

		// Format: server=/<domain>/<?>
		if f[0] != "server=" {
			continue
		}

		// Don't check f[2], see: http://manpages.ubuntu.com/manpages/bionic/man8/dnsmasq.8.html
		// Thus server=/<domain>/<ip>, server=/<domain>/, server=/<domain>/# won't be honored

		if !names.Add(f[1]) {
			log.Warningf("%q isn't a domain name", f[1])
		}
	}

	return names, totalLines
}

// Return true if NameItem updated
func (n *NameList) updateItemFromUrl(item *NameItem, bootstrap []string) bool {
	if item.whichType != NameItemTypeUrl || len(item.url) == 0 {
		panic("Function call misuse or bad URL config")
	}

	t1 := time.Now()
	content, err := getUrlContent(item.url, "text/plain", bootstrap, n.urlReadTimeout)
	t2 := time.Since(t1)
	if err != nil {
		log.Warningf("Failed to update %q, err: %v", item.url, err)
		return false
	}

	item.RLock()
	contentHash := item.contentHash
	item.RUnlock()
	contentHash1 := stringHash(content)
	if contentHash1 == contentHash {
		return true
	}

	names := make(domainSet)
	var totalLines uint64
	t3 := time.Now()
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		totalLines++

		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}

		f := strings.Split(line, "/")
		if len(f) != 3 {
			_ = names.Add(line)
			continue
		}

		if f[0] != "server=" {
			continue
		}

		if !names.Add(f[1]) {
			log.Warningf("%q isn't a domain name", f[1])
		}
	}
	t4 := time.Since(t3)
	log.Debugf("Fetched %v, time spent: %v %v, added: %v / %v, hash: %#x",
		item.url, t2, t4, names.Len(), totalLines, contentHash1)

	item.Lock()
	item.names = names
	item.contentHash = contentHash1
	item.Unlock()

	return true
}

// Initial name list population needs a working DNS upstream
//	thus we need to fallback to it(if any) in case of population failure
func (n *NameList) initialUpdateFromUrl(item *NameItem, bootstrap []string) {
	go func() {
		// Fast retry in case of unstable network
		retryIntervals := []time.Duration{
			500 * time.Millisecond,
			1500 * time.Millisecond,
		}
		i := 0
		for {
			if n.updateItemFromUrl(item, bootstrap) {
				break
			}
			if i == len(retryIntervals) {
				break
			}
			time.Sleep(retryIntervals[i])
			i++
		}
	}()
}
