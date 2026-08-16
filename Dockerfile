# 多阶段构建 - 第一阶段：编译
FROM golang:1.22-alpine AS builder

# 安装依赖
RUN apk add --no-cache git gcc musl-dev

# 设置工作目录
WORKDIR /build

# 克隆并编译
RUN git clone https://github.com/aib-protocol/aib . && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o aib-node ./cmd/aib-node

# 第二阶段：运行时镜像
FROM alpine:3.19

# 安装运行时依赖
RUN apk update && \
    apk add --no-cache ca-certificates wget tzdata && \
    rm -rf /var/cache/apk/*

# 创建非 root 用户
RUN addgroup -g 1000 aib && \
    adduser -D -u 1000 -G aib aib

# 设置工作目录
WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /build/aib-node /app/aib-node

# 创建数据目录
RUN mkdir -p /data && chown -R aib:aib /data

# 切换到非 root 用户
USER aib

# 暴露端口
EXPOSE 51211 31415

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget -q -s http://localhost:51211/health || exit 1

# 默认命令
CMD ["/app/aib-node", "--validator", "--api-port", "51211", "--p2p-port", "31415", "--data-dir", "/data", "--block-time", "60"]
