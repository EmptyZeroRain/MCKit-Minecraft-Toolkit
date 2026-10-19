# ---- 构建阶段 ----
FROM golang:1.27-alpine AS builder

WORKDIR /src
# 先拷贝 go.mod/go.sum 利用缓存
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# 编译 API 与迁移/导入/建号命令（禁用 CGO，减小体积）
RUN go build -ldflags="-s -w" -o /out/api ./cmd/api && \
    go build -ldflags="-s -w" -o /out/migrate ./cmd/migrate && \
    go build -ldflags="-s -w" -o /out/seed ./cmd/seed && \
    go build -ldflags="-s -w" -o /out/create-admin ./cmd/create-admin

# ---- 运行阶段 ----
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

# 拷贝二进制与迁移文件（迁移文件必须随镜像部署）
COPY --from=builder /out/ /usr/local/bin/
COPY --from=builder /src/migrations ./migrations

# 非 root 运行
RUN addgroup -S app && adduser -S app -G app
USER app

EXPOSE 8080

# 默认启动 API；可通过 docker compose command 覆盖为 migrate/seed
ENTRYPOINT ["/usr/local/bin/api"]