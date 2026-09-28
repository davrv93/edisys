import preset from '../packages/tokens/tailwind-preset.js';

/** @type {import('tailwindcss').Config} */
export default {
  presets: [preset],
  content: ['./src/**/*.{astro,js,jsx}'],
};
