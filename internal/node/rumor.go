package node

import "sync"

// rumor holds the datastructure used to work on a rumor
type rumor struct {
	sync.Mutex
	counter map[string]int
	trusted map[string]bool
}

// Add adds a new or existing rumor to the datastructure. It returns the number of registered rumors
func (r *rumor) Add(rumor string) int {
	r.Lock()
	defer r.Unlock()

	v, ok := r.counter[rumor]
	if ok {
		r.counter[rumor] = v + 1
	} else {
		r.counter[rumor] = 1
	}

	return r.counter[rumor]
}

func (r *rumor) Trusted(rumor string) {
	r.Lock()
	defer r.Unlock()
	r.trusted[rumor] = true
}
