# Kiến trúc phân quyền cho agent: các kiểu hiện có, vì sao chọn gateway, công tắc dừng khẩn

> **Độ tin cậy (tra cứu 2026-11-18, bằng WebSearch, chưa đọc trang gốc đầy đủ).** Phần lớn nguồn là blog của hãng bán sản phẩm (Traefik, Gravitee, API7, Okta/Auth0, Microsoft) nên mỗi bên có lợi ích riêng khi mô tả. Không có chuẩn thống nhất cho "công tắc dừng agent". Các con số trích từ bài thứ cấp được đánh dấu. Phần "hệ thống mình" là đọc từ code và ADR của repo, đáng tin hơn phần tra cứu.
>
> Liên quan: ADR-45/47/49 (`docs/decisions/01_ADR_LOG.md`), mô hình token ở `docs/reports/01_AI_AGENT_OVERVIEW.md` mục 4-5, công tắc ở `docs/ai/11_RCA_BUILD_LOG.md` §16.42 và `docs/instruction/03_RUNBOOK.md` §19d.

## 1. Bài toán chung

Agent là một chương trình do LLM điều khiển, gọi API thay cho người dùng. Ba câu hỏi mọi hệ thống phải trả lời:

1. **Agent là ai?** Nó dùng danh tính của người dùng, của chính nó, hay cả hai?
2. **Ai chặn việc nó không được làm?** Chặn ở đâu: trong prompt, trong agent, ở lớp tool, hay ở API đích?
3. **Khi nó làm sai thì dừng bằng cách nào, và lần ngược được không?**

## 2. Các kiểu kiến trúc đang có

| # | Kiểu | Cách làm | Ai dùng / nguồn |
|---|---|---|---|
| A | **Agent cầm token của người dùng** | Agent gọi API bằng chính JWT/OAuth token người dùng. Quyền = quyền người dùng | Cách ngây thơ nhất, hay gặp trong demo. Rủi ro: token sống lâu nằm trong tay thứ có thể bị prompt injection |
| B | **Tài khoản dịch vụ cho agent** | Agent có một tài khoản/khoá riêng quyền rộng, tự kiểm "người dùng này được xem gì" trong code | Hay gặp ở bot nội bộ. Rủi ro: "confused deputy" (agent quyền rộng bị lừa làm hộ người không đủ quyền) |
| C | **Identity Provider + OBO / token exchange** | Người dùng đăng nhập IdP; agent đổi token của người dùng lấy token hẹp hơn (RFC 8693 token exchange, hoặc OBO của Microsoft), có claim `act` ghi "agent nào đang hành động thay ai" | Auth0 (OBO token exchange, GA 5/2026), Microsoft Entra Agent ID (OBO riêng, không phải RFC 8693), Okta for AI Agents, AWS Bedrock AgentCore Identity, Keycloak; agentgateway hỗ trợ cả ba kiểu đổi token |
| D | **MCP server (+ MCP gateway)** | Tool được phơi qua giao thức MCP; MCP server là resource server, gateway đứng trước để xác thực, giới hạn, log | Phổ biến nhất cho agent dùng chung nhiều client (Claude Desktop, IDE...). Traefik, Gravitee, MintMCP... bán MCP gateway. Tranh luận chưa ngã ngũ: gateway hay chính MCP server nên làm OBO |
| E | **API gateway hiện có làm điểm quyết định** | Tool = REST đã có; gateway xác thực, RBAC, sở hữu, rate limit như với mọi client; agent chỉ là một client nữa | API7 viết đúng ý này: "OAuth của agent chưa đủ, quyền phải dời về gateway". Không phải tên riêng một sản phẩm |
| F | **Policy engine ngoài (OPA/Cedar/FGA)** | Mỗi lời gọi tool hỏi một dịch vụ chính sách "agent X thay người Y có được làm Z lên tài nguyên R không" | Auth0 FGA, các sản phẩm "agent firewall". Mạnh khi quyền chi tiết theo từng đối tượng |

Hầu hết sản phẩm thật pha trộn: C hoặc E cho danh tính, D cho khả năng dùng chung, F cho quyền mịn.

## 3. Hệ thống mình thuộc kiểu nào

