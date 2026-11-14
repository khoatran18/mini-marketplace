import { Fragment, type ReactNode } from 'react';

// Minimal, safe renderer for the assistant's answers: **bold** (may contain `code`), `code`, "#" headings, "- " / "1. " lists,
// | tables |, paragraphs. It builds React elements only (never HTML strings), so model output cannot inject markup.
// Unfinished markers (while streaming) stay as text.

const INLINE = /\*\*([^\n]+?)\*\*|`([^`\n]+)`/g;

function inline(text: string, keyBase: string, inBold = false): ReactNode[] {
  const out: ReactNode[] = [];
  const re = new RegExp(INLINE.source, 'g');
  let last = 0;
  let match: RegExpExecArray | null;
  let i = 0;
  while ((match = re.exec(text)) !== null) {
    if (match.index > last) out.push(text.slice(last, match.index));
    if (match[1] !== undefined && !inBold) {
      out.push(
        <strong key={`${keyBase}b${i}`} className="font-semibold">
          {inline(match[1], `${keyBase}b${i}`, true)}
        </strong>
      );
    } else if (match[2] !== undefined) {
      out.push(
        <code key={`${keyBase}c${i}`} className="rounded bg-surface px-1 py-0.5 text-[0.85em]">
          {match[2]}
        </code>
      );
    } else {
      out.push(match[0]);
    }
    last = match.index + match[0].length;
    i += 1;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

const BULLET = /^\s*[-*•]\s+(.*)$/;
const NUMBERED = /^\s*(\d+)[.)]\s+(.*)$/;
const HEADING = /^\s{0,3}#{1,6}\s+(.*)$/;
const TABLE_ROW = /^\s*\|.*\|\s*$/;
const TABLE_RULE = /^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$/;

function cells(row: string): string[] {
  return row.trim().replace(/^\|/, '').replace(/\|$/, '').split('|').map((c) => c.trim());
}

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
  const pushTable = (rows: string[]) => {
    const k = blocks.length;
    const head = cells(rows[0] ?? '');
    const body = rows.slice(2).map(cells);
    blocks.push(
      <div key={`t${k}`} className="overflow-x-auto rounded-lg border border-line">
        <table className="w-full border-collapse text-left text-xs">
          <thead className="bg-surface">
            <tr>
              {head.map((c, i) => (
                <th key={i} className="whitespace-nowrap border-b border-line px-2 py-1.5 font-semibold">
                  {inline(c, `t${k}h${i}`)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {body.map((r, ri) => (
              <tr key={ri} className="border-t border-line first:border-t-0">
                {head.map((_, ci) => (
                  <td key={ci} className="px-2 py-1.5 align-top">
                    {inline(r[ci] ?? '', `t${k}r${ri}c${ci}`)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    );
  };

  for (let n = 0; n < lines.length; n += 1) {
    const line = lines[n] ?? '';

    // A table is a header row, a |---|---| rule row, then zero or more rows. While streaming, a header without its rule yet stays text.
    if (TABLE_ROW.test(line) && TABLE_RULE.test(lines[n + 1] ?? '')) {
      flushPara();
      flushList();
      const rows = [line, lines[n + 1] ?? ''];
      n += 2;
      while (n < lines.length && TABLE_ROW.test(lines[n] ?? '')) {
        rows.push(lines[n] ?? '');
        n += 1;
      }
      n -= 1;
      pushTable(rows);
      continue;
    }

    const heading = HEADING.exec(line);
    if (heading) {
      flushPara();
      flushList();
      blocks.push(
        <p key={`h${blocks.length}`} className="font-semibold">
          {inline(heading[1] ?? '', `h${blocks.length}`)}
        </p>
      );
      continue;
    }

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
