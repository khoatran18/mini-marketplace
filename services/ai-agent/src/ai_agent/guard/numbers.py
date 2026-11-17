"""Rút và chuẩn hoá con số trong văn bản, dùng CHUNG cho kiểm chứng RCA (`rca/grounding.py`) và đo câu trả lời chat (`guard/grounding.py`),
để "5,000", "5.000", "5000" và "5k" là cùng một số ở cả hai nơi (trước đây RCA và chat chuẩn hoá khác nhau).

Một mẩu số có thể đọc theo hai nghĩa ("1.500" = một nghìn rưỡi hoặc 1,5), nên mỗi mẩu trả về TẬP các dạng chuẩn; hai mẩu khớp nếu có dạng chung.
Dạng chuẩn: không dấu phân nhóm, dấu thập phân là '.', bỏ số 0 thừa cuối phần thập phân."""
from __future__ import annotations

import re

_TOKEN = re.compile(r"(?<![\w.])\d+(?:[.,]\d+)*(?:\s?[kK](?![A-Za-z0-9]))?")
_GROUPED = re.compile(r"^\d{1,3}(?:[.,]\d{3})+$")


def _trim(s: str) -> str:
    if "." in s:
        s = s.rstrip("0").rstrip(".")
    return s or "0"


def forms(token: str) -> frozenset[str]:
    """Các dạng chuẩn của một mẩu số (xem đầu tệp)."""
    t = token.strip()
    kilo = t[-1] in "kK"
    if kilo:
        t = t[:-1].strip()
    out: set[str] = set()
    if _GROUPED.match(t):  # "5,000" / "1.250.000": có thể là dấu phân nhóm
        out.add(re.sub(r"[.,]", "", t))
    if t.count(",") + t.count(".") <= 1:  # có tối đa một dấu: có thể là dấu thập phân
        out.add(_trim(t.replace(",", ".")))
    elif not out:  # nhiều dấu mà không thành nhóm 3 chữ số (vd 10.0.0.5, 2026.11.17): giữ nguyên làm mã
        out.add(t)
    if kilo:
        out = {_trim(str(round(float(f) * 1000, 6))) for f in out if re.fullmatch(r"\d+(?:\.\d+)?", f)} or out
    return frozenset(out)


def digits(form: str) -> int:
    return len(re.sub(r"\D", "", form.split(".")[0]))  # số chữ số phần nguyên


def extract(text: str, min_digits: int = 1) -> list[frozenset[str]]:
    """Mọi mẩu số trong văn bản, mỗi mẩu là tập dạng chuẩn; bỏ mẩu mà phần nguyên (mọi cách đọc) ngắn hơn `min_digits` chữ số."""
    res = []
    for m in _TOKEN.finditer(text):
        fs = forms(m.group(0))
        if fs and max(digits(f) for f in fs) >= min_digits:
            res.append(fs)
    return res


def pool(*texts: str) -> set[str]:
    """Hợp mọi dạng chuẩn của mọi con số trong các văn bản (để tra thành viên)."""
    have: set[str] = set()
    for t in texts:
        for fs in extract(t):
            have |= fs
    return have


def missing(claims: str, *sources: str, min_digits: int = 1) -> tuple[int, int]:
    """(số con số trong `claims`, số con số không có trong các nguồn). Hai con số khớp nếu có chung một dạng chuẩn."""
    mine = extract(claims, min_digits)
    have = pool(*sources)
    uniq = {tuple(sorted(f)) for f in mine}
    bad = sum(1 for f in uniq if not (set(f) & have))
    return len(uniq), bad