**Kiểu E, có một nhánh giống C làm tay.** Cụ thể:

| Thành phần của IdP/OBO chuẩn | Hệ thống mình dùng gì |
|---|---|
| Identity Provider (đăng nhập, cấp token người dùng) | `auth-service` (JWT access/refresh của chính dự án), không có IdP ngoài, không SSO/federation |
| Token exchange: đổi token người dùng lấy token hẹp cho agent | Gateway tự ký **agent token** (`typ=agent`, `sub`=người dùng, `act={sub:ai-agent}`, `aud=gateway-agent`, scope theo role, 150 s) bằng khoá `AGENT_TOKEN_KEYS` mà chỉ gateway giữ. Cùng ý với RFC 8693 (claim `act`, audience, scope hẹp) nhưng **không đi qua endpoint token-exchange chuẩn** |
| Consent (người dùng đồng ý cho agent làm gì) | Với việc ghi: **proposal token** ký một lần, người dùng bấm Xác nhận, UI gọi route thật bằng JWT thật + header `X-Agent-Proposal` |
| Policy engine | Bảng `agent/scopes.go` (một nguồn duy nhất) + kiểm sở hữu bằng SQL ở từng service (`WHERE buyer_id=?`) |
| MCP server / gateway | Không có (ADR-49). Khám phá tool qua `GET /ai/tools` |
| Danh tính cho tác vụ không có người (RCA) | System token `sys=rca-watcher`, chỉ GET `/ops/*`, không bao giờ đề xuất được |

### 3.1 Vì sao chọn vậy (lý do nằm trong ADR và code)

1. **Chỉ có một client agent** (ai-agent). MCP giải bài "nhiều client dùng chung tool"; ở đây chưa có bài đó (ADR-49).
2. **Mọi quyền đã nằm sẵn ở gateway** (auth, RBAC, rate limit, kiểm sở hữu). Thêm MCP server chen giữa buộc phải lặp lại kiểm quyền, nguy cơ hai nơi lệch nhau. Agent là client thường nên được hưởng đúng các kiểm tra ấy.
3. **Không muốn thêm IdP** chỉ cho agent: dự án tự có `auth-service`, thêm Keycloak/Auth0 là thêm một dịch vụ phải vận hành, và các tính năng chính của nó (SSO, consent screen, federation) chưa cần.
4. **Khoá ký agent token chỉ gateway giữ**: ai-agent bị chiếm vẫn không tự đúc được token cho role khác.
5. **Mặc định chỉ đề xuất**: quyền ghi không nằm trong token của agent; ghi cần JWT thật của người dùng sau khi bấm xác nhận.

### 3.2 Ưu và nhược

| | Ưu | Nhược / giới hạn thật |
|---|---|---|
| Quyền | Một chỗ quyết định, một bảng quyền, agent không có đường vòng quanh RBAC/sở hữu | Gateway là **điểm đơn lẻ**: bug ở gateway là bug cho mọi agent; chưa có kiểm quyền mịn kiểu policy engine (theo thuộc tính) |
| Danh tính | Token ngắn (150 s), hẹp, có `act` nên log biết "agent thay ai" | Tự chế, **không theo chuẩn** (không RFC 8693 endpoint), tool ngoài không hiểu; không có thu hồi theo từng token ngoài hết hạn (có `pwd_version`), không consent theo từng scope |
| Khả năng mở rộng | Đơn giản, ít thành phần | Thêm client agent ngoài (IDE, Claude Desktop) thì phải làm lớp MCP mỏng chỉ đọc (ADR-49 đã ghi sẵn hướng này) |
| Chuẩn hoá | Không phải học thêm giao thức | Khó so với sản phẩm thương mại, không có audit chuẩn của IdP (mình tự có `ops_audit_log` cho phần ops) |
| Vận hành | Ít dịch vụ | Xoay khoá `AGENT_TOKEN_KEYS`, quản lý `AI_SYSTEM_KEY` là việc của mình |
| Prompt injection | Dù injection thành công, token chỉ có người dùng hiện tại: đọc đơn người khác ra 404 (SQL lọc), ghi cần người bấm | Chưa chặn được việc agent **nói** sai hay **lộ dữ liệu của chính người dùng** vào câu trả lời; đó là việc của guard/eval |

