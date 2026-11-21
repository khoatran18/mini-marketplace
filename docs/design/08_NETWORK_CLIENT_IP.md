# 08. Mạng Docker Swarm và IP của client

Kiến thức nền về việc **IP của người dùng đi từ trình duyệt đến gateway như thế nào**, vì sao giới hạn tốc độ "theo IP" cần nó, và cách
mô phỏng nhiều IP khi kiểm thử tải. Phát hiện ngày 2026-11-22 (xem `docs/planning/01_OPEN_ISSUES.md` S8).

Mức tin cậy từng phần được ghi rõ: **[đã kiểm]** là đo hoặc đọc trực tiếp trên máy này (Swarm một node); **[tài liệu]** là kiến thức chung
về Docker/Traefik/gin, hành vi khớp với kết quả đo nhưng chưa kiểm riêng trên phiên bản đang chạy.

## Trạng thái hiện tại và cách bật/tắt cấu hình thử tải

**Từ 2026-11-22 stack `marketplace` trên máy này đang chạy với `deploy/services.perf.yml`** (Traefik tin `10.0.0.0/24`, gateway `TRUSTED_PROXIES=10.0.0.0/24,10.0.1.0/24`):
mọi client ngoài giả được IP bằng `X-Forwarded-For`. **Chưa quay lại cấu hình bình thường.** Cập nhật dòng này khi quay lại.

```bash
# BẬT (đã dùng ngày 2026-11-22; giống scripts/deploy.sh, chỉ thêm services.perf.yml; chỉ Traefik và gateway khởi động lại)
docker compose --env-file deploy/.env -f deploy/services.yml -f deploy/services.perf.yml config \
  | sed -e '/^name:/d' -E -e 's/^( *published: )"([0-9]+)"$/\1\2/' | docker stack deploy --detach=true -c - marketplace
# TẮT (về cấu hình bình thường: TRUSTED_PROXIES rỗng, Traefik không có forwardedHeaders)
./scripts/deploy.sh services
# kiểm đang ở cấu hình nào
docker service inspect marketplace_api-gateway --format '{{range .Spec.TaskTemplate.ContainerSpec.Env}}{{println .}}{{end}}' | grep ^TRUSTED_PROXIES
```

## 1. Các mạng trong Docker Swarm

| Mạng | Loại | Dải trên máy này | Dùng để |
|---|---|---|---|
| `ingress` | overlay (Swarm) | `10.0.0.0/24` [đã kiểm] | Đường vào của cổng được **publish kiểu `ingress`** (routing mesh). Chỉ Traefik dùng (cổng 80) |
| `marketplace-net` | overlay (Swarm) | `10.0.1.0/24` [đã kiểm] | Mạng chung của các service: Traefik, gateway, mọi service Go, Redis, Postgres, Kafka... gọi nhau qua tên (DNS nội bộ) |
| `docker_gwbridge` | bridge (cục bộ) | (không kiểm) | Cầu nối mỗi node để container Swarm ra ngoài; không liên quan đến IP client |
| `bridge`, `host`, `none` | cục bộ | | Mạng mặc định của Docker, ngoài phạm vi tài liệu này |

- **Overlay** là mạng ảo trải qua nhiều node: container ở node khác nhau nói chuyện như cùng một LAN. Trên máy một node vẫn dùng overlay.
- Container nào gắn vào mạng nào thì có **một địa chỉ IP riêng trên mạng đó**. Traefik gắn vào **cả hai** mạng:
  `ingress=10.0.0.4`, `marketplace-net=10.0.1.38` [đã kiểm]. Gateway chỉ gắn vào `marketplace-net`.
- Mạng `ingress` là cơ chế của Swarm, không phải mạng của ứng dụng. Mạng `marketplace-net` do `deploy/*.yml` khai báo.

## 2. Hai kiểu publish cổng

