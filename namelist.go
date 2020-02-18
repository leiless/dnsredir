package redirect

import (
	"bufio"
	"io"
	"os"
	"sync"
	"time"
)

type Fileitem struct {
	path string
	mtime time.Time
	size int64
}

func PathsToFileitems(paths []string) []Fileitem {
	files := make([]Fileitem, len(paths))
	for i, path := range paths {
		files[i].path = path
	}
	return files
}

var EmptyStruct = struct{}{}

type Namelist struct {
	sync.RWMutex

	// Domain name set for lookups
	names map[string]struct{}

	// List of name files
	files []Fileitem

	// Time between two reload of the name file
	// All file files shared the same reload duration
	reload time.Duration
}

func (n *Namelist) parseNamelistCore(fi *Fileitem) {
	file, err := os.Open(fi.path)
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
		mtime := fi.mtime
		size := fi.size
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
	n.names = names
	n.Unlock()
}

func (n *Namelist) parse(r io.Reader) map[string]struct{} {
	names := make(map[string]struct{})
	names["test"] = EmptyStruct

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		log.Info(scanner.Text())
	}

	return names
}

func (n *Namelist) parseNamelist() {
	for _, file := range n.files {
		n.parseNamelistCore(&file)
	}
}

