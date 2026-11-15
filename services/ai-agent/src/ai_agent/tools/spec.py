"""Tool = một dòng của bảng gateway (GET /ai/tools). Dựng schema tham số cho mô hình và URL gọi từ đó, không viết tay."""
from __future__ import annotations

import re
import keyword
from dataclasses import dataclass, field
from typing import Any, Literal, Optional
from urllib.parse import quote

from pydantic import BaseModel, ConfigDict, Field, create_model

_PATH_VALUE = re.compile(r"^[A-Za-z0-9_-]{1,64}$")
_TYPES: dict[str, Any] = {"string": str, "integer": int, "number": float}


@dataclass(frozen=True)
class ParamSpec:
    name: str
    where: str  # "path" | "query" | "body" (chỉ ở hành động đề xuất)
    type: str
    required: bool = False
    description: str = ""
    enum: tuple[str, ...] = ()


@dataclass(frozen=True)
class ToolSpec:
    name: str
    method: str
    path: str  # mẫu route, vd /orders/:id
    description: str
    params: tuple[ParamSpec, ...] = field(default_factory=tuple)

    @staticmethod
    def from_json(d: dict[str, Any]) -> "ToolSpec":
        return ToolSpec(
            name=d["name"], method=d["method"], path=d["path"], description=d.get("description", ""),
            params=tuple(
                ParamSpec(p["name"], p["in"], p.get("type", "string"), bool(p.get("required")), p.get("description", ""), tuple(p.get("enum") or ()))
                for p in d.get("params") or []
            ),
        )


@dataclass(frozen=True)
class ActionSpec(ToolSpec):
    """Một hành động agent chỉ được ĐỀ XUẤT (gateway: `actions` trong GET /ai/tools). summary là mẫu câu người dùng đọc ở thẻ xác nhận."""
    summary: str = ""

    @staticmethod
    def from_json(d: dict[str, Any]) -> "ActionSpec":
        base = ToolSpec.from_json(d)
        return ActionSpec(name=base.name, method=base.method, path=base.path, description=base.description, params=base.params,
                          summary=d.get("summary", ""))


MAX_BODY_TEXT = 300


def py_name(name: str) -> str:
    """Từ khoá Python (from) làm tên trường: LangChain bỏ trường đó khỏi schema gửi mô hình, nên mô hình không bao giờ điền được. Đổi thành from_."""
    return name + "_" if keyword.iskeyword(name) else name


def restore_names(spec: ToolSpec, args: dict[str, Any]) -> dict[str, Any]:
    """Ngược lại của py_name: trả tham số về tên thật của gateway (from_ -> from)."""
    back = {py_name(p.name): p.name for p in spec.params}
    return {back.get(k, k): v for k, v in args.items()}


def args_model(spec: ToolSpec) -> type[BaseModel]:
    """Mô hình pydantic cho tham số của tool. Tham số lạ bị từ chối (extra=forbid): mô hình không nhét thêm được store_id, v.v."""
    fields: dict[str, Any] = {}
    for p in spec.params:
        t: Any = Literal[p.enum] if p.enum else _TYPES.get(p.type, str)  # type: ignore[valid-type]
        extra: dict[str, Any] = {"max_length": MAX_BODY_TEXT} if (p.where == "body" and t is str) else {}
        if p.required:
            fields[py_name(p.name)] = (t, Field(..., description=p.description, **extra))
        else:
            fields[py_name(p.name)] = (Optional[t], Field(None, description=p.description, **extra))
    return create_model(f"{spec.name}_args", __config__=ConfigDict(extra="forbid"), **fields)


def build_request(spec: ToolSpec, args: dict[str, Any]) -> tuple[str, dict[str, Any]]:
    """Điền :param của mẫu route và gom query. Giá trị path chỉ nhận [A-Za-z0-9_-] để không chèn được '/' hay '..'."""
    path = spec.path
    query: dict[str, Any] = {}
    for p in spec.params:
        if p.where == "body":
            continue  # body chỉ có ở hành động đề xuất, do build_action xử lý
        v = args.get(p.name)
        if v is None or v == "":
            if p.required:
                raise ValueError(f"thiếu tham số {p.name}")
            continue
        if p.where == "path":
            sv = str(v)
            if not _PATH_VALUE.match(sv):
                raise ValueError(f"giá trị không hợp lệ cho {p.name}")
            path = path.replace(f":{p.name}", quote(sv, safe=""))
        else:
            query[p.name] = v
    if ":" in path:
        raise ValueError("route còn tham số chưa điền")
    return path, query


class _SafeDict(dict):
    def __missing__(self, key: str) -> str:
        return "?"


def build_action(spec: ActionSpec, args: dict[str, Any]) -> tuple[str, dict[str, Any] | None, str]:
    """(đường dẫn thật, body, câu tóm tắt). Giá trị path chỉ nhận [A-Za-z0-9_-]; body chỉ gồm tham số khai báo; tóm tắt do mã điền
    từ chính các giá trị này nên luôn khớp với việc sẽ chạy (mô hình không tự viết câu người dùng đọc)."""
    path, _ = build_request(spec, {p.name: args.get(p.name) for p in spec.params if p.where == "path"})
    body: dict[str, Any] = {}
    for p in spec.params:
        if p.where != "body":
            continue
        v = args.get(p.name)
        if v is None or v == "":
            if p.required:
                raise ValueError(f"thiếu tham số {p.name}")
            continue
        if p.type == "integer" and (not isinstance(v, int) or isinstance(v, bool) or v == 0):
            raise ValueError(f"{p.name} phải là số nguyên khác 0")
        body[p.name] = v
    shown = {k: (f"{v:+d}" if isinstance(v, int) and k == "delta" else v) for k, v in {**args, **body}.items() if v is not None}
    summary = spec.summary.format_map(_SafeDict(shown)) if spec.summary else spec.name
    reason = body.get("reason")
    if reason:
        summary = f"{summary}. Lý do: {reason}"
    return path, (body or None), summary[:200]
