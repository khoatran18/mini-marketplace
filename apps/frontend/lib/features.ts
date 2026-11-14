'use client';

import { useEffect, useState } from 'react';
import { getFeatureFlagsRequest } from './api';
import type { FeatureFlags } from './types';

// Build-time switch to look at the AI UI before the backend flags are on (the chat then only shows the "coming soon" reply).
const PREVIEW = process.env.NEXT_PUBLIC_AI_UI_PREVIEW === 'true';

let cached: FeatureFlags | null = null;

/** GET /config/features once per page load. Flags default to off when the gateway is unreachable. */
export function useFeatureFlags(): FeatureFlags {
  const [flags, setFlags] = useState<FeatureFlags>(cached ?? { ai_chat: false, ai_ops: false });

  useEffect(() => {
    if (cached) return;
    let alive = true;
    getFeatureFlagsRequest()
      .then((value) => {
        cached = value;
        if (alive) setFlags(value);
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, []);

  return PREVIEW ? { ai_chat: true, ai_ops: true } : flags;
}
