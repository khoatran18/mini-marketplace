# Bộ eval của ai-agent

406 câu hỏi tự viết tay cho 4 persona (`cases/<persona>.yaml`), 6 nhóm: `lookup`, `filter`, `variant`, `propose`, `adversarial`, `scope`.
Định dạng case và cách chấm: docstring đầu `evalkit.py`. Chạy test tĩnh (không gọi mô hình): `python -m pytest tests/test_evals_dataset.py`.

```bash
conda activate base_python_3.12 && cd services/ai-agent
python evals/run_evals.py                                   # kế hoạch, KHÔNG gọi mô hình
python evals/run_evals.py --live --persona buyer --limit 10 # gọi mô hình thật (tốn tiền)
python evals/run_evals.py --live --out /tmp/eval.json       # cả 4 persona, ghi báo cáo
```
Đổi tên tool hay tham số ở `agent/scopes.go` thì `TestEvalToolSnapshotIsCurrent` (Go) đỏ: chạy `cd services/api-gateway && go run ./cmd/dump-agent-tools > ../ai-agent/evals/tools.snapshot.json`, rồi sửa các case bị `validate` báo lỗi.
Runner dùng gateway giả (dữ liệu rỗng): nó đo chọn tool, tham số, số lần gọi, đề xuất, rò rỉ và từ chối; KHÔNG đo độ đúng số liệu trong câu trả lời.

## Eval RCA (vòng lặp điều tra sự cố, ADR-46)
19 case (`rca/cases.yaml`) dựng từ đáp án của `scripts/e2e/resilience.sh` và các playbook; chấm: nguyên nhân trong top-3 giả thuyết, mở đúng playbook, hành động đầu đúng sổ, không có hành động bị cấm, kết cục, ngân sách, chỉ GET.
```bash
python evals/run_rca_evals.py                  # kiểm tĩnh bộ case + kế hoạch + ước tính số lượt gọi, KHÔNG gọi gì
python evals/run_rca_evals.py --oracle         # đồ thị với Brain "đáp án" (kiểm bộ eval), không LLM
python evals/run_rca_evals.py --live --ids C-payment-down   # gọi mô hình THẬT (tốn tiền): phải hỏi người dùng trước
```
Sổ hành động của ops-service đổi thì `TestAgentEvalSnapshotIsCurrent` (Go) đỏ: `cd services/ops-service && UPDATE_SNAPSHOT=1 go test ./internal/actions -run Snapshot`.