| | `ingress` (routing mesh, mặc định) | `host` |
|---|---|---|
| Cổng được mở ở đâu | **mọi** node của Swarm | chỉ node đang chạy container |
| Đường đi | `client → máy:80 → mạng ingress (IPVS) → container` | `client → máy:80 → container` (như `docker run -p`) |
| Cân bằng tải giữa node | có | không (làm bằng DNS hoặc LB ngoài) |
| Container thấy IP nguồn | **không**: Swarm đổi địa chỉ nguồn (SNAT) sang địa chỉ của mạng ingress | **có**: IP thật của client |
| Hai bản cùng node | được | không (tranh cổng) |

Cổng 80 của Traefik hiện là `ingress` (`deploy/services.yml`, `PublishMode: ingress` [đã kiểm]). Hệ quả:

- Access log của Traefik ghi client là **`10.0.0.2`** cho mọi yêu cầu, kể cả yêu cầu gửi từ `127.0.0.1` kèm `X-Forwarded-For` giả [đã kiểm].
  `10.0.0.2` là địa chỉ nội bộ của lớp ingress, **không phải IP người dùng**.
- `ingress` làm việc ở tầng mạng (IP và cổng), **không đọc và không sửa header HTTP**. Nó chỉ làm mất *địa chỉ kết nối thật*. Header
  `X-Forwarded-For` mà client gửi vẫn đến Traefik nguyên vẹn.
- Chỉ cổng publish mới đi qua `ingress`. Lưu lượng giữa các service (Traefik → gateway, gateway → gRPC...) đi trực tiếp trên `marketplace-net`
  và giữ nguyên địa chỉ nguồn (Traefik → gateway có nguồn `10.0.1.38` [đã kiểm]).
- Traefik chọn backend bằng **IP của từng task** gateway (access log: `http://10.0.1.54:8080`, `http://10.0.1.55:8080` [đã kiểm]), không qua
  IP ảo (VIP) của service.

## 3. Đường đi của một yêu cầu và IP ở mỗi bước

```
trình duyệt (IP thật A)
   │ TCP đến máy:80
   ▼
mạng ingress  ── SNAT: địa chỉ nguồn A bị thay bằng 10.0.0.2
   ▼
Traefik  (10.0.0.4 trên ingress, 10.0.1.38 trên marketplace-net)
   │  thấy kết nối từ 10.0.0.2;  header X-Forwarded-For (XFF) xử lý theo forwardedHeaders
   ▼
api-gateway  (10.0.1.54 / 10.0.1.55, marketplace-net)
   │  thấy kết nối từ 10.0.1.38 (Traefik);  c.ClientIP() quyết định IP client theo TRUSTED_PROXIES
   ▼
giới hạn tốc độ theo IP (Redis, khoá rate_limit:<phạm vi>:<IP>), log, ...
```

Có **ba nơi độc lập** có thể làm mất IP thật hoặc XFF giả. Chỉ cần một nơi là đủ:

1. **Ingress** thay địa chỉ nguồn kết nối (mất IP thật). Không đụng header.
2. **Traefik** xử lý XFF theo `forwardedHeaders` (mục 4).
3. **Gateway** (gin) chỉ đọc XFF khi kết nối đến từ proxy trong `TRUSTED_PROXIES` (mục 5).

## 4. Traefik: `forwardedHeaders.trustedIPs`

Cấu hình entrypoint (tĩnh, đặt ở `command` của Traefik): `--entrypoints.web.forwardedHeaders.trustedIPs=<CIDR,...>`.

| Kết nối đến Traefik từ... | Traefik làm gì với `X-Forwarded-For` của client [tài liệu] |
|---|---|
| địa chỉ **không** nằm trong `trustedIPs` (mặc định, danh sách rỗng) | **bỏ** XFF client gửi, **ghi lại** bằng đúng địa chỉ của kết nối (ở đây là `10.0.0.2`) |
| địa chỉ **nằm trong** `trustedIPs` | **giữ** XFF của client và **nối thêm** địa chỉ của kết nối vào cuối |

