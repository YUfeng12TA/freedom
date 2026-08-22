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
      const w = window.freedom.window;
      w.bindButtons({ min: '#tbMin', max: '#tbMax', close: '#tbClose' });

      // 应用图标：从 exe 内嵌图标提取，不依赖 resources 资源文件夹
      const iconUrl = await w.appIcon();
      const iconEl = document.getElementById('tbIcon');
      if (iconUrl && iconEl) {
        iconEl.src = iconUrl;
        iconEl.style.display = 'block';
      }

      // JS 拖动标题栏（保留双击 / 右键事件；非 Windows 平台桥接静默则退化为原生 drag 体验）
      const bar = document.getElementById('titlebar');
      if (bar) {
        bar.addEventListener('mousedown', (e) => {
          if (e.button === 0 && e.target.closest('.tb-btn') === null) {
            w.startDrag && w.startDrag();
          }
        });
        // 双击最大化 / 还原
        bar.addEventListener('dblclick', async (e) => {
          if (e.target.closest('.tb-btn') !== null) return;
          const isMax = await w.isMaximized();
          if (isMax) { w.restore ? w.restore() : w.toggleMaximize(); }
          else { w.maximize ? w.maximize() : w.toggleMaximize(); }
        });
        // 右键菜单
        bar.addEventListener('contextmenu', async (e) => {
          if (e.target.closest('.tb-btn') !== null) return;
          e.preventDefault();
          openTitlebarMenu(e.clientX, e.clientY);
        });
      }
    }
  } catch (e) {
    /* 桥接异常时保持默认样式 */
  }
}

// 标题栏右键菜单
const tbMenu = () => document.getElementById('tbMenu');

function openTitlebarMenu(x, y) {
  const menu = tbMenu();
  const win = window.freedom.window;
  menu.style.left = x + 'px';
  menu.style.top = y + 'px';
  menu.style.display = 'block';

  // 根据当前窗口状态动态置灰「最大化 / 还原」
  const updateItems = async () => {
    const isMax = await win.isMaximized();
    menu.querySelectorAll('.tb-menu-item').forEach((item) => {
      const action = item.dataset.action;
      const disabled = (isMax && action === 'maximize') || (!isMax && action === 'restore');
      item.style.opacity = disabled ? '0.4' : '1';
      item.style.pointerEvents = disabled ? 'none' : 'auto';
    });
  };
  updateItems();

  const close = () => {
    menu.style.display = 'none';
    document.removeEventListener('mousedown', onDocDown);
    window.removeEventListener('blur', close);
  };
  const onDocDown = (e) => {
    if (!menu.contains(e.target)) close();
  };
  document.addEventListener('mousedown', onDocDown);
  window.addEventListener('blur', close);
}

function initTitlebarMenu() {
  const menu = tbMenu();
  if (!menu) return;
  menu.addEventListener('click', (e) => {
    const item = e.target.closest('.tb-menu-item');
    if (!item) return;
    const action = item.dataset.action;
    const w = window.freedom.window;
    menu.style.display = 'none';
    if (action === 'close') w.close();
    else if (action === 'minimize') w.minimize();
    else if (action === 'maximize') w.maximize();
    else if (action === 'restore') w.restore ? w.restore() : w.toggleMaximize();
  });
}

initTitlebarMenu();
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
