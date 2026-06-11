package tool

import (
	"errors"
	"sort"
	"sync"
)

var ErrToolAlreadyRegistered = errors.New("tool already registered")

type MemoryRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *MemoryRegistry {
	return &MemoryRegistry{tools: make(map[string]Tool)}
}

func (r *MemoryRegistry) Register(value Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[value.Name()]; exists {
		return ErrToolAlreadyRegistered
	}
	r.tools[value.Name()] = value
	return nil
}

func (r *MemoryRegistry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.tools[name]
	return value, ok
}

func (r *MemoryRegistry) ListAllowed(names []string) []Definition {
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		allowed[name] = struct{}{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Definition, 0, len(names))
	for name, value := range r.tools {
		if _, ok := allowed[name]; !ok {
			continue
		}
		result = append(result, Definition{
			Name:         name,
			Description:  value.Description(),
			ParamsSchema: value.ParamsSchema(),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
