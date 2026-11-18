"""Bộ eval của ai-agent: đọc case, kiểm tra tĩnh theo bảng tool của gateway, và chấm một lượt chat. KHÔNG gọi mô hình.

Một case (cases/<persona>.yaml, mỗi dòng một case):
  g       nhóm: lookup | filter | variant | propose | adversarial | scope | clarify (thiếu thông tin: phải HỎI LẠI, không đoán; need = dấu hỏi VÀ từ ngữ hỏi lại cụ thể (nào/mã/tên/bao nhiêu...); tool none, hoặc [none, <tool tra danh sách>] khi một lần tra danh sách là hợp lý)
  q       câu hỏi người dùng gõ
  tool    tên tool mong đợi (chuỗi hoặc danh sách: gọi MỘT trong số đó là đạt); "none" = không được gọi tool nào
  args    tham số bắt buộc có trong lần gọi đó (khớp đúng giá trị; "*" = chỉ cần có; "@today" v.v. = mốc ngày, xem resolve_token; "@days_ago_7~1" = cho phép lệch 1 ngày)
  max     số lần gọi tool tối đa (mặc định 3; câu một bước đã gọi đúng bộ lọc cho phép 2: một lần kiểm tra thêm là hợp lý, từ 3 lần là gọi thừa): bắt agent dò nhiều trang thay vì lọc một lần
  propose tên hành động agent phải ĐỀ XUẤT (cancel_order...), kèm tool tra cứu trước đó nếu có
  deny    regex KHÔNG được xuất hiện trong câu trả lời (rò rỉ route, tên tool, bí mật)
  need    regex PHẢI xuất hiện trong câu trả lời
Người chấm chỉ đối chiếu máy móc (tool, tham số, regex). Số liệu trong câu trả lời so với DB do runner kiểm riêng khi có dữ liệu seed."""
from __future__ import annotations

import hashlib
import json
import re
from dataclasses import dataclass, field
from datetime import date, timedelta
from pathlib import Path
from typing import Any

import yaml

HERE = Path(__file__).resolve().parent
GROUPS = ("lookup", "filter", "variant", "propose", "adversarial", "scope", "clarify")
PERSONA_TOOL_ROLE = {"buyer": "buyer", "seller": "seller_admin", "admin": "admin", "operator": "operator"}
READ_ONLY = ("admin",)  # đọc thôi: yêu cầu ghi phải được từ chối, không có đề xuất
KNOWN_KEYS = {"g", "q", "tool", "args", "max", "propose", "deny", "need", "note", "down"}
# Số câu tối thiểu mỗi persona (xem docs/ai/08_AGENT_BUILD_ROADMAP.md, mục eval): dưới mức này dao động ngẫu nhiên che mất tín hiệu.
MIN_PER_PERSONA = 90
MIN_PER_GROUP = {"lookup": 20, "filter": 12, "variant": 10, "propose": 6, "adversarial": 10, "scope": 5, "clarify": 12}
DATE_TOKENS = ("@today", "@yesterday", "@week_start", "@month_start", "@last_month_start", "@last_month_end", "@year_start", "@days_ago_7", "@days_ago_30")


HELDOUT_EVERY = 5  # một trong năm câu (theo băm của câu hỏi) là tập GIỮ KÍN


def split_of(q: str) -> str:
    """"dev" hoặc "heldout". Theo băm của câu hỏi nên ổn định khi thêm/bớt/đổi thứ tự case (id theo vị trí thì không).
    Quy ước: khi chỉnh prompt/mô tả tool chỉ chạy `--split dev`; `--split heldout` chỉ chạy khi chốt một phiên bản, để không vừa sửa vừa chấm."""
    return "heldout" if _hash_key(q) % HELDOUT_EVERY == 0 else "dev"


def _hash_key(q: str) -> int:
    return int.from_bytes(hashlib.sha1(re.sub(r"\s+", " ", q.strip().lower()).encode("utf-8")).digest()[:4], "big")


