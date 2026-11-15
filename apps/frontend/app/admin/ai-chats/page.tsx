import { AiChatsClient } from './AiChatsClient';
import { PageTitle } from '../../../components/layout/PageTitle';

// Platform admin only (gateway 403s anyone else; the menu entry is admin-only too).
export default function AdminAiChatsPage() {
  return (
    <div className="grid gap-5">
      <header className="grid gap-1">
        <PageTitle icon="spark">Lịch sử chat AI</PageTitle>
        <p className="text-sm text-muted">
          Xem lại các cuộc trò chuyện với trợ lý để kiểm tra chất lượng và xử lý khiếu nại. Chỉ có câu hỏi và câu trả lời cuối (đã che số điện thoại, email), giữ 30 ngày; mỗi lần mở một cuộc chat đều được ghi log.
        </p>
      </header>
      <AiChatsClient />
    </div>
  );
}
