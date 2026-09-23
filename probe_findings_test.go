package freedom

// 探针测试：驱动"全量找问题"任务锁定的 4 个嫌疑设计缺陷。
// 这不是回归测试（不锁定预期），而是复现器：
// 每个用例刻意构造触发条件，观察实际行为是否与设计文档承诺一致。
// 运行：go test -count=1 -run TestProbe -v .

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 嫌疑 1：EmbedBackend 参数数量无容错 + 类型不匹配的静默坑
// ---------------------------------------------------------------------------

// Probe1b：绑定 func(a int) 后，前端传字符串 "3"（不是数字 3）会发生什么？
// 设计承诺：参数与返回值通过 JSON 编解码。但 int 反序列化 "3" 字符串会直接失败。
func TestProbe1bEmbedTypeMismatch(t *testing.T) {
	eb := NewEmbedBackend()
	if err := eb.Bind("AddInt", func(a int) int { return a + 1 }); err != nil {
		t.Fatalf("bind: %v", err)
	}

	// 前端传数字 3（正确）
	res, err := eb.Handle("AddInt", []json.RawMessage{json.RawMessage(`3`)})
	if err != nil {
		t.Fatalf("AddInt(3): unexpected error: %v", err)
	}
	if n, ok := res.(int); !ok || n != 4 {
		t.Fatalf("AddInt(3) -> %#v (want 4)", res)
	}
	t.Logf("AddInt(数字3) -> %v ✓", res)

	// 前端传字符串 "3"（类型不符）
	res, err = eb.Handle("AddInt", []json.RawMessage{json.RawMessage(`"3"`)})
	t.Logf("AddInt(字符串\"3\") -> res=%#v err=%v", res, err)
	if err == nil {
		t.Logf("  ⚠ 字符串参数被静默接受（JSON 默认把 \"3\" 转成 int 0？实际=%v）", res)
	}
}

// Probe1a：非变参函数，前端多传一个参数 -> 直接报错（约定是否应文档化？）
func TestProbe1aEmbedArgOver(t *testing.T) {
	eb := NewEmbedBackend()
	_ = eb.Bind("One", func(a int) int { return a })
	_, err := eb.Handle("One", []json.RawMessage{json.RawMessage(`1`), json.RawMessage(`2`)})
	t.Logf("One(多传参) -> err=%v", err)
}

// ---------------------------------------------------------------------------
// 嫌疑 2：collectResults 对 (T, error) 中 T 为 nil 的处理
// ---------------------------------------------------------------------------

// Probe2：函数返回 (interface{}, error) 且值为 nil 时，最终 JSON 是 null 还是缺省？
func TestProbe2NilReturn(t *testing.T) {
	eb := NewEmbedBackend()
	_ = eb.Bind("NilVal", func() (interface{}, error) { return nil, nil })
	res, err := eb.Handle("NilVal", nil)
	t.Logf("NilVal -> res=%#v (type %T) err=%v", res, res, err)
	// 设计意图：返回空值时 result 应为 null；若为 nil interface，json.Marshal(nil)=null，OK。
	if res != nil {
		t.Logf("  ⚠ 期望 nil，实际 %v", res)
	}
}

// ---------------------------------------------------------------------------
// 嫌疑 3：ProcBackend 并发调用 + 进程退出时序
// ---------------------------------------------------------------------------

