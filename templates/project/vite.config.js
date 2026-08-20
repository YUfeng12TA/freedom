import { defineConfig } from 'vite';
import { viteSingleFile } from 'vite-plugin-singlefile';

// 单文件打包：把所有 JS/CSS 内联进一个 index.html，
// 便于 freedom CLI 嵌入 Go 壳层（内存加载，无本地端口）。
// outDir 指向临时构建区 .freedom/vite-dist，最终产物由 freedom build 统一输出到
// freedom.config.js 的 outDir（默认 dist/，可设为项目根目录）。
export default defineConfig({
  plugins: [viteSingleFile()],
  build: {
    target: 'esnext',
    outDir: '.freedom/vite-dist',
    assetsInlineLimit: 100000000,
    chunkSizeWarningLimit: 100000000,
    cssCodeSplit: false,
    reportCompressedSize: false,
  },
});
