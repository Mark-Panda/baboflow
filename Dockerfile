# ---- 后端构建 ----
FROM golang:1.25-alpine AS build
WORKDIR /src
# 先拉依赖（利用镜像层缓存）
COPY go.mod go.sum ./
RUN GOPROXY=https://goproxy.cn,direct go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOPROXY=https://goproxy.cn,direct \
    go build -trimpath -ldflags="-s -w" -o /out/baboflow ./cmd/baboflow

# ---- 前端构建 ----
FROM node:20-alpine AS web
WORKDIR /web
# 固定 pnpm@8（与本地 8.6.1 / lockfileVersion 6.1 对齐；corepack 的 pnpm11 在 node20 不兼容）
RUN npm install -g pnpm@8
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web ./
RUN pnpm run build   # 产物 dist/

# ---- 运行 ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/baboflow /app/baboflow
# 后端以 ./web/dist 相对路径托管前端（见 internal/server/http.go），故放到 /app/web/dist
COPY --from=web /web/dist /app/web/dist
# 工作区（Agent 内置工具沙箱），可被 volume 覆盖持久化
RUN mkdir -p /app/workspace
ENV BABO_WORKSPACE=/app/workspace
EXPOSE 8000
ENTRYPOINT ["/app/baboflow"]
