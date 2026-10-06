'use client';

import { useEffect, useRef, type ReactNode } from 'react';
import { track, type TrackContext } from '../../lib/tracking';

interface Props extends TrackContext {
  children: ReactNode;
  className?: string;
  /** visible share of the element that counts (default 50 %) */
  threshold?: number;
  /** how long it has to stay visible, ms (default 1 s) */
  dwellMs?: number;
}

/** Fires an `impression` once the element has been >= 50 % visible for >= 1 s (once per session and position). */
export function TrackImpression({ children, className, surface, item, threshold = 0.5, dwellMs = 1000 }: Props) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const element = ref.current;
    if (!element || typeof IntersectionObserver === 'undefined') return;
    let timer: ReturnType<typeof setTimeout> | null = null;
    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries.some((entry) => entry.isIntersecting && entry.intersectionRatio >= threshold);
        if (visible && timer === null) {
          timer = setTimeout(() => {
            track('impression', {}, { surface, item });
            observer.disconnect();
          }, dwellMs);
        } else if (!visible && timer !== null) {
          clearTimeout(timer);
          timer = null;
        }
      },
      { threshold: [threshold] }
    );
    observer.observe(element);
    return () => {
      observer.disconnect();
      if (timer !== null) clearTimeout(timer);
    };
  }, [surface, item?.product_id, item?.position, threshold, dwellMs]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div ref={ref} className={className}>
      {children}
    </div>
  );
}