Ví dụ, client gửi `X-Forwarded-For: 10.9.9.9`, kết nối đến từ `10.0.0.2`:
- không tin `10.0.0.2` ⇒ Traefik chuyển tiếp `X-Forwarded-For: 10.0.0.2` (header giả mất).
- tin `10.0.0.0/24` ⇒ Traefik chuyển tiếp `X-Forwarded-For: 10.9.9.9, 10.0.0.2`.

Cờ cùng họ: `forwardedHeaders.insecure=true` (tin mọi nguồn, chỉ để thử), `proxyProtocol.trustedIPs` (giao thức PROXY, khi có LB hỗ trợ).
Lưu ý: khai báo cờ Traefik bằng `command` thì **không dùng được biến môi trường `TRAEFIK_*` cùng lúc** [tài liệu].

## 5. Gateway: `TRUSTED_PROXIES` và `c.ClientIP()`

Gateway dùng gin. `engine.SetTrustedProxies(TRUSTED_PROXIES)` (`services/api-gateway/cmd/main.go`), giá trị là danh sách CIDR ngăn bằng dấu phẩy
(`envList` trong `internal/config/env_config.go`).

Cách `c.ClientIP()` chọn IP [tài liệu về gin; kết quả khớp phép đo]:

1. Lấy địa chỉ **kết nối trực tiếp** (RemoteAddr). Nếu nó **không** nằm trong `TRUSTED_PROXIES` ⇒ dùng luôn nó, **bỏ qua** mọi header XFF.
2. Nếu nằm trong danh sách tin cậy ⇒ đọc XFF **từ phải sang trái**, bỏ qua các địa chỉ cũng nằm trong danh sách tin cậy, lấy địa chỉ
   **đầu tiên không tin cậy** làm IP client.
3. Nếu **mọi** địa chỉ đều tin cậy (kể cả đặt `0.0.0.0/0`), không còn địa chỉ "không tin cậy" nên gin quay về địa chỉ kết nối trực tiếp.
   Vì vậy **không đặt `0.0.0.0/0`** nếu muốn đọc IP từ XFF.

`TRUSTED_PROXIES` rỗng (mặc định hiện tại, `deploy/.env.example`) = không tin proxy nào ⇒ luôn dùng địa chỉ kết nối. Đây là thiết kế an toàn
(client không giả IP được), nhưng khi cổng đi qua `ingress` thì địa chỉ kết nối là Traefik nên **mọi client chung một IP**.

## 6. "Trusted IP" của Traefik và "Trusted proxy" của gateway khác gì

| | Traefik `forwardedHeaders.trustedIPs` | Gateway `TRUSTED_PROXIES` |
|---|---|---|
| Câu hỏi nó trả lời | Có tin XFF mà **kết nối này** mang theo không? | Có tin XFF mà **kết nối này** mang theo không? |
| Nếu **không** tin | **ghi đè** XFF bằng địa chỉ kết nối (header client mất) | **bỏ qua** XFF, dùng địa chỉ kết nối làm IP client |
| Nếu tin | giữ XFF, **nối thêm** địa chỉ kết nối | đọc XFF từ phải, bỏ các địa chỉ tin cậy, lấy địa chỉ đầu tiên không tin cậy |
| Tác dụng phụ | chuyển tiếp header đã chỉnh xuống gateway | trả IP client cho giới hạn tốc độ, log, ... |

Hai thứ cùng ý nghĩa ("tin hay không tin người gửi header này") nhưng ở **hai bước khác nhau** của chuỗi, nên cả hai phải tin đúng người:
Traefik phải tin `10.0.0.2` (nơi kết nối từ ngoài đi vào), gateway phải tin Traefik (`10.0.1.38`) **và** các địa chỉ nối thêm trong XFF (`10.0.0.2`),
nếu không gateway coi `10.0.0.2` là client.

## 7. Ba cấu hình và hệ quả (đã thử trên máy này)

