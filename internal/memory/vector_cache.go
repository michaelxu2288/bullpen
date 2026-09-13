package memory

import "sync"

type VectorCache struct {
	mu    sync.RWMutex
	items map[string][]float64
}

func NewVectorCache() *VectorCache {
	return &VectorCache{items: map[string][]float64{}}
}

func (v *VectorCache) Put(key string, vector []float64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	dup := make([]float64, len(vector))
	copy(dup, vector)
	v.items[key] = dup
}

func (v *VectorCache) Get(key string) ([]float64, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	val, ok := v.items[key]
	if !ok {
		return nil, false
	}
	dup := make([]float64, len(val))
	copy(dup, val)
	return dup, true
}
