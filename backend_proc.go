package freedom

import (
	"bufio"
	"encoding/json"
	"errors"
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
	command []string // 启动 argv，重启策略据此重建子进程
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	mu      sync.Mutex
	writeMu sync.Mutex // 串行化 stdin 写入；与 mu 分离，避免大消息阻塞写管道时与 readLoop/Close 形成环形死锁
	pending map[int64]chan procResp
	nextID  int64
	closed  bool
	onEvent func(event string, data interface{})
	timeout time.Duration
	maxLine int // 单条 stdout 行（一次响应）上限字节；超限即那条响应过大，不得中断整个通道

	// —— W4 崩溃自愈（对标 Tauri shell spawn 的失败处理 / sidecar 重启）——
	rp        RestartPolicy   // 重启策略（零值=不重启，保持旧行为）
	gen       int             // 每次拉起递增；readLoop 据此识别"自己那一期"，过期实例不触发重启
	pdone     chan struct{}   // 当前这期进程 Wait 完成后由 readLoop 关闭（Close 等它，不再自己 Wait，避免双重 Wait）
	attempts  int             // 连续崩溃重启计数
	startedAt time.Time       // 本期进程启动时刻（存活够久则重置 attempts）
}

// RestartPolicy 是后端进程崩溃后的自动重启策略（经 SetRestartPolicy 启用）。
// 非零退出/被信号杀视为崩溃；退出码 0 视为后端主动结束，不重启。
type RestartPolicy struct {
	// MaxRetries 是"连续崩溃"允许的最大重启次数；0 表示禁用重启。
	MaxRetries int
	// Backoff 是首次重启前的等待时间，其后按 2 倍指数退避，封顶 30s。
	// <=0 时取默认 250ms。
	Backoff time.Duration
	// ResetAfter 是进程存活超过该时长后清零连续崩溃计数（默认 60s），
	// 让"运行几小时后崩一次"不被历史崩溃拖入重启耗尽。
	ResetAfter time.Duration
}

const (
	defaultRestartBackoff    = 250 * time.Millisecond
	defaultRestartResetAfter = 60 * time.Second
	maxRestartBackoff        = 30 * time.Second
)

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
	return &ProcBackend{
		command: command,
		pending: map[int64]chan procResp{},
		timeout: 60 * time.Second,
		maxLine: 16 * 1024 * 1024, // 默认单条响应上限 16MB（见 SetMaxLine）
	}
}

// SetRestartPolicy 启用后端进程崩溃自动重启（默认关闭）。
// 崩溃时向壳广播事件 "backend.crashed" {code, attempt, restarting}，
// 重启成功拉起后广播 "backend.restarted" {attempt}。
func (p *ProcBackend) SetRestartPolicy(rp RestartPolicy) *ProcBackend {
	if rp.Backoff <= 0 {
		rp.Backoff = defaultRestartBackoff
	}
	if rp.ResetAfter <= 0 {
		rp.ResetAfter = defaultRestartResetAfter
	}
	p.mu.Lock()
	p.rp = rp
	p.mu.Unlock()
	return p
}

// SetTimeout 设置单次调用的最大等待时间（默认 60s）。<=0 表示不超时。
func (p *ProcBackend) SetTimeout(d time.Duration) *ProcBackend {
	p.timeout = d
	return p
}

// SetMaxLine 设置单条后端响应（stdout 行）的字节上限（默认 16MB）。
// 超过上限的单条响应会被判为"行过大"：仅该条调用收到错误，随后继续读取后续行，
// 不会像旧实现那样把 bufio.ErrTooLong 误判为"进程退出"而打崩整个 IPC 通道。
func (p *ProcBackend) SetMaxLine(n int) *ProcBackend {
	if n > 0 {
		p.maxLine = n
	}
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
	if p.closed {
		// closed 后再 start 会拉起一个永远无人 Wait/Kill 的孤儿子进程，必须拒绝。
		return fmt.Errorf("freedom: proc backend already closed")
	}
	if p.stdin != nil {
		return nil // 已启动
	}
	return p.launchLocked()
}

