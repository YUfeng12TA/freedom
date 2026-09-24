// Freedom 前端入口。
// window.freedom 由壳层注入，无需 import。
//   call(method, ...args) -> Promise    调用后端方法
//   on(event, cb) / off(event, cb)       订阅 / 取消订阅后端事件
//   window.minimize() / maximize() / close() 等窗口控制（无边框模式自绘按钮用）

// 无边框模式：显示自绘标题栏并绑定按钮。
// 壳桥接（__freedom_window）注入时机与页面脚本加载先后不定，早期调用可能被 reject 吞掉导致标题栏不显示，
// 因此先轮询等待桥接就绪（最多 150 次 × 100ms = 15s）再初始化；超时后明确告警而非静默失效。
function waitForBridge(tries = 150, interval = 100) {
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

// 统一执行窗口控制动作：成功返回结果，失败打印告警并返回 undefined。
// 避免 mac/linux 等平台窗口控制不可用时的"静默无反应"假死体验。
function runWindow(w, action, fallback) {
  return w[action]().catch((e) => {
    console.warn('[freedom] window action "' + action + '" failed:', e && e.message ? e.message : e);
    if (typeof fallback === 'function') return fallback();
    return undefined;
  });
}

// 同步最大化状态到自绘按钮图标（最大化 ⇄ 还原）与 body 状态类。
async function syncMaxState(w) {
  const maxBtn = document.getElementById('tbMax');
  if (!maxBtn) return;
  try {
    const isMax = await w.isMaximized();
    // Segoe MDL2：E922=最大化，E923=还原
    maxBtn.innerHTML = isMax ? '\uE923' : '\uE922';
    document.body.classList.toggle('freedom-maximized', !!isMax);
  } catch (e) {
    // 平台不支持状态查询（mac/linux）时保持默认最大化图标
  }
}

async function initFrameless() {
  const ready = await waitForBridge();
  if (!ready) {
    console.warn('[freedom] 桥接 15s 未就绪，无边框标题栏未初始化（窗口仍可正常使用）。');
    return;
  }
  try {
    const frameless = await window.freedom.window.isFrameless();
    if (!frameless) return;
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

    // JS 拖动标题栏（保留双击 / 右键事件；非 Windows 平台桥接失败则退化为原生 drag 体验）
    const bar = document.getElementById('titlebar');
    if (bar) {
      bar.addEventListener('mousedown', (e) => {
        if (e.button === 0 && e.target.closest('.tb-btn') === null) {
          runWindow(w, 'startDrag');
        }
      });
      // 双击最大化 / 还原
      bar.addEventListener('dblclick', async (e) => {
        if (e.target.closest('.tb-btn') !== null) return;
        const isMax = await runWindow(w, 'isMaximized', async () => false);
        if (isMax) { await runWindow(w, 'restore', () => runWindow(w, 'toggleMaximize')); }
        else { await runWindow(w, 'maximize', () => runWindow(w, 'toggleMaximize')); }
        syncMaxState(w);
      });
      // 右键菜单
      bar.addEventListener('contextmenu', async (e) => {
        if (e.target.closest('.tb-btn') !== null) return;
        e.preventDefault();
        openTitlebarMenu(e.clientX, e.clientY);
      });
    }

    // 初始化同步最大化按钮状态；窗口尺寸变化时（含拖拽到屏幕边缘触发的系统最大化）重新同步
    syncMaxState(w);
    window.addEventListener('resize', () => syncMaxState(w));
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
    const isMax = await runWindow(win, 'isMaximized', () => false);
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
    if (action === 'close') runWindow(w, 'close');
    else if (action === 'minimize') runWindow(w, 'minimize');
    else if (action === 'maximize') { runWindow(w, 'maximize', () => runWindow(w, 'toggleMaximize')); syncMaxState(w); }
    else if (action === 'restore') { runWindow(w, 'restore', () => runWindow(w, 'toggleMaximize')); syncMaxState(w); }
  });
}

initTitlebarMenu();
initFrameless();

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