| Cấu hình | Traefik `trustedIPs` | Gateway `TRUSTED_PROXIES` | Kết quả [đã kiểm] |
|---|---|---|---|
| Mặc định | (rỗng) | (rỗng) | XFF giả bị bỏ; mọi client chung khoá `rate_limit:global:10.0.1.38` (IP Traefik) |
| Thử tải `deploy/services.perf.yml` | `10.0.0.0/24` | `10.0.0.0/24,10.0.1.0/24` | XFF giả được dùng: XFF 10.1.1.1 / 10.2.2.2 / 10.3.3.3 cho 3 khoá Redis riêng, bộ đếm riêng (299, 299, 299, 298) |
| Chỉ sửa gateway (không sửa Traefik) | (rỗng) | có dải | Không đủ: Traefik đã ghi đè XFF thành `10.0.0.2` [suy luận từ mục 4] |

Trong cấu hình thử tải, mọi client ngoài đến Traefik với `10.0.0.2`, nên tin `10.0.0.2` = **tin XFF của bất kỳ ai** (giả IP để né giới hạn
tốc độ). **Chỉ dùng cho thử tải, không dùng ở production.**

## 8. Cách đúng cho production (có IP thật)

IP thật chỉ có ở nơi nhận kết nối từ internet. Chọn một:

1. **Publish cổng Traefik kiểu `host`** (một node): Traefik thấy IP thật, ghi vào XFF. Gateway chỉ cần `TRUSTED_PROXIES` = dải của Traefik
   (`10.0.1.0/24`). Mất: cân bằng tải giữa node, và cập nhật Traefik phải dừng bản cũ trước.
2. **Cân bằng tải ngoài Swarm** (nginx, HAProxy, LB của cloud): LB thấy IP thật và đặt XFF (hoặc dùng giao thức PROXY). Traefik tin LB
   (`forwardedHeaders.trustedIPs=<IP LB>`), XFF đi tiếp thành `<IP thật>, <IP LB>`; gateway tin **cả dải Traefik và địa chỉ LB**.
3. **Traefik `global` + `host`** trên mọi node, chia tải bằng DNS nhiều địa chỉ hoặc LB ngoài.

Không có tuỳ chọn nào của `ingress` giữ nguyên IP nguồn của client (hạn chế đã biết của routing mesh) [tài liệu].

## 9. Mô phỏng nhiều IP khi kiểm thử tải (perf test)

- **IP giả chỉ là giá trị header `X-Forwarded-For` mỗi người dùng ảo gửi**; không cần mỗi người một container. Một tiến trình tạo tải (k6, Locust)
  mô phỏng hàng trăm đến hàng nghìn người dùng ảo.
- Cần cấu hình mục 7 (thử tải) để gateway **dùng** XFF đó. Hoặc đặt máy tạo tải **trong `marketplace-net`**, gọi thẳng gateway (không qua
  Traefik và ingress), `TRUSTED_PROXIES=10.0.1.0/24`: bỏ được cả ba lớp ở mục 3.
- IP khác nhau chỉ cần khi **kiểm chính bộ giới hạn tốc độ**. Muốn đo công suất hệ thống thì nâng giới hạn (`RATE_LIMIT_PER_MINUTE`,
  `AUTH_RATE_LIMIT_PER_MINUTE`, `AI_RATE_LIMIT_PER_MINUTE`, `AI_USER_RATE_LIMIT_PER_MINUTE`) thay vì giả IP.
- Giới hạn theo người dùng (`AI_USER_RATE_LIMIT_PER_MINUTE`, khoá theo `userID` từ JWT) **không** phụ thuộc IP: mỗi người dùng ảo cần một tài khoản riêng.

## 10. Cách kiểm

```bash
# bộ đếm riêng từng IP giả: mỗi IP lần đầu phải ra 299 (giới hạn chung 300/phút)
for ip in 10.1.1.1 10.2.2.2 10.1.1.1; do
  curl -s -o /dev/null -D - -H "Host: api.${PUBLIC_HOST:-marketplace.swarm.localhost}" -H "X-Forwarded-For: $ip" "http://127.0.0.1/products?page_size=1" \
    | grep -i x-ratelimit-remaining
done
# khoá Redis tương ứng
docker exec <redis container> redis-cli --scan --pattern 'rate_limit:*'
# IP mà Traefik thấy (cột đầu của access log)
docker service logs marketplace_traefik --since 1m | grep products
```

