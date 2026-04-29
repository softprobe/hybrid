# docs.softprobe.dev — Documentation site

This directory is the **source** for [docs.softprobe.dev](https://docs.softprobe.dev). It is intentionally separate from `docs/` (internal design notes) so that user-facing content can be versioned, reviewed, and deployed independently.

The site is built with [VitePress](https://vitepress.dev) and deployed to [Cloudflare Pages](https://pages.cloudflare.com).

## Local development

```bash
cd docs-site
npm install
npm run docs:dev      # http://localhost:5173
```

## Build

```bash
npm run docs:build    # outputs to .vitepress/dist
npm run docs:preview  # serve the built site locally
```

## Deployment (Cloudflare Pages)

Configure a Cloudflare Pages project with:

- **Build command:** `cd docs-site && npm install && npm run docs:build`
- **Build output directory:** `docs-site/.vitepress/dist`
- **Environment variables:** `NODE_VERSION=20`
- **Custom domain:** `docs.softprobe.dev`

Pushes to `main` publish to production; PR branches get preview URLs automatically.

## Agent context (`public/ai-context.md`)

[`public/ai-context.md`](./public/ai-context.md) is copied to the live site as **`/ai-context.md`** (for example `https://docs.softprobe.dev/ai-context.md`). Published agent skills instruct models to read it first.

**Checklist when you change replay behavior, CLI flags, SDK APIs, or header/session rules:**

- Update `public/ai-context.md` in the same PR (or immediately after), including the `Last updated: YYYY-MM-DD` line at the top.
- Run `python3 scripts/validate-ai-context.py` or `make validate-ai-context` from the repo root before pushing.

## Writing guidelines

1. **One page, one outcome.** If a page has two distinct "I want to…" goals, split it.
2. **Show, then explain.** Lead with the command or code block a user would paste; follow with prose.
3. **Version every link that points outside the docs.** Prefer linking to `spec/` schemas with a pinned path.
4. **Keep concept pages free of CLI flags and SDK signatures.** Those belong in `reference/`.
5. **Every how-to ends with a "Next" section** linking to the next most useful page.

## Directory layout

```
docs-site/
├── .vitepress/
│   └── config.ts           # navigation, sidebar, theme
├── public/                 # static assets (favicon, og images) + ai-context.md for agents
├── index.md                # landing page
├── introduction.md
├── quickstart.md
├── installation.md
├── concepts/               # mental models, no commands
├── guides/                 # step-by-step how-tos
├── reference/              # API/CLI/schema reference
├── deployment/             # runtime ops
└── faq.md
```
