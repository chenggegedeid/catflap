# ═══════════════════════════════════════════════════════════════════════════
# Catflap —— 多阶段构建
#
# 注意顺序：后端用 //go:embed 嵌入 web/admin/dist，所以**必须先构建前端**。
# （导航主页 web/home 已在 v2.1.0 之后移除，这里只剩一个前端。）
# ═══════════════════════════════════════════════════════════════════════════

# ── 阶段 1：构建前端 ────────────────────────────────────────────────────────
FROM node:22-alpine AS web
WORKDIR /src

COPY web/ ./web/

# 用 npm ci 保证按 package-lock.json 精确安装
RUN cd web/admin && npm ci --no-audit --no-fund && npm run build

# ── 阶段 2：构建后端 ────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS backend
WORKDIR /src

# go.mod 要求 go 1.25.1；如果基础镜像的补丁版本更低，GOTOOLCHAIN=auto 会自动
# 下载对应的工具链，此时构建机需要能访问网络或代理。
ENV GOTOOLCHAIN=auto

# 先拷依赖清单，让 go mod download 单独成层（改代码不会重新下载依赖）
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# 用阶段 1 构建出来的 dist 覆盖仓库里的那份，保证镜像里的前端与源码一致
COPY --from=web /src/web/admin/dist ./web/admin/dist

ARG VERSION=dev
# ⚠️ 注入的是**裸版本号**（1.0.0，不带 v）：
#    modules/update/update.go 的 newerThan() 用 strconv.Atoi 逐段比数字，
#    本地版本带 v 会被解析成 0，导致程序永远认为「有新版本」。
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X linkstar/api/system_api.Version=${VERSION}" \
      -o /out/catflap .

# ── 阶段 3：运行 ────────────────────────────────────────────────────────────
FROM alpine:3.20

# ca-certificates 是必须的：ACME 签发、GitHub 更新检查、DNS 服务商 API 都走 HTTPS。
# tzdata 让日志时间与计划任务时区正确。
RUN apk add --no-cache ca-certificates tzdata

# 专用非 root 用户。面板能读写明文凭据、能把内网服务暴露到公网，
# 用 root 跑会把后果放大 —— 详见 SECURITY.md 的部署安全清单。
RUN adduser -D -H -u 10001 catflap

COPY --from=backend /out/catflap /usr/local/bin/catflap

# ⚠️ 工作目录是这个程序的命门：它按**工作目录**读写 config/ 与 data/。
WORKDIR /opt/catflap
RUN install -d -o catflap -g catflap -m 0700 config data

USER catflap
# 3791 = core.DefaultPort（core/listen.go）。改了那边的默认值这里要跟着改，
# 否则健康检查会一直打在一个没人听的端口上，容器永远 unhealthy。
EXPOSE 3791

# 这两个目录必须挂卷，否则容器重建就丢配置
VOLUME ["/opt/catflap/config", "/opt/catflap/data"]

# 健康检查：只要进程还能响应版本接口就算活着
# （路由是 /api/version，见 routers/system_routers.go 的 SystemRouters）
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD wget -qO- http://127.0.0.1:3791/api/version >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/usr/local/bin/catflap"]
