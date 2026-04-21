import { defineConfig } from 'vitepress';

export default defineConfig({
  title: 'Softprobe',
  description:
    'Capture production HTTP traffic, replay it deterministically in tests. Cross-language, proxy-first, CI-ready.',
  lang: 'en-US',
  cleanUrls: true,
  lastUpdated: true,
  srcExclude: ['**/README.md'],

  head: [
    ['link', { rel: 'icon', href: '/favicon.svg', type: 'image/svg+xml' }],
    ['meta', { name: 'theme-color', content: '#0b7285' }],
    ['meta', { property: 'og:title', content: 'Softprobe documentation' }],
    [
      'meta',
      {
        property: 'og:description',
        content:
          'Capture real HTTP traffic and replay it as deterministic tests — in TypeScript, Python, Java, or Go.',
      },
    ],
    ['meta', { property: 'og:url', content: 'https://docs.softprobe.dev' }],
  ],

  themeConfig: {
    logo: '/logo.svg',

    nav: [
      { text: 'Quick Start', link: '/quickstart' },
      { text: 'Concepts', link: '/concepts/architecture' },
      { text: 'Guides', link: '/guides/capture-your-first-session' },
      { text: 'Reference', link: '/reference/cli' },
      {
        text: 'v0.5',
        items: [
          { text: 'Release notes', link: 'https://github.com/softprobe/softprobe/releases' },
          { text: 'Roadmap', link: 'https://github.com/softprobe/softprobe/projects' },
        ],
      },
    ],

    sidebar: {
      '/': [
        {
          text: 'Getting started',
          items: [
            { text: 'Introduction', link: '/introduction' },
            { text: 'Quick start', link: '/quickstart' },
            { text: 'Installation', link: '/installation' },
          ],
        },
        {
          text: 'Concepts',
          items: [
            { text: 'Architecture', link: '/concepts/architecture' },
            { text: 'Sessions & cases', link: '/concepts/sessions-and-cases' },
            { text: 'Capture & replay', link: '/concepts/capture-and-replay' },
            { text: 'Rules & policy', link: '/concepts/rules-and-policy' },
          ],
        },
        {
          text: 'How-to guides',
          items: [
            { text: 'Capture your first session', link: '/guides/capture-your-first-session' },
            { text: 'Replay in a Jest test', link: '/guides/replay-in-jest' },
            { text: 'Replay in pytest', link: '/guides/replay-in-pytest' },
            { text: 'Replay in JUnit', link: '/guides/replay-in-junit' },
            { text: 'Replay in Go', link: '/guides/replay-in-go' },
            { text: 'Mock an external dependency', link: '/guides/mock-external-dependency' },
            { text: 'Run a suite at scale', link: '/guides/run-a-suite-at-scale' },
            { text: 'Write a hook', link: '/guides/write-a-hook' },
            { text: 'CI integration', link: '/guides/ci-integration' },
            { text: 'Troubleshooting', link: '/guides/troubleshooting' },
          ],
        },
        {
          text: 'Reference',
          items: [
            { text: 'CLI', link: '/reference/cli' },
            { text: 'Suite YAML', link: '/reference/suite-yaml' },
            { text: 'TypeScript SDK', link: '/reference/sdk-typescript' },
            { text: 'Python SDK', link: '/reference/sdk-python' },
            { text: 'Java SDK', link: '/reference/sdk-java' },
            { text: 'Go SDK', link: '/reference/sdk-go' },
            { text: 'Session headers', link: '/reference/session-headers' },
            { text: 'HTTP control API', link: '/reference/http-control-api' },
            { text: 'Case file schema', link: '/reference/case-schema' },
          ],
        },
        {
          text: 'Deployment',
          items: [
            { text: 'Local (Docker Compose)', link: '/deployment/local' },
            { text: 'Kubernetes', link: '/deployment/kubernetes' },
            { text: 'Hosted (o.softprobe.ai)', link: '/deployment/hosted' },
          ],
        },
        { text: 'FAQ', link: '/faq' },
      ],
    },

    socialLinks: [
      { icon: 'github', link: 'https://github.com/softprobe' },
      { icon: 'x', link: 'https://x.com/softprobe' },
    ],

    editLink: {
      pattern: 'https://github.com/softprobe/softprobe/edit/main/docs-site/:path',
      text: 'Edit this page on GitHub',
    },

    search: {
      provider: 'local',
    },

    footer: {
      message: 'Released under the Apache-2.0 license.',
      copyright: 'Copyright © 2026 Softprobe Inc.',
    },
  },
});
