"""Phát hiện "đề xuất ma": câu trả lời nói đã soạn đề xuất / bảo bấm Xác nhận, nhưng lượt đó KHÔNG tạo đề xuất nào.

Sự thật nằm ở `RunContext.proposals` (tools/propose.py chỉ tăng sau khi gateway nhận POST /ai/proposals; sự kiện `action_proposal` và
nút Xác nhận trên UI sinh ra từ đúng chỗ đó). Phần phải đoán chỉ là câu trả lời có TỰ NHẬN đã soạn hay không, nên kiểm theo từng câu,
sau khi bỏ dấu và bỏ markdown, với hai nhóm tín hiệu (đã soạn / bảo bấm nút) và loại câu phủ định, điều kiện, đề nghị ("chưa tạo được",
"nếu bạn muốn mình sẽ soạn", "mình có thể soạn"). Chỉ xét khi persona có tool đề xuất mà lượt không tạo đề xuất nào.

Đo/chặn theo `limits.ghost_proposal`: off | log (ghi log + số liệu) | notice (log + số liệu + thêm một dòng báo cho người dùng).
Không đổi câu trả lời của mô hình, chỉ nối thêm (token đã stream đi rồi, giao diện đang hiện)."""
from __future__ import annotations

import re
import unicodedata

MODES = ("off", "log", "notice")

NOTICE = ("\n\n⚠️ Lưu ý: mình chưa tạo được đề xuất nào nên sẽ không có nút Xác nhận và chưa có gì được thực hiện. "
          "Bạn nhắn lại yêu cầu (kèm mã đơn hoặc sản phẩm) để mình soạn lại nhé.")

_MD = re.compile(r"[*_`#>~]+")
_SPLIT = re.compile(r"[.!?;\n]+")
_VERB = r"(chuan bi|soan|tao|lap|gui|dua ra|len|de san)"
_PROPOSAL = r"(de xuat|de nghi|the xac nhan|yeu cau xac nhan)"
# (a) tự nhận đã soạn đề xuất  (b) bảo người dùng bấm nút / kiểm tra rồi xác nhận  (c) chỉ vị trí thẻ
_DONE = [  # tự nhận ĐÃ soạn: luôn tính
    re.compile(rf"\b(da|vua|xong|dang)\s+{_VERB}\s+(\w+\s+){{0,5}}?{_PROPOSAL}"),
    re.compile(rf"\b{_PROPOSAL}\s+(\w+\s+){{0,6}}?(da|duoc)\s+(san sang|chuan bi|soan|tao|lap|gui)"),
]
_BUTTON = [  # bảo bấm nút: chỉ tính khi cả câu trả lời không phải lời giải thích cách làm việc chung
    re.compile(r"\b(bam|nhan|click|chon|bam vao|nhan vao)\s+(vao\s+)?(nut\s+)?(xac nhan|dong y|duyet)\b"),
    re.compile(r"\bnut\s+(xac nhan|dong y)\b"),
    re.compile(rf"\b({_PROPOSAL}|the xac nhan|nut xac nhan)\s+(hien\s+)?(o\s+)?(ben\s+)?(duoi|ngay duoi|phia duoi)"),
]
# phủ định / điều kiện / đề nghị đứng TRƯỚC chỗ khớp trong cùng câu thì không coi là khẳng định đã làm
_HEDGE = re.compile(r"\b(chua|khong|ko|chang|neu|khi nao|khi|co the|muon|ban co muon|de minh|minh se|toi se|chi|luon|sau khi)\b")
# câu trả lời đang GIẢI THÍCH quy trình ("mình chỉ soạn đề xuất để bạn bấm Xác nhận", "luôn cần bước xác nhận"): lời chỉ nút trong đó không phải khẳng định đã tạo
_PROCESS = re.compile(r"\b(chi\s+(co the\s+)?(soan|gui|lap|tao|de xuat)|khong tu|luon can|sau khi (minh|toi)|de ban (kiem tra|bam)|moi (lan|thao tac))\b")


def normalize(text: str) -> str:
    """Chữ thường, bỏ dấu (đ -> d), bỏ ký hiệu markdown, gộp khoảng trắng; để khớp cả câu có dấu lẫn không dấu."""
    t = unicodedata.normalize("NFD", _MD.sub(" ", text or "").lower().replace("đ", "d"))
    t = "".join(c for c in t if unicodedata.category(c) != "Mn")
    return re.sub(r"[ \t]+", " ", t)


def claims_proposal(answer: str) -> bool:
    """Câu trả lời có câu nào tự nhận đã soạn đề xuất / bảo bấm nút Xác nhận (không phủ định, không điều kiện) không."""
    text = normalize(answer)
    pats = list(_DONE) if _PROCESS.search(text) else [*_DONE, *_BUTTON]
    for sentence in _SPLIT.split(text):
        s = sentence.strip()
        for pat in pats:
            m = pat.search(s) if s else None
            if m and not _HEDGE.search(s[: m.start()]):
                return True
    return False


def is_ghost(answer: str, proposals: int, *, can_propose: bool) -> bool:
    """Đúng khi persona có tool đề xuất, lượt không tạo đề xuất nào mà câu trả lời lại nói đã soạn."""
    return can_propose and proposals <= 0 and claims_proposal(answer)


def mode_of(value: object) -> str:
    """Giá trị `limits.ghost_proposal`; giá trị lạ thì lùi về `log` (chỉ đo), không bao giờ tự bật thông báo nhầm."""
    v = str(value or "notice").strip().lower()
    return v if v in MODES else "log"