Nếu mọi IP giả cho cùng một bộ đếm giảm dần (299, 298, 297...) thì một trong ba lớp ở mục 3 đang bỏ XFF.

## 11. Giải thích thêm (hỏi đáp)

**Node và container.** *Node* là một máy (thật hoặc ảo) chạy Docker và tham gia Swarm; *container* là một tiến trình đóng gói chạy trên một node. Hệ thống
này hiện chỉ có **1 node** (`ubuntu-desktop`, vai trò manager/leader [đã kiểm]) và toàn bộ container (khoảng 33 container: service ứng dụng, Kafka,
Postgres, Redis, Prometheus, Grafana...) chạy trên node đó. Khi có nhiều node, Swarm rải container ra các máy.

**"Ingress mở cổng ở mọi node" nghĩa là gì.** Chỉ service **khai báo `ports`** mới có cổng publish (ở đây chỉ Traefik, cổng 80). Các service khác
(ai-agent, product...) **không** mở cổng ra ngoài, chỉ nói chuyện qua `marketplace-net`. Chữ "mọi node" chỉ có nghĩa khi có nhiều node: cổng 80 được mở trên
cả máy không chạy Traefik, và Swarm chuyển kết nối đến máy đang chạy Traefik. Với một node thì không có khác biệt.

**CIDR** là *Classless Inter-Domain Routing*, cách viết một dải địa chỉ IP: `10.0.0.0/24` nghĩa là 24 bit đầu cố định, tức 256 địa chỉ từ `10.0.0.0` đến
`10.0.0.255`. `/32` là đúng một địa chỉ, `/0` là mọi địa chỉ.

**Traefik là proxy à?** Có: là *reverse proxy* (nhận yêu cầu từ client rồi chuyển tiếp đến service phía sau). Gateway cũng chuyển tiếp yêu cầu (HTTP sang gRPC)
nhưng với gin nó là *máy chủ đọc IP client*, còn Traefik là *proxy đứng trước nó*. Vì vậy từ phía gateway, Traefik là "proxy".

