// The widget is written against the React API and runs on Preact. Every
// import of React, including those inside dependencies (Tiptap, Base UI,
// Floating UI), resolves to preact/compat so the bundle contains exactly one
// runtime. vite.config.ts applies these to the build and tests/setup.ts to
// bun test, which does not read the Vite config.
export const preactAliases: ReadonlyArray<readonly [specifier: string, target: string]> = [
  ['react', 'preact/compat'],
  ['react-dom', 'preact/compat'],
  ['react-dom/client', 'preact/compat/client'],
  ['react-dom/test-utils', 'preact/test-utils'],
  ['react/jsx-runtime', 'preact/jsx-runtime'],
  ['react/jsx-dev-runtime', 'preact/jsx-dev-runtime'],
]