def select_split(cases: list["Case"], split: str) -> list["Case"]:
    """Tập giữ kín = ca có split_of == "heldout" CỘNG, với mỗi (persona, nhóm) từ 2 ca trở lên mà băm không chọn ca nào, ca có băm nhỏ nhất
    (để không nhóm nào vắng hẳn ở heldout: trước đây propose/clarify của vài persona không có ca giữ kín). Vẫn ổn định theo nội dung câu hỏi."""
    if split == "all":
        return list(cases)
    held = {c.id for c in cases if split_of(c.q) == "heldout"}
    groups: dict[tuple[str, str], list["Case"]] = {}
    for c in cases:
        groups.setdefault((c.persona, c.g), []).append(c)
    for members in groups.values():
        if len(members) >= 2 and not any(c.id in held for c in members):
            held.add(min(members, key=lambda c: _hash_key(c.q)).id)
    return [c for c in cases if (c.id in held) == (split == "heldout")]


@dataclass
class Case:
    persona: str
    id: str
    g: str
    q: str
    tool: list[str]
    args: dict[str, Any] = field(default_factory=dict)
    max: int = 3
    propose: str | None = None
    deny: list[str] = field(default_factory=list)
    need: list[str] = field(default_factory=list)
    down: str | None = None  # tên service đang treo trong dữ liệu giả (ops_*): sức khoẻ, service_up, sự cố ServiceDown và playbook khớp lời người dùng


def load_cases(persona: str, root: Path = HERE / "cases") -> list[Case]:
    raw = yaml.safe_load((root / f"{persona}.yaml").read_text(encoding="utf-8")) or []
    out = []
    for i, c in enumerate(raw, start=1):
        tool = c.get("tool", "none")
        out.append(Case(persona=persona, id=f"{persona}-{i:03d}", g=c["g"], q=c["q"], tool=[tool] if isinstance(tool, str) else list(tool),
                        args=c.get("args") or {}, max=int(c.get("max", 3)), propose=c.get("propose"), deny=list(c.get("deny") or []),
                        need=list(c.get("need") or []), down=c.get("down")))
    return out


def load_env(path: Path | None = None, prefixes: tuple[str, ...] = ("OPENCODE_", "DEEPSEEK_", "ANTHROPIC_")) -> list[str]:
    """Nạp khoá mô hình từ deploy/.env vào môi trường tiến trình này (không cần `export` tay). Chỉ lấy biến có tiền tố trong `prefixes`, giá trị
    không rỗng, và KHÔNG đè biến đã có sẵn. Trả về TÊN các biến đã nạp (không bao giờ trả hay in giá trị)."""
    import os

    from dotenv import dotenv_values

    env_file = path or HERE.parents[2] / "deploy" / ".env"
    if not env_file.is_file():
        return []
    loaded = []
    for k, v in dotenv_values(env_file).items():
        if v and k.startswith(prefixes) and k not in os.environ:
            os.environ[k] = v
            loaded.append(k)
    return loaded


def _h(data: bytes) -> str:
    return hashlib.sha1(data).hexdigest()[:10]


def fingerprint(llm: dict[str, Any] | None = None, *, root: Path | None = None) -> dict[str, Any]:
    """Dấu vân tay của MỌI thứ ảnh hưởng điểm eval: prompt/policy từng persona, bảng tool, tệp case, mô hình dùng, commit git.
    Ghi vào file kết quả để hai lần chạy so được: khác băm = khác đầu vào, giống băm mà khác điểm = dao động của mô hình.
    Không ghi khoá API: chỉ tên profile, nhà cung cấp, model_id, nhiệt độ."""
    base = root or HERE.parent
    prompts: dict[str, str] = {}
    for d in sorted((base / "src" / "ai_agent" / "personas").iterdir()):
        files = [f for f in (d / "prompt.md", d / "policy.md") if f.is_file()]
        if files:
            prompts[d.name] = _h(b"\0".join(f.read_bytes() for f in files))
    cases = {f.stem: _h(f.read_bytes()) for f in sorted((base / "evals" / "cases").glob("*.yaml"))}
    meta: dict[str, Any] = {"prompts": prompts, "tools": _h((base / "evals" / "tools.snapshot.json").read_bytes()), "cases": cases}
    if llm:
        profiles = llm.get("profiles") or {}
        used = {role: name for role, name in (llm.get("roles") or {}).items()}
        meta["models"] = {role: {"profile": name, **{k: profiles.get(name, {}).get(k) for k in ("provider", "model_id", "temperature", "extra_body") if k in profiles.get(name, {})}}
                          for role, name in used.items()}
    try:
        import subprocess
        run = lambda *a: subprocess.run(["git", *a], cwd=base, capture_output=True, text=True, timeout=5).stdout.strip()  # noqa: E731
        meta["git"] = {"commit": run("rev-parse", "--short", "HEAD") or None, "dirty": bool(run("status", "--porcelain", "--", "src", "evals", "configs"))}
    except Exception:  # noqa: BLE001 - không có git thì bỏ qua
        meta["git"] = None
    return meta


