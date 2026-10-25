# Bức tranh AI của các sàn TMĐT

> **Ngày tra cứu:** 2026-10-25. **Mục đích:** biết các sàn lớn dùng AI vào việc gì để đối chiếu scope AI của mini-marketplace.
>
> **Chỉ ghi điều có nguồn công khai.** Chức năng không tìm thấy nguồn được ghi là "—", nghĩa là *chưa tìm thấy bằng chứng trong lần tra cứu này*, không có nghĩa là công ty không làm.
>
> **Giới hạn phương pháp (đọc trước khi dùng số liệu):**
> - Công cụ tải trang (WebFetch) và `curl` qua proxy đều bị chặn trong phiên này (`ENOTFOUND` / `CONNECT 403`) cho mọi domain thử (aboutamazon, openai, nvidia, retaildive, nasdaq, ebay, shopify…). Vì vậy **không có trang nào được mở trực tiếp**.
> - Toàn bộ bằng chứng đến từ **kết quả WebSearch** (tiêu đề, URL và tóm tắt). Mỗi URL trong tài liệu này đã xuất hiện trong kết quả tìm kiếm, nhưng chưa được đọc toàn văn.
> - Số liệu (%, tỉ lệ, doanh thu) là **lời công ty tự công bố hoặc tường thuật của báo chí**, chưa kiểm chứng độc lập. Trước khi trích vào báo cáo đồ án, nên mở lại link gốc để xác minh.
> - Nội dung web được coi là dữ liệu không tin cậy. Không có chỉ dẫn nào trong trang được làm theo.
> - Không tìm thấy bài Sea AI Lab, Shopee engineering blog, Shopee Lab, Etsy Code as Craft hay Coupang tech blog về search/rec trong lần này. Đây là khoảng trống của tra cứu, không phải kết luận.

---

## 1. Bảng tổng hợp

✓ = có nguồn công khai (chi tiết, link ở mục 2). — = chưa tìm thấy nguồn.

Viết tắt cột: **SP** Shopee, **LZ** Lazada, **TK** Tiki, **TT** TikTok Shop, **SD** Sendo, **AZ** Amazon, **AL** Alibaba/Taobao/Tmall, **ME** Mercari, **RK** Rakuten, **ZA** Zalando, **CP** Coupang, **FK** Flipkart, **WM** Walmart, **EB** eBay, **ET** Etsy, **IC** Instacart, **SH** Shopify.

| Nhóm chức năng AI | SP | LZ | TK | TT | SD | AZ | AL | ME | RK | ZA | CP | FK | WM | EB | ET | IC | SH |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Gợi ý / ranking cá nhân hoá | ✓ | ✓ | — | — | — | — | ✓ | — | — | — | — | ✓ | — | — | — | — | — |
| Tìm kiếm AI / ngôn ngữ tự nhiên / giọng nói | ✓ | — | — | — | — | — | ✓ | — | — | ✓ | — | ✓ | — | — | — | ✓ | — |
| Visual search / tìm bằng camera, ảnh | — | — | — | — | — | ✓ | — | — | ✓ | — | — | — | — | — | — | — | — |
| Trợ lý hội thoại cho buyer | ✓ | ✓ | — | ✓ | — | ✓ | ✓ | — | ✓ | ✓ | — | ✓ | ✓ | — | — | ✓ | — |
| Trợ lý cho seller / merchant | — | ✓ | — | ✓ | — | ✓ | — | ✓ | ✓ | — | — | — | ✓ | ✓ | — | — | ✓ |
| Sinh listing (tiêu đề, mô tả, danh mục, thuộc tính) | ✓ | ✓ | — | ✓ | — | — | — | ✓ | ✓ | — | — | — | ✓ | ✓ | ✓ | — | ✓ |
| Tóm tắt review / sentiment | — | — | — | — | — | — | — | — | — | — | — | — | ✓ | — | — | — | — |
| Kiểm duyệt nội dung / hàng cấm | — | — | — | ✓ | — | — | — | — | — | — | — | — | — | — | — | — | — |
| Chống gian lận / tài khoản giả | — | — | — | ✓ | — | — | — | ✓ | — | — | — | — | — | — | — | — | — |
| Dự báo nhu cầu / tồn kho | — | — | — | — | — | — | — | — | — | — | ✓ | — | ✓ | — | — | — | — |
| Logistics / ETA / tuyến giao | — | — | — | — | — | — | — | — | — | — | ✓ | — | — | — | — | — | — |
| Giá, khuyến mãi, voucher, theo dõi giá | ✓ | — | — | — | — | ✓ | ✓ | — | — | — | — | — | — | — | — | — | — |
| Quảng cáo (CTR, matching, tạo nội dung quảng cáo) | ✓ | — | — | ✓ | — | — | ✓ | — | — | — | — | — | — | — | — | ✓ | — |
| OCR / giấy tờ / KYC | — | — | — | — | — | — | — | — | — | — | — | — | — | — | — | — | — |
| Video / ảnh sinh bằng AI, người mẫu ảo | — | ✓ | — | ✓ | — | — | — | — | — | — | — | ✓ | — | — | — | — | — |
| Thử đồ ảo / tư vấn size | — | ✓ | — | — | — | — | ✓ | — | — | ✓ | — | — | — | — | — | — | — |
| Livestream (AI host do chính sàn làm) | — | — | — | — | — | — | — | — | — | — | — | — | — | — | — | — | — |
| Dịch tự động | — | ✓ | — | ✓ | — | — | — | — | — | — | — | — | — | — | — | — | — |
| CSKH / hậu mãi tự động | ✓ | — | — | ✓ | — | — | ✓ | — | — | — | — | — | — | — | — | — | — |
| Đặt hàng tự động / agentic checkout | — | — | — | ✓ | — | ✓ | ✓ | — | — | ✓ | — | — | — | — | — | — | — |
| AI nội bộ: LLM tự xây, hạ tầng GPU, code | ✓ | — | — | — | — | — | — | — | — | — | ✓ | ✓ | — | — | — | — | — |
| AIOps / giám sát vận hành | — | — | — | — | — | — | — | — | — | — | — | — | — | — | — | — | — |

