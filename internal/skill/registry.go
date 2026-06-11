package skill

import (
	"errors"
	"sort"
	"sync"

	"CleanCaregent/internal/intent"
)

var ErrSkillAlreadyRegistered = errors.New("skill already registered")

type Registry struct {
	mu     sync.RWMutex
	skills map[string]Skill
}

func NewRegistry() *Registry {
	return &Registry{skills: make(map[string]Skill)}
}

func (r *Registry) Register(value Skill) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.skills[value.Name()]; exists {
		return ErrSkillAlreadyRegistered
	}
	r.skills[value.Name()] = value
	return nil
}

func (r *Registry) Get(name string) (Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.skills[name]
	return value, ok
}

func (r *Registry) Find(intentType intent.Type) (Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.skills))
	for name := range r.skills {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if r.skills[name].CanHandle(intentType) {
			return r.skills[name], true
		}
	}
	return nil, false
}
