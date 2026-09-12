# Dependency maintenance — September 2026

Astro is aligned at 7.3.2 across the root, dashboard, and website. Vite is 8.3.0 and Biome is 2.5.13. The editor uses Tiptap 3.31.3, Base UI 1.8.0, React 19.3.0, and happy-dom 20.14.5. Compatible transitive dependencies and the CLI Node types are refreshed in the Bun lockfile.

Two root overrides are intentional:

- `prosemirror-model` 1.25.11 keeps every Tiptap/ProseMirror consumer on one model instance. Mixed versions caused list and blockquote commands to throw; the existing editor regressions cover this boundary.
- `fflate` 0.7.5 replaces Satori's exact 0.7.3 pin to resolve GHSA-px8p-9vwx-vf98. The website's generated image and documentation build is validated with the override.

Go updates include x/crypto 0.57.0, x/oauth2 0.37.0, x/sync 0.23.0, SQLite 1.58.0, and the transitive requirements selected by Go. Atlas and the native libSQL driver retain their current versions; migration CLI checksums remain pinned independently.

## Deferred upgrades

- Starlight stays at 0.41.11. Version 0.42.0 reproduced a native Satteri module-resolution failure during static MDX rendering in both the existing install and a clean frozen-lockfile install. Astro 7.3.2 works with 0.41.11.
- TypeScript 7, Lucide 1, and Clack 1 are major migrations, not part of this compatible maintenance update.
- Oat 0.8 remains separate from this release because it changes the website's pre-1.0 UI library contract.

## Validation

Run dashboard tests from `apps/dashboard` so Bun uses the Preact JSX configuration. The release checks include frontend lint, widget type-check and tests, both Astro checks and builds, the CLI build, backend tests and lint for default/cloud/self-hosted modes, and self-hosted container startup/restart checks. Migration 00011 requires eleven revision rows in the container assertion.

Biome reports no errors; five existing accessibility warnings remain in favicon SVGs and modal overlay event handlers. No lint rules were weakened. Package advisory scans and exact review evidence are recorded separately for each release.
