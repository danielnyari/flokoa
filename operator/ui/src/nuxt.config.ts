// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  modules: [
    '@nuxt/eslint',
    '@nuxt/ui',
    '@vueuse/nuxt',
    '@nuxt/test-utils'
  ],

  // Generate as SPA for embedding in the Go binary
  ssr: false,

  devtools: {
    enabled: true
  },

  css: ['~/assets/css/main.css'],

  // In development, proxy API requests to the operator server.
  // In production (embedded in Go binary), the Go server handles both.
  routeRules: {
    '/api/**': {
      proxy: `${process.env.FLOKOA_API_URL || 'http://localhost:8080'}/api/**`
    }
  },

  compatibilityDate: '2024-07-11',

  // Use static preset so `nuxt build` produces a self-contained SPA with index.html
  // (the default node-server preset renders the HTML shell at runtime via Nitro)
  nitro: {
    preset: 'static'
  },

  eslint: {
    config: {
      stylistic: {
        commaDangle: 'never',
        braceStyle: '1tbs'
      }
    }
  }
})
