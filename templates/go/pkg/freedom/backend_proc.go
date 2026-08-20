package freedom

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// ProcBackend 使用任意语言实现的独立后端进程。
//
// 通信协议（语言无关，换行分隔 JSON / NDJSON，走 stdin/stdout）：
//
//	壳 -> 后端（写 stdin）：
//	    {"id":1,"method":"Greet","params":["老板"]}
//	后端 -> 壳（写 stdout）：
//	    {"id":1,"result":{...}}         调用成功响应（result 可为任意 JSON 或 null）
//	    {"id":1,"error":"boom"}         调用失败响应
//	    {"event":"tick","data":{...}}   主动事件推送（无 id 字段）
//
// 后端的 stderr 仅用于人类日志，壳原样转发到控制台，不参与协议。
//
// 壳启动后端进程时注入环境变量：FREEDOM_BACKEND=1、FREEDOM_IPC=stdio，
// 后端可据此判断自己运行在 Freedom 壳内。
type ProcBackend struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	mu      sync.Mutex
	pending map[int64]chan procResp
	nextID  int64
	closed  bool
	onEvent func(event string, data interface{})
	timeout time.Duration
}

// procResp 是一次调用在壳侧的等待结果。
type procResp struct {
	result json.RawMessage
	err    error
}

// procMessage 是协议的统一消息结构。
type procMessage struct {
	ID     int64           `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Event  string          `json:"event,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// NewProcBackend 创建一个进程后端。command 是任意语言后端可执行文件的启动命令
// （如 "node"、"python"、"./backend.py" 或可执行文件绝对路径），后续参数原样透传。
// 返回的 ProcBackend 在 App.Run 启动窗口时自动拉起后端进程。
func NewProcBackend(command ...string) *ProcBackend {
	if len(command) == 0 {
		panic("freedom: NewProcBackend requires at least one command argument")
	}
	cmd := exec.Command(command[0], command[1:]...)
	hideWindow(cmd) // Windows 下隐藏后端的 cmd 黑窗（跨平台空实现）
	return &ProcBackend{
		cmd:     cmd,
		pending: map[int64]chan procResp{},
		timeout: 60 * time.Second,
	}
}

// SetDir 设置后端进程的工作目录。
// 相对路径参数（如 "backend/main.mjs"）将相对该目录解析。
// 通用壳在加载 resources/config.json 时自动设为 resources 目录。
func (p *ProcBackend) SetDir(dir string) *ProcBackend {
	if dir != "" {
		p.cmd.Dir = dir
	}
	return p
}

// SetTimeout 设置单次调用的最大等待时间（默认 60s）。<=0 表示不超时。
func (p *ProcBackend) SetTimeout(d time.Duration) *ProcBackend {
	p.timeout = d
	return p
}

// OnEvent 注册后端进程推送事件时的回调（由框架在 Run 时注入）。
func (p *ProcBackend) OnEvent(fn func(event string, data interface{})) {
	p.mu.Lock()
	p.onEvent = fn
	p.mu.Unlock()
}

// start 启动后端进程并开始读取其 stdout。幂等，可从任意 goroutine 调用。
func (p *ProcBackend) start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stdin != nil {
		return nil // 已启动
	}
	p.cmd.Env = append(os.Environ(), "FREEDOM_BACKEND=1", "FREEDOM_IPC=stdio")
	p.cmd.Stderr = os.Stderr // 后端 stderr 日志原样转发
	stdin, err := p.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("freedom: proc backend stdin pipe: %w", err)
	}
	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("freedom: proc backend stdout pipe: %w", err)
	}
	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("freedom: proc backend start %q: %w", p.cmd.Path, err)
	}
	p.stdin = stdin
	go p.readLoop(stdout)
	return nil
}

// Handle 实现 Backend 接口：把一次前端调用转发给后端进程并等待其响应。
func (p *ProcBackend) Handle(method string, params []json.RawMessage) (interface{}, error) {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("freedom: proc backend: marshal params: %w", err)
	}

	p.mu.Lock()
	if p.closed || p.stdin == nil {
		p.mu.Unlock()
		return nil, fmt.Errorf("freedom: proc backend is not running")
	}
	p.nextID++
	id := p.nextID
	ch := make(chan procResp, 1)
	p.pending[id] = ch
	msg := procMessage{ID: id, Method: method, Params: paramsJSON}
	line, _ := json.Marshal(msg)
	_, err = p.stdin.Write(append(line, '\n'))
	p.mu.Unlock()

	if err != nil {
		p.cancel(id, fmt.Errorf("freedom: proc backend write: %w", err))
		return nil, err
	}

	var resp procResp
	if p.timeout > 0 {
		select {
		case resp = <-ch:
		case <-time.After(p.timeout):
			p.cancel(id, fmt.Errorf("freedom: proc backend: method %q timed out after %s", method, p.timeout))
			return nil, fmt.Errorf("freedom: proc backend: method %q timed out after %s", method, p.timeout)
		}
	} else {
		resp = <-ch
	}
	if resp.err != nil {
		return nil, resp.err
	}
	if resp.result == nil {
		return nil, nil
	}
	var out interface{}
	if err := json.Unmarshal(resp.result, &out); err != nil {
		return nil, fmt.Errorf("freedom: proc backend: bad result: %w", err)
	}
	return out, nil
}

// cancel 移除并唤醒一个等待中的调用（超时 / 写失败 / 进程退出）。
func (p *ProcBackend) cancel(id int64, err error) {
	p.mu.Lock()
	if ch, ok := p.pending[id]; ok {
		delete(p.pending, id)
		ch <- procResp{err: err}
	}
	p.mu.Unlock()
}

// readLoop 持续读取后端 stdout，解析协议消息并分发。
func (p *ProcBackend) readLoop(stdout io.Reader) {
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var msg procMessage
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			fmt.Fprintf(os.Stderr, "freedom: proc backend: bad message: %v\n", err)
			continue
		}
		if msg.Event != "" {
			p.dispatchEvent(msg.Event, msg.Data)
			continue
		}
		p.finish(msg)
	}
	// 后端进程已退出：唤醒所有仍等待中的调用。
	err := fmt.Errorf("freedom: proc backend exited unexpectedly")
	p.mu.Lock()
	for id, ch := range p.pending {
		delete(p.pending, id)
		ch <- procResp{err: err}
	}
	p.mu.Unlock()
}

func (p *ProcBackend) finish(msg procMessage) {
	p.mu.Lock()
	ch, ok := p.pending[msg.ID]
	delete(p.pending, msg.ID)
	p.mu.Unlock()
	if !ok {
		return
	}
	if msg.Error != "" {
		ch <- procResp{err: fmt.Errorf("%s", msg.Error)}
		return
	}
	ch <- procResp{result: msg.Result}
}

func (p *ProcBackend) dispatchEvent(event string, dataJSON json.RawMessage) {
	var data interface{}
	if len(dataJSON) > 0 {
		_ = json.Unmarshal(dataJSON, &data)
	}
	p.mu.Lock()
	fn := p.onEvent
	p.mu.Unlock()
	if fn != nil {
		fn(event, data)
	}
}

// Close 终止后端进程。先关闭 stdin 通知其优雅退出，超时则强杀。线程安全。
func (p *ProcBackend) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	cmd := p.cmd
	if p.stdin != nil {
		_ = p.stdin.Close() // stdin EOF，后端可自行退出
	}
	p.mu.Unlock()

	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			<-done
		}
	}
	return nil
}
