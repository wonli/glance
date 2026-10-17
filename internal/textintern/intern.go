package textintern

import (
	"sync"
)

var (
	poolMu sync.RWMutex
	pool   = make(map[string]string, 1024)
)

// String returns a canonical shared instance of string s.
// If s is empty or longer than 256 bytes, it is returned directly to avoid pool bloat.
func String(s string) string {
	if s == "" || len(s) > 256 {
		return s
	}

	poolMu.RLock()
	interned, ok := pool[s]
	poolMu.RUnlock()
	if ok {
		return interned
	}

	poolMu.Lock()
	defer poolMu.Unlock()
	if interned, ok = pool[s]; ok {
		return interned
	}
	pool[s] = s
	return s
}