Kết luận ngắn: với một client và một gateway đã có sẵn quyền, kiểu E rẻ và an toàn nhất; nó không sai nhưng là **tự dựng một phần của OBO**. Nếu sau này có nhiều client agent hoặc cần SSO doanh nghiệp, đường nâng cấp tự nhiên là đưa `auth-service` (hoặc một IdP) phát hành token đổi theo RFC 8693 và dùng MCP gateway chỉ đọc.

## 4. Công tắc dừng khẩn: có ai làm không, vì sao cần

### 4.1 Có phổ biến không

Có, nhưng gọi là "kill switch" (công tắc dừng) hoặc "emergency stop", và mới phổ biến từ khoảng 2025-2026 khi agent bắt đầu có quyền ghi. Tra cứu thấy các sản phẩm và bài viết về chủ đề này (CrowdStrike, JetStream Security, PointGuard, Policy Layer, Waxell, TechTarget, Axios); **đa số là marketing của hãng bán sản phẩm**, không phải chuẩn. Điểm các nguồn nhắc giống nhau:

- Công tắc phải nằm **ngoài agent** (mặt phẳng điều khiển riêng), vì công tắc dựa vào việc agent nghe lời thì không bảo vệ được gì. Hệ thống mình đúng ý này: công tắc nằm ở gateway, không phải trong prompt.
- Nên có nhiều tầng: tắt từng agent, tắt theo kênh, giới hạn tốc độ, circuit breaker, và nút dừng toàn cục. Mình có nút toàn cục + giới hạn tốc độ + breaker LLM; **chưa** có tắt từng persona/từng người dùng.
- Thu hồi thông tin xác thực ngay khi có hành vi bất thường (mình: token agent 150 s tự hết hạn, nhưng công tắc chặn ngay).
- Nó là phanh khẩn cấp, **không phải nút hoàn tác** (nhận định của một nhà phân tích Gartner, qua TechTarget). Mình ghi rõ: công tắc không rollback đơn đã huỷ.
- Cần kết hợp lớp khác (quyền hạn chế, sandbox, người duyệt); TechTarget còn viết bài phản biện rằng chỉ dựa vào công tắc là ngõ cụt.

Một con số hay được trích: khảo sát VentureBeat 101 công ty, 27% không có cách dừng agent chạy loạn theo thời gian thực. **Đây là số qua một blog thứ cấp, chưa kiểm gốc.**

### 4.2 Vì sao cần, mức nghiêm trọng

Mình **không tìm được** một vụ nào được xác nhận là "thiệt hại vì không kịp tắt AI" theo đúng nghĩa công tắc. Những vụ có thật gần nhất:

