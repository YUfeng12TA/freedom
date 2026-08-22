package freedom

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
)

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// EmbedBackend 是把 Go 方法集合直接注册在壳进程内的后端（传统模式）。
// 前端调用 window.freedom.call(name, ...args) 时，桥接层按方法签名反射调用。
type EmbedBackend struct {
	methods map[string]reflect.Value
	mu      sync.RWMutex
}

// NewEmbedBackend 创建一个内嵌 Go 后端。
func NewEmbedBackend() *EmbedBackend {
	return &EmbedBackend{methods: map[string]reflect.Value{}}
}

// Bind 把后端 Go 方法暴露给前端。
//
//   - fn 必须是函数。
//   - 参数与返回值通过 JSON 编解码，前端用 window.freedom.call(name, ...args) 调用，
//     返回 Promise（resolve 为返回值，reject 为 error 的字符串表示）。
//   - 返回值约定：可返回 (T, error) 或仅 (T) 或仅 error 或空。
func (b *EmbedBackend) Bind(name string, fn interface{}) error {
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func {
		return fmt.Errorf("freedom: Bind(%q): only functions can be bound", name)
	}
	if n := v.Type().NumOut(); n > 2 {
		return fmt.Errorf("freedom: Bind(%q): function may return at most a value and an error", name)
	}
	b.mu.Lock()
	b.methods[name] = v
	b.mu.Unlock()
	return nil
}

// Unbind 移除先前 Bind 的方法。
func (b *EmbedBackend) Unbind(name string) {
	b.mu.Lock()
	delete(b.methods, name)
	b.mu.Unlock()
}

// Handle 实现 Backend 接口：按方法名反射调用已绑定的 Go 函数。
func (b *EmbedBackend) Handle(method string, params []json.RawMessage) (interface{}, error) {
	b.mu.RLock()
	fn, ok := b.methods[method]
	b.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("freedom: method %q is not bound", method)
	}
	return callFunction(fn, params)
}

// Close 内嵌后端没有需要释放的资源。
func (b *EmbedBackend) Close() error { return nil }

// callFunction 反射调用一个已绑定的 Go 函数。
// 返回值约定：0 个（忽略）、1 个（值或 error）、2 个（值, error）。
func callFunction(fn reflect.Value, params []json.RawMessage) (interface{}, error) {
	t := fn.Type()

	if t.IsVariadic() {
		fixed := t.NumIn() - 1
		if len(params) < fixed {
			return nil, fmt.Errorf("freedom: arg count mismatch: want >=%d got %d", fixed, len(params))
		}
		args := make([]reflect.Value, len(params))
		for i, p := range params {
			var typ reflect.Type
			if i < fixed {
				typ = t.In(i)
			} else {
				typ = t.In(fixed).Elem()
			}
			ptr := reflect.New(typ)
			if err := json.Unmarshal(p, ptr.Interface()); err != nil {
				return nil, fmt.Errorf("freedom: arg %d: %w", i, err)
			}
			args[i] = ptr.Elem()
		}
		return collectResults(fn.Call(args))
	}

	if len(params) != t.NumIn() {
		return nil, fmt.Errorf("freedom: arg count mismatch: want %d got %d", t.NumIn(), len(params))
	}
	args := make([]reflect.Value, len(params))
	for i, p := range params {
		ptr := reflect.New(t.In(i))
		if err := json.Unmarshal(p, ptr.Interface()); err != nil {
			return nil, fmt.Errorf("freedom: arg %d: %w", i, err)
		}
		args[i] = ptr.Elem()
	}
	return collectResults(fn.Call(args))
}

func collectResults(res []reflect.Value) (interface{}, error) {
	switch len(res) {
	case 0:
		return nil, nil
	case 1:
		if res[0].Type().Implements(errorType) {
			if !res[0].IsNil() {
				return nil, res[0].Interface().(error)
			}
			return nil, nil
		}
		return res[0].Interface(), nil
	case 2:
		if !res[1].Type().Implements(errorType) {
			return nil, fmt.Errorf("freedom: second return value must be error")
		}
		if !res[1].IsNil() {
			return res[0].Interface(), res[1].Interface().(error)
		}
		return res[0].Interface(), nil
	default:
		return nil, fmt.Errorf("freedom: function may return at most a value and an error")
	}
}
