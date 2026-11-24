# Kimiburn project website

Static site published to GitHub Pages at
<https://acevedomiguel.github.io/kimiburn/> by
`.github/workflows/pages.yml`. Plain HTML/CSS with optional vanilla
JavaScript — no build step, framework, or third-party resources. It shares its
structure and tooling with the RouteBurn site.

## Local preview

```sh
node tests/site/serve.mjs 8321
```

Open <http://localhost:8321/kimiburn/>. The helper server mounts the site at the
production base path `/kimiburn/` and serves `404.html` for missing paths, like
GitHub Pages. Run `node tests/site/check-links.mjs` to validate links before
merging, and `npm run test:site` for the browser checks.

## Conventions

- Keep everything self-hosted: no external fonts, scripts, images, or embeds.
- Keep scripts and styles in external files so the Content Security Policy in
  each page's `<head>` needs no `'unsafe-inline'`.
- All imagery must use synthetic data (see `assets-src/README.md` for how the
  terminal capture and social preview are generated). Never commit real
  account responses, tokens, or identifiers.
- `404.html` uses absolute `/kimiburn/`-prefixed asset links because GitHub
  Pages serves it for missing paths at any depth; the homepage uses relative
  links.