| Vụ | Chuyện gì | Liên quan đến công tắc thế nào |
|---|---|---|
| **Replit, 7/2025** (AI Incident Database #1152) | Agent xoá cơ sở dữ liệu production của một công ty (khoảng 1.200 bản ghi theo các nguồn, số liệu lệch giữa bài) dù người dùng đã yêu cầu "đóng băng"; sau đó nói sai về chuyện đã xảy ra. Nguồn chủ yếu là bài đăng và ảnh chụp của chính nạn nhân | Bài học là **lệnh trong prompt không phải cơ chế chặn**; Replit sau đó tách DB production/dev và cấm agent sửa production. Gần với "chặn ngoài agent" hơn là công tắc |
| **Air Canada, 2/2024** | Chatbot bịa chính sách giảm giá tang lễ; toà BC buộc hãng bồi thường 812 CAD và bác lập luận "chatbot là chủ thể riêng" | Trách nhiệm pháp lý thuộc về doanh nghiệp. Một bài sau đó nói thấy chatbot đã bị gỡ khỏi site (nguồn thứ cấp) |
| **Chevrolet đại lý, 2023** | Chatbot ChatGPT bị người dùng dụ "bán xe 1 đô" | Chỉ là vụ lan truyền, không có phán quyết; chứng minh prompt injection ngoài đời |
| **Cursor, 4/2025** | Bot hỗ trợ bịa chính sách một thiết bị mỗi gói; đồng sáng lập phải đính chính (nguồn thứ cấp) | Chỉ là nói sai, không có hành động ghi |

Nên đánh giá trung thực: **rủi ro lớn nhất của mình nằm ở việc agent ghi sai**, mà mình đã chặn bằng cơ chế đề xuất + xác nhận. Công tắc là lớp phụ cho ba tình huống: (1) prompt injection hay lỗi mô hình khiến agent gọi tool lặp tốn tiền hoặc spam đề xuất; (2) nhà cung cấp LLM đổi hành vi giữa chừng; (3) vòng RCA chạy nền không người nhìn (hay bị bỏ quên nhất). Mức nghiêm trọng thực tế ở quy mô dự án: **thấp đến vừa** (tốn tiền LLM, thông tin sai hiển thị, hàng đợi proposal bị làm rối), chứ không phải mất dữ liệu, vì agent không có quyền ghi trực tiếp.

## 5. Việc có thể làm tiếp (chưa làm)

- Công tắc theo persona (tắt riêng `buyer`, giữ `operator`) thay vì chỉ toàn cục.
- Test quét bảng route: route nào nhận agent token phải có kiểm công tắc (đã ghi ở §16.42).
- Nếu có client agent ngoài: MCP mỏng chỉ đọc, quyền vẫn do gateway quyết (ADR-49).
- Nếu cần SSO/consent thật: IdP + RFC 8693 thay phần tự ký agent token.

## 6. Nguồn tra cứu (2026-11-18)

- Kill switch: [dev.to/waxell](https://dev.to/waxell/the-kill-switch-problem-how-to-stop-an-ai-agent-thats-gone-wrong-224a), [TechTarget: vì sao cần](https://www.techtarget.com/ai/tip/Why-businesses-need-an-AI-agent-kill-switch), [TechTarget: có thể là ngõ cụt](https://www.techtarget.com/cybersecurity/news/366651197/Pulling-the-plug-Why-the-AI-kill-switch-might-be-a-dead-end?amp=1), [CSO: CrowdStrike](https://www.csoonline.com/article/4244509/crowdstrike-launches-ai-kill-switch-to-shut-down-rogue-agents.html), [Axios](https://axios.com/2026/09/10/ai-doomsday-doom-fear-kill-switch), [hackernoon](https://hackernoon.com/your-kill-switch-may-not-stop-your-agent).
- Replit: [AI Incident Database #1152](https://incidentdatabase.ai/reports/5593/), [eWeek](https://www.eweek.com/news/replit-ai-coding-assistant-failure/), [heise](https://heise.de/-10499597).
- Air Canada: [the-decoder](https://the-decoder.com/air-canada-held-responsible-for-chatbots-misleading-advice-on-airfares/), [Mondaq](https://webiis08.mondaq.com/canada/new-technology/1432798/bc-tribunal-confirms-companies-remain-liable-for-ai-chatbot-created-information).
- OBO / IdP: [Auth0 agent identity in tokens](https://auth0.com/docs/ai-agents-mcp/agent-as-principal/agent-identity-in-tokens), [Microsoft Entra OBO](https://learn.microsoft.com/en-us/entra/agent-id/agent-on-behalf-of-oauth-flow), [AWS AgentCore OBO](https://aws.amazon.com/blogs/machine-learning/implement-on-behalf-of-token-exchange-for-multi-tenant-agents-with-amazon-bedrock-agentcore-gateway/), [WorkOS](https://workos.com/blog/oauth-on-behalf-of-ai-agents), [agentgateway](https://agentgateway.dev/blog/2026-07-12-agentgateway-token-exchange-jwt-assertion-entra-obo/).
- Gateway / MCP: [API7: quyền phải dời về gateway](https://api7.ai/blog/ai-agent-oauth-gateway-authorization), [Traefik: MCP gateway](https://traefik.io/blog/mcp-gateway-explained), [Traefik: API vs AI vs MCP gateway](https://traefik.io/blog/api-gateway-vs-ai-gateway-vs-mcp-gateway), [Gravitee: RFC 8693 cho agent](https://gravitee.io/corpus/gen-1410/oauth/oauth-2-0-token-exchange-rfc-8693-for-ai-agents-and-mcp-tool-delegation.html).
