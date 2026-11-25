import js from '@eslint/js'
import prettier from 'eslint-config-prettier'
import pluginVue from 'eslint-plugin-vue'
import globals from 'globals'
import tseslint from 'typescript-eslint'

export default tseslint.config(
  {
    ignores: [
      'dist/**',
      'node_modules/**',
      'playwright-report/**',
      'test-results/**',
      'src/api/schema.d.ts',
    ],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  ...pluginVue.configs['flat/recommended'],
  {
    files: ['**/*.vue'],
    languageOptions: { parserOptions: { parser: tseslint.parser } },
  },
  {
    // shadcn-vue components are copied in from upstream and keep its style.
    files: ['src/components/ui/**'],
    rules: { 'vue/multi-word-component-names': 'off', 'vue/require-default-prop': 'off' },
  },
  {
    // Node scripts (the e2e runner, the dist cleaner) and tool config files run
    // under Node, not in the browser, so they get Node's globals.
    files: ['e2e/**/*.{mjs,ts}', 'scripts/**/*.mjs', '*.config.{js,ts,mjs}'],
    languageOptions: { globals: { ...globals.node } },
  },
  prettier,
)