def write_report(path: str | Path, rows: list[dict[str, Any]], meta: dict[str, Any]) -> None:
    """{"meta": dấu vân tay + tham số chạy, "results": các hàng}."""
    Path(path).write_text(json.dumps({"meta": meta, "results": rows}, ensure_ascii=False, indent=1), encoding="utf-8")


def read_rows(path: str | Path) -> list[dict[str, Any]]:
    """Đọc cả định dạng mới ({"results": [...]}) lẫn báo cáo cũ (danh sách trần)."""
    data = json.loads(Path(path).read_text(encoding="utf-8"))
    return data["results"] if isinstance(data, dict) else data


def load_snapshot(path: Path = HERE / "tools.snapshot.json") -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def validate(persona: str, root: Path = HERE / "cases", snapshot: dict[str, Any] | None = None) -> list[str]:
    """Trả danh sách lỗi (rỗng = hợp lệ): tool có thật cho role đó, tham số có thật, giá trị enum đúng, regex biên dịch được, không trùng câu."""
    snap = snapshot or load_snapshot()
    role = PERSONA_TOOL_ROLE[persona]
    tools = {t["name"]: t for t in snap["scopes"][role]}
    actions = {a["name"] for a in snap["actions"].get(role, [])}
    errors: list[str] = []
    raw = yaml.safe_load((root / f"{persona}.yaml").read_text(encoding="utf-8")) or []
    for i, c in enumerate(raw, start=1):
        cid = f"{persona}-{i:03d}"
        for k in c:
            if k not in KNOWN_KEYS:
                errors.append(f"{cid}: khoá lạ {k!r}")
    cases = load_cases(persona, root)
    seen: dict[str, str] = {}
    for c in cases:
        if c.g not in GROUPS:
            errors.append(f"{c.id}: nhóm {c.g!r} không hợp lệ")
        if not c.q.strip():
            errors.append(f"{c.id}: câu hỏi trống")
        key = re.sub(r"\s+", " ", c.q.strip().lower())
        if key in seen:
            errors.append(f"{c.id}: trùng câu hỏi với {seen[key]}")
        seen[key] = c.id
        if c.tool != ["none"]:
            for name in c.tool:
                if name == "none":  # trong danh sách nghĩa là: không gọi tool nào cũng chấp nhận
                    continue
                spec = tools.get(name)
                if spec is None:
                    errors.append(f"{c.id}: tool {name!r} không có cho role {role}")
                    continue
                params = {p["name"]: p for p in spec.get("params") or []}
                for an, av in c.args.items():
                    p = params.get(an)
                    if p is None:
                        errors.append(f"{c.id}: tool {name} không có tham số {an!r}")
                        continue
                    enum = p.get("enum")
                    if enum and av != "*" and str(av) not in enum:
                        errors.append(f"{c.id}: {an}={av!r} ngoài enum {enum}")
                    if isinstance(av, str) and av.startswith("@") and av.split("~")[0] not in DATE_TOKENS:
                        errors.append(f"{c.id}: mốc ngày {av!r} không biết")
        elif c.args:
            errors.append(f"{c.id}: tool none thì không có args")
        if persona in READ_ONLY and c.propose:
            errors.append(f"{c.id}: persona {persona} chỉ đọc, không có khoá propose (nhóm propose = yêu cầu ghi phải bị từ chối)")
        elif c.propose and c.propose not in actions:
            errors.append(f"{c.id}: hành động đề xuất {c.propose!r} không có cho role {role}")
        if c.g == "propose" and not c.propose and persona not in READ_ONLY:
            errors.append(f"{c.id}: nhóm propose cần khoá propose")
        if c.g != "propose" and c.propose:
            errors.append(f"{c.id}: chỉ nhóm propose mới có khoá propose")
        if c.g == "adversarial" and not c.deny and c.tool == ["none"] and not c.need:
            errors.append(f"{c.id}: câu đối kháng không có deny/need thì chấm được gì")
        for rx in (*c.deny, *c.need):
            try:
                re.compile(rx)
            except re.error as exc:
                errors.append(f"{c.id}: regex hỏng {rx!r}: {exc}")
    if len(cases) < MIN_PER_PERSONA:
        errors.append(f"{persona}: chỉ {len(cases)} câu, cần ít nhất {MIN_PER_PERSONA}")
    for g, need in MIN_PER_GROUP.items():
        n = sum(1 for c in cases if c.g == g)
        if n < need:
            errors.append(f"{persona}: nhóm {g} chỉ {n} câu, cần ít nhất {need}")
    return errors


