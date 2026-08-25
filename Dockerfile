# 多阶段构建：前端 → Go 后端 → 精简运行镜像（内置 web/dist）
#
# 注意：2026-08-24 XPanel-Node v0.1.0 发布后已解除本地 replace——
# 构建上下文 = master 仓库根（deploy/master 的 ../..），协议包由 go.mod require v0.1.0 从 GitHub 拉取。
# ---- 前端构建 ----
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---- Go 后端构建 ----
FROM golang:1.26-alpine AS go
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/master ./cmd/master

# ---- 运行镜像 ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D app
WORKDIR /app
COPY --from=go /out/master /app/master
COPY --from=web /web/dist /app/web/dist
COPY configs/config.example.yaml /app/configs/config.yaml
RUN mkdir -p /app/data && chown -R app:app /app
USER app
EXPOSE 18080 18082 6000
ENV APP_PORT=18080
ENV APP_WS_PORT=18082
ENV APP_SUB_PORT=6000
ENV APP_ENV=prod
CMD ["/app/master", "-config", "/app/configs/config.yaml"]