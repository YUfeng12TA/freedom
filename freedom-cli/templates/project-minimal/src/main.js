// Freedom 前端入口（极简模板）。
// window.freedom 由壳层注入，无需 import：
//   call(method, ...args) -> Promise    调用后端方法
//   on(event, cb) / off(event, cb)       订阅 / 取消订阅后端事件
//   window.minimize() / maximize() / close() 等窗口控制（frameless 模式下自绘按钮用）

// 桥接自检：__freedom__ping 是壳注入的全局函数，优先直调；
// 低版本壳不存在时回退经 bridge 路由（壳层 bridge 对 ping 特判兼容）。
document.getElementById('pingBtn').addEventListener('click', async () => {
  const el = document.getElementById('result');
  try {
    const r = (typeof window.__freedom__ping === 'function')
      ? await window.__freedom__ping()
      : await window.freedom.call('__freedom__ping');
    el.textContent = '桥接正常：' + r;
  } catch (e) {
    el.textContent = '桥接异常：' + e.message;
  }
});
