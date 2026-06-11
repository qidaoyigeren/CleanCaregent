// Package prompt provides a versioned template management system for LLM prompts.
// Templates are organized by scenario (intent, rewrite, plan, generate, reflect, clarify)
// and support placeholder substitution with {key} syntax.
package prompt

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Scenario identifies the purpose of a prompt template.
type Scenario string

const (
	ScenarioSystem           Scenario = "system"
	ScenarioIntent           Scenario = "intent"
	ScenarioRewrite          Scenario = "rewrite"
	ScenarioPlan             Scenario = "plan"
	ScenarioGenerateGeneric  Scenario = "generate_generic"
	ScenarioGenerateCompare  Scenario = "generate_compare"
	ScenarioGenerateDiagnose Scenario = "generate_diagnose"
	ScenarioGeneratePolicy   Scenario = "generate_policy"
	ScenarioReflect          Scenario = "reflect"
	ScenarioClarify          Scenario = "clarify"
	ScenarioSummarize        Scenario = "summarize"
	ScenarioEvalJudge        Scenario = "eval_judge"
)

// Template holds a single prompt template with version tracking.
type Template struct {
	Scenario Scenario `json:"scenario"`
	Version  string   `json:"version"`
	System   string   `json:"system"`
	User     string   `json:"user"`
}

// Render substitutes {key} placeholders with values from params.
// Missing keys are left as-is to allow partial rendering.
func (t *Template) Render(params map[string]string) (system, user string) {
	system = t.System
	user = t.User
	for key, value := range params {
		placeholder := "{" + key + "}"
		system = strings.ReplaceAll(system, placeholder, value)
		user = strings.ReplaceAll(user, placeholder, value)
	}
	return system, user
}

// RenderSystemOnly returns only the system prompt with substitutions.
func (t *Template) RenderSystemOnly(params map[string]string) string {
	system, _ := t.Render(params)
	return system
}

// RenderUserOnly returns only the user prompt with substitutions.
func (t *Template) RenderUserOnly(params map[string]string) string {
	_, user := t.Render(params)
	return user
}

// Registry manages a versioned collection of prompt templates.
// It is safe for concurrent use.
type Registry struct {
	mu        sync.RWMutex
	templates map[Scenario]*Template
	versions  map[Scenario]string // scenario -> current version
	history   map[Scenario]map[string]*Template
}

// NewRegistry creates a Registry pre-populated with the default templates.
func NewRegistry() *Registry {
	r := &Registry{
		templates: make(map[Scenario]*Template),
		versions:  make(map[Scenario]string),
		history:   make(map[Scenario]map[string]*Template),
	}
	r.loadDefaults()
	return r
}

// Get retrieves the current version of a template for the given scenario.
func (r *Registry) Get(scenario Scenario) (*Template, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tmpl, ok := r.templates[scenario]
	if !ok {
		return nil, fmt.Errorf("prompt: no template for scenario %q", scenario)
	}
	return cloneTemplate(tmpl), nil
}

// Set installs a new template version, replacing the current one for its scenario.
func (r *Registry) Set(tmpl *Template) {
	if tmpl == nil || tmpl.Scenario == "" || strings.TrimSpace(tmpl.Version) == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.storeLocked(tmpl)
	r.templates[tmpl.Scenario] = cloneTemplate(tmpl)
	r.versions[tmpl.Scenario] = tmpl.Version
}

// GetVersion retrieves a specific version without changing the active version.
func (r *Registry) GetVersion(scenario Scenario, version string) (*Template, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions := r.history[scenario]
	tmpl, ok := versions[version]
	if !ok {
		return nil, fmt.Errorf("prompt: no template for scenario %q version %q", scenario, version)
	}
	return cloneTemplate(tmpl), nil
}

// Activate switches a scenario to a previously registered version.
func (r *Registry) Activate(scenario Scenario, version string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	versions := r.history[scenario]
	tmpl, ok := versions[version]
	if !ok {
		return fmt.Errorf("prompt: no template for scenario %q version %q", scenario, version)
	}
	r.templates[scenario] = cloneTemplate(tmpl)
	r.versions[scenario] = version
	return nil
}

// Versions returns all registered versions for a scenario.
func (r *Registry) Versions(scenario Scenario) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions := r.history[scenario]
	result := make([]string, 0, len(versions))
	for version := range versions {
		result = append(result, version)
	}
	sort.Strings(result)
	return result
}

// Version returns the current version string for a scenario.
func (r *Registry) Version(scenario Scenario) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.versions[scenario]
}

// MustGet is like Get but panics if the template is not found.
// Only use during initialization.
func (r *Registry) MustGet(scenario Scenario) *Template {
	tmpl, err := r.Get(scenario)
	if err != nil {
		panic(err)
	}
	return tmpl
}

// BuildMessages constructs an OpenAI-compatible messages slice from a template and parameters.
func (t *Template) BuildMessages(params map[string]string) []map[string]string {
	system, user := t.Render(params)
	msgs := make([]map[string]string, 0, 2)
	if strings.TrimSpace(system) != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": system})
	}
	if strings.TrimSpace(user) != "" {
		msgs = append(msgs, map[string]string{"role": "user", "content": user})
	}
	return msgs
}

func (r *Registry) storeLocked(tmpl *Template) {
	if r.history[tmpl.Scenario] == nil {
		r.history[tmpl.Scenario] = make(map[string]*Template)
	}
	r.history[tmpl.Scenario][tmpl.Version] = cloneTemplate(tmpl)
}

func cloneTemplate(tmpl *Template) *Template {
	if tmpl == nil {
		return nil
	}
	cloned := *tmpl
	return &cloned
}
