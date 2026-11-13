'use client';

import { useEffect, useState } from 'react';
import { Icon } from './Icon';
import { applyTheme, readThemeMode, saveThemeMode, type ThemeMode } from '../../lib/theme';

const options: { mode: ThemeMode; label: string; icon: string }[] = [
  { mode: 'light', label: 'Sáng', icon: 'sun' },
  { mode: 'system', label: 'Hệ thống', icon: 'system' },
  { mode: 'dark', label: 'Tối', icon: 'moon' }
];

export function ThemeToggle() {
  const [mode, setMode] = useState<ThemeMode>('system');

  useEffect(() => {
    setMode(readThemeMode());
  }, []);

  // follow the OS while in "system" mode
  useEffect(() => {
    if (mode !== 'system') return;
    const query = window.matchMedia('(prefers-color-scheme: dark)');
    const onChange = () => applyTheme('system');
    query.addEventListener('change', onChange);
    return () => query.removeEventListener('change', onChange);
  }, [mode]);

  return (
    <div role="group" aria-label="Giao diện sáng/tối" className="inline-flex overflow-hidden rounded-xl border border-line text-sm">
      {options.map((option) => (
        <button
          key={option.mode}
          type="button"
          title={option.label}
          aria-label={option.label}
          aria-pressed={mode === option.mode}
          onClick={() => {
            setMode(option.mode);
            saveThemeMode(option.mode);
          }}
          className={`px-2.5 py-1.5 transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-brand ${
            mode === option.mode ? 'bg-brand-soft font-semibold text-brand' : 'bg-surface text-muted hover:bg-surface2'
          }`}
        >
          <Icon name={option.icon} className="h-4 w-4" />
        </button>
      ))}
    </div>
  );
}