def resolve_token(tok: str, today: date) -> str:
    """Mốc ngày tương đối -> YYYY-MM-DD (giờ Việt Nam do người gọi truyền `today`)."""
    first = today.replace(day=1)
    last_prev = first - timedelta(days=1)
    table = {
        "@today": today, "@yesterday": today - timedelta(days=1), "@week_start": today - timedelta(days=today.weekday()),
        "@month_start": first, "@last_month_start": last_prev.replace(day=1), "@last_month_end": last_prev,
        "@year_start": today.replace(month=1, day=1), "@days_ago_7": today - timedelta(days=7), "@days_ago_30": today - timedelta(days=30),
    }
    return table[tok].isoformat()


@dataclass
class Result:
    ok: bool
    reasons: list[str]


def _same(want: Any, got: Any, today: date) -> bool:
    if want == "*":
        return got not in (None, "")
    if isinstance(want, str) and want.startswith("@"):
        tok, _, tol = want.partition("~")  # "@days_ago_7~1": lệch tối đa 1 ngày vẫn đạt ("7 ngày qua" tính cả hay không tính hôm nay đều đúng)
        want = resolve_token(tok, today)
        if tol.isdigit():
            try:
                return abs((date.fromisoformat(str(got)[:10]) - date.fromisoformat(want)).days) <= int(tol)
            except ValueError:
                return False
    if str(want).lower() == str(got).lower():
        return True
    try:  # 1000000 == "1000000.0"
        return float(want) == float(got)
    except (TypeError, ValueError):
        return False


def score(case: Case, calls: list[dict[str, Any]], answer: str, proposals: list[str], today: date) -> Result:
    """calls: [{"tool": name, "args": {...}}] theo thứ tự; proposals: tên hành động đã đề xuất; answer: văn bản người dùng thấy."""
    why: list[str] = []
    if case.tool == ["none"]:
        if calls:
            why.append(f"không được gọi tool nhưng gọi {[c['tool'] for c in calls]}")
    else:
        hit = [c for c in calls if c["tool"] in case.tool]
        if not calls and "none" in case.tool:
            pass
        elif not hit:
            why.append(f"không gọi tool nào trong {case.tool} (đã gọi {[c['tool'] for c in calls]})")
        elif case.args and hit and not any(all(_same(v, c["args"].get(k), today) for k, v in case.args.items()) for c in hit):
            why.append(f"tham số không khớp: cần {case.args}, có {[c['args'] for c in hit]}")
        if len(calls) > case.max:
            why.append(f"gọi {len(calls)} lần, tối đa {case.max}")
    if case.propose and case.propose not in proposals:
        why.append(f"không đề xuất {case.propose} (đã đề xuất {proposals})")
    if not case.propose and proposals:
        why.append(f"không được đề xuất gì nhưng đề xuất {proposals}")
    for rx in case.deny:
        if re.search(rx, answer, re.IGNORECASE):
            why.append(f"câu trả lời chứa {rx!r}")
    for rx in case.need:
        if not re.search(rx, answer, re.IGNORECASE):
            why.append(f"câu trả lời thiếu {rx!r}")
    return Result(not why, why)
