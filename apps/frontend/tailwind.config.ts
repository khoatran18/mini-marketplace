import type { Config } from 'tailwindcss';

// Colours are CSS variables (see app/globals.css) so that light/dark share the same class names:
// use `bg-surface text-text border-line` instead of hard-coded `slate-*` classes.
const token = (name: string) => `rgb(var(--${name}) / <alpha-value>)`;

const config: Config = {
  darkMode: 'class',
  content: [
    './app/**/*.{js,ts,jsx,tsx}',
    './components/**/*.{js,ts,jsx,tsx}',
    './lib/**/*.{js,ts,jsx,tsx}'
  ],
  theme: {
    extend: {
      colors: {
        bg: token('bg'),
        surface: token('surface'),
        surface2: token('surface-2'),
        text: token('text'),
        muted: token('muted'),
        line: token('line'),
        brand: {
          DEFAULT: token('brand'),
          solid: token('brand-solid'),
          soft: token('brand-soft')
        },
        success: { DEFAULT: token('success'), soft: token('success-soft') },
        warning: { DEFAULT: token('warning'), soft: token('warning-soft') },
        danger: { DEFAULT: token('danger'), soft: token('danger-soft') },
        info: { DEFAULT: token('info'), soft: token('info-soft') }
      }
    }
  },
  plugins: []
};

export default config;
