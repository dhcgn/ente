package watcher

import (
	"sync"
	"time"
)

// DebounceQueue handles debouncing of file events to detect when writes are complete
type DebounceQueue struct {
	entries  map[string]*DebounceEntry
	duration time.Duration
	mu       sync.Mutex
}

// DebounceEntry tracks a single file's debounce state
type DebounceEntry struct {
	FilePath  string
	LastWrite time.Time
	Timer     *time.Timer
}

// NewDebounceQueue creates a new DebounceQueue with the specified debounce duration
func NewDebounceQueue(duration time.Duration) *DebounceQueue {
	return &DebounceQueue{
		entries:  make(map[string]*DebounceEntry),
		duration: duration,
	}
}

// Add adds a file to the debounce queue or resets its timer if already present
// The callback will be invoked after the debounce duration with no new events
func (d *DebounceQueue) Add(filePath string, callback func(string)) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Cancel existing timer if present
	if entry, exists := d.entries[filePath]; exists {
		entry.Timer.Stop()
		delete(d.entries, filePath)
	}

	// Create new timer that will fire after the debounce duration
	timer := time.AfterFunc(d.duration, func() {
		d.mu.Lock()
		delete(d.entries, filePath)
		d.mu.Unlock()

		// Call the callback after removing from map
		callback(filePath)
	})

	// Store the entry
	d.entries[filePath] = &DebounceEntry{
		FilePath:  filePath,
		LastWrite: time.Now(),
		Timer:     timer,
	}
}

// Stop cancels all pending timers and clears the queue
func (d *DebounceQueue) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, entry := range d.entries {
		entry.Timer.Stop()
	}
	d.entries = make(map[string]*DebounceEntry)
}

// Pending returns the number of files currently in the debounce queue
func (d *DebounceQueue) Pending() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.entries)
}