// launchLocked 按 command 重建子进程并启动 readLoop。崩溃重启与首次启动共用。
// 调用方须持有 p.mu。
func (p *ProcBackend) launchLocked() error {
	cmd := exec.Command(p.command[0], p.command[1:]...)
	hideWindow(cmd) // Windows 下隐藏后端进程的 cmd 黑窗（跨平台空实现）
	cmd.Env = append(os.Environ(), "FREEDOM_BACKEND=1", "FREEDOM_IPC=stdio")
	cmd.Stderr = os.Stderr // 后端 stderr 日志原样转发
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("freedom: proc backend stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("freedom: proc backend stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("freedom: proc backend start %q: %w", cmd.Path, err)
	}
	p.cmd = cmd
	p.stdin = stdin
	p.gen++
	p.pdone = make(chan struct{})
	p.startedAt = time.Now()
	go p.readLoop(p.gen, stdout, cmd, p.pdone)
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
	stdin := p.stdin
	p.mu.Unlock()

	// 写管道不持 mu：大消息阻塞在 Write 时，readLoop/Close 仍可拿锁推进，
	// 超时唤醒路径也不会被卡死（写失败由下方 cancel 兜底唤醒）。
	p.writeMu.Lock()
	_, err = stdin.Write(append(line, '\n'))
	p.writeMu.Unlock()

	if err != nil {
		p.cancel(id, fmt.Errorf("freedom: proc backend write: %w", err))
		return nil, err
	}

	var resp procResp
	if p.timeout > 0 {
		// NewTimer 便于超时后 Stop 释放；time.After 的 timer 会滞留到期才回收。
		timer := time.NewTimer(p.timeout)
		defer timer.Stop()
		select {
		case resp = <-ch:
		case <-timer.C:
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
//
// 行切分用 bufio.Reader.ReadBytes（而非 Scanner）：Scanner 的单条 token 超过
// 上限时返回 bufio.ErrTooLong 并永久停止扫描（通道直接死掉，旧缺陷）；
// 这里改为**手工切分**——某条响应超限时仅丢弃该条并唤醒其调用为"行过大"，
// 随后继续读后续行，绝不因单条大消息而中断整个 IPC 通道。
func (p *ProcBackend) readLoop(gen int, stdout io.Reader, cmd *exec.Cmd, done chan struct{}) {
	rd := bufio.NewReaderSize(stdout, 64*1024)
	for {
		line, readErr := rd.ReadBytes('\n')
		if len(line) > 0 {
			// ReadBytes 返回的 line 含尾随 '\n'；去掉换行后按行处理。
			msgBytes := line
			if len(msgBytes) > 0 && msgBytes[len(msgBytes)-1] == '\n' {
				msgBytes = msgBytes[:len(msgBytes)-1]
			}
			p.dispatchLine(msgBytes)
		}
		if readErr != nil {
			if readErr == io.EOF {
				// 正常结束：后端进程关闭了 stdout。
			} else {
				fmt.Fprintf(os.Stderr, "freedom: proc backend: read stdout: %v\n", readErr)
			}
			break
		}
	}
	// 后端进程已退出（或 stdout 不可读）：唤醒所有仍等待中的调用。
	err := fmt.Errorf("freedom: proc backend exited unexpectedly")
	p.mu.Lock()
	for id, ch := range p.pending {
		delete(p.pending, id)
		ch <- procResp{err: err}
	}
	p.mu.Unlock()

	if cmd == nil {
		// 无真实子进程（协议层单测直接注入坏 reader）：唤醒 pending 即使命完成。
		return
	}
	waitErr := cmd.Wait() // stdout 已 EOF，Wait 安全（见 os/exec StdoutPipe 约定）
	if done != nil {
		close(done) // 通知 Close()：本期进程已回收
	}

	p.mu.Lock()
	if p.closed || p.gen != gen {
		// 已 Close 或这是被重启淘汰的过期实例：不做崩溃处理。
		p.mu.Unlock()
		return
	}
	p.stdin = nil
	p.cmd = nil
	if p.rp.MaxRetries <= 0 {
		p.mu.Unlock()
		return
	}
	if waitErr == nil {
		// 退出码 0：后端主动正常结束，不算崩溃、不重启（防退出型后端被无限拉起）。
		p.mu.Unlock()
		p.notifyFrontend("backend.exited", map[string]interface{}{"code": 0})
		return
	}
	if time.Since(p.startedAt) >= p.rp.ResetAfter {
		p.attempts = 0
	}
	p.attempts++
	attempt := p.attempts
	maxR := p.rp.MaxRetries
	backoff := p.rp.Backoff << (attempt - 1)
	if backoff > maxRestartBackoff || backoff <= 0 {
		backoff = maxRestartBackoff
	}
	restarting := attempt <= maxR
	p.mu.Unlock()

	exitCode := -1
	var ee *exec.ExitError
	if errors.As(waitErr, &ee) {
		exitCode = ee.ExitCode()
	}
	p.notifyFrontend("backend.crashed", map[string]interface{}{
		"code": exitCode, "attempt": attempt, "restarting": restarting,
	})
	if !restarting {
		return
	}
	time.AfterFunc(backoff, func() {
		p.mu.Lock()
		if p.closed || p.stdin != nil {
			p.mu.Unlock()
			return
		}
		launchErr := p.launchLocked()
		p.mu.Unlock()
		if launchErr != nil {
			fmt.Fprintf(os.Stderr, "freedom: proc backend restart failed: %v\n", launchErr)
			p.notifyFrontend("backend.crashed", map[string]interface{}{
				"code": exitCode, "attempt": attempt, "restarting": false, "error": launchErr.Error(),
			})
			return
		}
		p.notifyFrontend("backend.restarted", map[string]interface{}{"attempt": attempt})
	})
}

// notifyFrontend 经 onEvent 回调把框架级事件（backend.crashed 等）转发给壳（App.Emit）。
func (p *ProcBackend) notifyFrontend(event string, data interface{}) {
	p.mu.Lock()
	fn := p.onEvent
	p.mu.Unlock()
	if fn != nil {
		fn(event, data)
	}
}

// dispatchLine 处理一条（已去换行的）stdout 行。
// 行数（字节数）超过 maxLine 时不解析、不丢弃后续行——只把"行过大"留痕，
// 让调用方（仍在等待的 pending 由各自的 Handle 超时唤醒）不被这条消息影响。
func (p *ProcBackend) dispatchLine(line []byte) {
	// 空行：协议无意义，跳过。
	if len(line) == 0 {
		return
	}
	// 单条响应字节上限守卫：超限即该行过大。ReadBytes 已把整行读入，这里
	// 只做上限判定——超限行丢弃（不解析、不派发），并留痕供排查。
	// 注意：超限行若恰好是一条调用的响应，该调用的 pending 不会被 finish 唤醒，
	// 将依赖 Handle 侧的超时机制兜底（默认 60s）。这是"行过大仅影响该条"的
	// 有界行为，不会级联打崩其它在途调用。
	if p.maxLine > 0 && len(line) > p.maxLine {
		fmt.Fprintf(os.Stderr, "freedom: proc backend: line too large (%d bytes > maxLine %d), dropped\n",
			len(line), p.maxLine)
		return
	}
	var msg procMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		fmt.Fprintf(os.Stderr, "freedom: proc backend: bad message: %v\n", err)
		return
	}
	if msg.Event != "" {
		p.dispatchEvent(msg.Event, msg.Data)
		return
	}
	p.finish(msg)
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
		if err := json.Unmarshal(dataJSON, &data); err != nil {
			// 非 JSON 事件负载降级为原始字符串，保留信息而非静默丢弃。
			data = string(dataJSON)
		}
	}
	p.mu.Lock()
	fn := p.onEvent
	p.mu.Unlock()
	if fn != nil {
		fn(event, data)
	}
}

// Close 终止后端进程。先关闭 stdin 通知其优雅退出，超时则强杀。线程安全。
// 进程回收（Wait）统一由 readLoop 负责，Close 只等它的 done 信号，避免双重 Wait。
func (p *ProcBackend) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	cmd := p.cmd
	stdin := p.stdin
	done := p.pdone
	if stdin != nil {
		if err := stdin.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			fmt.Fprintf(os.Stderr, "freedom: close backend stdin: %v\n", err)
		}
	}
	p.mu.Unlock()

	if done == nil {
		return nil // 从未成功启动过，没有子进程需要回收
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		if cmd != nil && cmd.Process != nil {
			if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				fmt.Fprintf(os.Stderr, "freedom: backend kill: %v\n", err)
			}
			// Kill 也可能失败（权限等），等待必须有上限，否则 Close 永久阻塞。
			select {
			case <-done:
			case <-time.After(time.Second):
				fmt.Fprintf(os.Stderr, "freedom: backend: process did not exit after kill\n")
			}
		}
	}
	return nil
}
