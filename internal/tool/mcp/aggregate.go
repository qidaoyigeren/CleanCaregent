package mcp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"CleanCaregent/internal/tool"
)

type NamedClient struct {
	Name   string
	Client tool.Client
}

type AggregateClient struct {
	clients []NamedClient

	mu      sync.RWMutex
	targets map[string]aggregateTarget
}

type aggregateTarget struct {
	client tool.Client
	name   string
}

func NewAggregateClient(clients []NamedClient) (*AggregateClient, error) {
	if len(clients) == 0 {
		return nil, errors.New("mcp aggregate client requires at least one server")
	}
	seen := make(map[string]struct{}, len(clients))
	cleaned := make([]NamedClient, 0, len(clients))
	for _, value := range clients {
		name := strings.TrimSpace(value.Name)
		if name == "" {
			return nil, errors.New("mcp aggregate server name is required")
		}
		if strings.Contains(name, "/") {
			return nil, fmt.Errorf("mcp aggregate server name %q must not contain /", name)
		}
		if value.Client == nil {
			return nil, fmt.Errorf("mcp aggregate server %q has no client", name)
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("mcp aggregate server name %q is duplicated", name)
		}
		seen[name] = struct{}{}
		cleaned = append(cleaned, NamedClient{Name: name, Client: value.Client})
	}
	return &AggregateClient{clients: cleaned, targets: make(map[string]aggregateTarget)}, nil
}

func (c *AggregateClient) ListTools(ctx context.Context) ([]tool.Definition, error) {
	if c == nil {
		return nil, errors.New("mcp aggregate client is nil")
	}
	result := make([]tool.Definition, 0)
	targets := make(map[string]aggregateTarget)
	shortTargets := make(map[string]aggregateTarget)
	prefix := len(c.clients) > 1
	for _, entry := range c.clients {
		definitions, err := entry.Client.ListTools(ctx)
		if err != nil {
			return nil, fmt.Errorf("mcp aggregate tools/list %s: %w", entry.Name, err)
		}
		for _, definition := range definitions {
			remoteName := definition.Name
			alias := remoteName
			if prefix {
				alias = entry.Name + "/" + remoteName
			}
			if _, exists := targets[alias]; exists {
				return nil, fmt.Errorf("mcp aggregate tool alias %q is duplicated", alias)
			}
			definition.Name = alias
			result = append(result, definition)
			target := aggregateTarget{client: entry.Client, name: remoteName}
			targets[alias] = target
			if _, exists := shortTargets[remoteName]; !exists {
				shortTargets[remoteName] = target
			}
		}
	}
	if prefix {
		for name, target := range shortTargets {
			if _, exists := targets[name]; !exists {
				targets[name] = target
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	c.mu.Lock()
	c.targets = targets
	c.mu.Unlock()
	return result, nil
}

func (c *AggregateClient) CallTool(ctx context.Context, call tool.Call) (tool.Result, error) {
	if c == nil {
		return tool.Result{CallID: call.CallID}, errors.New("mcp aggregate client is nil")
	}
	target, ok := c.lookup(call.Name)
	if !ok {
		if _, err := c.ListTools(ctx); err != nil {
			return tool.Result{CallID: call.CallID}, err
		}
		target, ok = c.lookup(call.Name)
	}
	if !ok {
		return tool.Result{CallID: call.CallID, ErrorCode: "TOOL_NOT_FOUND"}, fmt.Errorf("mcp aggregate tool %q not found", call.Name)
	}
	remoteCall := call
	remoteCall.Name = target.name
	return target.client.CallTool(ctx, remoteCall)
}

func (c *AggregateClient) Close() error {
	if c == nil {
		return nil
	}
	var closeErr error
	for _, entry := range c.clients {
		closer, ok := entry.Client.(interface{ Close() error })
		if !ok {
			continue
		}
		if err := closer.Close(); err != nil && closeErr == nil {
			closeErr = fmt.Errorf("close mcp aggregate server %s: %w", entry.Name, err)
		}
	}
	return closeErr
}

func (c *AggregateClient) lookup(name string) (aggregateTarget, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	target, ok := c.targets[name]
	return target, ok
}
