'use client';

import type { ReactNode } from 'react';
import { track, type TrackContext, type TrackEventType } from '../../lib/tracking';

interface Props extends TrackContext {
  children: ReactNode;
  className?: string;
  type?: TrackEventType;
  props?: Record<string, unknown>;
}

/** Wraps children and records a click (default `product_click`) without changing their behaviour. */
export function TrackClick({ children, className, type = 'product_click', props, surface, item }: Props) {
  return (
    <div className={className} onClickCapture={() => track(type, props ?? {}, { surface, item })}>
      {children}
    </div>
  );
}
