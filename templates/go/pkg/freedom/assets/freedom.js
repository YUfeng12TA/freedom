// Freedom 前端 SDK。
// window.__freedom_bridge 由 go-webview2 的 Bind 机制注册（异步桥接函数，返回 Promise），
// 注入脚本与 SDK 的执行先后顺序不固定，因此 SDK 采用"每次调用时动态读取"策略，
// 保证与注入顺序无关。
(function () {
  'use strict';

  function getBridge() {
    return (typeof window.__freedom_bridge === 'function') ? window.__freedom_bridge : null;
  }

  var listeners = {};

  function call(method) {
    var params = Array.prototype.slice.call(arguments, 1);
    var bridge = getBridge();
    if (!bridge) {
      return Promise.reject(new Error('[freedom] 未检测到原生桥接（__freedom_bridge），当前不在桌面壳内运行。'));
    }
    return bridge(method, JSON.stringify(params));
  }

  function on(event, cb) {
    if (typeof cb !== 'function') return function () {};
    (listeners[event] = listeners[event] || []).push(cb);
    return function () { off(event, cb); };
  }

  function off(event, cb) {
    var l = listeners[event] || [];
    var i = l.indexOf(cb);
    if (i >= 0) l.splice(i, 1);
  }

  // 后端通过 app.Emit(event, data) 触发（Eval 调用本函数）。
  function emit(event, data) {
    var l = listeners[event] || [];
    for (var i = 0; i < l.length; i++) {
      try { l[i](data); } catch (e) { /* 监听器异常不影响其余监听器 */ }
    }
  }

  var freedom = {
    call: call,
    invoke: call,
    on: on,
    off: off,
    emit: emit,
    window: {
      // 窗口控制（无边框/隐藏标题栏模式下前端自绘按钮使用）。
      // 全部经桥接调用原生实现，返回 Promise。
      minimize: function () { return windowAction('minimize'); },
      maximize: function () { return windowAction('maximize'); },
      unmaximize: function () { return windowAction('unmaximize'); },
      restore: function () { return windowAction('unmaximize'); },
      toggleMaximize: function () { return windowAction('toggleMaximize'); },
      close: function () { return windowAction('close'); },
      isMaximized: function () { return windowAction('isMaximized'); },
      isFrameless: function () { return windowAction('isFrameless'); },
      // 应用图标（data URL）。从 exe 内嵌图标提取，供自绘标题栏显示，不依赖 resources 资源文件夹。
      appIcon: function () { return windowAction('appIcon'); },
      // 标题栏 JS 拖动（WM_NCLBUTTONDOWN + HTCAPTION），比 -webkit-app-region: drag
      // 更稳（保留双击最大化 / 右键菜单等页面事件）。非 Windows 平台为 no-op。
      startDrag: function () { return windowAction('startDrag'); },
    },
  };

  // 前端自绘按钮的便捷绑定：把按钮 DOM 接到窗口控制。
  // freedom.window.bindButtons({ min: '#minBtn', max: '#maxBtn', close: '#closeBtn' })
  freedom.window.bindButtons = function (sel) {
    var q = function (s) { return typeof s === 'string' ? document.querySelector(s) : s; };
    var min = q(sel && sel.min), max = q(sel && sel.max), close = q(sel && sel.close);
    var self = this;
    if (min) min.addEventListener('click', function () { self.minimize(); });
    if (max) max.addEventListener('click', function () {
      self.isMaximized().then(function (m) {
        if (m) self.unmaximize(); else self.maximize();
      });
    });
    if (close) close.addEventListener('click', function () { self.close(); });
  };

  function windowAction(action) {
    // __freedom_window 是 webview_go Bind 注入的异步全局函数（返回 Promise）
    if (typeof window.__freedom_window !== 'function') {
      return Promise.reject(new Error('[freedom] 未检测到原生窗口控制桥接。'));
    }
    return window.__freedom_window(action);
  }

  Object.defineProperty(freedom, 'isDesktop', {
    get: function () { return !!getBridge(); },
  });

  // 兼容 Wails 风格：window.go 对象，便于迁移既有代码。
  var go = {};
  Object.defineProperty(go, 'backend', {
    get: function () { return { call: call, invoke: call }; },
  });
  window.go = go;

  window.freedom = freedom;
})();
