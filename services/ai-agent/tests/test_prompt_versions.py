"""Mỗi persona có phiên bản prompt ghi trong personas/VERSIONS.md; dòng cuối của persona phải khớp băm file prompt.md hiện tại (sửa prompt mà quên nâng version thì test đỏ)."""
import hashlib
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1] / "src" / "ai_agent" / "personas"


def _rows():
    rows: dict[str, list[tuple[str, str]]] = {}
    for line in (ROOT / "VERSIONS.md").read_text(encoding="utf-8").splitlines():
        m = re.match(r"\|\s*([a-z]+)\s*\|\s*(v\d+)\s*\|\s*[\d-]+\s*\|\s*([0-9a-f]{12}|\([^)]*\))\s*\|", line)
        if m:
            rows.setdefault(m.group(1), []).append((m.group(2), m.group(3)))
    return rows


def test_every_persona_prompt_has_a_current_version_row():
    rows = _rows()
    for d in sorted(p for p in ROOT.iterdir() if (p / "prompt.md").exists()):
        assert d.name in rows, f"{d.name}: chưa có dòng nào trong VERSIONS.md"
        digest = hashlib.sha256((d / "prompt.md").read_bytes()).hexdigest()[:12]
        version, recorded = rows[d.name][-1]
        assert recorded == digest, f"{d.name}: prompt.md đã đổi (băm {digest}) nhưng dòng cuối VERSIONS.md ({version}) ghi {recorded}; thêm dòng version mới"
        nums = [int(v[1:]) for v, _ in rows[d.name]]
        assert nums == sorted(set(nums)), f"{d.name}: version phải tăng dần"
