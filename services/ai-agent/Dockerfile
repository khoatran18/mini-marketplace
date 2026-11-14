# syntax=docker/dockerfile:1
# Build context = REPO ROOT như các service khác: docker build -f services/ai-agent/Dockerfile .
FROM python:3.12-slim
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PIP_NO_CACHE_DIR=1
WORKDIR /app
# phụ thuộc trước (tận dụng cache lớp); dùng requirements.lock.txt nếu đã sinh (pip freeze), không thì requirements.txt
COPY services/ai-agent/requirements*.txt ./
RUN if [ -f requirements.lock.txt ]; then pip install -r requirements.lock.txt; else pip install -r requirements.txt; fi
COPY services/ai-agent/src ./src
COPY services/ai-agent/configs ./configs
ARG SERVICE_VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown
ENV SERVICE_VERSION=$SERVICE_VERSION GIT_COMMIT=$GIT_COMMIT BUILD_TIME=$BUILD_TIME PYTHONPATH=/app/src APP_ENV=prod
RUN useradd --system --no-create-home app
USER app
EXPOSE 8090
CMD ["uvicorn", "ai_agent.main:create_app", "--factory", "--host", "0.0.0.0", "--port", "8090"]
