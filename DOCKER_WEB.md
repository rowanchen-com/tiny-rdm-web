# Tiny RDM Docker Web 版

基于 Tiny RDM v1.2.6，新增 Docker Web 部署模式。一套代码同时支持 Wails 桌面客户端和 Docker Web 两种运行方式，原始组件零修改。

---

## 目录

- [快速启动](#快速启动)
- [环境变量](#环境变量)
- [登录认证](#登录认证)
- [架构概览](#架构概览)
- [后端详解](#后端详解)
- [前端详解](#前端详解)
- [Docker 构建](#docker-构建)
- [CI/CD](#cicd)
- [安全特性](#安全特性)
- [数据持久化](#数据持久化)
- [移动端适配](#移动端适配)
- [功能对比](#功能对比)
- [完整文件清单](#完整文件清单)
- [同步上游更新](#同步上游更新)

---

## 快速启动

```bash
# 构建并启动
docker-compose up -d --build

# 访问
http://localhost:8088

# 查看日志
docker-compose logs -f

# 停止
docker-compose down
```

### docker-compose.yml 示例

```yaml
services:
  tiny-rdm:
    build: .
    ports:
      - "8088:8088"
    environment:
      - ADMIN_USERNAME=admin
      - ADMIN_PASSWORD=your_password
      - SESSION_TTL=24h
    volumes:
      - ./data:/app/TinyRDM
    restart: unless-stopped
```

### 反向代理（Nginx 示例）

```nginx
server {
    listen 443 ssl;
    server_name rdm.example.com;

    location / {
        proxy_pass http://127.0.0.1:8088;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Host $host;
    }

    location /ws {
        proxy_pass http://127.0.0.1:8088;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Host $host;
    }
}
```

> CORS 验证支持 `X-Forwarded-Host`，反向代理场景下需设置此头。

---

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PORT` | `8088` | HTTP 监听端口 |
| `GIN_MODE` | `release` | Gin 框架运行模式 |
| `ADMIN_USERNAME` | 无 | 登录用户名（与密码同时设置才启用认证） |
| `ADMIN_PASSWORD` | 无 | 登录密码 |
| `SESSION_TTL` | `24h` | 会话有效期（Go Duration 格式，如 `12h`、`30m`） |

不设置用户名密码则免登录直接使用（向后兼容）。

---

## 登录认证

### 工作流程

1. 用户打开页面 → 前端调用 `GET /api/auth/status` 检查认证状态
2. 认证已启用且未登录 → 显示登录页面（自动检测浏览器语言，支持 10 种语言）
3. 输入凭据 → `POST /api/auth/login` 验证
4. 通过 → 后端生成 HMAC-SHA256 Token，写入 httpOnly Cookie
5. 前端调用 `ReconnectWebSocket()` 重建带 Cookie 的 WebSocket 连接
6. 加载偏好设置、同步 i18n 语言、切换到桌面布局 viewport
7. 后续所有请求和 WebSocket 连接自动携带 Cookie
8. 退出 → `POST /api/auth/logout` 清除 Cookie，触发 `rdm:unauthorized` 事件返回登录页

### 登录页面

- 自动检测浏览器语言（zh/tw/en/ja/ko/es/fr/ru/pt/tr）
- NaiveUI 组件，契合 Tiny RDM UI 风格，卡片宽度 420px
- 主题切换（自动/浅色/暗黑）+ 语言选择器，位于登录按钮下方工具栏，`n-dropdown` hover 触发
- 主题和语言选择存储在 `localStorage`（`rdm_login_theme` / `rdm_login_lang`），登录成功后同步到后端偏好设置
- 启动时自动应用已保存的主题，防止页面闪烁
- SVG stroke 图标（太阳/月亮/半圆/语言），颜色跟随 NaiveUI 主题
- 页脚显示动态版本号 + GitHub 链接
- 副标题 "Redis Web Manager"（区别于桌面版）
- 暗色模式下标题颜色正确跟随主题（`v-bind('themeVars.textColor1')`）
- 移动端自适应（`@media max-width: 480px`）

### API 路由

```
公开路由（无需认证）：
  POST /api/auth/login       POST /api/auth/logout
  GET  /api/auth/status      GET  /api/version

受保护路由（需要认证）：
  /api/connection/*    /api/browser/*     /api/cli/*
  /api/monitor/*       /api/pubsub/*      /api/preferences/*
  /api/system/*        /ws (WebSocket)
```

---

## 架构概览

### 项目结构（Web 新增部分标注 ★）

```
├── main.go                          # 桌面入口（添加 //go:build !web）
├── main_web.go                    ★ # Web 入口（//go:build web）
├── go.mod / go.sum                  # 新增 gin 依赖
├── Dockerfile                     ★ # 三阶段构建
├── docker-compose.yml             ★ # 一键部署
├── .dockerignore                  ★ # 构建排除
├── DOCKER_WEB.md                  ★ # 本文档
├── docker/
│   └── entrypoint.sh              ★ # 容器入口脚本
├── .github/workflows/
│   ├── docker-publish.yml         ★ # Docker 镜像发布
│   └── release-windows.yaml       ★ # Windows 桌面发布
├── backend/
│   ├── api/                       ★ # 全部 //go:build web（10 个文件）
│   │   ├── router.go                # Gin 路由器
│   │   ├── auth.go                  # 登录认证
│   │   ├── websocket_hub.go         # WebSocket 连接池
│   │   ├── connection_api.go        # 连接 API
│   │   ├── browser_api.go           # 数据浏览 API
│   │   ├── cli_api.go               # CLI API
│   │   ├── monitor_api.go           # Monitor API
│   │   ├── pubsub_api.go            # Pubsub API
│   │   ├── preferences_api.go       # 偏好设置 API
│   │   └── system_api.go            # 系统 API
│   └── services/
│       ├── platform_desktop.go    ★ # //go:build !web
│       ├── platform_web.go        ★ # //go:build web
│       ├── connection_service_web.go ★ # //go:build web
│       └── *_service.go             # 7 个文件修改（runtime.* → services.*）
└── frontend/src/
    ├── utils/
    │   ├── api.js                 ★ # HTTP API 适配器
    │   ├── wails_runtime.js       ★ # Wails Runtime 替代
    │   ├── websocket.js           ★ # WebSocket 客户端
    │   └── platform.js              # 新增 isWeb()
    ├── components/
    │   ├── LoginPage.vue          ★ # 登录页面
    │   └── icons/Logout.vue       ★ # 退出图标
    ├── App.vue                      # 认证门控 + viewport
    ├── AppContent.vue               # 隐藏窗口按钮
    └── public/favicon.png         ★ # 浏览器图标
```

### 双模式对比

| | 桌面模式 (Wails) | Web 模式 (Docker) |
|---|---|---|
| 构建命令 | `wails build` | `go build -tags web` |
| 入口文件 | `main.go` (`//go:build !web`) | `main_web.go` (`//go:build web`) |
| 前后端通信 | Wails RPC（进程内调用） | HTTP REST API (Gin) |
| 事件系统 | Wails Runtime Events | WebSocket 双向推送 |
| 文件对话框 | 原生系统对话框 | HTML `<input type="file">` + 后端暂存到 TempDir |
| 剪贴板 | Wails Runtime | `navigator.clipboard` API + `execCommand` 降级 |
| 认证 | 无（本地应用） | Cookie + HMAC Token（可选） |
| 窗口管理 | 原生窗口控制 | 浏览器标签页（隐藏窗口按钮） |
| 连接导入导出 | 原生文件对话框 | HTTP 文件上传/下载 |

### 架构图

```
┌──────────────────────────────────────────────────────────┐
│  桌面模式 (!web)                                          │
│                                                          │
│  Vue 前端 → wailsjs/ RPC 绑定 → Wails Runtime → Go      │
│                                               Services   │
│  （backend/api/ 整个目录不参与编译）                        │
└──────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────┐
│  Web 模式 (web)                                          │
│                                                          │
│  Vue 前端                                                 │
│    ├─ api.js ─── HTTP POST/GET ──→ Gin Router → Go      │
│    │                                          Services   │
│    └─ wails_runtime.js ── WebSocket ──→ WSHub ← Go      │
│                                          Services        │
│                                                          │
│  platform_web.go 回调:                                    │
│    EmitEventFunc → api.Hub().Emit (服务端推送事件)         │
│    RegisterHandlerFunc → api.RegisterHandler (客户端事件)  │
│                                                          │
│  main_web.go 启动时连接:                                   │
│    services.EmitEventFunc = api.Hub().Emit               │
│    services.RegisterHandlerFunc = api.RegisterHandler    │
└──────────────────────────────────────────────────────────┘
```

### 核心机制：Go 构建标签

通过 `//go:build` 构建标签实现编译时切换：

```
桌面编译 (!web):  main.go → services/*.go + platform_desktop.go → Wails Runtime

Web 编译 (web):   main_web.go → services/*.go + platform_web.go → 回调 → api 包
```

- `platform_desktop.go` / `platform_web.go` 提供完全相同的函数签名和类型定义
- Service 层调用 `services.EventsEmit()` 等平台抽象函数，编译时自动链接正确实现
- `backend/api/` 下全部 10 个文件均带 `//go:build web`，桌面编译完全不包含

### 核心机制：Vite 条件别名

通过 `VITE_WEB=true` 环境变量控制（桌面构建时别名不生效）：

| 原始导入路径 | Web 模式实际指向 |
|---|---|
| `wailsjs/runtime/runtime.js` | `src/utils/wails_runtime.js` |
| `wailsjs/go/services/connectionService.js` | `src/utils/api.js` |
| `wailsjs/go/services/browserService.js` | `src/utils/api.js` |
| `wailsjs/go/services/cliService.js` | `src/utils/api.js` |
| `wailsjs/go/services/monitorService.js` | `src/utils/api.js` |
| `wailsjs/go/services/pubsubService.js` | `src/utils/api.js` |
| `wailsjs/go/services/preferencesService.js` | `src/utils/api.js` |
| `wailsjs/go/services/systemService.js` | `src/utils/api.js` |

> `wailsjs` 通配别名放在最后，确保具体路径优先匹配。Store 层和组件代码零修改。

### 避免循环导入

`services` 包不能直接导入 `api` 包（`api` 已导入 `services`）。
解决方案：`platform_web.go` 定义回调变量 `EmitEventFunc` / `RegisterHandlerFunc`，由 `main_web.go` 启动时赋值为 `api` 包的函数。

---

## 后端详解

### 原始文件修改

**`main.go`** — 仅添加 `//go:build !web` 构建标签，其余不变。

**`go.mod`** — 新增 `github.com/gin-gonic/gin v1.10.1` direct 依赖（`gorilla/websocket` 已是原版 indirect 依赖）。Gin 引入的 indirect 依赖包括 `bytedance/sonic`、`go-playground/validator`、`goccy/go-json`、`ugorji/go/codec`、`google.golang.org/protobuf` 等。

**`backend/storage/local_storage.go`** — 文件/目录创建权限从 `0777` 改为 `0600`（文件）/ `0700`（目录），安全加固。

**7 个 Service 文件** — 统一替换模式：

```go
// 改动前（直接调用 Wails）：
import "github.com/wailsapp/wails/v2/pkg/runtime"
runtime.EventsEmit(s.ctx, "event_name", data)

// 改动后（调用平台抽象层）：
services.EventsEmit(s.ctx, "event_name", data)
```

涉及：`browser_service.go`、`cli_service.go`、`connection_service.go`、`monitor_service.go`、`pubsub_service.go`、`preferences_service.go`、`system_service.go`

同理，`runtime.OpenFileDialog`、`runtime.SaveFileDialog`、`runtime.EventsOn` 等调用也做相同替换。

### 新增文件

**平台抽象层**（构建标签切换，提供相同函数签名）：

| 文件 | 标签 | 说明 |
|---|---|---|
| `backend/services/platform_desktop.go` | `!web` | 封装 Wails Runtime 函数 + 类型别名指向 Wails 类型 |
| `backend/services/platform_web.go` | `web` | 回调变量桥接到 WebSocket + Stub 类型替代 Wails 类型 |
| `backend/services/connection_service_web.go` | `web` | Web 专用 `ExportConnectionsToBytes()` / `ImportConnectionsFromBytes()` |

**HTTP API 层**（全部 `//go:build web`，桌面编译不包含）：

| 文件 | 说明 |
|---|---|
| `backend/api/router.go` | Gin 路由器：请求体 10MB 限制、安全头、CORS、CSRF、静态文件服务 |
| `backend/api/auth.go` | 登录认证：HMAC Token、限速器（maxEntries=10000）、安全中间件 |
| `backend/api/websocket_hub.go` | WebSocket 连接池（最大 50 连接）、事件广播、消息分发 |
| `backend/api/connection_api.go` | 连接管理 API + Web 专用导出下载/导入上传端点 |
| `backend/api/browser_api.go` | 数据浏览 API（~50 个端点，覆盖所有数据类型） |
| `backend/api/cli_api.go` | CLI 会话 API（输入通过 WebSocket 事件） |
| `backend/api/monitor_api.go` | Monitor API |
| `backend/api/pubsub_api.go` | Pubsub API |
| `backend/api/preferences_api.go` | 偏好设置 API |
| `backend/api/system_api.go` | 系统信息 + 文件上传下载 API（含路径遍历防护） |

**Web 入口**：

| 文件 | 说明 |
|---|---|
| `main_web.go` | `//go:build web`，Gin HTTP 服务器、回调连接、服务初始化、优雅关闭 |

---

## 前端详解

### 原始文件修改

**`vite.config.js`** — 通过 `process.env.VITE_WEB` 条件启用 8 个别名 + dev server proxy，桌面模式不受影响。

**`App.vue`** — 改动最大的文件：
- `isWebMode` 编译时常量（`import.meta.env.VITE_WEB === 'true'`）完全分离桌面/Web 流程
- `LoginPage` 使用 `defineAsyncComponent` 懒加载，避免桌面模式引入 WebSocket 依赖
- Web 模式：认证门控 → viewport 管理 → WebSocket 重连 → 初始化应用
- 桌面模式：直接调用 `initApp()`，与原版行为一致
- `onLogin()` 通过动态 `import()` 调用 `ReconnectWebSocket()`，避免静态导入
- `onLogin()` 登录后同步 `localStorage` 中的主题和语言选择到后端偏好设置
- `onMounted()` 启动时应用已保存的登录主题，防止页面闪烁
- 模板三段条件渲染：`authChecking` → 空白 / `!authenticated` → LoginPage / `else` → 原始内容

**`AppContent.vue`** — `isWeb()` 判断隐藏窗口控制按钮 + Web 模式使用 Windows 样式 + `100dvh`。

**`Ribbon.vue`** — Web 模式侧边栏底部新增退出登录按钮。

**`platform.js`** — 新增 `isWeb()` 函数，导入路径保持原版不变。

**`ContentCli.vue`** — `WaitForWebSocket` 从静态导入改为 `import.meta.env.VITE_WEB` 条件动态导入（Wails runtime 不导出此函数，静态导入会导致桌面构建失败）。

**`connections.js` (store)** — 修复 `exportConnections()` / `importConnections()` 取消操作时仍显示"操作成功"的 bug（`return` 移到外层 `if (!success)`）。

**`style.scss`** — 新增 `overscroll-behavior: none`（禁止页面过度滚动弹性效果）+ `height: 100dvh`（动态视口高度）。

**`frontend/src/langs/*.json`（10 个语言文件）** — 每个文件新增 `"logout"` 翻译键（zh/tw/en/ja/ko/es/fr/ru/pt/tr），用于侧边栏退出按钮。

**`index.html`** — 新增 `<link rel="icon" href="/favicon.png" />`。

### 新增文件

| 文件 | 说明 |
|---|---|
| `src/utils/api.js` | HTTP API 适配器，~80 个导出函数，签名与 Wails 绑定完全一致。内含 `post()`/`get()`/`del()` 基础函数，401 时自动触发 `rdm:unauthorized`。连接导入导出使用文件上传/下载替代原生对话框。 |
| `src/utils/wails_runtime.js` | Wails Runtime Web 替代：事件→WebSocket、剪贴板→`navigator.clipboard`（含权限拒绝抛错）、窗口管理→no-op、`Environment()` 返回 `platform: 'web'`。导出 `ReconnectWebSocket` / `WaitForWebSocket`。 |
| `src/utils/websocket.js` | WebSocket 客户端：自动重连（3 秒间隔）、事件监听/分发（`Map<event, Set<callback>>`）、`waitForWebSocket()` Promise、`reconnectWebSocket()` 强制重连。协议自动检测（`ws:`/`wss:`）。 |
| `src/components/LoginPage.vue` | 登录页面：10 语言自动检测（内置翻译字典）、NaiveUI 组件、版本号动态获取、移动端响应式。 |
| `src/components/icons/Logout.vue` | 退出登录 SVG 图标（48x48 viewBox，`stroke="currentColor"`）。 |
| `public/favicon.png` | 浏览器标签页图标（复制自 `src/assets/images/icon.png`）。 |

---

## Docker 构建

### 三阶段构建

```dockerfile
# 阶段 1：构建前端（Node 20 Alpine）
FROM node:20-alpine AS frontend-builder
# npm ci → VITE_WEB=true npm run build → 产出 frontend/dist/

# 阶段 2：构建后端（Go 1.24 Alpine）
FROM golang:1.24-alpine AS backend-builder
# 复制 go.mod + 源码 + 前端 dist
# go mod tidy → go build -tags web → 产出 tinyrdm 单二进制

# 阶段 3：运行时（Alpine 3.21 + Noto 字体）
FROM alpine:3.21
# 包含 tinyrdm 二进制 + entrypoint.sh + Noto 字体（支持字体选择列表）
```

### 关键构建参数

| 参数 | 说明 |
|---|---|
| `-tags web` | 激活 Web 模式构建标签 |
| `-ldflags "-s -w"` | 去除调试信息，减小二进制体积 |
| `-X main.version=1.2.6` | 注入版本号 |
| `CGO_ENABLED=0` | 纯静态编译，无 C 依赖 |
| `VITE_WEB=true` | 激活 Vite 条件别名 |
| `GOPROXY=goproxy.cn,goproxy.io,direct` | Go 模块代理（中国网络优化） |
| `NODE_OPTIONS=--max-old-space-size=4096` | 前端构建内存限制 |
| `XDG_CONFIG_HOME=/app` | 配置文件存储在 `/app/TinyRDM/` |

### 相关文件

| 文件 | 说明 |
|---|---|
| `Dockerfile` | 三阶段构建定义 |
| `docker-compose.yml` | 一键部署：端口映射、环境变量、数据卷、`restart: unless-stopped` |
| `docker/entrypoint.sh` | 入口脚本，透传环境变量 |
| `.dockerignore` | 排除 .git、node_modules、docs、build、*.md 等 |

---

## CI/CD

### Docker 镜像发布（`docker-publish.yml`）

- 触发条件：仅 `release published` + 手动触发（`workflow_dispatch`）
- 镜像仓库：`ghcr.io/rowanchen-com/tiny-rdm-web`
- 标签策略：语义化版本（`{{version}}`、`{{major}}.{{minor}}`）+ `latest`
- 构建平台：`linux/amd64`

> 不在每次 push 时自动构建，仅发布 Release 时触发。

### Windows 桌面应用发布（`release-windows.yaml`）

- 触发条件：`release published` + 手动触发
- 构建平台：`windows/amd64` + `windows/arm64`
- 签名方式：CI 中 `New-SelfSignedCertificate` 生成自签名证书（无需外部密钥）
- 产物：`TinyRDM_Portable_*.zip`（便携版）+ `TinyRDM_Setup_*.exe`（NSIS 安装版，已签名）
- 时间戳服务器：`http://timestamp.digicert.com`

#### 相比原版 v1.2.6 的修复（3 处）

1. **新增 "Add NSIS to PATH" 步骤** — chocolatey 安装 NSIS 后，全局写入 `$env:GITHUB_PATH`，修复 `makensis not found` 导致 Wails 静默跳过安装包生成的 bug（原版 v1.2.6 和 v1.2.5 均未加此步骤，v1.2.5 能用是因为当时 GitHub runner 镜像 NSIS 自动在 PATH，后来 runner 更新后不再自动加入）
2. **签名步骤替换** — 原版使用 `dlemstra/code-sign-action@v1` + `WIN_SIGNING_CERT` secret（付费证书），改为 `New-SelfSignedCertificate` 自签名 + 检测 `*-installer.exe` 是否真正生成
3. **Rename 步骤修正** — 原版 `Rename-Item -Path "Tiny RDM.exe"`（实际是 portable exe），改为 `Get-ChildItem -Filter "*-installer.exe"` 动态查找 NSIS 真正输出的 installer，带错误处理

---

## 安全特性

### 认证安全

| 特性 | 说明 |
|---|---|
| httpOnly Cookie | JavaScript 无法读取，防 XSS 窃取 |
| SameSite=Strict | 仅同站请求发送，防 CSRF |
| IP 绑定 | Token 与客户端 IP 绑定 |
| 登录限速 | 同一 IP 每分钟 5 次，`maxEntries=10000` + 5 分钟周期清理 |
| 常量时间比较 | `hmac.Equal` 比较凭据，失败延迟 500ms |
| 随机签名密钥 | 每次容器启动随机生成 32 字节，重启即失效 |

### 网络安全

| 特性 | 说明 |
|---|---|
| CORS | Origin 与 Host 同源验证（支持 `X-Forwarded-Host` 反向代理） |
| CSRF | 非 GET 请求验证 Origin/Referer |
| 请求体限制 | 全局 10MB |
| WebSocket | 最大 50 连接，Cookie + Origin 认证 |

### 文件操作安全

| 特性 | 说明 |
|---|---|
| 上传 | `sanitizeFilename()` 去除路径分隔符和 `..`，防路径遍历 |
| 下载 | `safeTempPath()` 验证路径必须在 `os.TempDir()` 内 |

### HTTP 安全响应头

`X-Content-Type-Options: nosniff`、`X-Frame-Options: SAMEORIGIN`、`X-XSS-Protection: 1; mode=block`、`Referrer-Policy: strict-origin-when-cross-origin`、`Content-Security-Policy`（`script-src` 和 `connect-src` 白名单包含 `https://static.cloudflareinsights.com` 和 `https://analytics.tinycraft.cc`，仅允许这两个特定域名）

---

## 数据持久化

容器内 `/app/TinyRDM/`，通过 `./data:/app/TinyRDM` 映射到宿主机。

| 文件 | 说明 |
|---|---|
| `connections.yaml` | Redis 连接配置（密码明文，与原版一致，注意文件权限） |
| `preferences.yaml` | 偏好设置（语言、主题、字体等） |
| `device.txt` | 设备 ID（GA 统计用，Web 版未启用 GA，无实际作用） |

---

## 移动端适配

### Viewport 动态管理

`App.vue` 中 `setViewport(mode)` 根据场景切换：

| 场景 | viewport 策略 |
|---|---|
| 登录页 | `width=device-width, initial-scale=1.0, user-scalable=no`（标准响应式） |
| 主界面 - 竖屏手机 | `width=680, user-scalable=yes` |
| 主界面 - 横屏/小屏 | `width=1280, user-scalable=yes` |
| 主界面 - 桌面/平板 | `width=1024, user-scalable=yes` |

### CSS 适配

- `AppContent.vue`：`height: 100dvh`（动态视口高度，解决移动端地址栏遮挡）
- `LoginPage.vue`：`@media (max-width: 480px)` 响应式样式
- 监听 `orientationchange` + `resize` 事件，200ms 防抖重新计算 viewport

---

## 功能对比

| 功能 | 桌面版 | Web 版 |
|---|---|---|
| Redis 全数据类型操作 | ✅ | ✅ |
| CLI 终端 | ✅ | ✅（WebSocket 实时交互） |
| Monitor / Pubsub | ✅ | ✅ |
| 多语言（10 种） | ✅ | ✅ |
| 键导入导出（CSV） | ✅ | ✅ |
| 连接导入导出 | ✅ 原生对话框 | ✅ HTTP 上传/下载 |
| 慢日志 / 客户端列表 | ✅ | ✅ |
| 窗口控制按钮 | ✅ 显示 | ❌ 隐藏（浏览器自带） |
| 登录认证 | ❌ 无 | ✅ 可选 |
| 退出登录按钮 | ❌ 无 | ✅ 侧边栏底部 |
| 移动端适配 | ❌ 不适用 | ✅ 动态 viewport |
| Favicon | ❌ 无 | ✅ |
| 刷新拦截 | ❌ 不需要 | ✅ 拦截 F5/Ctrl+R/Cmd+R 防止丢失状态 |
| 字体列表 | ✅ 系统字体 | ✅ Noto 字体（Docker 内置） |
| Google Analytics | ✅ 启用 | ❌ 禁用 |
| CORS/CSRF 防护 | ❌ 不需要 | ✅ |

---

## 完整文件清单

### 修改的原始文件（33 个）

| 文件 | 改动说明 | 改动量 |
|---|---|---|
| `main.go` | 添加 `//go:build !web` | +1 行 |
| `go.mod` | 新增 gin direct 依赖 | ~5 行 |
| `go.sum` | 新增依赖校验和 | ~25 行 |
| `backend/services/browser_service.go` | `runtime.*` → `services.*` | ~15 处 |
| `backend/services/cli_service.go` | 同上 | ~5 处 |
| `backend/services/connection_service.go` | 同上 | ~10 处 |
| `backend/services/monitor_service.go` | 同上 | ~5 处 |
| `backend/services/pubsub_service.go` | 同上 | ~5 处 |
| `backend/services/preferences_service.go` | 同上 | ~3 处 |
| `backend/services/system_service.go` | 同上 | ~3 处 |
| `backend/storage/local_storage.go` | 文件权限 `0777` → `0600`/`0700`（安全加固） | 2 处 |
| `frontend/vite.config.js` | 条件别名 + proxy | +30 行 |
| `frontend/src/App.vue` | 认证门控 + viewport + 双模式分离 + 拦截 F5/Ctrl+R 刷新 | 重写 |
| `frontend/src/AppContent.vue` | 隐藏窗口按钮 + `isWeb()` + `100dvh` | +10 行 |
| `frontend/src/components/sidebar/Ribbon.vue` | 退出登录按钮 | +15 行 |
| `frontend/src/utils/platform.js` | 新增 `isWeb()` | +3 行 |
| `frontend/src/assets/styles/style.scss` | `overscroll-behavior: none` + `height: 100dvh` | +2 行 |
| `frontend/src/components/content_value/ContentCli.vue` | `WaitForWebSocket` 动态导入 | +15 行 |
| `frontend/src/stores/connections.js` | 修复取消操作误显示成功 | 2 处 |
| `frontend/src/utils/analytics.js` | `loadModule` 加 try-catch 防 CSP 阻断，`trackEvent` 检查 `typeof umami` | +5 行 |
| `frontend/src/components/dialogs/AboutDialog.vue` | 版权年份 2025 → 2026 | 1 处 |
| `frontend/index.html` | favicon | +1 行 |
| `frontend/src/main.js` | Web 模式跳过 `loadPreferences()`（App.vue 认证后处理） | +3 行 |
| `frontend/src/langs/*.json`（10 个语言文件） | 每个文件新增 `"logout"` 翻译键 | 每文件 +1 行 |

### 新增文件（27 个）

| 文件 | 行数 | 说明 |
|---|---|---|
| `main_web.go` | 85 | Web 入口，`//go:build web` |
| `backend/services/platform_desktop.go` | 75 | 桌面平台抽象，`//go:build !web` |
| `backend/services/platform_web.go` | 110 | Web 平台抽象，`//go:build web` |
| `backend/services/connection_service_web.go` | 95 | Web 专用连接导入导出，`//go:build web` |
| `backend/api/router.go` | 165 | Gin 路由器，`//go:build web` |
| `backend/api/auth.go` | 260 | 登录认证（HMAC Token、限速器、安全中间件、CSP 白名单），`//go:build web` |
| `backend/api/websocket_hub.go` | 120 | WebSocket 连接池，`//go:build web` |
| `backend/api/connection_api.go` | 170 | 连接 API，`//go:build web` |
| `backend/api/browser_api.go` | ~600 | 数据浏览 API，`//go:build web` |
| `backend/api/cli_api.go` | 40 | CLI API，`//go:build web` |
| `backend/api/monitor_api.go` | 45 | Monitor API，`//go:build web` |
| `backend/api/pubsub_api.go` | 50 | Pubsub API，`//go:build web` |
| `backend/api/preferences_api.go` | 55 | 偏好设置 API，`//go:build web` |
| `backend/api/system_api.go` | 120 | 系统 API，`//go:build web` |
| `frontend/src/utils/api.js` | ~420 | HTTP API 适配器 |
| `frontend/src/utils/wails_runtime.js` | 80 | Wails Runtime 替代（WebSocket 延迟连接，登录后才建立） |
| `frontend/src/utils/websocket.js` | 110 | WebSocket 客户端 |
| `frontend/src/components/LoginPage.vue` | 200 | 登录页面（含主题/语言选择器、10 语言内置翻译） |
| `frontend/src/components/icons/Logout.vue` | 35 | 退出图标 |
| `frontend/public/favicon.png` | — | 浏览器图标 |
| `DOCKER_WEB.md` | ~470 | 本文档 |
| `Dockerfile` | 40 | 三阶段构建 |
| `docker-compose.yml` | 18 | 部署配置 |
| `docker/entrypoint.sh` | 10 | 入口脚本 |
| `.dockerignore` | 10 | 构建排除 |
| `.github/workflows/docker-publish.yml` | 45 | Docker 镜像发布 |
| `.github/workflows/release-windows.yaml` | 120 | Windows 桌面发布（修复 NSIS PATH + 自签名 + installer 检测） |

### 删除的原始文件（1 个）

| 文件 | 说明 |
|---|---|
| `frontend/package.json.md5` | 原版存在，当前版本已删除（无实际用途） |

---

## 本地开发调试

### Web 模式开发

```bash
# 终端 1：启动 Go 后端（Web 模式）
cd /path/to/tiny-rdm
go run -tags web . 

# 终端 2：启动 Vite dev server（带代理）
cd frontend
VITE_WEB=true npm run dev
# 访问 http://localhost:5173（自动代理 /api 和 /ws 到 :8088）
```

### 桌面模式开发

```bash
# 正常 Wails 开发流程，不受 Web 改动影响
wails dev
```

> Web 相关代码通过构建标签和 `VITE_WEB` 环境变量完全隔离，桌面开发无需任何额外配置。

---

## 同步上游更新

当原版 Tiny RDM 发布新版本时：

1. **Service 文件** — 新版中的 `runtime.EventsEmit` / `runtime.EventsOn` / `runtime.OpenFileDialog` 等调用替换为 `services.*`
2. **新增 Service 方法** — 在对应 `backend/api/*_api.go` 添加端点 + `frontend/src/utils/api.js` 添加同签名函数
3. **新增 Service 文件** — 新建 `*_api.go` + `router.go` 注册路由 + `vite.config.js` 添加别名 + `api.js` 添加函数
4. **新增 runtime 调用** — 在 `platform_desktop.go` 和 `platform_web.go` 同时添加对应函数
5. **go.mod** — 合并上游依赖，保留 gin 相关
6. **vite.config.js** — 保留别名和 proxy，合并其他变更
7. **App.vue / AppContent.vue** — 手动合并认证门控和 `isWeb()` 逻辑
8. **版本号** — 更新 Dockerfile 中 `-X main.version=x.x.x`
9. **语言文件** — 确保 `LoginPage.vue` 内置翻译和 `ribbon.logout` 键同步
