import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  base: '/catflap/',
  plugins: [react(), tailwindcss()],
  server: {
    host: '0.0.0.0',
    port: 3010,
    strictPort: true,
    proxy: {
      '/api': 'http://localhost:3791',
      '/data': 'http://localhost:3791',
    },
  },
})
