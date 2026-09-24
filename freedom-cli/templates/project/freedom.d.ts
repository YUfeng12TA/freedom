// Freedom 桌面壳前端 SDK 类型声明（window.freedom）
// 运行时由壳注入（assets/freedom.js）；本文件仅类型提示，不参与打包产物。
// 用法：tsconfig "include": ["src", "freedom.d.ts"]，直接 window.freedom.* 获得补全。

export {};

interface FreedomWindowAPI {
  minimize(): Promise<void>;
  maximize(): Promise<void>;
  unmaximize(): Promise<void>;
  toggleMaximize(): Promise<boolean>;
  close(): Promise<void>;
  isMaximized(): Promise<boolean>;
  isFrameless(): Promise<boolean>;
  show(): Promise<void>;
  hide(): Promise<void>;
  focus(): Promise<void>;
  setPosition(x: number, y: number): Promise<void>;
  setSize(w: number, h: number): Promise<void>;
  getPosition(): Promise<{ x: number; y: number }>;
  getSize(): Promise<{ width: number; height: number }>;
  setAlwaysOnTop(on: boolean): Promise<void>;
  setFullscreen(on: boolean): Promise<void>;
  startDrag(): Promise<void>;
  /** 多窗口：spec.url 或 spec.html 指定页面，返回 {id} */
  create(spec: { title?: string; url?: string; html?: string; width?: number; height?: number }): Promise<{ id: string }>;
  list(): Promise<Array<{ id: string; title?: string }>>;
  focusWindow(id: string): Promise<void>;
  closeWindow(id: string): Promise<void>;
}

interface FreedomAPI {
  /** 调用后端方法（任意语言后端，NDJSON/JSON-RPC over stdio），返回 Promise。 */
  call(method: string, ...args: unknown[]): Promise<any>;
  invoke(method: string, ...args: unknown[]): Promise<any>;
  /** 订阅后端推送事件，返回取消订阅函数。 */
  on(event: string, fn: (data: any) => void): () => void;
  off(event: string, fn: (data: any) => void): void;
  once(event: string, fn: (data: any) => void): () => void;
  emit(event: string, data?: unknown): void;

  window: FreedomWindowAPI;
  /** 系统能力总入口（taskbar/backdrop/dialog 等，平台差异见 README 能力矩阵）。 */
  sys: Record<string, (params?: unknown) => Promise<any>>;
  clipboard: { read(): Promise<string>; write(text: string): Promise<void> };
  shell: { open(url: string): Promise<void> };
  notification: { show(opts: { title: string; body?: string }): Promise<void> };
  shortcut: { register(combo: string, fn: () => void): Promise<() => void> };
  autostart: { enable(): Promise<void>; disable(): Promise<void>; isEnabled(): Promise<boolean> };
  protocol: { register(scheme: string): Promise<void>; unregister(scheme: string): Promise<void> };
  app: { launchArgs(): Promise<string[]> };
  path: { get(kind: string, name?: string): Promise<string> };
  store: {
    get(namespace: string, key: string): Promise<any>;
    set(namespace: string, key: string, value: unknown): Promise<void>;
    delete(namespace: string, key: string): Promise<void>;
    keys(namespace: string): Promise<string[]>;
  };
  os: { info(): Promise<Record<string, any>> };
  process: { exit(): Promise<void>; restart(): Promise<void> };
  /** 自动更新：check 验签后缓存条目；install 仅认 check 结果，进度经 update.* 事件回推。 */
  update: { check(): Promise<void>; install(): Promise<void> };
  taskbar: Record<string, (params?: unknown) => Promise<any>>;
  dialog: {
    message(opts: { title?: string; text: string; type?: string }): Promise<any>;
    open(opts?: Record<string, any>): Promise<string | string[] | null>;
    save(opts?: Record<string, any>): Promise<string | null>;
  };
  tray: Record<string, (params?: unknown) => Promise<any>>;
  menu: Record<string, (params?: unknown) => Promise<any>>;
}

declare global {
  interface Window {
    freedom: FreedomAPI;
  }
}
