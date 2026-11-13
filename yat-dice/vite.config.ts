import { defineConfig, loadEnv, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'

// Mağaza derlemesi sunucu adresi olmadan yapılırsa uygulama telefonda localhost'a bağlanmaya çalışır
// ve iOS/Android yalnızca https/wss bağlantılarına izin verir. Derleme bu durumda açıkça uyarır.
function apiUrlCheck(mode: string): Plugin {
  return {
    name: 'yat-api-url-check',
    apply: 'build',
    buildStart() {
      const env = loadEnv(mode, process.cwd(), 'VITE_')
      const problems: string[] = []
      if (!env.VITE_API_URL?.startsWith('https://')) problems.push('VITE_API_URL https:// ile başlamalı')
      if (!env.VITE_WS_URL?.startsWith('wss://')) problems.push('VITE_WS_URL wss:// ile başlamalı')
      if (problems.length) {
        this.warn(`Bu derleme mağazaya gönderilemez (${problems.join('; ')}). Uygulama sunucuya bağlanamaz.`)
      }
    },
  }
}

// https://vite.dev/config/
export default defineConfig(({ mode }) => ({
  plugins: [react(), apiUrlCheck(mode)],
}))
