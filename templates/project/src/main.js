// Freedom 前端入口。
// window.freedom 由壳层注入，无需 import。
//   call(method, ...args) -> Promise    调用后端方法
//   on(event, cb) / off(event, cb)       订阅 / 取消订阅后端事件
//   window.minimize() / maximize() / close() 等窗口控制（无边框模式自绘按钮用）

// 无边框模式：显示自绘标题栏并绑定按钮。
if (window.freedom && window.freedom.window) {
  window.freedom.window.isFrameless().then((frameless) => {
    if (frameless) {
      document.body.classList.add('freedom-frameless');
      window.freedom.window.bindButtons({ min: '#tbMin', max: '#tbMax', close: '#tbClose' });
    }
  }).catch(() => {});
}

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
