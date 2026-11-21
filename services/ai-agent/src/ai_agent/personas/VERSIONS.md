# Phiên bản prompt các persona

Mỗi lần sửa `personas/<tên>/prompt.md` phải thêm một dòng MỚI cho persona đó (version tăng, băm sha256 12 ký tự đầu của file `prompt.md` sau khi sửa) rồi mới commit; test `tests/test_prompt_versions.py` so băm của dòng cuối với file thật. Lệnh lấy băm: `sha256sum src/ai_agent/personas/<tên>/prompt.md | cut -c1-12`. Số đo của từng phiên bản nằm ở `docs/ai/11_RCA_BUILD_LOG.md` §16.43 (file `--out` của eval cũng ghi băm trong `meta.prompts`).

| persona | version | ngày | băm | thay đổi |
|---|---|---|---|---|
| buyer | v1 | 2026-11-15 | (trước nhật ký này) | bản gốc đã tune cho deepseek |
| buyer | v2 | 2026-11-21 | 712eeac4206d | câu chỉ để đọc mà ý rõ thì tra trước theo phạm vi hợp lý; có số hay mã chỉ rõ đối tượng thì tra ngay, "đơn nào ..." là liệt kê; vẫn hỏi khi thiếu thông tin bắt buộc và trước khi ghi |
| seller | v1 | 2026-11-21 | 3943f3e7e8b5 | bản hiện hành lúc bắt đầu nhật ký phiên bản |
| admin | v1 | 2026-11-21 | c8297248edda | bản hiện hành lúc bắt đầu nhật ký phiên bản |
| rca | v1 | 2026-11-21 | 64aa61d95810 | bản hiện hành lúc bắt đầu nhật ký phiên bản |
| operator | v1 | 2026-11-15 | (trước nhật ký này) | bản gốc đã tune cho deepseek |
| operator | v2 | 2026-11-21 | f0c2003858eb | RCA chỉ cho câu hỏi nguyên nhân (~5 lần gọi, dành lượt cuối); yêu cầu hành động đã rõ thì kiểm tra nhanh 2-3 lần rồi soạn; câu tra cứu một mục gọi 1-2 công cụ; chỉ nói "đã soạn" khi `propose_ops_action` đã chạy |
