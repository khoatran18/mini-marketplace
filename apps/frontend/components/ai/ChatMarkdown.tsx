import { Fragment, type ReactNode } from 'react';

// Minimal, safe renderer for the assistant's answers: **bold**, `code`, "- " / "1. " lists, paragraphs. It builds React
// elements only (never HTML strings), so model output cannot inject markup. Unfinished markers (while streaming) stay as text.

function inline(text: string, keyBase: string): ReactNode[] {
  const out: ReactNode[] = [];
  const re = /\*\*([^*\n]+)\*\*|`([^`\n]+)`/g;
  let last = 0;
  let match: RegExpExecArray | null;
  let i = 0;
  while ((match = re.exec(text)) !== null) {
    if (match.index > last) out.push(text.slice(last, match.index));
    out.push(
      match[1] !== undefined ? (
        <strong key={`${keyBase}b${i}`} className="font-semibold">
          {match[1]}
        </strong>
      ) : (
        <code key={`${keyBase}c${i}`} className="rounded bg-surface px-1 py-0.5 text-[0.85em]">
          {match[2]}
        </code>
      )
    );
    last = match.index + match[0].length;
    i += 1;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

const BULLET = /^\s*[-*•]\s+(.*)$/;
const NUMBERED = /^\s*(\d+)[.)]\s+(.*)$/;

export function ChatMarkdown({ text }: { text: string }) {
  const lines = text.split('\n');
  const blocks: ReactNode[] = [];
  let list: { ordered: boolean; items: string[] } | null = null;
  let para: string[] = [];

  const flushPara = () => {
    if (para.length === 0) return;
    const k = blocks.length;
    blocks.push(
      <p key={`p${k}`}>
        {para.map((line, i) => (
          <Fragment key={i}>
            {i > 0 ? <br /> : null}
            {inline(line, `p${k}l${i}`)}
          </Fragment>
        ))}
      </p>
    );
    para = [];
  };
  const flushList = () => {
    if (!list) return;
    const k = blocks.length;
    const items = list.items.map((item, i) => <li key={i}>{inline(item, `l${k}i${i}`)}</li>);
    blocks.push(list.ordered ? <ol key={`l${k}`} className="ml-5 list-decimal space-y-0.5">{items}</ol> : <ul key={`l${k}`} className="ml-5 list-disc space-y-0.5">{items}</ul>);
    list = null;
  };

  for (const line of lines) {
    const bullet = BULLET.exec(line);
    const numbered = bullet ? null : NUMBERED.exec(line);
    if (bullet || numbered) {
      flushPara();
      const ordered = numbered !== null;
      if (list && list.ordered !== ordered) flushList();
      list ??= { ordered, items: [] };
      list.items.push((bullet ? bullet[1] : numbered![2]) ?? '');
    } else if (line.trim() === '') {
      flushPara();
      flushList();
    } else {
      flushList();
      para.push(line);
    }
  }
  flushPara();
  flushList();
  return <div className="space-y-2">{blocks}</div>;
}
