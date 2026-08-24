# 多阶段构建：前端 → Go 后端 → 精简运行镜像（内置 web/dist）
#
# 注意：构建上下文 = master 与 agent 两仓库的容器目录（XPanel 项目根），
# 因 master go.mod 暂以 `replace github.com/acdc-awa/xpanel-node => ../agent` 本地引用协议包，
# Docker 构建需要 agent 源码在上下文内。XPanel-Node 发布到 GitHub 后，
# 可删除 replace 并把上下文改回 master 仓库根。
# ---- 前端构建 ----
FROM node:22-alpine AS web
WORKDIR /web
COPY master/web/package.json master/web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY master/web/ ./
RUN npm run build

# ---- Go 后端构建 ----
FROM golang:1.26-alpine AS go
WORKDIR /build
COPY agent/go.mod agent/go.sum ./agent/
COPY master/go.mod master/go.sum ./master/
RUN cd master && go mod download
COPY agent/ ./agent/
COPY master/ ./master/
RUN cd master && CGO_ENABLED=0 GOOS=linux go build -o /out/master ./cmd/master

# ---- 运行镜像 ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D app
WORKDIR /app
COPY --from=go /out/master /app/master
COPY --from=web /web/dist /app/web/dist
COPY master/configs/config.example.yaml /app/configs/config.yaml
RUN mkdir -p /app/data && chown -R app:app /app
USER app
EXPOSE 18080
ENV APP_PORT=18080
ENV APP_ENV=prod
CMD ["/app/master", "-config", "/app/configs/config.yaml"]