**Vì sao Traefik gọi `trustedIPs` còn gin gọi `TrustedProxies`?** Chỉ là cách đặt tên của hai dự án khác nhau; cùng ý nghĩa ("danh sách địa chỉ/dải được tin khi mang header
chuyển tiếp"). Khác biệt kỹ thuật nằm ở hành vi (mục 6), không ở tên: nginx gọi `set_real_ip_from`, Express gọi `trust proxy`.

**Quy tắc "chỉ tin XFF từ proxy tin cậy" có phổ biến không, vì sao.** Rất phổ biến (nginx, Express, Django, gin, Traefik, Cloudflare đều có tương đương) [tài liệu].
Lý do: `X-Forwarded-For` chỉ là một header HTTP, **ai cũng tự đặt được**. Địa chỉ kết nối TCP thì không giả được (muốn nhận trả lời thì phải dùng đúng IP thật).
Nên cách làm đúng là: proxy ở **rìa** (nơi nhận kết nối từ internet) bỏ XFF client gửi và ghi IP kết nối thật của nó; mỗi proxy tiếp theo nối thêm
địa chỉ kết nối nó thấy. Chuỗi `client-gửi-giả, IP-thật, proxy1` có phần do client viết ở **trái** và phần do hạ tầng của mình viết ở **phải**. Máy chủ đọc từ phải
sang trái, bỏ qua các proxy của mình, và dừng ở địa chỉ đầu tiên không phải proxy của mình: đó là địa chỉ cuối cùng mà hạ tầng của mình *tự quan sát* được; phần
client viết thêm bên trái không bao giờ được xét. Proxy được tin **không cần công nghệ chống giả riêng**; điều cần là nó là hop đầu tiên thấy kết nối thật và
ghi địa chỉ đó (không tin header của client). Cấu hình thử tải phá điều này (tin mọi thứ đến từ `10.0.0.2`) nên ai cũng giả được IP: chỉ dùng khi thử.

**Traefik có ghi đè XFF nếu nguồn không trong `trustedIPs` không?** Có [tài liệu]; kết quả thử khớp (trước khi cấu hình, XFF giả không có tác dụng).

**Trong `services.perf.yml`, Traefik tin những gì?** Tin dải mạng ingress hiện có `10.0.0.0/24`, **không** tin mọi nguồn (`0.0.0.0/0`). Vì mọi client ngoài đều đến Traefik
qua ingress (địa chỉ `10.0.0.2`), kết quả thực tế là tin XFF của mọi client ngoài, nhưng không tin các container khác trong mạng `marketplace-net`.

**IP của Traefik và gateway có đổi không, làm sao luôn tin đúng chỗ.** *Có đổi.* Sau lần khởi động lại ngày 2026-11-22: Traefik `10.0.1.38` → `10.0.1.66`; hai bản gateway
`10.0.1.54` và `10.0.1.55` → `10.0.1.67` và `10.0.1.68` [đã kiểm]. Mỗi lần container được tạo lại, nó nhận một địa chỉ mới trong dải. Vì vậy **không bao giờ tin một địa chỉ đơn lẻ**
mà tin **cả dải mạng** (`10.0.1.0/24` cho `marketplace-net`, `10.0.0.0/24` cho `ingress`): dải do Swarm đặt khi tạo mạng và chỉ đổi nếu tạo lại mạng hoặc khởi tạo lại Swarm.
Muốn dải cố định hơn thì khai báo `ipam` với subnet tường minh cho mạng trong file compose (chưa làm). Với production nên tin dải nhỏ nhất mà chỉ có proxy của mình
nằm trong đó (hoặc địa chỉ của LB cố định), không tin cả mạng dùng chung với mọi service.

**Proxy đầu tiên và các proxy sau.** Proxy ở rìa nhận kết nối trực tiếp từ client nên biết địa chỉ thật: nó bỏ XFF client gửi và ghi địa chỉ thật đó. Các proxy
sau tin nó vì nó là hạ tầng của mình (địa chỉ kết nối nằm trong danh sách trusted), không phải vì tin giá trị header.

**Tin một dải mạng nghĩa là tin mọi máy trong dải.** `TRUSTED_PROXIES=10.0.1.0/24` làm gateway tin mọi container có địa chỉ trong `marketplace-net` (ai-agent,
product, Kafka...), không riêng Traefik: bất kỳ container nào trong mạng cũng gọi gateway kèm XFF giả và được tin. Chấp nhận ở môi trường thử;
ở production tin dải nhỏ nhất chỉ chứa proxy của mình hoặc đúng địa chỉ LB cố định.

**Ý nghĩa `/N` trong CIDR.** IPv4 có 32 bit (4 số, mỗi số 8 bit); `/N` giữ N bit đầu cố định, N càng lớn dải càng nhỏ.

| CIDR | Phần cố định | Số địa chỉ | Ví dụ |
|---|---|---|---|
| `/32` | cả 4 số | 1 | `10.0.1.66` |
| `/24` | 3 số đầu | 256 | `10.0.1.0` đến `10.0.1.255` |
| `/16` | 2 số đầu | 65 536 | `10.0.0.0` đến `10.0.255.255` |
| `/0` | không | tất cả | mọi địa chỉ |

## 12. Liên quan

- `deploy/services.perf.yml`: cấu hình thử tải; áp dụng bằng cách gộp vào `services.yml` (xem đầu file); quay lại bình thường bằng `scripts/deploy.sh services`.
- `docs/planning/01_OPEN_ISSUES.md` S8: giới hạn "theo IP" đang đếm chung (production cần mục 8).
- `docs/instruction/02_DEPLOYMENT.md`: triển khai Swarm.
- Lưu ý khi gộp file compose: `docker compose` 5.x **ghi đè** `command` (không nối thêm), nên `services.perf.yml` chép lại đủ cờ Traefik;
  `services.https.yml` có thể bị ảnh hưởng (xem S8).
