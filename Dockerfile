# 多阶段构建：前端 → Go 后端 → 精简运行镜像（内置 web/dist）
# ---- 前端构建 ----
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---- Go 后端构建 ----
FROM golang:1.26-alpine AS go
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/master ./cmd/master && \
    CGO_ENABLED=0 GOOS=linux go build -o /out/agent ./cmd/agent

# ---- 运行镜像 ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=go /out/master /app/master
COPY --from=go /out/agent /app/agent
COPY --from=web /web/dist /app/web/dist
COPY configs/config.example.yaml /app/configs/config.yaml
EXPOSE 18080
ENV APP_PORT=18080
CMD ["/app/master", "-config", "/app/configs/config.yaml"]