// Probe3：两个并发 Handle，后端在第二个响应前就退出，验证两个都被唤醒（无泄漏、无挂起）。
func TestProbe3ConcurrentExit(t *testing.T) {
	// 用一个"发一条就退出"的桩后端：Go 一次性 echo
	src := fmt.Sprintf(`# 桩：读第一条请求，回响应，立即退出
import sys, json
for line in sys.stdin:
    m = json.loads(line)
    print(json.dumps({"id": m["id"], "result": "ok-"+m["id"]}), flush=True)
    break
`)
	_ = src
	// 直接用 node 桩：逐行解析请求，每条回一个响应，最后退出。
	// 关键：进程在处理完第二条前就 exit，模拟"中途退出"。
	p := NewProcBackend("node", `-e`, `
const lines=[];let buf="";
process.stdin.on("data",d=>{
  buf+=d;
  let i;
  while((i=buf.indexOf("\n"))>=0){
    const l=buf.slice(0,i);buf=buf.slice(i+1);
    lines.push(l);
    const m=JSON.parse(l);
    console.log(JSON.stringify({id:m.id,result:"ok"}));
  }
  if(lines.length>=2){process.exit(0);}
});
`)
	if err := p.start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	type out struct {
		r    interface{}
		err  error
		id   int
		done chan struct{}
	}
	const N = 2
	done := make([]chan struct{}, N)
	for i := range N {
		done[i] = make(chan struct{})
		go func(id int) {
			_, _ = p.Handle("m"+string(rune('0'+id)), nil)
			close(done[id])
		}(i)
	}
	// 两个调用都应在退出路径被唤醒（不挂起）
	select {
	case <-done[0]:
		select {
		case <-done[1]:
			t.Logf("两个并发调用均被唤醒（无挂起）")
		case <-time.After(3 * time.Second):
			t.Fatalf("调用 1 在进程退出后未唤醒（疑似泄漏/挂起）")
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("调用 0 未唤醒（疑似泄漏/挂起）")
	}
}

// ---------------------------------------------------------------------------
// 嫌疑 4：readLoop 16MB 行上限 -> 超长响应被误判为"后端退出"
// ---------------------------------------------------------------------------

// oversizedReader：模拟一段 stdout 字节流 = [合法行1] + [20MB 超限行(以\n终止)] + [合法行2] + EOF。
// 用单一连续字节流 + 游标实现，语义清晰：超长的那一行本身以 '\n' 结束（真实 NDJSON
// 一行必以换行终止），其后仍有一条合法行可读——证明"通道存活"。
type oversizedReader struct {
	data []byte
	off  int
}

func newOversizedReader() *oversizedReader {
	var b []byte
	b = append(b, `{"id":1,"result":"ok1"}`+"\n"...)
	huge := make([]byte, 20*1024*1024) // 20MB 无换行块（> 默认 16MB 上限）
	b = append(b, huge...)
	b = append(b, '\n') // 超限行以换行终止
	b = append(b, `{"id":3,"result":"ok3"}`+"\n"...)
	return &oversizedReader{data: b}
}

func (r *oversizedReader) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}

// P1 RED 锁（修复前必失败）：一条 >maxLine 的响应不得终止 readLoop。
// 不变量：超长行出现后，*随后* 的合法响应（id=3）仍必须送达——通道活着。
// 现状缺陷：readLoop 把 bufio.ErrTooLong 与真实退出同路，超长行一来就退出，
// 于是 id=3 永远收不到（被误报 exited unexpectedly）。修复后本用例 GREEN。
func TestProbe4OversizedLine(t *testing.T) {
	p := NewProcBackend("freedom-probe")

	// 三条在途调用：id=1 正常响应；id=2 对应那条超长行（本应被单独判"行过大"）；
	// id=3 对应超长行之后的合法响应——修复后必须仍送达。
	ch1 := make(chan procResp, 1)
	ch2 := make(chan procResp, 1)
	ch3 := make(chan procResp, 1)
	p.pending[1] = ch1
	p.pending[2] = ch2
	p.pending[3] = ch3

	huge := make([]byte, 20*1024*1024) // 20MB > 默认 16MB
	_ = huge
	reader := newOversizedReader()

	done := make(chan struct{})
	go func() {
		p.readLoop(reader)
		close(done)
	}()

	// 不变量 1：id=1 正常送达。
	select {
	case r1 := <-ch1:
		if r1.err != nil {
			t.Fatalf("id=1 应正常送达，err=%v", r1.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("id=1 未送达（通道被误判中断？）")
	}

	// 不变量 2（核心 RED）：id=3 在超长行之后仍必须送达——证明通道没被打崩。
	select {
	case r3 := <-ch3:
		if r3.err != nil {
			// 修复前：id=3 会被"exited unexpectedly"误唤醒（err 非 nil）-> 仍 RED。
			t.Fatalf("通道应存活，id=3 却收到 err=%v（readLoop 被 ErrTooLong 误判为退出）", r3.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("id=3 未送达：超长行把整个 IPC 通道打崩了（P1 缺陷）")
	}
	<-done

	// 不变量 3（信息项）：id=2 那条超长响应本身应被单独判"行过大"而非静默丢弃。
	// 修复后它应收到 "line too long" 类错误；此处仅作记录，不断言，避免锁定具体文案。
	select {
	case r2 := <-ch2:
		t.Logf("id=2（超长行）-> err=%v result=%#v（信息项）", r2.err, r2.result)
	default:
		t.Logf("id=2 仍无响应（信息项：超长行来源调用挂起，符合'仅该行过大'的局部影响）")
	}
	_ = runtime.GOOS
}
