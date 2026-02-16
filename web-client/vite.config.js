import { defineConfig } from 'vite'
import solidPlugin from 'vite-plugin-solid'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [
    solidPlugin(),
    tailwindcss(),
  ],
  base: process.env.VITE_BASE_PATH || '/',
  build: {
    target: 'esnext',
  },
})
