import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// Code-Shield 前端构建配置
export default defineConfig({
  plugins: [react()],
  server: {
    // 开发环境将 /api 代理到后端服务（后端默认端口 8080）
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
  },
});