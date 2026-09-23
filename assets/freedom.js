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

  // 一次性订阅：触发一次后自动退订（对标 Tauri listen+unlisten）。
  function once(event, cb) {
    if (typeof cb !== 'function') return function () {};
    var un = on(event, function (data) {
      off(event, un);
      cb(data);
    });
    return un;
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
    once: once,
    emit: emit,
    window: {
      // 窗口控制（无边框/隐藏标题栏模式下前端自绘按钮使用）。
      // 全部经桥接调用原生实现，返回 Promise。
      minimize: function () { return windowAction('minimize'); },
      maximize: function () { return windowAction('maximize'); },
      unmaximize: function () { return windowAction('unmaximize'); },
      toggleMaximize: function () { return windowAction('toggleMaximize'); },
      // close(force)：force=true 时绕过 interceptClose 拦截直接关闭。
      close: function (force) { return windowAction('close', { force: !!force }); },
      isMaximized: function () { return windowAction('isMaximized'); },
      isFrameless: function () { return windowAction('isFrameless'); },
      // —— W1 对标 Tauri：位置 / 尺寸（物理像素）——
      setPosition: function (x, y) { return windowAction('setPosition', { x: x, y: y }); },
      setSize: function (width, height) { return windowAction('setSize', { width: width, height: height }); },
      getPosition: function () { return windowAction('getPosition'); },
      getSize: function () { return windowAction('getSize'); },
      innerSize: function () { return windowAction('innerSize'); },
      center: function () { return windowAction('center'); },
      setTitle: function (title) { return windowAction('setTitle', { title: String(title) }); },
      // —— 可见性 / 层级 / 焦点 ——
      show: function () { return windowAction('show'); },
      hide: function () { return windowAction('hide'); },
      focus: function () { return windowAction('focus'); },
      isVisible: function () { return windowAction('isVisible'); },
      isFocused: function () { return windowAction('isFocused'); },
      isMinimized: function () { return windowAction('isMinimized'); },
      setAlwaysOnTop: function (on) { return windowAction('setAlwaysOnTop', { on: !!on }); },
      setSkipTaskbar: function (on) { return windowAction('setSkipTaskbar', { on: !!on }); },
      // —— 行为开关 ——
      setResizable: function (on) { return windowAction('setResizable', { on: !!on }); },
      setMaximizable: function (on) { return windowAction('setMaximizable', { on: !!on }); },
      setMinimizable: function (on) { return windowAction('setMinimizable', { on: !!on }); },
      // —— 全屏 / 关闭拦截 ——
      setFullscreen: function (on) { return windowAction('setFullscreen', { on: !!on }); },
      enterFullscreen: function () { return windowAction('setFullscreen', { on: true }); },
      exitFullscreen: function () { return windowAction('setFullscreen', { on: false }); },
      isFullscreen: function () { return windowAction('isFullscreen'); },
      // 拦截系统/按钮关闭：开启后关闭动作变为 window.closeRequested 事件，
      // 前端处理后以 close(true) 放行或不做操作保持窗口存活。
      interceptClose: function (on) { return windowAction('interceptClose', { on: !!on }); },
      // 聚合信息：title/outerX/outerY/outerWidth/outerHeight/innerWidth/innerHeight/
      // scaleFactor/visible/focused/maximized/minimized/alwaysOnTop/skipTaskbar/frameless/fullscreen。
      getInfo: function () { return windowAction('getInfo'); },
      // 显示器列表（经 sys 桥接）：[{x,y,width,height,workX,workY,workWidth,workHeight,scaleFactor,isPrimary}]
      monitors: function () { return sysCall('window.monitors', {}); },
      // 窗口视觉效果（Win11）：backdrop: auto|none|solid|mica|acrylic；corner: round|roundSmall|square|default；borderColor: 0x00BBGGRR
      setBackdrop: function (mode) { return sysCall('window.backdrop', { mode: mode }); },
      setCorner: function (mode) { return sysCall('window.corner', { mode: mode }); },
      setBorderColor: function (color) { return sysCall('window.borderColor', { color: color }); },
    },
    // ---- W2 系统集成命名空间（均经 __freedom_sys / __freedom_tray 桥接）----
    sys: sysCall,
    clipboard: {
      readText: function () { return sysCall('clipboard.read', {}); },
      writeText: function (text) { return sysCall('clipboard.write', { text: String(text) }); },
    },
    shell: {
      // 用系统默认程序打开 URL/文件（openExternal）。
      open: function (target) { return sysCall('shell.open', { target: String(target) }); },
    },
    notification: {
      show: function (title, body) { return sysCall('notification.show', { title: String(title), body: String(body || '') }); },
    },
    shortcut: {
      // combo 形如 "ctrl+alt+k"；触发时收到事件 freedom.on('shortcut.triggered', ({id}) => ...)。
      register: function (id, combo) { return sysCall('shortcut.register', { id: id, combo: combo }); },
      unregister: function (id) { return sysCall('shortcut.unregister', { id: id }); },
      list: function () { return sysCall('shortcut.list', {}); },
    },
    autostart: {
      isEnabled: function (name) { return sysCall('autostart.get', { name: name || '' }); },
      set: function (enabled, opts) { return sysCall('autostart.set', { name: (opts && opts.name) || '', enabled: !!enabled, args: (opts && opts.args) || '' }); },
      enable: function (opts) { return this.set(true, opts); },
      disable: function (opts) { return this.set(false, opts); },
    },
    protocol: {
      // URL Scheme 注册（deep link）；拉起参数经 'app.secondInstance' 事件或 app.launchArgs() 获取。
      register: function (scheme, displayName) { return sysCall('protocol.register', { scheme: scheme, name: displayName || '' }); },
      unregister: function (scheme) { return sysCall('protocol.unregister', { scheme: scheme }); },
    },
    app: {
      launchArgs: function () { return sysCall('app.launchArgs', {}); },
    },
    taskbar: {
      setProgress: function (value) { return sysCall('taskbar.progress', { value: value }); },
      setState: function (state) { return sysCall('taskbar.state', { state: state }); }, // normal|paused|error|indeterminate
      clear: function () { return sysCall('taskbar.clear', {}); },
      setOverlay: function (iconDataURL) { return sysCall('taskbar.overlay', { icon: iconDataURL }); },
      clearOverlay: function () { return sysCall('taskbar.clearOverlay', {}); },
    },
    dialog: {
      message: function (opts) { return sysCall('dialog.message', opts || {}); },
      open: function (opts) { return sysCall('dialog.open', opts || {}); },
      save: function (opts) { return sysCall('dialog.save', opts || {}); },
    },
    tray: {
      // create({icon: dataURL, tooltip})；事件：tray:click / tray:double-click / tray:menu({id})。
      create: function (opts) { return trayCall('tray.create', opts || {}); },
      destroy: function () { return trayCall('tray.destroy', {}); },
      setTooltip: function (text) { return trayCall('tray.tooltip', { tooltip: text }); },
      setMenu: function (items) { return trayCall('tray.menu', { items: items }); },
    },
    menu: {
      // 原生菜单栏（挂主窗口）：items 同 tray.setMenu 结构；点击走 'tray:menu' 事件。
      set: function (items) { return trayCall('menu.set', { items: items }); },
    },
  };

  function trayCall(method, args) {
    if (typeof window.__freedom_tray !== 'function') {
      return Promise.reject(new Error('[freedom] 未检测到托盘桥接。'));
    }
    return window.__freedom_tray(method, JSON.stringify(args || {}));
  }

  // sys 桥接：原生系统能力（taskbar.* / window.* / dialog.*）。
  // W5 会扩展为 freedom.sys 命名空间，先行提供内部包装。
  function sysCall(method, args) {
    if (typeof window.__freedom_sys !== 'function') {
      return Promise.reject(new Error('[freedom] 未检测到原生系统能力桥接。'));
    }
    return window.__freedom_sys(method, JSON.stringify(args || {}));
  }

  // 前端自绘按钮的便捷绑定：把按钮 DOM 接到窗口控制。
  // freedom.window.bindButtons({ min: '#minBtn', max: '#maxBtn', close: '#closeBtn' })
  freedom.window.bindButtons = function (sel) {
    var q = function (s) { return typeof s === 'string' ? document.querySelector(s) : s; };
    var min = q(sel && sel.min), max = q(sel && sel.max), close = q(sel && sel.close);
    var self = this;
    var ignoreErr = function () { /* 桥接未就绪（如窗口销毁中）时忽略本次操作 */ };
    if (min) min.addEventListener('click', function () { self.minimize().catch(ignoreErr); });
    if (max) max.addEventListener('click', function () {
      self.isMaximized().then(function (m) {
        if (m) self.unmaximize(); else self.maximize();
      }).catch(ignoreErr);
    });
    if (close) close.addEventListener('click', function () { self.close().catch(ignoreErr); });
  };

  function windowAction(action, args) {
    // __freedom_window 是 webview_go Bind 注入的异步全局函数（返回 Promise）。
    // Go 侧签名为 (action, paramsJSON)，paramsJSON 恒为 JSON object 字符串。
    if (typeof window.__freedom_window !== 'function') {
      return Promise.reject(new Error('[freedom] 未检测到原生窗口控制桥接。'));
    }
    return window.__freedom_window(action, JSON.stringify(args || {}));
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

  // H3：页面就绪回调。Go 侧 onReady 改为在 __freedom__ready 被调用时触发
  //（此前在 SetHtml 前触发，期间 Emit 的初始化事件因 SDK 尚未建立而丢失）。
  // DOMContentLoaded 后上报就绪；若脚本执行时 DOM 已加载完成则立即上报。
  function signalReady() {
    if (typeof window.__freedom__ready === 'function') {
      try { window.__freedom__ready(); } catch (e) { /* 忽略 */ }
    }
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', signalReady);
  } else {
    signalReady();
  }
})();
