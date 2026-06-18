package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"CleanCaregent/internal/tool"
)

type StdioClientConfig struct {
	Command        string
	Args           []string
	Env            map[string]string
	Timeout        time.Duration
	MaxRestarts    int
	RestartBackoff time.Duration
}

type StdioClient struct {
	command        string
	args           []string
	env            map[string]string
	timeout        time.Duration
	maxRestarts    int
	restartBackoff time.Duration
	nextID         atomic.Uint64

	mu          sync.Mutex
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	stdout      *bufio.Scanner
	initialized bool
	closed      bool
}

func NewStdioClient(config StdioClientConfig) (*StdioClient, error) {
	command := strings.TrimSpace(config.Command)
	if command == "" {
		return nil, errors.New("mcp stdio command is required")
	}
	if config.Timeout <= 0 {
		config.Timeout = 5 * time.Second
	}
	if config.MaxRestarts < 0 {
		config.MaxRestarts = 0
	}
	if config.RestartBackoff <= 0 {
		config.RestartBackoff = 100 * time.Millisecond
	}
	env := make(map[string]string, len(config.Env))
	for key, value := range config.Env {
		key = strings.TrimSpace(key)
		if key != "" {
			env[key] = value
		}
	}
	return &StdioClient{
		command:        command,
		args:           append([]string(nil), config.Args...),
		env:            env,
		timeout:        config.Timeout,
		maxRestarts:    config.MaxRestarts,
		restartBackoff: config.RestartBackoff,
	}, nil
}

func (c *StdioClient) ListTools(ctx context.Context) ([]tool.Definition, error) {
	var response ListToolsResult
	if err := c.call(ctx, "tools/list", map[string]any{}, &response); err != nil {
		return nil, fmt.Errorf("mcp tools/list stdio: %w", err)
	}
	return definitionsFromMCPTools(response.Tools), nil
}

func (c *StdioClient) CallTool(ctx context.Context, call tool.Call) (tool.Result, error) {
	params := CallToolParams{
		Name:      call.Name,
		Arguments: call.Arguments,
		Meta: CallMeta{
			TraceID:        call.TraceID,
			CallID:         call.CallID,
			UserID:         call.UserID,
			ConversationID: call.ConversationID,
			IdempotencyKey: call.IdempotencyKey,
		},
	}
	var response CallToolResult
	if err := c.call(ctx, "tools/call", params, &response); err != nil {
		return tool.Result{CallID: call.CallID}, fmt.Errorf("mcp tools/call stdio %s: %w", call.Name, err)
	}
	result := tool.Result{
		CallID:     call.CallID,
		Data:       resultData(response),
		DataScope:  response.DataScope,
		ErrorCode:  response.ErrorCode,
		Message:    response.Message,
		StartedAt:  response.StartedAt,
		FinishedAt: response.FinishedAt,
	}
	if response.IsError {
		if result.Message == "" {
			result.Message = "mcp tool call failed"
		}
		return result, fmt.Errorf("mcp tools/call %s: %s", call.Name, result.Message)
	}
	return result, nil
}

func (c *StdioClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return c.stopLocked()
}

func (c *StdioClient) call(ctx context.Context, method string, params any, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("mcp stdio client is closed")
	}
	if err := c.ensureProcessLocked(ctx); err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt <= c.maxRestarts; attempt++ {
		err := c.callLocked(ctx, method, params, result)
		if err == nil {
			return nil
		}
		lastErr = err
		_ = c.stopLocked()
		c.initialized = false
		if attempt == c.maxRestarts {
			break
		}
		timer := time.NewTimer(c.restartBackoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if err := c.ensureProcessLocked(ctx); err != nil {
			lastErr = err
			break
		}
	}
	return lastErr
}

func (c *StdioClient) ensureProcessLocked(ctx context.Context) error {
	if c.cmd != nil && c.stdin != nil && c.stdout != nil && c.initialized {
		return nil
	}
	if c.cmd == nil || c.stdin == nil || c.stdout == nil {
		if err := c.startLocked(); err != nil {
			return err
		}
	}
	var response InitializeResult
	if err := c.callLocked(ctx, "initialize", InitializeParams{
		ProtocolVersion: ProtocolVersion,
		Capabilities:    ClientCapabilities{},
		ClientInfo: ImplementationInfo{
			Name:        "cleancare-agent",
			Title:       "CleanCare Agent",
			Version:     "1.0.0",
			Description: "CleanCare Agent MCP stdio client",
		},
	}, &response); err != nil {
		_ = c.stopLocked()
		return fmt.Errorf("mcp initialize stdio: %w", err)
	}
	if err := c.notifyLocked(ctx, "notifications/initialized", nil); err != nil {
		_ = c.stopLocked()
		return fmt.Errorf("mcp initialized notification stdio: %w", err)
	}
	c.initialized = true
	return nil
}

func (c *StdioClient) startLocked() error {
	cmd := exec.Command(c.command, c.args...)
	cmd.Env = os.Environ()
	for key, value := range c.env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("open mcp stdio stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("open mcp stdio stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("start mcp stdio command: %w", err)
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), maxRPCBodyBytes)
	c.cmd = cmd
	c.stdin = stdin
	c.stdout = scanner
	return nil
}

