import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { VitePWA } from 'vite-plugin-pwa'

const apiProxyTarget = process.env.VITE_API_PROXY_TARGET || 'http://localhost:8080'

export default defineConfig({
  plugins: [
    react(),
    VitePWA({
      registerType: 'autoUpdate',
      includeAssets: ['favicon.ico', 'robots.txt'],
      manifest: {
        name: 'CleanCaregent',
        short_name: 'CleanCare',
        description: 'AI-powered cleaning care assistant',
        theme_color: '#2563eb',
        background_color: '#ffffff',
        display: 'standalone',
        icons: [
          {
            src: 'favicon.ico',
            sizes: '64x64 32x32 16x16',
            type: 'image/x-icon',
          },
        ],
      },
      workbox: {
        // Cache the app shell (HTML, CSS, JS)
        globPatterns: ['**/*.{js,css,html,ico,svg,woff2}'],
        // Runtime caching strategies
        runtimeCaching: [
          {
            // Cache conversation list API responses (stale-while-revalidate)
            urlPattern: /^\/api\/v1\/conversations(\?.*)?$/,
            handler: 'StaleWhileRevalidate',
            options: {
              cacheName: 'api-conversations',
              expiration: {
                maxEntries: 50,
                maxAgeSeconds: 24 * 60 * 60, // 24 hours
              },
              cacheableResponse: {
                statuses: [200],
              },
            },
          },
          {
            // Cache messages API responses (stale-while-revalidate)
            urlPattern: /^\/api\/v1\/conversations\/[^/]+\/messages/,
            handler: 'StaleWhileRevalidate',
            options: {
              cacheName: 'api-messages',
              expiration: {
                maxEntries: 100,
                maxAgeSeconds: 7 * 24 * 60 * 60, // 7 days
              },
              cacheableResponse: {
                statuses: [200],
              },
            },
          },
          {
            // Cache trace API responses
            urlPattern: /^\/api\/v1\/traces\/[^/]+$/,
            handler: 'StaleWhileRevalidate',
            options: {
              cacheName: 'api-traces',
              expiration: {
                maxEntries: 100,
                maxAgeSeconds: 7 * 24 * 60 * 60, // 7 days
              },
              cacheableResponse: {
                statuses: [200],
              },
            },
          },
          {
            // Network-first for everything else (SSE, mutations, etc.)
            urlPattern: /^\/api\/.*/,
            handler: 'NetworkFirst',
            options: {
              cacheName: 'api-other',
              expiration: {
                maxEntries: 50,
                maxAgeSeconds: 24 * 60 * 60,
              },
              networkTimeoutSeconds: 10,
              cacheableResponse: {
                statuses: [200],
              },
            },
          },
        ],
      },
    }),
  ],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: apiProxyTarget,
        changeOrigin: true,
      },
    },
  },
})
