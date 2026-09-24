package freedom

import (
	"reflect"
	"testing"
)

// W6 回归：collectResults 对"值类型实现的 error"不得调 reflect.Value.IsNil（会 panic），
// 必须经 isNilValue 的 Kind 分派。

type valueErr struct{ msg string }

func (v valueErr) Error() string { return v.msg }

func TestCollectResultsValueError(t *testing.T) {
	// 仅返回 error（值类型）
	res, err := collectResults([]reflect.Value{reflect.ValueOf(valueErr{"boom"})})
	if err == nil || err.Error() != "boom" || res != nil {
		t.Fatalf("single valueErr: res=%v err=%v", res, err)
	}
	// (T, error)（值类型 error）
	res, err = collectResults([]reflect.Value{reflect.ValueOf(7), reflect.ValueOf(valueErr{"e2"})})
	if err == nil || err.Error() != "e2" || res != 7 {
		t.Fatalf("(T, valueErr): res=%v err=%v", res, err)
	}
	// 常规 *errors.errorString（指针型，nilable）路径不受影响
	var nilErr *valueErr
	res, err = collectResults([]reflect.Value{reflect.ValueOf(nilErr)})
	if err != nil || res != nil {
		t.Fatalf("nil ptr error: res=%v err=%v", res, err)
	}
}
