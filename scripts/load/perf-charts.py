#!/usr/bin/env python3
"""Vẽ biểu đồ SVG từ dữ liệu thô của một đợt đo (chỉ dùng thư viện chuẩn).

Dùng:  python3 scripts/load/perf-charts.py docs/perf-results/2026-11-23-dot6
Ra:    <thư mục>/charts/{closed,open}-{cpu,ram}.svg và lag.svg

Mỗi biểu đồ là một lưới ô nhỏ, mỗi ô một service (các bản gộp thành một đường), vì các service chênh nhau cả trăm lần nên mỗi ô có trục Y riêng.
Trong ô: đường liền = mức dùng trung bình (CPU) hoặc đỉnh (RAM), đường chấm = đỉnh CPU, nét đứt = mức được cấp (tổng các bản),
vòng đỏ = mức tải mà service bị throttle >= 10% chu kỳ CPU. Trục X là các mức tải. Chọn vòng đo theo bảng PICK bên dưới.
"""
import json, os, sys

PICK = {"closed-400": "r2"}          # đóng 400 r1 bị nhiễu CPU máy, dùng r2
CLOSED = [100, 200, 300, 400, 500, 600, 700, 800]
OPEN = [10, 20, 30, 40, 50]
# (nhãn, [service trong số liệu]); thứ tự = thứ tự ô
PANELS = [
    ("traefik", ["marketplace_traefik"]), ("frontend", ["marketplace_frontend"]), ("api-gateway", ["marketplace_api-gateway"]),
    ("auth-service", ["marketplace_auth-service"]), ("user-service", ["marketplace_user-service"]), ("product-service", ["marketplace_product-service"]),
    ("order-service", ["marketplace_order-service"]), ("payment-service", ["marketplace_payment-service"]), ("analytics-service", ["marketplace_analytics-service"]),
    ("ops-service", ["marketplace_ops-service"]), ("ai-agent", ["marketplace_ai-agent"]),
    ("postgres", ["marketplace-infra_postgres"]), ("redis", ["marketplace-infra_redis"]),
    ("kafka (3 broker)", ["marketplace-infra_broker1", "marketplace-infra_broker2", "marketplace-infra_broker3"]),
    ("clickhouse", ["marketplace-infra_clickhouse"]),
    ("vector", ["marketplace-obs_vector"]), ("prometheus", ["marketplace-obs_prometheus"]), ("loki", ["marketplace-obs_loki"]),
    ("tempo", ["marketplace-obs_tempo"]), ("cadvisor", ["marketplace-obs_cadvisor"]), ("grafana", ["marketplace-obs_grafana"]),
    ("exporter (4)", ["marketplace-obs_node-exporter", "marketplace-obs_postgres-exporter", "marketplace-obs_redis-exporter", "marketplace-obs_kafka-exporter"]),
]
INK, INK2, GRID, SURF = "#0b0b0b", "#52514e", "#e3e2dc", "#fcfcfb"
BLUE, BLUE_L, LIMIT, RING = "#2a78d6", "#86b6ef", "#52514e", "#d03b3b"


def fmt(v):
    s = f"{v:.2f}".rstrip("0").rstrip(".") if v < 10 else f"{v:.0f}"
    return s.replace(".", ",")


def load(d, kind, lv):
    base = f"{kind}-{lv}"
    base += "-" + PICK.get(base, "r1")
    res = json.load(open(f"{d}/{base}.resources.json"))["services"]
    thr = {}
    for r in json.load(open(f"{d}/{base}.throttle.json"))["data"]["result"]:
        thr[r["metric"]["container_label_com_docker_swarm_service_name"]] = float(r["value"][1]) * 100
    run = json.load(open(f"{d}/{base}.json"))
    return res, thr, run


