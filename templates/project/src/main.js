// Freedom 前端入口。
// window.freedom 由壳层注入，无需 import。
//   call(method, ...args) -> Promise    调用后端方法
//   on(event, cb) / off(event, cb)       订阅 / 取消订阅后端事件
//   window.minimize() / maximize() / close() 等窗口控制（无边框模式自绘按钮用）

// 无边框模式：显示自绘标题栏并绑定按钮。
// 壳桥接（__freedom_window）注入时机与页面脚本加载先后不定，早期调用可能被 reject 吞掉导致标题栏不显示，
// 因此先轮询等待桥接就绪（最多 100 次 × 100ms）再初始化。
function waitForBridge(tries = 100, interval = 100) {
  return new Promise((resolve) => {
    const check = () => {
      if (window.freedom && window.freedom.window && typeof window.__freedom_window === 'function') {
        resolve(true);
      } else if (tries-- > 0) {
        setTimeout(check, interval);
      } else {
        resolve(false);
      }
    };
    check();
  });
}

async function initFrameless() {
  if (!(await waitForBridge())) return;
  try {
    const frameless = await window.freedom.window.isFrameless();
    if (frameless) {
      document.body.classList.add('freedom-frameless');
      window.freedom.window.bindButtons({ min: '#tbMin', max: '#tbMax', close: '#tbClose' });
    }
  } catch (e) {
    /* 桥接异常时保持默认样式 */
  }
}

initFrameless();

// 桥接自检。
document.getElementById('pingBtn').addEventListener('click', async () => {
  const el = document.getElementById('result');
  try {
    const r = await window.freedom.call('__freedom__ping');
    el.textContent = '桥接正常：' + r;
  } catch (e) {
    el.textContent = '桥接异常：' + e.message;
  }
});