Nhận xét nhanh:
- **Tiki và Sendo**: không tìm thấy thông tin AI công khai nào đáng tin. Một kết quả chỉ là tiện ích Chrome của Tiki gợi ý sản phẩm khi tìm kiếm ngoài trang.
- **Hàng "—" cả dòng** (OCR/KYC, AIOps, livestream AI host do sàn làm): chưa có nguồn công khai trong lần tra cứu, nên không kết luận là sàn không làm.
- Nhóm xuất hiện nhiều nhất: **trợ lý hội thoại buyer**, **sinh listing cho seller**, **trợ lý seller**. Đây cũng là các nhóm sàn đang đầu tư mạnh từ 2023 đến nay.

---

## 2. Chi tiết theo công ty

Quy ước: mỗi bullet ghi chức năng, nguồn (link) và mức tin cậy. "Số liệu tự công bố" = lời công ty hoặc báo chí tường thuật.

### Shopee (Sea Limited)
- **CSKH bằng chatbot**: một phân tích của Nasdaq/Zacks nói chatbot AI của Sea xử lý khoảng 80% truy vấn khách và giảm chi phí CSKH mỗi liên hệ khoảng 30% so với năm trước. Đây là bài phân tích bên ngoài, không phải số liệu trực tiếp từ Sea. [Nasdaq](https://www.nasdaq.com/articles/seas-ai-push-openai-could-deepen-shopees-competitive-edge-now)
- **Search, recommendation, công cụ nội dung seller**: Sea nói AI cho search, recommendation và seller content tools góp phần tăng 14% conversion mua hàng so với cùng kỳ trong Q1 2026 (số liệu tự công bố). Cùng nguồn trích CFO Q1 2025 nói dùng AI để nhắm user hiệu quả hơn khi tìm kiếm và gợi ý chính xác hơn. [Nasdaq](https://www.nasdaq.com/articles/seas-ai-push-openai-could-deepen-shopees-competitive-edge-now)
- **LLM tự xây**: case study của NVIDIA nói Shopee thấy LLM đa dụng không đáp ứng tác vụ TMĐT đa ngôn ngữ nên tự xây, token của "Compass API" tăng từ 3 tỉ lên 340 tỉ mỗi tháng. Nguồn là vendor nên mang tính marketing. [NVIDIA](https://www.nvidia.com/en-us/case-studies/shopee/)
- **Kênh trợ lý bên ngoài**: Shopee có mặt trong ChatGPT ở nhiều nước, trong đó có Việt Nam. Sea dự kiến đưa ChatGPT for Business cho seller đủ điều kiện (listing, marketing, CSKH). Google và Sea thoả thuận "khám phá" nguyên mẫu agentic shopping. [ContentGrip](https://www.contentgrip.com/shopee-chatgpt-sea-openai-partnership/), [Fintool](https://fintool.com/news/google-sea-agentic-ai-partnership)
- **Quảng cáo/voucher**: trên earnings call Q2 2026, Forrest Li nêu voucher cá nhân hoá, Shop GMV Max, Brand Max, ad matching tốt hơn, nội dung do AI sinh; doanh thu quảng cáo tăng hơn 70% so với cùng kỳ. [Webull](https://www.webull.com/news/15422657858520064)
- **Không dùng làm bằng chứng cho Shopee thương mại**: AI underwriting của mảng fintech (tăng approval khoảng 10%); Chrome extension bên thứ ba cho chat Shopee; bài blog của hosting nói về "Shop AI Assistant" nhưng không có nguồn chính thức Shopee. [Exabytes](https://www.exabytes.my/blog/shopee-shop-ai-assistant/) (chưa xác nhận)

### Lazada (Alibaba)
- **Trợ lý buyer**: LazzieChat (2023) là chatbot TMĐT đầu tiên ở Đông Nam Á chạy trên Azure OpenAI; sau đó Lazzie là trợ lý mua sắm cá nhân. Lazada nói Lazzie giúp conversion tăng 68% khi có gợi ý cá nhân hoá (tự công bố). [VnEconomy](https://vneconomy.vn/techconnect/lazada-ra-mat-chatbot-ai-thuong-mai-dien-tu-tai-dong-nam-a.htm), [SGGP](https://www.sggp.org.vn/lazada-cong-bo-buoc-tien-moi-trong-trai-nghiem-mua-sam-truc-tuyen-ung-dung-ai-post766448.html), [e27](https://e27.co/ai-at-the-core-lazada-shows-how-tech-can-supercharge-sellers-and-shoppers-20250911/)
- **Gợi ý và nội dung sản phẩm**: đề xuất sản phẩm thông minh, thông tin sản phẩm do AI tạo, "selling points" theo sở thích người mua (khoảng 25% seller dùng), người mẫu ảo do AI tạo. [SGGP](https://www.sggp.org.vn/lazada-cong-bo-buoc-tien-moi-trong-trai-nghiem-mua-sam-truc-tuyen-ung-dung-ai-post766448.html), [Bernama](https://www.bernama.com/en/news.php?id=2358222)
- **Công cụ seller**: Lazzie Seller (chatbot trong Seller Centre: FAQ, điều hướng, đánh giá rủi ro cửa hàng, tư vấn kinh doanh), AI Smart Product Optimisation (gợi ý sửa tiêu đề, mô tả, ảnh, thử đồ ảo, đổi nền), Lazada Business Advisor (insight thời gian thực). [e27](https://e27.co/ai-at-the-core-lazada-shows-how-tech-can-supercharge-sellers-and-shoppers-20250911/), [BigSeller](https://www.bigseller.pro/blog/articleDetails/3130/lazada-AI-tools.htm)
- **Dịch**: Marco MT của Alibaba (dựa trên Qwen) giúp seller tạo trang sản phẩm đa ngôn ngữ. [SGGP](https://www.sggp.org.vn/lazada-cong-bo-buoc-tien-moi-trong-trai-nghiem-mua-sam-truc-tuyen-ung-dung-ai-post766448.html), [e27](https://e27.co/ai-at-the-core-lazada-shows-how-tech-can-supercharge-sellers-and-shoppers-20250911/)
- Chưa xác nhận feature set năm 2026.

### Tiki
- Chỉ thấy tiện ích Chrome "Tiki Assistant" tự gợi ý sản phẩm Tiki khi tìm kiếm trên web. Không có thông báo AI trong app hay website, không thấy "TikiBot". [Chrome Web Store (Tiki Assistant)](https://chrome.google.com/webstore/detail/bee-tracker-trợ-lý-mua-sắ/detail/tiki-assistant/ncpaceoemnbcjffjpjcgnbaklmkhdmak)

### Sendo
- Không tìm thấy thông tin AI. Một báo cáo 2026 nói Sendo đang chuyển hướng hoặc thu hẹp hoạt động. [VnEconomy](https://vneconomy.vn/techconnect/tiktok-shop-shopee-va-lazada-thau-tom-99-thi-truong-thuong-mai-dien-tu-dong-nam-a.htm)

### TikTok Shop
- **Trợ lý buyer + checkout một chạm**: TechCrunch (2026-10-05) viết TikTok ra mắt agent mua sắm hội thoại có nhớ sở thích và one-click checkout từ feed For You, làm cùng Salesforce, Shopify, Shoplazza, Stripe. [TechCrunch](https://techcrunch.com/2026/10/05/tiktok-rolls-out-an-ai-shopping-assistant-and-one-click-checkout/)
- **Trợ lý seller**: Seller Assistant trong Seller Center (hỏi đáp listing, insight hiệu suất, chuyển sang nhân viên thật); "List with AI" (từ ảnh + mô tả ngắn ra tiêu đề, mô tả, danh mục); dashboard beta với các agent theo tác vụ, ví dụ agent quét sản phẩm giảm doanh số, traffic thấp, conversion kém. [PPC Land](https://ppc.land/tiktok-shop-replaces-seller-homepage-with-5-agent-ai-dashboard-in-beta/), [Social Media Today](https://www.socialmediatoday.com/news/tiktok-shop-rolls-out-new-tools-including-expanded-chatbot-access/812401/), [MediaPost](https://www.mediapost.com/publications/article/412106/tiktok-shop-offers-merchants-more-ai-powered-tools.html)
- **Kiểm duyệt và chống gian lận**: AI quét listing trước khi hiển thị (hàng giả, vi phạm an toàn, hàng cấm), người duyệt xử lý ca sâu. Báo cáo an toàn H1 2025 (theo tường thuật): chặn trên 70 triệu sản phẩm, đóng hơn 700.000 tài khoản seller; seller mới có giai đoạn thử việc (probation) giới hạn upload và số đơn. [Digital Information World](https://www.digitalinformationworld.com/2025/11/tiktok-shop-tightens-grip-on-ai-driven.html)
- **Video và dịch**: AI Fashion Video Maker (ảnh sản phẩm thành video), AI dubbing giọng tổng hợp có lip sync. [MediaPost](https://www.mediapost.com/publications/article/412106), [Global Dating Insights](https://www.globaldatinginsights.com/featured/tiktok-shop-expands-ai-toolkit-with-video-listing-and-dubbing-tools/)
- **Quảng cáo**: có bài về công cụ AI mới cho advertiser (chỉ thấy tiêu đề và URL, chưa rõ chi tiết). [Ads-up](https://ads-up.fr/en/tiktok-shop-rolls-out-new-ai-tools-for-advertisers/)
- **Livestream AI host**: chỉ thấy do bên thứ ba (Syntopia, AI Studios), không phải TikTok.

### Amazon
- **Trợ lý mua sắm (Rufus, được báo là đổi tên thành "Alexa for Shopping" ngày 2026-05-13 theo trang Amazon trích trong kết quả)**: giá lịch sử 30/90/365 ngày; Auto Buy (đặt giá mục tiêu, tự mua cho thành viên Prime, chỉ hàng FBA); Scheduled Actions; Shop Direct (gợi ý cả hàng bên ngoài). Hơn 50 triệu khách đã dùng; CEO nói MAU tăng 115% (tự công bố). [EcomCrew](https://www.ecomcrew.com/amazons-rufus-can-now-buy-things-for-you-heres-what-that-means-for-sellers/), [PPC Land](https://ppc.land/amazons-rufus-now-shows-a-full-year-of-price-history-to-50m-shoppers/), [ChannelNews](https://www.channelnews.com.au/amazon-expands-ai-shopping-assistant-with-12-month-price-tracking-as-usage-surges/)
- **Visual search**: Lens Live (2025-09-02) quét camera thời gian thực và trả carousel sản phẩm; Amazon nói dùng mô hình visual embedding so khớp với hàng tỉ sản phẩm. [Amazon](https://aboutamazon.com/news/retail/search-image-amazon-lens-live-shopping-rufus), [GeekWire](https://www.geekwire.com/2025/amazons-lens-live-mobile-shopping-feature-uses-camera-and-ai-to-find-real-time-product-matches/)
- **Trợ lý seller**: Project Amelia (beta 09/2024, trên Bedrock) trả lời về doanh số, đơn vị bán, traffic so với cùng kỳ; sau đó phát triển thành Seller Assistant có tính agentic (xử lý sự cố tài khoản, lập kế hoạch tồn kho, tinh chỉnh listing; chưa xác nhận chi tiết). [Amazon](https://aboutamazon.com/news/innovation-at-amazon/amazon-project-amelia), [TechCrunch](https://techcrunch.com/2024/09/19/amazon-debuts-an-ai-assistant-for-sellers-project-amelia), [Nasdaq](https://www.nasdaq.com/articles/amazon-unveils-ai-powered-seller-assistant-agentic-capabilities)
- **Chưa xác minh được trong lần này**: AI review highlights, công cụ sinh listing cho seller, Alexa+ shopping. Các bài Amazon liên quan không tải được. Không kết luận Amazon không có.

### Alibaba / Taobao / Tmall
- **Trợ lý Qwen trong Taobao** (công bố 2026-05-11): Qwen app kết nối toàn bộ catalog Taobao/Tmall (trên 4 tỉ sản phẩm); trợ lý trong app có so sánh/gợi ý dựa trên catalog và hàng tỉ review; thử đồ ảo, theo dõi giá 30 ngày, coupon một chạm; agent có kỹ năng quản lý đơn, logistics, hậu mãi. [Alibaba Group](https://www.alibabagroup.com/document-1991231293551017984), [Alizila](https://www.alizila.com/?p=66879), [SCMP](https://www.scmp.com/tech/article/3353132/alibaba-taps-chinas-chat-buy-trend-qwen-ai-and-taobao-integration)
- **CTR prediction cho quảng cáo**: Deep Interest Network (Zhou et al., 2017) và DIEN (2018, tác giả báo triển khai trên hệ thống quảng cáo display Taobao, tăng 20,7% CTR). [arXiv DIN](https://arxiv.org/pdf/1706.06978v1), [arXiv DIEN](https://arxiv.org/pdf/1809.03672v5)
- Không thấy trong lần tra cứu: AliExpress AI, Accio, "Taobao Wen Wen".

### Mercari
- **Tối ưu listing**: Mercari AI Assistant (10/2023) so sánh listing lâu chưa bán với best practice và đề xuất cải thiện; báo tăng doanh số trung bình mỗi user có ý nghĩa thống kê. **AI Listing Support** (09/2024): đăng bằng ảnh, gợi ý danh mục, sinh tiêu đề, mô tả; ban đầu GPT-4, sau chuyển GPT-4o mini để giảm chi phí và độ trễ. Nguồn là case study của OpenAI. [OpenAI](https://openai.com/index/mercari)
- **Gian lận**: báo cáo minh bạch H1 2025 nói huấn luyện AI trên hành vi đáng ngờ đã giám sát để nhận diện và loại tài khoản gian lận. [Mercari Transparency Report](https://speakerdeck.com/mercari_inc/transparency-report-first-half-of-2025)

### Rakuten
- **Rakuten AI trong app Ichiba**: concierge giúp thu hẹp sản phẩm bằng hội thoại (ngân sách, mục đích, kịch bản dùng) bằng text, giọng nói hoặc ảnh; catalog khoảng 500 triệu mặt hàng; dùng cả kết quả web search. Số liệu tự công bố qua nguồn thứ cấp: quyết định mua nhanh hơn 41%, AOV tăng 17%. [TelecomTV](https://www.telecomtv.com/content/ai/rakuten-integrates-cutting-edge-agentic-ai-tool-rakuten-ai-into-rakuten-ichiba-mobile-app-54595), [Stellagent](https://stellagent.ai/insights/rakuten-ai-store-managers)
- **Phía merchant**: RMS AI Assistant (mô tả sản phẩm, hình ảnh marketing, trả lời review, phân tích dữ liệu shop); kế hoạch avatar AI cho từng shop trả lời khách và chốt đơn (thử nghiệm). [Stellagent](https://stellagent.ai/insights/rakuten-ai-store-managers), [PYMNTS](https://www.pymnts.com/news/artificial-intelligence/2025/rakuten-taps-agentic-ai-to-enhance-omotenashi-ultimate-service/)

### Zalando
- **Trợ lý thời trang** (beta ChatGPT 2023; theo báo cáo mới hơn đã trên 6 triệu user, gắn với hồ sơ khách, thêm giỏ và checkout ngay trong trợ lý). **Tư vấn size** từ lịch sử mua, size ưa thích, phản hồi; size theo số đo từ hai ảnh; Virtual Fitting Room avatar 3D. Giảm trả hàng do sai size khoảng 8 đến 10% (tự công bố, các nguồn khác nhau). [PYMNTS](https://pymnts.com/news/retail/2023/zalando-add-chatgpt-powered-fashion-assistant-online-platform), [Apparel Resources](https://apparelresources.com/technology-news/retail-tech/zalando-add-chatgpt-powered-fashion-assistant/), [ICT Business](https://www.ictbusiness.biz/business/ai-is-reshaping-zalando-from-within)
- **Tìm kiếm**: có bài "Zalando enlists ChatGPT for new search function" (chỉ thấy tiêu đề). [Yahoo](https://www.yahoo.com/lifestyle/zalando-enlists-chatgpt-search-function-145741671.html)
- **Nội dung**: tiêu đề FashionUnited nói 90% nội dung nay do máy tạo (chưa đọc nội dung). [FashionUnited](https://fashionunited.com/news/business/zalando-fully-embraces-ai-90-percent-of-content-now-created-by-machines/2026031371157)

### Coupang
- **Logistics và dự báo vận hành**: mô hình AI ước tính thời gian giao và thứ tự giao theo thời tiết, địa lý; dự đoán cách xếp hàng ở trung tâm fulfillment và tối ưu tuyến giao; hạ tầng Coupang Intelligent Cloud với NVIDIA, GPU utilization từ 65% lên 95% (tự công bố). [About Coupang](https://www.aboutcoupang.com/coupang-nvidia-ai-logistics-collaboration/), [Business Wire](https://www.businesswire.com/news/home/20260317974705/en/Coupang-NVIDIA-Collaboration-on-AI-Factory-Is-Powering-the-Future-of-E-Commerce-Logistics)
- **Gian lận**: chỉ thấy tin tuyển dụng nói có tổ chức Risk, Fraud & Trust xây nền tảng phát hiện gian lận real-time và batch. Không có tài liệu kỹ thuật, nên không đánh ✓. [Coupang Jobs](https://www.coupang.jobs/cn/jobs/8064140/senior-staff-backend-engineer-fraud-detection/)

### Flipkart
- **Voice assistant đa ngôn ngữ** (2020, Supermart, Hindi và English, tự phát hiện ngôn ngữ, hiểu câu trộn ngôn ngữ; tự xây ASR, NLU, MT, TTS). [The Next Web](https://thenextweb.com/in/2020/06/09/walmart-owned-flipkart-introduces-multilingual-voice-assistant-for-grocery-shopping/), [VoiceBot](https://voicebot.ai/2020/06/09/walmarts-indian-e-commerce-subsidiary-flipkart-debuts-voice-assistant-for-grocery-shopping-in-hindi-and-english/)
- **Trợ lý GenAI**: Flippi (2023, gợi ý theo sở thích, ngân sách); báo 01/2026 nói sẽ thay bằng ứng dụng độc lập SLAP (Shop Like a Pro), chưa xác nhận đã ra mắt đầy đủ. Flipkart nói đã triển khai trên 250 mô hình AI và mua cổ phần đa số Minivet AI (video sinh bằng AI từ catalog). [Flipkart Stories](https://stories.flipkart.com/gen-ai-indian-ecommerce-mayur-datar), [Apparel Resources](https://apparelresources.com/business-news/retail/flipkart-accelerates-ai-strategy-250-models-custom-llms/), [MediaNama](https://www.medianama.com/2026/01/223-flipkart-conversational-ai-commerce-slap/), [Outlook Business](https://www.outlookbusiness.com/news/flipkart-buys-majority-stake-in-minivet-ai-to-drive-genai-led-ecomm-innovation)
- **AI viết code**: có bài với tiêu đề "AI now writes 40% of its code" (chỉ thấy tiêu đề). [NewsBytes](https://www.newsbytesapp.com/news/science/flipkart-says-ai-now-writes-40-of-its-code/story)

### Walmart
- **Sparky** (06/2025): trợ lý trong app, tóm tắt và hỏi đáp review, gợi ý, so sánh sản phẩm, trả lời câu hỏi rộng; kế hoạch thêm đặt lại hàng, đặt dịch vụ, đầu vào ảnh/audio/video. Bản thân giới phân tích cho là chưa agentic hoàn toàn. [Retail TouchPoints](https://www.retailtouchpoints.com/news/walmart-introduces-gen-ai-powered-assistant-in-mobile-app/152257), [Retail Dive](https://www.retaildive.com/news/walmart-launches-generative-ai-assistant-sparky/750300/), [Chief Marketer](https://chiefmarketer.com/walmart-launches-generative-ai-shopping-chatbot)
- **Dự báo nhu cầu và tồn kho**: CTO nói tại CES 2024 mô hình ML dùng nhiều loại dữ liệu (lịch sử bán, thời tiết, xu hướng, mạng xã hội) để ước lượng cần bao nhiêu hàng và ở đâu. [Constellation Research](https://www.constellationr.com/insights/news/walmart-ceo-mcmillon-highlights-adaptive-retail-applied-ai-ces-2024), [Retail Dive](https://www.retaildive.com/news/4-walmart-supply-chain-ai-uses/802279)
- **Catalog**: CEO nói dùng nhiều LLM để tạo hoặc cải thiện hơn 850 triệu mẩu dữ liệu catalog (tự công bố). [Constellation Research](https://www.constellationr.com/blog-news/insights/walmarts-genai-automation-omnichannel-initiatives-pay-q2)
- **Trợ lý merchant "Wally"**: nhập dữ liệu, tìm nguyên nhân gốc, hỗ trợ vận hành. [Food Business MEA](https://www.foodbusinessmea.com/?p=2141249)

### eBay
- **Magical Listing**: AI sinh listing từ ảnh sản phẩm (tiêu đề, danh mục, item specifics, giá, vận chuyển); bản bulk trong Seller Hub; còn có thể nhận diện sai món và để trống item specifics nên seller phải kiểm tra. CEO nói số listing mỗi seller tăng hơn 50% ở Mỹ và trên 500 triệu listing có AI hỗ trợ (số tự công bố qua nguồn thứ cấp). [eBay Innovation](https://innovation.ebayinc.com/stories/magical-listing-tool-harnesses-the-power-of-ai-to-make-selling-on-ebay-faster-easier-and-more-accurate/), [3Dsellers](https://www.3dsellers.com/blog/ebay-magical-listing-tool), [CryptoBriefing](https://cryptobriefing.com/ebay-ai-listings-50-percent-increase/), [The Decoder](https://the-decoder.com/ebays-new-ai-app-generates-listings-based-only-on-product-photos/)
- **Trả lời tin nhắn người mua**: tính năng giúp seller Mỹ và Anh trả lời câu hỏi buyer. [eMarketer](https://www.emarketer.com/content/how-shopify-ebay-etsy-use-ai-strengthen-seller-relationships-customer-experience)
- **Agentic commerce**: hợp tác với OpenAI Operator (chưa thấy chi tiết ra mắt). [Digital Commerce 360](https://www.digitalcommerce360.com/?p=1338369)

### Etsy
- **Listing title tool**: gợi ý tiêu đề tối ưu tìm kiếm, bảng Search Visibility, sửa hàng loạt kèm phản hồi thời gian thực. Không tìm thấy nguồn về Gift Mode hay LLM search. [Retail Dive](https://www.retaildive.com/news/etsy-ai-seller-features/760739/)

### Instacart
- **Ask Instacart** (2023): hỏi đáp ngôn ngữ tự nhiên trong thanh tìm kiếm, chạy trên ChatGPT API kết hợp mô hình riêng và dữ liệu catalog, cá nhân hoá theo lịch sử mua, có thể ghép nhu cầu với chiến dịch sponsored của brand; có plugin ChatGPT tạo đơn có thể mua. [Grocery Dive](https://www.grocerydive.com/news/instacart-launches-chatgpt-search-tool-online-grocery/651604), [Supermarket News](https://www.supermarketnews.com/grocery-technology/instacart-rolls-out-ask-instacart-chatgpt-feature), [VoiceBot](https://voicebot.ai/2023/06/02/instacart-embeds-chatgpt-search-tool-for-grocery-shopping/)
- **Caper Cart**: xe đẩy có camera, cảm biến nhận diện món hàng (computer vision). Phần chống trộm/gian lận chỉ thấy tin tuyển dụng, không đánh ✓. [Khosla Jobs](https://jobs.khoslaventures.com/companies/instacart/jobs/65720633-senior-machine-learning-engineer-caper-fraud)

### Shopify
- **Sidekick** (2023, trợ lý trong admin): giải thích xu hướng doanh số, tạo discount, phân tích, sửa theme, tạo customer segment; cuối 2025 nâng thành agent chủ động, dựng automation workflow và app admin tuỳ biến, hỗ trợ giọng nói/video. [WebsiteBuilderExpert](https://www.websitebuilderexpert.com/news/shopify-sidekick), [Retail Brew](https://www.retailbrew.com/stories/2025/12/11/shopify-plugs-in-more-ai-power-to-merchant-assistant)
- **Shopify Magic**: sinh mô tả sản phẩm, gợi ý và gán thuộc tính cho sản phẩm mới, giảm lỗi listing. [WebsiteBuilderExpert](https://www.websitebuilderexpert.com/news/shopify-sidekick)

---

## 3. Must-have / nice-to-have / để sau cho mini-marketplace

**Dữ liệu hệ thống đang có** (theo `docs/design/01_ARCHITECTURE.md` và `02_DATA_MODEL.md`): tracking click/view/cart/impression qua gateway tới analytics-service tới ClickHouse `events_raw`; fact bảng đơn, thanh toán (mô phỏng, `mockpay`), hoàn tiền; tồn kho available/reserved/sold cùng sổ cái chỉ thêm (`inventory_ledgers`); catalog có tìm kiếm không dấu; outbox `domain_events` đẩy Kafka. Không có dữ liệu thật nên cần dữ liệu demo/sinh giả (`SEED_DEMO_DATA`, `scripts/dev/gen_traffic.py`).

Tiêu chí: (a) có dữ liệu sẵn hoặc sinh được; (b) dựng được trong quy mô đồ án (một người/nhóm nhỏ, không GPU riêng); (c) demo thấy ngay giá trị; (d) rủi ro an toàn thấp.

### Must-have
| Chức năng | Vì sao |
|---|---|
| Gợi ý theo click/view/cart | Nhóm phổ biến nhất ở Shopee, Lazada, Alibaba, Flipkart; dữ liệu tracking đã thu; có thể làm bằng item-to-item co-occurrence hoặc matrix factorization nhẹ, không cần LLM. |
| Tìm kiếm ngữ nghĩa + NLP tiếng Việt cho truy vấn | Search nằm đầu phễu ở mọi sàn (Shopee, Instacart, Zalando, Flipkart); catalog đã có tìm kiếm không dấu nên chỉ cần thêm embedding, spell/diacritic restoration, synonym. |
| Trợ lý mua sắm buyer dùng tool-calling (tìm, so sánh, xem đơn) | Hầu hết sàn lớn đã có (Lazada, Amazon, Walmart, Rakuten, TikTok, Alibaba). Làm bằng LLM gọi API sẵn có là khả thi. Phải đi cùng an toàn agent: danh tính lấy từ JWT, không tin ID từ body. |
| Trợ lý seller/admin hỏi đáp số liệu (tool hoặc text-to-SQL) | Amazon Amelia, TikTok Seller Assistant, Shopify Sidekick, Lazada Lazzie Seller; analytics đã có báo cáo theo scope nên tool-calling vào API báo cáo đã phân quyền an toàn hơn text-to-SQL thô. |
| Sinh/phân loại listing (danh mục, thuộc tính, tiêu đề, mô tả) | Gần như mọi sàn có (eBay, Mercari, TikTok, Rakuten, Shopify, Lazada); chi phí thấp, giá trị demo cao, ít rủi ro. |
| Kiểm duyệt + an toàn agent + eval agent | TikTok dùng AI lọc trước khi hiển thị; với dự án, đây cũng là lớp bảo vệ bắt buộc khi cho LLM đọc dữ liệu người dùng (prompt injection qua review/mô tả). Không có eval thì không biết agent có đúng không. |

### Nice-to-have
| Chức năng | Vì sao |
|---|---|
| Review sentiment/spam + tóm tắt review | Chỉ Walmart có nguồn tóm tắt review trong lần tra cứu này, nhưng bài toán gọn, dữ liệu tự sinh được. Cần có bảng review trước. |
| Phát hiện bất thường/gian lận | TikTok, Mercari có bằng chứng. Với dữ liệu mô phỏng thì làm rule + isolation forest trên đơn/thanh toán/tracking là đủ; không có nhãn thật nên khó đánh giá. |
| RAG luật/chính sách | Hữu ích cho trợ lý CSKH (chính sách đổi trả, hoàn tiền của chính dự án); Rakuten, Shopify chỉ gián tiếp. |
| Visual search | Amazon Lens Live, Rakuten; cần embedding ảnh và vector index, ảnh demo ít nên hiệu quả khó thấy. |
| Dự báo nhu cầu/tồn kho | Walmart, Coupang; có `inventory_ledgers` + đơn nên làm được time-series đơn giản, nhưng cần đủ lịch sử (sinh giả). |
| OCR hoá đơn | Không thấy sàn nào công bố trong tra cứu này, nhưng thuộc scope dự án (KYC seller, hoá đơn nhập hàng). |

### Để sau
| Chức năng | Vì sao |
|---|---|
| CLV/churn/định giá động | Cần lịch sử dài và A/B test thật; rủi ro đạo đức/công bằng khi đổi giá theo người dùng. |
| Dự báo logistics/ETA | Coupang cần mạng lưới giao hàng thật; dự án chỉ có trạng thái đơn và carrier mô phỏng. |
| AIOps giám sát | Hạ tầng đã có Prometheus/Grafana; không thấy sàn nào công bố AIOps, giá trị đồ án thấp so với công sức. |
| Video/ảnh sinh, thử đồ ảo, người mẫu ảo, livestream AI | TikTok, Lazada, Alibaba làm nhưng cần mô hình sinh ảnh/video và GPU; không liên quan lõi dữ liệu của dự án. |
| Quảng cáo CTR/auto-bidding | Alibaba (DIN), Shopee; dự án chưa có hệ thống quảng cáo hay đấu giá. |
| Agentic checkout tự chốt đơn | Amazon Auto Buy, TikTok one-click; chỉ nên làm khi đã có đủ guardrail (xác nhận của người dùng, giới hạn giá, idempotency). |

---

## 4. Scope AI chưa có trong danh sách hiện tại

Danh sách hiện tại: gợi ý theo click, tìm kiếm ngữ nghĩa, OCR hoá đơn, hỏi đáp số liệu seller/admin, trợ lý mua sắm/CSKH, RAG luật/chính sách, review sentiment/spam, gian lận/bất thường, dự báo nhu cầu/logistics, phân loại danh mục/thuộc tính/listing, visual search, kiểm duyệt + an toàn agent, CLV/churn/giá, AIOps, NLP tiếng Việt cho truy vấn, eval agent.

Đề xuất bổ sung (dựa trên khoảng trống thấy khi đối chiếu, một số là suy luận của người viết chứ không có nguồn từ sàn nào):

| Đề xuất | Căn cứ |
|---|---|
| **Theo dõi giá + cảnh báo giảm giá / lịch sử giá** | Amazon (giá 30/90/365 ngày, Auto Buy), Alibaba (theo dõi giá 30 ngày). Dự án đã có `product.changed` mang `price`, nên dựng bảng lịch sử giá rất rẻ. |
| **Hiểu truy vấn: autocomplete, sửa lỗi chính tả, khôi phục dấu tiếng Việt, đồng nghĩa** | Mở rộng "NLP tiếng Việt cho truy vấn"; tách riêng vì cần đo bằng click/null-result từ tracking. |
| **Chuẩn hoá địa chỉ Việt Nam** (tách tỉnh/huyện/xã, sửa lỗi) | Suy luận từ `user-service` có địa chỉ giao hàng; giảm giao thất bại. Chưa thấy nguồn sàn công bố. |
| **Đánh giá chất lượng listing và phát hiện trùng lặp/hàng giả** | Mercari (thẩm định hàng giả), eBay (sai item specifics), Lazada (gợi ý tối ưu ảnh/tiêu đề). Khác với "phân loại listing" ở chỗ chấm điểm và gợi ý sửa. |
| **Phân loại lý do trả hàng/hoàn tiền và khiếu nại** | Zalando (giảm trả hàng do size). Dữ liệu `return_reason`, `cancel_reason` đã có trong `orders`. |
| **Cold-start và khám phá (bandit/exploration), đánh giá gợi ý offline** | Gợi ý theo click dễ bị thiên lệch hiển thị; có `impression` nên dùng được để tính CTR có hiệu chỉnh vị trí. |
| **Voucher/khuyến mãi cá nhân hoá và chấm điểm rủi ro lạm dụng voucher** | Shopee (voucher cá nhân hoá). Dự án chưa có voucher thì chỉ là đề xuất sau này. |
| **Seller risk scoring + onboarding probation** | TikTok (probation cho seller mới), Lazada (đánh giá rủi ro cửa hàng). |
| **LLM gateway: quản lý prompt, chi phí, cache, rate limit, ghi log, che PII** | Mercari chuyển GPT-4 sang GPT-4o mini vì chi phí và độ trễ; Shopee tự xây LLM vì lý do tương tự. Là hạ tầng chung cho mọi tính năng LLM. |
| **Feature/data pipeline cho ML (xuất ClickHouse → tập huấn luyện, versioning, giám sát drift)** | ClickHouse chỉ giữ dẫn xuất ~7 ngày từ Kafka; cần chốt cách lưu dữ liệu huấn luyện lâu hơn. |
| **Chống prompt injection từ nội dung người dùng (review, mô tả, tin nhắn)** | Suy luận; mọi trợ lý đọc UGC đều gặp. Gộp vào "kiểm duyệt + an toàn agent" nhưng nên thành mục kiểm thử riêng. |
| **Đồng ý dữ liệu / quyền riêng tư cho cá nhân hoá** | Tracking đã có consent và che PII; cần quy tắc rõ khi dùng dữ liệu cho gợi ý và LLM. |
| **Gắn nhãn nội dung do AI sinh (listing, review) và cho seller duyệt trước khi đăng** | eBay nhắc seller phải kiểm tra bản nháp vì AI có thể nhận sai món. |

Không đề xuất thêm: livestream AI host và tạo video, vì không có nguồn sàn tự làm (chỉ vendor bên thứ ba) và ngoài lõi dữ liệu.

---

## 5. Nguồn (URL)

Mọi URL dưới đây **đã xuất hiện trong kết quả WebSearch** của phiên 2026-10-25; trang gốc chưa được tải toàn văn do WebFetch và curl bị chặn. Tóm tắt trong tài liệu bám theo phần tóm tắt của kết quả tìm kiếm. Riêng các mục ghi "chỉ thấy tiêu đề" thì không có nội dung nào ngoài tiêu đề.

**Shopee / Sea**
- https://www.nasdaq.com/articles/seas-ai-push-openai-could-deepen-shopees-competitive-edge-now
- https://www.nvidia.com/en-us/case-studies/shopee/
- https://www.contentgrip.com/shopee-chatgpt-sea-openai-partnership/
- https://fintool.com/news/google-sea-agentic-ai-partnership
- https://www.webull.com/news/15422657858520064
- https://www.exabytes.my/blog/shopee-shop-ai-assistant/ (bên thứ ba, chưa xác nhận)

**Lazada / Tiki / Sendo**
- https://vneconomy.vn/techconnect/lazada-ra-mat-chatbot-ai-thuong-mai-dien-tu-tai-dong-nam-a.htm
- https://www.sggp.org.vn/lazada-cong-bo-buoc-tien-moi-trong-trai-nghiem-mua-sam-truc-tuyen-ung-dung-ai-post766448.html
- https://e27.co/ai-at-the-core-lazada-shows-how-tech-can-supercharge-sellers-and-shoppers-20250911/
- https://www.bernama.com/en/news.php?id=2358222
- https://www.bigseller.pro/blog/articleDetails/3130/lazada-AI-tools.htm
- https://chrome.google.com/webstore/detail/bee-tracker-trợ-lý-mua-sắ/detail/tiki-assistant/ncpaceoemnbcjffjpjcgnbaklmkhdmak
- https://vneconomy.vn/techconnect/tiktok-shop-shopee-va-lazada-thau-tom-99-thi-truong-thuong-mai-dien-tu-dong-nam-a.htm

**TikTok Shop**
- https://techcrunch.com/2026/10/05/tiktok-rolls-out-an-ai-shopping-assistant-and-one-click-checkout/
- https://ppc.land/tiktok-shop-replaces-seller-homepage-with-5-agent-ai-dashboard-in-beta/
- https://www.socialmediatoday.com/news/tiktok-shop-rolls-out-new-tools-including-expanded-chatbot-access/812401/
- https://www.mediapost.com/publications/article/412106/tiktok-shop-offers-merchants-more-ai-powered-tools.html
- https://www.digitalinformationworld.com/2025/11/tiktok-shop-tightens-grip-on-ai-driven.html
- https://www.globaldatinginsights.com/featured/tiktok-shop-expands-ai-toolkit-with-video-listing-and-dubbing-tools/
- https://ads-up.fr/en/tiktok-shop-rolls-out-new-ai-tools-for-advertisers/ (chỉ thấy tiêu đề)

**Amazon**
- https://aboutamazon.com/news/retail/search-image-amazon-lens-live-shopping-rufus
- https://www.geekwire.com/2025/amazons-lens-live-mobile-shopping-feature-uses-camera-and-ai-to-find-real-time-product-matches/
- https://www.ecomcrew.com/amazons-rufus-can-now-buy-things-for-you-heres-what-that-means-for-sellers/
- https://ppc.land/amazons-rufus-now-shows-a-full-year-of-price-history-to-50m-shoppers/
- https://www.channelnews.com.au/amazon-expands-ai-shopping-assistant-with-12-month-price-tracking-as-usage-surges/
- https://aboutamazon.com/news/innovation-at-amazon/amazon-project-amelia
- https://techcrunch.com/2024/09/19/amazon-debuts-an-ai-assistant-for-sellers-project-amelia
- https://www.nasdaq.com/articles/amazon-unveils-ai-powered-seller-assistant-agentic-capabilities (chỉ thấy tiêu đề)

**Alibaba / Taobao**
- https://www.alibabagroup.com/document-1991231293551017984
- https://www.alizila.com/?p=66879
- https://www.scmp.com/tech/article/3353132/alibaba-taps-chinas-chat-buy-trend-qwen-ai-and-taobao-integration
- https://arxiv.org/pdf/1706.06978v1
- https://arxiv.org/pdf/1809.03672v5

**Mercari / Rakuten / Zalando**
- https://openai.com/index/mercari
- https://speakerdeck.com/mercari_inc/transparency-report-first-half-of-2025
- https://www.telecomtv.com/content/ai/rakuten-integrates-cutting-edge-agentic-ai-tool-rakuten-ai-into-rakuten-ichiba-mobile-app-54595
- https://stellagent.ai/insights/rakuten-ai-store-managers
- https://www.pymnts.com/news/artificial-intelligence/2025/rakuten-taps-agentic-ai-to-enhance-omotenashi-ultimate-service/
- https://pymnts.com/news/retail/2023/zalando-add-chatgpt-powered-fashion-assistant-online-platform
- https://apparelresources.com/technology-news/retail-tech/zalando-add-chatgpt-powered-fashion-assistant/
- https://www.ictbusiness.biz/business/ai-is-reshaping-zalando-from-within
- https://www.yahoo.com/lifestyle/zalando-enlists-chatgpt-search-function-145741671.html (chỉ thấy tiêu đề)
- https://fashionunited.com/news/business/zalando-fully-embraces-ai-90-percent-of-content-now-created-by-machines/2026031371157 (chỉ thấy tiêu đề)

**Coupang / Flipkart / Walmart**
- https://www.aboutcoupang.com/coupang-nvidia-ai-logistics-collaboration/
- https://www.businesswire.com/news/home/20260317974705/en/Coupang-NVIDIA-Collaboration-on-AI-Factory-Is-Powering-the-Future-of-E-Commerce-Logistics
- https://www.coupang.jobs/cn/jobs/8064140/senior-staff-backend-engineer-fraud-detection/ (tin tuyển dụng)
- https://thenextweb.com/in/2020/06/09/walmart-owned-flipkart-introduces-multilingual-voice-assistant-for-grocery-shopping/
- https://voicebot.ai/2020/06/09/walmarts-indian-e-commerce-subsidiary-flipkart-debuts-voice-assistant-for-grocery-shopping-in-hindi-and-english/
- https://stories.flipkart.com/gen-ai-indian-ecommerce-mayur-datar
- https://apparelresources.com/business-news/retail/flipkart-accelerates-ai-strategy-250-models-custom-llms/
- https://www.medianama.com/2026/01/223-flipkart-conversational-ai-commerce-slap/
- https://www.outlookbusiness.com/news/flipkart-buys-majority-stake-in-minivet-ai-to-drive-genai-led-ecomm-innovation
- https://www.newsbytesapp.com/news/science/flipkart-says-ai-now-writes-40-of-its-code/story (chỉ thấy tiêu đề)
- https://www.retailtouchpoints.com/news/walmart-introduces-gen-ai-powered-assistant-in-mobile-app/152257
- https://www.retaildive.com/news/walmart-launches-generative-ai-assistant-sparky/750300/
- https://chiefmarketer.com/walmart-launches-generative-ai-shopping-chatbot
- https://www.constellationr.com/insights/news/walmart-ceo-mcmillon-highlights-adaptive-retail-applied-ai-ces-2024
- https://www.constellationr.com/blog-news/insights/walmarts-genai-automation-omnichannel-initiatives-pay-q2
- https://www.retaildive.com/news/4-walmart-supply-chain-ai-uses/802279
- https://www.foodbusinessmea.com/?p=2141249

**eBay / Etsy / Instacart / Shopify**
- https://innovation.ebayinc.com/stories/magical-listing-tool-harnesses-the-power-of-ai-to-make-selling-on-ebay-faster-easier-and-more-accurate/
- https://www.3dsellers.com/blog/ebay-magical-listing-tool
- https://cryptobriefing.com/ebay-ai-listings-50-percent-increase/
- https://the-decoder.com/ebays-new-ai-app-generates-listings-based-only-on-product-photos/
- https://www.emarketer.com/content/how-shopify-ebay-etsy-use-ai-strengthen-seller-relationships-customer-experience
- https://www.digitalcommerce360.com/?p=1338369
- https://www.retaildive.com/news/etsy-ai-seller-features/760739/
- https://www.grocerydive.com/news/instacart-launches-chatgpt-search-tool-online-grocery/651604
- https://www.supermarketnews.com/grocery-technology/instacart-rolls-out-ask-instacart-chatgpt-feature
- https://voicebot.ai/2023/06/02/instacart-embeds-chatgpt-search-tool-for-grocery-shopping/
- https://jobs.khoslaventures.com/companies/instacart/jobs/65720633-senior-machine-learning-engineer-caper-fraud (tin tuyển dụng)
- https://www.websitebuilderexpert.com/news/shopify-sidekick
- https://www.retailbrew.com/stories/2025/12/11/shopify-plugs-in-more-ai-power-to-merchant-assistant

**Tài liệu nội bộ dùng ở mục 3**
- `docs/design/01_ARCHITECTURE.md`, `docs/design/02_DATA_MODEL.md`
