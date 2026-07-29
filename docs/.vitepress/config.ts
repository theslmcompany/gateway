import { defineConfig } from 'vitepress'
import type { Plugin } from 'vite'

// ── Single source of truth ────────────────────────────────────────────────────
// Change this one line and every code example, command, and
// configuration snippet updates automatically.
const GATEWAY_URL = 'https://gateway.theslmcompany.com'

// Vite plugin: replaces {{GATEWAY_URL}} in raw .md files before they're parsed
function gatewayUrlPlugin(): Plugin {
  return {
    name: 'gateway-url-replace',
    enforce: 'pre',
    transform(code, id) {
      if (id.endsWith('.md')) {
        return code.replace(/\{\{GATEWAY_URL\}\}/g, GATEWAY_URL)
      }
    },
  }
}

// ── Base path: /docs in production, / in development ──────────────────────────
const base = process.env.DOCS_BASE || '/'

// ── Config ────────────────────────────────────────────────────────────────────
export default defineConfig({
  title: 'AI Gateway',
  description: 'Secure, observable middleware for agentic AI systems',
  lang: 'en-US',
  base,

  vite: {
    plugins: [gatewayUrlPlugin()],
  },

  themeConfig: {
    nav: [
      { text: 'Home', link: '/' },
      { text: 'Quickstart', link: '/quickstart' },
      { text: 'Getting Started', link: '/getting-started' },
      { text: 'Integration', link: '/integration' },
    ],

    sidebar: [
      {
        text: 'Welcome',
        items: [
          { text: 'Overview', link: '/' },
          { text: 'Quickstart (self-host)', link: '/quickstart' },
          { text: 'Getting Started', link: '/getting-started' },
        ],
      },
      {
        text: 'Connect',
        items: [
          { text: 'Integration Guide', link: '/integration' },
          { text: 'CLI (aigw)', link: '/cli' },
          { text: 'API Keys', link: '/api-keys' },
          { text: 'Provider Keys (Upstreams)', link: '/upstreams' },
        ],
      },
      {
        text: 'Manage',
        items: [
          { text: 'Tenants', link: '/tenants' },
          { text: 'Agents', link: '/agents' },
          { text: 'Members', link: '/members' },
          { text: 'Single Sign-On (SSO)', link: '/sso' },
        ],
      },
      {
        text: 'Safety & Visibility',
        items: [
          { text: 'Guardrails', link: '/guardrails' },
          { text: 'Router', link: '/router' },
          { text: 'Traces', link: '/traces' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'Glossary', link: '/reference/glossary' },
        ],
      },
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/theslmcompany/gateway' },
    ],

    search: {
      provider: 'local',
    },

    outline: 'deep',
  },
})

export { GATEWAY_URL }
