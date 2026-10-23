package fontcatalog

import "sync"

var defaultCatalog struct {
	sync.RWMutex
	snapshot Snapshot
}

// SetDefault publishes the live application's current catalog for new headless
// render tasks. Existing tasks retain their previously acquired snapshot.
func SetDefault(s Snapshot) {
	defaultCatalog.Lock()
	defaultCatalog.snapshot = s
	defaultCatalog.Unlock()
}
func Default() Snapshot {
	defaultCatalog.RLock()
	defer defaultCatalog.RUnlock()
	return defaultCatalog.snapshot
}