func (c *StdioClient) stopLocked() error {
	var err error
	if c.stdin != nil {
		err = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
	}
	c.cmd = nil
	c.stdin = nil
	c.stdout = nil
	return err
}

func (c *StdioClient) callLocked(ctx context.Context, method string, params any, result any) error {
	requestID := c.nextID.Add(1)
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode mcp params: %w", err)
	}
	request := rpcRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(fmt.Sprintf("%d", requestID)),
		Method:  method,
		Params:  paramsRaw,
	}
	if err := c.writeMessageLocked(request); err != nil {
		return err
	}
	deadline := time.NewTimer(c.timeout)
	defer deadline.Stop()
	for {
		response, err := c.readResponseLocked(ctx, deadline.C)
		if err != nil {
			return err
		}
		if !bytes.Equal(bytes.TrimSpace(response.ID), bytes.TrimSpace(request.ID)) {
			continue
		}
		if response.Error != nil {
			return response.Error
		}
		if len(response.Result) == 0 {
			return errors.New("mcp stdio response result is empty")
		}
		if err := json.Unmarshal(response.Result, result); err != nil {
			return fmt.Errorf("decode mcp stdio response result: %w", err)
		}
		return nil
	}
}

func (c *StdioClient) notifyLocked(_ context.Context, method string, params any) error {
	request := rpcRequest{
		JSONRPC: "2.0",
		Method:  method,
	}
	if params != nil {
		request.Params = mustRaw(params)
	}
	return c.writeMessageLocked(request)
}

func (c *StdioClient) writeMessageLocked(message any) error {
	raw, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode mcp stdio message: %w", err)
	}
	raw = append(raw, '\n')
	if _, err := c.stdin.Write(raw); err != nil {
		return fmt.Errorf("write mcp stdio message: %w", err)
	}
	return nil
}

func (c *StdioClient) readResponseLocked(ctx context.Context, deadline <-chan time.Time) (rpcResponse, error) {
	resultCh := make(chan struct {
		response rpcResponse
		err      error
	}, 1)
	go func() {
		if !c.stdout.Scan() {
			if err := c.stdout.Err(); err != nil {
				resultCh <- struct {
					response rpcResponse
					err      error
				}{err: err}
				return
			}
			resultCh <- struct {
				response rpcResponse
				err      error
			}{err: io.EOF}
			return
		}
		var response rpcResponse
		if err := json.Unmarshal(c.stdout.Bytes(), &response); err != nil {
			resultCh <- struct {
				response rpcResponse
				err      error
			}{err: fmt.Errorf("decode mcp stdio response: %w", err)}
			return
		}
		resultCh <- struct {
			response rpcResponse
			err      error
		}{response: response}
	}()
	select {
	case <-ctx.Done():
		return rpcResponse{}, ctx.Err()
	case <-deadline:
		return rpcResponse{}, context.DeadlineExceeded
	case result := <-resultCh:
		return result.response, result.err
	}
}

func ServeStdio(ctx context.Context, server *Server, input io.Reader, output io.Writer) error {
	if server == nil {
		return errors.New("mcp stdio server is nil")
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 64*1024), maxRPCBodyBytes)
	initializeAccepted := false
	initialized := false
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var request rpcRequest
		if err := json.Unmarshal(line, &request); err != nil {
			writeStdioResponse(output, rpcResponse{
				JSONRPC: "2.0",
				Error:   &RPCError{Code: -32700, Message: "parse JSON-RPC request: " + err.Error()},
			})
			continue
		}
		if request.JSONRPC != "2.0" || strings.TrimSpace(request.Method) == "" {
			writeStdioResponse(output, rpcResponse{
				JSONRPC: "2.0",
				ID:      request.ID,
				Error:   &RPCError{Code: -32600, Message: "invalid JSON-RPC request"},
			})
			continue
		}
		if len(request.ID) == 0 {
			if request.Method == "notifications/initialized" {
				if initializeAccepted {
					initialized = true
				}
			}
			_ = server.HandleNotification(ctx, request.Method, request.Params)
			continue
		}
		if request.Method != "initialize" && request.Method != "ping" && !initialized {
			writeStdioResponse(output, rpcResponse{
				JSONRPC: "2.0",
				ID:      request.ID,
				Error:   &RPCError{Code: -32004, Message: "mcp session is not initialized"},
			})
			continue
		}
		result, rpcErr := server.HandleRequest(ctx, request.Method, request.Params)
		if request.Method == "initialize" && rpcErr == nil {
			initializeAccepted = true
			initialized = false
		}
		response := rpcResponse{JSONRPC: "2.0", ID: request.ID}
		if rpcErr != nil {
			response.Error = rpcErr
		} else {
			response.Result = mustRaw(result)
		}
		writeStdioResponse(output, response)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read mcp stdio input: %w", err)
	}
	return nil
}

func writeStdioResponse(output io.Writer, response rpcResponse) {
	raw, err := json.Marshal(response)
	if err != nil {
		return
	}
	_, _ = output.Write(append(raw, '\n'))
}