def series(data, members, what):
    """what: cpu | ram. Trả (avg[], peak[], limit, thr[]) theo các mức tải."""
    avg, peak, thr = [], [], []
    limit = 0
    for res, th, _ in data:
        a = p = 0.0
        t = 0.0
        limit = 0.0
        for m in members:
            s = res.get(m)
            if not s:
                continue
            n = s["replicas"] or 1
            if what == "cpu":
                a += s["cpu_pct_of_core"]["avg_all_replicas"] / 100
                p += s["cpu_pct_of_core"]["max_all_replicas"] / 100
                limit += n * s["cpu_limit_cores_per_replica"]
            else:
                a += s["ram_max_mib"] * n
                p += s["ram_max_mib"] * n
                limit += n * s["ram_limit_mib"]
            t = max(t, th.get(m, 0.0))
        avg.append(a); peak.append(p); thr.append(t)
    return avg, peak, limit, thr


def grid_svg(d, kind, levels, what, out, title, unit):
    data = [load(d, kind, lv) for lv in levels]
    cols, pw, ph = 4, 330, 190
    rows = (len(PANELS) + cols - 1) // cols
    W, H = cols * pw + 20, rows * ph + 110
    o = [f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width="{W}" font-family="system-ui,Segoe UI,Roboto,sans-serif">',
         f'<rect width="{W}" height="{H}" fill="{SURF}"/>',
         f'<text x="16" y="28" font-size="18" font-weight="600" fill="{INK}">{title}</text>',
         f'<text x="16" y="48" font-size="12" fill="{INK2}">Trục X: mức tải ({"VU" if kind=="closed" else "phiên/giây"}). Mỗi ô một service (các bản gộp), trục Y riêng, đơn vị {unit}.</text>']
    lx = 16
    items = [("mức dùng " + ("trung bình" if what == "cpu" else "đỉnh"), "line", BLUE, "", 2)]
    if what == "cpu":
        items.append(("đỉnh (mẫu 3 s)", "line", BLUE_L, "1.5 3", 1.5))
    items.append(("trần được cấp (tổng các bản)", "line", LIMIT, "6 4", 1.5))
    if what == "cpu":
        items.append(("throttle >= 10% chu kỳ CPU", "ring", RING, "", 2))
    for label, kind_, col, dash, sw in items:
        if kind_ == "line":
            o.append(f'<line x1="{lx}" y1="68" x2="{lx+26}" y2="68" stroke="{col}" stroke-width="{sw}"' + (f' stroke-dasharray="{dash}"' if dash else "") + "/>")
        else:
            o.append(f'<circle cx="{lx+13}" cy="68" r="5" fill="none" stroke="{col}" stroke-width="2"/>')
        o.append(f'<text x="{lx+32}" y="72" font-size="12" fill="{INK2}">{label}</text>')
        lx += 32 + 7 * len(label) + 28
    for i, (name, members) in enumerate(PANELS):
        avg, peak, limit, thr = series(data, members, what)
        cx, cy = 10 + (i % cols) * pw, 90 + (i // cols) * ph
        L, R, T, B = cx + 46, cx + pw - 18, cy + 30, cy + ph - 36
        ymax = max(limit * 1.12, max(peak) * 1.05, 1e-9)
        X = lambda k: L + (R - L) * (k / (len(levels) - 1))
        Y = lambda v: B - (B - T) * v / ymax
        o.append(f'<text x="{cx+8}" y="{cy+16}" font-size="13" font-weight="600" fill="{INK}">{name}</text>')
        for tv in (0, limit / 2, limit):
            o.append(f'<line x1="{L}" y1="{Y(tv):.1f}" x2="{R}" y2="{Y(tv):.1f}" stroke="{GRID}" stroke-width="1"/>')
            o.append(f'<text x="{L-5}" y="{Y(tv)+4:.1f}" font-size="10" fill="{INK2}" text-anchor="end">{fmt(tv)}</text>')
        for k, lv in enumerate(levels):
            o.append(f'<text x="{X(k):.1f}" y="{B+14}" font-size="10" fill="{INK2}" text-anchor="middle">{lv}</text>')
        o.append(f'<line x1="{L}" y1="{Y(limit):.1f}" x2="{R}" y2="{Y(limit):.1f}" stroke="{LIMIT}" stroke-width="1.5" stroke-dasharray="6 4"/>')
        o.append(f'<text x="{R}" y="{Y(limit)-4:.1f}" font-size="10" fill="{INK2}" text-anchor="end">trần {fmt(limit)}</text>')
        if what == "cpu":
            o.append(f'<polyline fill="none" stroke="{BLUE_L}" stroke-width="1.5" stroke-dasharray="1.5 3" points="' + " ".join(f"{X(k):.1f},{Y(v):.1f}" for k, v in enumerate(peak)) + '"/>')
        o.append(f'<polyline fill="none" stroke="{BLUE}" stroke-width="2" stroke-linejoin="round" points="' + " ".join(f"{X(k):.1f},{Y(v):.1f}" for k, v in enumerate(avg)) + '"/>')
        first = None
        for k, v in enumerate(avg):
            o.append(f'<circle cx="{X(k):.1f}" cy="{Y(v):.1f}" r="3" fill="{BLUE}" stroke="{SURF}" stroke-width="2"/>')
            if what == "cpu" and thr[k] >= 10:
                o.append(f'<circle cx="{X(k):.1f}" cy="{Y(v):.1f}" r="6" fill="none" stroke="{RING}" stroke-width="2"/>')
                first = levels[k] if first is None else first
        if first is not None:
            o.append(f'<text x="{cx+8}" y="{cy+28}" font-size="10" fill="{RING}">throttle từ {first}</text>')
    o.append("</svg>")
    open(out, "w").write("\n".join(o))


def lag_svg(d, out):
    W, H, pw, ph = 1000, 330, 470, 250
    o = [f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width="{W}" font-family="system-ui,Segoe UI,Roboto,sans-serif">',
         f'<rect width="{W}" height="{H}" fill="{SURF}"/>',
         f'<text x="16" y="28" font-size="18" font-weight="600" fill="{INK}">Độ trễ payment (tạo đơn -> thanh toán xuất hiện), p95 và lớn nhất</text>',
         f'<text x="16" y="46" font-size="12" fill="{INK2}">Trục Y dạng log (ms). Đo bằng hỏi lại mỗi 500 ms nên các giá trị ~1008 là số lượng tử hoá. Không phải consumer lag Kafka (không được ghi trong các lượt này).</text>']
    for j, (kind, levels, name) in enumerate((("closed", CLOSED, "Đóng (VU)"), ("open", OPEN, "Mở (phiên/giây)"))):
        data = [load(d, kind, lv) for lv in levels]
        p95 = [x[2]["payment"]["lag"]["p95"] for x in data]
        mx = [x[2]["payment"]["lag"]["max"] for x in data]
        to = [x[2]["payment"]["timeoutsOver8s"] for x in data]
        cx, cy = 10 + j * (pw + 20), 60
        L, R, T, B = cx + 56, cx + pw - 16, cy + 30, cy + ph - 30
        import math
        lo, hi = math.log10(300), math.log10(40000)
        X = lambda k: L + (R - L) * (k / (len(levels) - 1))
        Y = lambda v: B - (B - T) * (math.log10(max(v, 300)) - lo) / (hi - lo)
        o.append(f'<text x="{cx+8}" y="{cy+16}" font-size="13" font-weight="600" fill="{INK}">{name}</text>')
        for tv in (500, 1000, 2000, 5000, 10000, 20000):
            o.append(f'<line x1="{L}" y1="{Y(tv):.1f}" x2="{R}" y2="{Y(tv):.1f}" stroke="{GRID}"/><text x="{L-5}" y="{Y(tv)+4:.1f}" font-size="10" fill="{INK2}" text-anchor="end">{tv}</text>')
        for k, lv in enumerate(levels):
            o.append(f'<text x="{X(k):.1f}" y="{B+16}" font-size="10" fill="{INK2}" text-anchor="middle">{lv}</text>')
        o.append(f'<polyline fill="none" stroke="{BLUE_L}" stroke-width="1.5" stroke-dasharray="1.5 3" points="' + " ".join(f"{X(k):.1f},{Y(v):.1f}" for k, v in enumerate(mx)) + '"/>')
        o.append(f'<polyline fill="none" stroke="{BLUE}" stroke-width="2" points="' + " ".join(f"{X(k):.1f},{Y(v):.1f}" for k, v in enumerate(p95)) + '"/>')
        for k, v in enumerate(p95):
            o.append(f'<circle cx="{X(k):.1f}" cy="{Y(v):.1f}" r="3" fill="{BLUE}" stroke="{SURF}" stroke-width="2"/>')
            if to[k]:
                o.append(f'<circle cx="{X(k):.1f}" cy="{Y(v):.1f}" r="6" fill="none" stroke="{RING}" stroke-width="2"/>')
        o.append(f'<line x1="{cx+60}" y1="{cy+ph+4}" x2="{cx+86}" y2="{cy+ph+4}" stroke="{BLUE}" stroke-width="2"/><text x="{cx+92}" y="{cy+ph+8}" font-size="11" fill="{INK2}">p95</text>'
                 f'<line x1="{cx+140}" y1="{cy+ph+4}" x2="{cx+166}" y2="{cy+ph+4}" stroke="{BLUE_L}" stroke-width="1.5" stroke-dasharray="1.5 3"/><text x="{cx+172}" y="{cy+ph+8}" font-size="11" fill="{INK2}">lớn nhất</text>'
                 f'<circle cx="{cx+250}" cy="{cy+ph+4}" r="5" fill="none" stroke="{RING}" stroke-width="2"/><text x="{cx+260}" y="{cy+ph+8}" font-size="11" fill="{INK2}">có đơn quá 8 s</text>')
    o.append("</svg>")
    open(out, "w").write("\n".join(o))


FIX = {"auth-service": "#2a78d6", "ai-agent": "#eb6834", "postgres": "#1baf7a", "frontend": "#eda100", "api-gateway": "#e87ba4", "order-service": "#008300", "traefik": "#6250d6"}


def combined_svg(d, kind, levels, what, out, title):
    """Một biểu đồ chung: Y = % mức được cấp (mức dùng / trần), nên mọi service dùng chung một trục và nét đứt trần của mọi service trùng nhau ở 100%."""
    data = [load(d, kind, lv) for lv in levels]
    W, H, L, R, T, B = 1120, 560, 60, 880, 96, 500
    top = 120
    X = lambda k: L + (R - L) * (k / (len(levels) - 1))
    Y = lambda v: B - (B - T) * min(v, top) / top
    o = [f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width="{W}" font-family="system-ui,Segoe UI,Roboto,sans-serif">',
         f'<rect width="{W}" height="{H}" fill="{SURF}"/>',
         f'<text x="16" y="28" font-size="18" font-weight="600" fill="{INK}">{title}</text>',
         f'<text x="16" y="48" font-size="12" fill="{INK2}">Y = mức dùng / trần được cấp (%), nên trần của mọi service là cùng một đường nét đứt ở 100%. Service có màu = 7 service chính; xám = còn lại (chỉ ghi tên nếu ở mức tải cuối quá 60% trần).</text>',
         f'<text x="16" y="68" font-size="12" fill="{INK2}">' + ("Đường liền = ĐỈNH CPU của bản bận nhất / trần mỗi bản (mẫu 3 s); đường chấm = CPU trung bình. Vòng đỏ = mức tải bị throttle >= 10% chu kỳ CPU." if what == "cpu" else "Đường liền = RAM đỉnh / trần.") + "</text>"]
    for tv in (0, 25, 50, 75, 100):
        o.append(f'<line x1="{L}" y1="{Y(tv):.1f}" x2="{R}" y2="{Y(tv):.1f}" stroke="{GRID}"/><text x="{L-6}" y="{Y(tv)+4:.1f}" font-size="11" fill="{INK2}" text-anchor="end">{tv}%</text>')
    o.append(f'<line x1="{L}" y1="{Y(100):.1f}" x2="{R}" y2="{Y(100):.1f}" stroke="{LIMIT}" stroke-width="1.5" stroke-dasharray="6 4"/><text x="{L+4}" y="{Y(100)-5:.1f}" font-size="11" fill="{INK2}">trần được cấp (100%)</text>')
    for k, lv in enumerate(levels):
        o.append(f'<text x="{X(k):.1f}" y="{B+18}" font-size="11" fill="{INK2}" text-anchor="middle">{lv}</text>')
    o.append(f'<text x="{(L+R)/2}" y="{B+38}" font-size="12" fill="{INK2}" text-anchor="middle">mức tải ({"VU" if kind=="closed" else "phiên/giây"})</text>')
    rows = []
    for name, members in PANELS:
        avg, peak, limit, thr = series(data, members, what)
        if what == "cpu":  # trần áp cho TỪNG bản, nên lấy bản bận nhất (cpu_pct_of_limit_max), không phải tổng các bản
            pct = [max((res.get(m, {}).get("cpu_pct_of_limit_max", 0) for m in members), default=0) for res, _, _ in data]
            mean = [v / limit * 100 if limit else 0 for v in avg]
        else:
            pct = [v / limit * 100 if limit else 0 for v in avg]
            mean = None
        rows.append((name, pct, thr, mean))
    for name, pct, thr, mean in sorted(rows, key=lambda r: (r[0] in FIX, max(r[1]))):  # màu vẽ sau cùng nằm trên
        col = FIX.get(name, "#b4b3ab")
        o.append(f'<polyline fill="none" stroke="{col}" stroke-width="{2 if name in FIX else 1.2}" stroke-linejoin="round" points="' + " ".join(f"{X(k):.1f},{Y(v):.1f}" for k, v in enumerate(pct)) + '"/>')
        if mean and name in FIX:
            o.append(f'<polyline fill="none" stroke="{col}" stroke-width="1.2" stroke-dasharray="1.5 3" opacity="0.8" points="' + " ".join(f"{X(k):.1f},{Y(v):.1f}" for k, v in enumerate(mean)) + '"/>')
        if name in FIX:
            for k, v in enumerate(pct):
                o.append(f'<circle cx="{X(k):.1f}" cy="{Y(v):.1f}" r="3" fill="{col}" stroke="{SURF}" stroke-width="2"/>')
        if what == "cpu":
            for k, v in enumerate(pct):
                if thr[k] >= 10 and (name in FIX or v >= 60):
                    o.append(f'<circle cx="{X(k):.1f}" cy="{Y(v):.1f}" r="6" fill="none" stroke="{RING}" stroke-width="2"/>')
    # nhãn đầu bên phải, tránh chồng nhau
    lab = sorted([(Y(p[-1]), n, p) for n, p, _, _ in rows if n in FIX or p[-1] >= 60])
    prev = -99
    for y, n, p in lab:
        yy = max(y, prev + 14); prev = yy
        col = FIX.get(n, INK2)
        o.append(f'<text x="{R+10}" y="{yy+4:.1f}" font-size="12" fill="{INK}">{n} ({p[-1]:.0f}%)</text><rect x="{R+2}" y="{yy-3:.1f}" width="6" height="6" fill="{col}"/>')
    o.append("</svg>")
    open(out, "w").write("\n".join(o))


if __name__ == "__main__":
    d = sys.argv[1]
    os.makedirs(f"{d}/charts", exist_ok=True)
    for kind, levels, lab in (("closed", CLOSED, "tải đóng"), ("open", OPEN, "tải mở")):
        grid_svg(d, kind, levels, "cpu", f"{d}/charts/{kind}-cpu.svg", f"CPU từng service theo mức {lab}", "core")
        grid_svg(d, kind, levels, "ram", f"{d}/charts/{kind}-ram.svg", f"RAM từng service theo mức {lab}", "MiB")
        combined_svg(d, kind, levels, "cpu", f"{d}/charts/{kind}-cpu-all.svg", f"CPU mọi service trên một biểu đồ, theo mức {lab} (% so với trần)")
        combined_svg(d, kind, levels, "ram", f"{d}/charts/{kind}-ram-all.svg", f"RAM mọi service trên một biểu đồ, theo mức {lab} (% so với trần)")
    lag_svg(d, f"{d}/charts/lag.svg")
    print("xong:", ", ".join(sorted(os.listdir(f"{d}/charts"))))
