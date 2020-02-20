package redirect

import (
	"bufio"
	"io"
	"os"
	"sync"
	"time"
)

type stringSet map[string]struct{}

/**
 * @return	true if `str' already in set previously
 *			false otherwise
 */
func (set *stringSet) Add(str string) bool {
	_, found := (*set)[str]
	(*set)[str] = struct{}{}
	return found
}

type Nameitem struct {
	// Domain name set for lookups
	names stringSet

	path string
	mtime time.Time
	size int64
}

func PathsToNameitems(paths []string) []Nameitem {
	items := make([]Nameitem, len(paths))
	for i, path := range paths {
		items[i].path = path
	}
	return items
}

type Namelist struct {
	sync.RWMutex

	// List of name items
	items []Nameitem

	// Time between two reload of a name item
	// All name items shared the same reload duration
	reload time.Duration
}

func (n *Namelist) parseNamelistCore(i int) {
	item := &n.items[i]

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
		n.RLock()
		mtime := item.mtime
		size := item.size
		n.RUnlock()

		if stat.ModTime() == mtime && stat.Size() == size {
			return
		}
	} else {
		// Proceed parsing anyway
		log.Warningf("%v", err)
	}

	log.Debugf("Parsing " + file.Name())
	names := n.parse(file)

	n.Lock()
	item.names = names
	item.mtime = stat.ModTime()
	item.size = stat.Size()
	n.Unlock()
}

func (n *Namelist) parse(r io.Reader) stringSet {
	names := make(stringSet)
	names.Add("foobar")

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		log.Info(scanner.Text())
	}

	return names
}

func (n *Namelist) parseNamelist() {
	for i := range n.items {
		n.parseNamelistCore(i)
	}

	for _, item := range n.items {
		log.Debugf(">>> %v", item)
	}
}

