import js from '@eslint/js';
import globals from 'globals';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';

// rounded-[…], shadow-[…], duration-[…], ease-[…], delay-[…], transition-all, animate-pulse
const VALORES_SUELTOS = '/(rounded(-[a-z]{1,2})?-\\[|shadow-\\[|duration-\\[|ease-\\[|delay-\\[|transition-all|animate-pulse)/';

export default [
  { ignores: ['dist', 'dev-dist', '.astro', 'eslint.config.js'] },
  {
    files: ['**/*.{js,jsx}'],
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: 'module',
      globals: { ...globals.browser, ...globals.node },
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
    plugins: { 'react-hooks': reactHooks, 'react-refresh': reactRefresh },
    rules: {
      ...js.configs.recommended.rules,
      ...reactHooks.configs.recommended.rules,
      'no-unused-vars': ['warn', { varsIgnorePattern: '^[A-Z_]', argsIgnorePattern: '^_' }],
      // Regla del design system: nada de diálogos nativos.
      'no-restricted-globals': ['error', 'alert', 'confirm', 'prompt'],
      'no-restricted-properties': [
        'error',
        { object: 'window', property: 'alert', message: 'Usa useDialog().' },
        { object: 'window', property: 'confirm', message: 'Usa useDialog().' },
        { object: 'window', property: 'prompt', message: 'Usa useDialog().' },
      ],
      // Regla de tokens v2 (plan de la segunda pasada, bloque A): nada de valores sueltos de
      // radio, sombra o duración, ni transition-all ni animate-pulse. Usa rounded-tarjeta/control/chip,
      // shadow-flotante, duration-rapida/media/lenta, transition-colors/transform y la clase «esqueleto».
      'no-restricted-syntax': [
        'error',
        ...['Literal', 'TemplateElement'].map((nodo) => ({
          selector: `${nodo}[${nodo === 'Literal' ? 'value' : 'value.raw'}=${VALORES_SUELTOS}]`,
          message: 'Valor suelto de radio, sombra o duración (o transition-all / animate-pulse): usa los tokens v2.',
        })),
      ],
    },
  },
];
