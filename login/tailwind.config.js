import preset from "./tokens/tailwind-preset.js";

/** @type {import('tailwindcss').Config} */
export default {
  presets: [preset],
  content: ["./src/**/*.{ts,tsx}"],
};
