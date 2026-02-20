# Tiny RDM Docker Web 版 — 完整开发文档

基于 Tiny RDM v1.2.6 源码，新增 Docker Web 部署模式。
一套代码同时支持 **Wails 桌面客户端** 和 **Docker Web** 两种运行方式，原始组件零修改。

---

## 目录

- [快速启动](#快速启动)
- [环境变量](#环境变量)
- [登录认证](#登录认证)
- [安全特性](#安全特性)
- [双模式架构](#双模式架构)
- [后端改动详解](#后端改动详解)
- [前端改动详解](#前端改动详解)
- [Docker 构建详解](#docker-构建详解)
- [CI/CD 自动化](#cicd-自动化)
- [数据持久化](#数据持久化)
- [移动端适配](#移动端适配)
- [与原版的区别总结](#与原版的区别总结)
- [文件变更清单](#文件变更清单)
- [逐文件 Diff 对比](#逐文件-diff-对比)
- [解决方案汇总](#解决方案汇总)
- [同步上游更新指南](#同步上游更新指南)

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

---

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PORT` | `8088` | HTTP 监听端口 |
| `GIN_MODE` | `release` | Gin 框架运行模式 |
| `ADMIN_USERNAME` | 无 | 登录用户名（与 ADMIN_PASSWORD 同时设置才启用认证） |
| `ADMIN_PASSWORD` | 无 | 登录密码（与 ADMIN_USERNAME 同时设置才启用认证） |
| `SESSION_TTL` | `24h` | 登录会话有效期，支持 Go Duration 格式（如 `12h`、`30m`） |

不设置 `ADMIN_USERNAME` 和 `ADMIN_PASSWORD` 则不启用登录，直接进入主界面（向后兼容）。

---

## 登录认证

### 工作流程

1. 用户打开 Web 页面 → 前端调用 `GET /api/auth/status` 检查认证状态
2. 如果认证已启用且未登录 → 显示登录页面（移动端响应式布局）
3. 用户输入用户名密码 → `POST /api/auth/login` 验证
4. 验证通过 → 后端生成 HMAC-SHA256 Token，写入 httpOnly Cookie
5. 前端调用 `ReconnectWebSocket()` 重新建立带 Cookie 的 WebSocket 连接
6. 前端加载偏好设置并同步 i18n 语言，切换到桌面布局 viewport
7. 后续所有 API 请求和 WebSocket 连接自动携带 Cookie 认证
8. 退出登录 → `POST /api/auth/logout` 清除 Cookie，触发 `rdm:unauthorized` 事件返回登录页

### 登录页面特性

- 自动检测浏览器语言，支持 10 种语言（zh/tw/en/ja/ko/es/fr/ru/pt/tr）
- 契合 Tiny RDM UI 风格（NaiveUI 组件、相同配色）
- 页脚显示动态版本号（通过 `GET /api/version` 获取）+ GitHub 项目链接
- 副标题为 "Redis Web Manager"（区别于桌面版的 "Redis Desktop Manager"）
- 移动端自适应布局（`@media (max-width: 480px)` 响应式样式）

---

## 安全特性

### 认证安全

| 特性 | 说明 |
|---|---|
| httpOnly Cookie | Token 存储在 httpOnly Cookie 中，JavaScript 无法读取，防 XSS 窃取 |
| SameSite=Strict | Cookie 仅在同站请求中发送，防 CSRF 攻击 |
| IP 绑定 | Token 与客户端 IP 绑定，即使 Cookie 被窃取也无法在其他 IP 使用 |
| 登录限速 | 同一 IP 每分钟最多 5 次登录尝试，防暴力破解 |
| 失败延迟 | 登录失败后延迟 500ms 响应，防时序攻击 |
| 常量时间比较 | 使用 `hmac.Equal` 比较凭据，防时序侧信道攻击 |
| 随机签名密钥 | 每次容器启动随机生成 32 字节 HMAC 密钥，重启即失效旧 Token |
| WebSocket 认证 | WebSocket 连接同样需要 Cookie + Origin 认证，未认证返回 401 |
| 限速器内存保护 | `rateLimiter` 设置 `maxEntries=10000` 上限 + 5 分钟周期清理，防分布式攻击导致内存耗尽 |

### HTTP 安全响应头

| 响应头 | 值 | 作用 |
|---|---|---|
| `X-Content-Type-Options` | `nosniff` | 防 MIME 类型嗅探 |
| `X-Frame-Options` | `SAMEORIGIN` | 防点击劫持 |
| `X-XSS-Protection` | `1; mode=block` | 浏览器 XSS 过滤 |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | 控制 Referer 泄露 |
| `Content-Security-Policy` | `default-src 'self'; ...` | 限制资源加载来源 |

### CORS 与 CSRF 防护

- CORS：验证 `Origin` 头与请求 `Host` 是否同源（仅比较主机名，忽略端口，支持反向代理场景）
- CSRF：对所有非 GET/HEAD/OPTIONS 请求验证 `Origin` 或 `Referer` 头
- 支持 `X-Forwarded-Host` 反向代理头
- WebSocket 连接同样验证 Origin

### 请求体大小限制

- 全局限制请求体最大 10MB（`maxRequestBodySize = 10 << 20`），防止内存耗尽攻击

### 文件操作安全

- 文件上传：`sanitizeFilename()` 去除路径分隔符和 `..`，防止路径遍历
- 文件下载：`safeTempPath()` 验证路径必须在 `os.TempDir()` 内，防止任意文件读取

### API 路由保护

```
公开路由（无需认证）：
  POST /api/auth/login
  POST /api/auth/logout
  GET  /api/auth/status
  GET  /api/version

受保护路由（需要认证）：
  /api/connection/*
  /api/browser/*
  /api/cli/*
  /api/monitor/*
  /api/pubsub/*
  /api/preferences/*
  /api/system/*
  /ws (WebSocket)
```

---

## 双模式架构

### 对比总览

| | 桌面模式 (Wails) | Web 模式 (Docker) |
|---|---|---|
| 构建命令 | `wails build` | `go build -tags web` |
| 入口文件 | `main.go` (`//go:build !web`) | `main_web.go` (`//go:build web`) |
| 前后端通信 | Wails RPC 绑定（进程内调用） | HTTP REST API (Gin) |
| 事件系统 | Wails Runtime Events | WebSocket 双向通信 |
| 窗口管理 | 原生窗口（最小化/最大化/关闭） | 浏览器标签页（隐藏窗口控制按钮） |
| 文件对话框 | 原生系统对话框 | HTML `<input type="file">` + 后端暂存到 `os.TempDir()` |
| 剪贴板 | Wails Runtime | `navigator.clipboard` API + `execCommand('copy')` 降级 |
| 认证 | 无（本地应用） | Cookie + HMAC Token（可选） |
| 退出 | 关闭窗口 | 侧边栏退出登录按钮 |
| 浏览器标题 | "Tiny RDM" | "Tiny RDM"（保持一致） |
| Viewport | 固定窗口尺寸 | 动态 viewport（登录页响应式，主界面桌面布局） |
| 连接导入导出 | 原生文件对话框 | HTTP 文件上传/下载（`/api/connection/export-download`、`/api/connection/import-upload`） |

### 核心机制：Go 构建标签

通过 Go 的 `//go:build` 构建标签实现编译时切换，同一个 `services` 包在不同模式下链接不同的平台实现：

```
桌面模式编译 (!web):
  main.go ──→ services/*.go + platform_desktop.go ──→ Wails Runtime

Web 模式编译 (web):
  main_web.go ──→ services/*.go + platform_web.go ──→ 回调函数 ──→ api 包
```

`platform_desktop.go` 和 `platform_web.go` 提供完全相同的函数签名：
- `EventsEmit()` / `EventsOn()` / `EventsOnce()` / `EventsOff()`
- `OpenFileDialog()` / `SaveFileDialog()`
- `ScreenGetAll()` / `WindowMaximise()` / `WindowIsFullscreen()` 等

以及相同的类型定义：
- `OpenDialogOptions` / `SaveDialogOptions` / `FileFilter` / `Screen` / `ScreenSize`

Service 层代码调用这些函数时无需关心底层是 Wails 还是 WebSocket。

### 核心机制：Vite 条件别名重定向

前端通过 `vite.config.js` 中的环境变量 `VITE_WEB` 条件启用 8 个别名，将所有 `wailsjs/*` 导入重定向到 Web 适配器：

```javascript
// 原始 Store 代码（不修改）：
import { OpenConnection } from 'wailsjs/go/services/browserService.js'

// 桌面模式（VITE_WEB 未设置）：解析到 wailsjs/go/services/browserService.js（Wails 生成的 RPC 绑定）
// Web 模式（VITE_WEB=true）：通过 Vite 别名解析到 src/utils/api.js（HTTP 请求）
```

别名映射：
| 原始导入路径 | Web 模式实际路径 |
|---|---|
| `wailsjs/runtime/runtime.js` | `src/utils/wails_runtime.js` |
| `wailsjs/go/services/connectionService.js` | `src/utils/api.js` |
| `wailsjs/go/services/browserService.js` | `src/utils/api.js` |
| `wailsjs/go/services/cliService.js` | `src/utils/api.js` |
| `wailsjs/go/services/monitorService.js` | `src/utils/api.js` |
| `wailsjs/go/services/pubsubService.js` | `src/utils/api.js` |
| `wailsjs/go/services/preferencesService.js` | `src/utils/api.js` |
| `wailsjs/go/services/systemService.js` | `src/utils/api.js` |

> 注意：`wailsjs` 通配别名放在最后，确保具体路径别名优先匹配。别名仅在 `VITE_WEB=true` 时启用，桌面模式构建不受影响。

### 架构图

```
┌─────────────────────────────────────────────────────────────┐
│                    桌面模式 (!web)                            │
│                                                             │
│  Vue 前端 ──→ wailsjs/ RPC 绑定 ──→ Wails Runtime ──→ Go   │
│                                                    Services │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                    Web 模式 (web)                            │
│                                                             │
│  Vue 前端                                                    │
│    ├── api.js ──── HTTP POST/GET ────→ Gin Router ──→ Go   │
│    │                                              Services  │
│    └── wails_runtime.js ── WebSocket ──→ WSHub ←── Go      │
│                                              Services       │
│                                                             │
│  platform_web.go 回调变量:                                    │
│    EmitEventFunc ──→ api.Hub().Emit (服务端推送事件)           │
│    RegisterHandlerFunc ──→ api.RegisterHandler (客户端事件)   │
│                                                             │
│  main_web.go 启动时连接:                                      │
│    services.EmitEventFunc = api.Hub().Emit                  │
│    services.RegisterHandlerFunc = api.RegisterHandler       │
└─────────────────────────────────────────────────────────────┘
```

### 避免循环导入

`services` 包不能直接导入 `api` 包（因为 `api` 已经导入了 `services`）。
解决方案：`platform_web.go` 中定义回调变量（`EmitEventFunc`、`RegisterHandlerFunc`），由 `main_web.go` 在启动时赋值为 `api` 包的函数。

---

## 后端改动详解

### 原始文件修改

#### 1. `main.go` — 仅添加构建标签
```go
//go:build !web   // ← 新增这一行，其余不变
package main
```

#### 2. `go.mod` — 新增依赖
```
github.com/gin-gonic/gin v1.10.1   // HTTP 框架（新增 direct 依赖）
```
> `gorilla/websocket v1.5.3` 已是原版的 indirect 依赖，无需额外添加。
> Gin 引入了一系列新的 indirect 依赖：`bytedance/sonic`、`cloudwego/base64x`、`cloudwego/iasm`、`gabriel-vasile/mimetype`、`gin-contrib/sse`、`go-playground/*`、`goccy/go-json`、`json-iterator/go`、`klauspost/cpuid`、`leodido/go-urn`、`modern-go/*`、`pelletier/go-toml`、`twitchyliquid64/golang-asm`、`ugorji/go/codec`、`google.golang.org/protobuf` 等。

#### 3. 七个 Service 文件 — 平台抽象替换

以下文件中，将 `runtime.EventsEmit(s.ctx, ...)` 等 Wails 直接调用替换为 `services.EventsEmit(s.ctx, ...)`（调用平台抽象层）：

- `backend/services/browser_service.go`
- `backend/services/cli_service.go`
- `backend/services/connection_service.go`
- `backend/services/monitor_service.go`
- `backend/services/pubsub_service.go`
- `backend/services/preferences_service.go`
- `backend/services/system_service.go`

改动模式统一：
```go
// 改动前（直接调用 Wails）：
import "github.com/wailsapp/wails/v2/pkg/runtime"
runtime.EventsEmit(s.ctx, "event_name", data)

// 改动后（调用平台抽象层）：
// 去掉 wails/runtime 导入
services.EventsEmit(s.ctx, "event_name", data)
```

同理，`runtime.OpenFileDialog`、`runtime.SaveFileDialog`、`runtime.EventsOn` 等调用也做相同替换。

#### 4. `backend/services/connection_service_web.go` — Web 专用方法（`//go:build web`）

原版 `connection_service.go` 中的 `ExportConnections` / `ImportConnections` 使用原生文件对话框，Web 模式下不可用。新增独立文件 `connection_service_web.go`（带 `//go:build web` 构建标签），包含两个 Web 专用方法：

```go
// ExportConnectionsToBytes 将连接配置导出为 zip 字节数组（Web 模式下载用）
func (c *connectionService) ExportConnectionsToBytes() ([]byte, string, error)

// ImportConnectionsFromBytes 从 zip 字节数组导入连接配置（Web 模式上传用）
func (c *connectionService) ImportConnectionsFromBytes(data []byte) (resp types.JSResp)
```

这两个方法通过 `//go:build web` 标签与桌面模式完全隔离，桌面编译时不会包含这些代码。`bytes` 包的导入也随之移到了 Web 专用文件中，保持 `connection_service.go` 与原版的最小差异。

### 新增文件

#### 平台抽象层（构建标签切换）

| 文件 | 构建标签 | 说明 |
|---|---|---|
| `backend/services/platform_desktop.go` | `!web` | 封装 Wails Runtime 函数 + 类型别名指向 Wails 类型 |
| `backend/services/platform_web.go` | `web` | 回调变量桥接到 WebSocket + Stub 类型替代 Wails 类型 |

#### HTTP API 层

| 文件 | 构建标签 | 说明 |
|---|---|---|
| `backend/api/router.go` | 无 | Gin 路由器：请求体限制、安全头、CORS、CSRF、公开路由、认证中间件、静态文件服务 |
| `backend/api/auth.go` | `web` | 登录认证：Token 生成/验证、限速器（含内存保护）、安全中间件 |
| `backend/api/websocket_hub.go` | 无 | WebSocket 连接池管理（最大 50 连接）、事件广播、消息分发、读取限制 1MB |
| `backend/api/event_bridge.go` | 无 | 事件桥接辅助函数 |
| `backend/api/connection_api.go` | 无 | 连接管理 REST API + Web 专用导出下载/导入上传端点 |
| `backend/api/browser_api.go` | 无 | 数据浏览 REST API（约 50 个端点，覆盖所有数据类型操作） |
| `backend/api/cli_api.go` | 无 | CLI 会话 REST API（CLI 输入通过 WebSocket 事件处理） |
| `backend/api/monitor_api.go` | 无 | Monitor REST API |
| `backend/api/pubsub_api.go` | 无 | Pubsub REST API |
| `backend/api/preferences_api.go` | 无 | 偏好设置 REST API |
| `backend/api/system_api.go` | 无 | 系统信息 + 文件上传下载 REST API（含路径遍历防护） |

#### Web 入口

| 文件 | 说明 |
|---|---|
| `main_web.go` | `//go:build web`，Gin HTTP 服务器、回调连接、服务初始化、GA 禁用（空密钥）、优雅关闭（SIGINT/SIGTERM） |

---

## 前端改动详解

### 原始文件修改

#### 1. `frontend/vite.config.js` — 条件别名 + 代理

原版仅有 3 个别名（`@`、`stores`、`wailsjs`），修改后通过 `VITE_WEB` 环境变量条件启用 8 个 Web 别名 + dev server proxy：

```javascript
const isWeb = process.env.VITE_WEB === 'true'

// 条件别名（仅 VITE_WEB=true 时启用）
...(isWeb ? {
    'wailsjs/runtime/runtime.js': rootPath + 'src/utils/wails_runtime.js',
    'wailsjs/go/services/connectionService.js': rootPath + 'src/utils/api.js',
    // ... 共 8 个
} : {}),

// 条件 dev server proxy（仅 VITE_WEB=true 时启用）
...(isWeb ? { server: { proxy: { '/api': ..., '/ws': ... } } } : {}),
```

> 关键：桌面模式（`wails build`）不设置 `VITE_WEB`，别名不生效，`wailsjs/*` 导入解析到 Wails 真实 RPC 绑定。Docker 构建时 Dockerfile 设置 `ENV VITE_WEB=true`，别名生效，重定向到 HTTP/WebSocket 适配器。

#### 2. `frontend/src/App.vue` — 登录认证门控 + Viewport 管理

这是改动最大的文件。原版直接在 `onMounted` 中初始化应用，修改后增加了完整的认证流程和移动端适配：

新增内容：
- `import LoginPage` 和 `import { ReconnectWebSocket }` 导入
- `authChecking` / `authenticated` / `authEnabled` 响应式状态
- `checkAuth()` 函数：调用 `/api/auth/status` 检查认证状态
- `onLogin()` 回调：认证成功后重连 WebSocket、切换 viewport、初始化应用
- `onUnauthorized()` 事件处理：401 时自动跳回登录页
- `setViewport(mode)` 函数：登录页使用移动端响应式 viewport，主界面使用桌面布局 viewport（根据屏幕方向和尺寸动态计算 `width` 值）
- `onOrientationChange()` 监听：屏幕旋转时重新计算 viewport
- `initApp()` 开头新增 `await prefStore.loadPreferences()` + `i18n.locale.value = prefStore.currentLanguage`，修复登录后语言不正确的 bug
- 模板中新增三段 `<template>` 条件渲染：`authChecking` → 空白、`authEnabled && !authenticated` → `LoginPage`、`else` → 原始内容

#### 3. `frontend/src/AppContent.vue` — 隐藏窗口控制按钮 + Web 样式适配

改动点：
- 新增 `import { isWeb } from '@/utils/platform.js'`
- `wrapperStyle` 和 `spinStyle` 计算属性中增加 `isWeb()` 判断（Web 模式与 Windows 相同处理，无圆角边框）
- 窗口控制按钮条件从 `v-if="!isMacOS()"` 改为 `v-if="!isMacOS() && !isWeb()"`
- CSS 新增 `height: 100dvh`（动态视口高度，解决移动端浏览器地址栏遮挡问题）

#### 4. `frontend/src/components/sidebar/Ribbon.vue` — 退出登录按钮

新增内容：
- `import Logout from '@/components/icons/Logout.vue'` 和 `import { isWeb } from '@/utils/platform.js'`
- `handleLogout()` 异步函数：调用 `/api/auth/logout`，然后触发 `rdm:unauthorized` 自定义事件
- 模板中在 GitHub 图标下方新增退出登录按钮（`v-if="isWeb()"`）

#### 5. `frontend/src/utils/platform.js` — 新增 Web 平台检测

导入路径保持原版不变（`wailsjs/runtime/runtime.js`），Web 模式下通过 Vite 条件别名自动重定向到 `wails_runtime.js`。新增：

```javascript
export function isWeb() {
    return os === 'web'
}
```

#### 6. `frontend/index.html` — Favicon

原版无 favicon，新增：
```html
<link rel="icon" type="image/png" href="/favicon.png" />
```
> 标题保持 "Tiny RDM" 不变（原版文档中写的 "Tiny RDM WEB" 已修正回原始标题）。

### 新增文件

| 文件 | 说明 |
|---|---|
| `frontend/src/utils/api.js` | HTTP API 适配器，约 80 个导出函数，签名与 Wails 绑定完全一致。内含 `post()`/`get()` 基础函数，401 时自动触发 `rdm:unauthorized` 事件。`ExportConnections` 和 `ImportConnections` 使用文件下载/上传替代原生对话框。`SelectFile` 使用 `<input type="file">` + 上传到 `/api/system/select-file`。`SaveFile` 使用 `/api/system/download` 触发浏览器下载。 |
| `frontend/src/utils/wails_runtime.js` | Wails Runtime 的 Web 替代：事件→WebSocket（`onWsEvent`/`offWsEvent`/`sendWsMessage`）、剪贴板→`navigator.clipboard` API（含 `execCommand('copy')` 降级）、窗口管理→no-op、`Environment()` 返回 `platform: 'web'`。新增 `ReconnectWebSocket` 和 `WaitForWebSocket` 导出。 |
| `frontend/src/utils/websocket.js` | WebSocket 客户端：自动重连（3 秒间隔）、事件监听/分发（`Map<event, Set<callback>>`）、`waitForWebSocket()` Promise、`reconnectWebSocket()` 强制重连（登录后使用）。协议自动检测（`ws:`/`wss:`）。 |
| `frontend/src/components/LoginPage.vue` | 登录页面：NaiveUI 组件、10 语言自动检测（内置翻译字典）、版本号动态获取、GitHub 页脚、移动端响应式（`@media max-width: 480px`）、Enter 键提交、密码显示切换。 |
| `frontend/src/components/icons/Logout.vue` | 退出登录 SVG 图标（48x48 viewBox），风格与现有图标一致（`stroke="currentColor"`、可配置 `strokeWidth`）。 |
| `frontend/public/favicon.png` | 浏览器标签页图标（复制自 `src/assets/images/icon.png`） |

---

## Docker 构建详解

### 三阶段构建

```dockerfile
# 阶段 1：构建前端（Node 20 Alpine）
FROM node:20-alpine AS frontend-builder
# npm ci → npm run build → 产出 frontend/dist/

# 阶段 2：构建后端（Go 1.24 Alpine）
FROM golang:1.24-alpine AS backend-builder
# 复制 go.mod + 源码 + 前端 dist
# go mod tidy → go build -tags web → 产出 tinyrdm 单二进制

# 阶段 3：运行时（Alpine 3.21，约 30MB）
FROM alpine:3.21
# 仅包含 tinyrdm 二进制 + entrypoint.sh
```

### 关键构建参数

- `-tags web`：激活 Web 模式构建标签
- `-ldflags "-s -w -X main.version=1.2.6"`：去除调试信息、注入版本号
- `CGO_ENABLED=0`：纯静态编译，无 C 依赖
- `GOPROXY=https://goproxy.cn,https://goproxy.io,direct`：Go 模块代理（中国网络优化）
- `GOFLAGS=-mod=mod`：允许 go mod tidy 自动修改
- `NODE_OPTIONS=--max-old-space-size=4096`：前端构建内存限制
- `VITE_WEB=true`：激活 Vite 条件别名，将 `wailsjs/*` 重定向到 Web 适配器
- `XDG_CONFIG_HOME=/app`：配置文件存储在 `/app/TinyRDM/`

### 相关文件

| 文件 | 说明 |
|---|---|
| `Dockerfile` | 三阶段构建定义 |
| `docker-compose.yml` | 一键部署：端口映射、环境变量、数据卷、`restart: unless-stopped` |
| `docker/entrypoint.sh` | 入口脚本，透传环境变量（PORT、REDIS_HOST 等） |
| `.dockerignore` | 排除 node_modules、.git、.github、docs、build、*.md |

---

## CI/CD 自动化

新增 `.github/workflows/docker-publish.yml`，实现 GitHub Container Registry 自动构建推送：

- 触发条件：`main` 分支 push、`v*` 标签、手动触发
- 镜像仓库：`ghcr.io/rowanchen-com/tiny-rdm-web`
- 标签策略：分支名、语义化版本（`{{version}}`、`{{major}}.{{minor}}`）、commit SHA、`latest`
- 构建平台：`linux/amd64`

---

## 数据持久化

容器内配置目录：`/app/TinyRDM/`（通过 `XDG_CONFIG_HOME=/app` 环境变量控制）

映射方式：`./data:/app/TinyRDM`

| 文件 | 说明 |
|---|---|
| `connections.yaml` | Redis 连接配置（地址、端口、密码等），明文存储（与原版一致） |
| `preferences.yaml` | 偏好设置（语言、主题、字体、扫描大小等） |
| `device.txt` | 设备 ID（GA 统计用，Web 版未启用 GA，此文件无实际作用） |

> 注意：`connections.yaml` 中的 Redis 密码为明文存储，这与原版桌面客户端行为一致。请确保 `./data` 目录的文件权限控制。

---

## 移动端适配

Web 版新增了移动端浏览器的基础适配：

### Viewport 动态管理

`App.vue` 中的 `setViewport(mode)` 函数根据场景切换 viewport：

| 场景 | viewport 策略 |
|---|---|
| 登录页 | `width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no`（标准响应式） |
| 主界面 - 竖屏手机 | `width=680, user-scalable=yes`（缩小桌面布局适配窄屏） |
| 主界面 - 横屏手机 | `width=1280, user-scalable=yes`（更大宽度确保内容完整显示） |
| 主界面 - 桌面/平板 | `width=1024, user-scalable=yes`（标准桌面布局） |

### CSS 适配

- `AppContent.vue` 新增 `height: 100dvh`（动态视口高度），解决移动端浏览器地址栏遮挡底部内容的问题
- `LoginPage.vue` 包含 `@media (max-width: 480px)` 响应式样式

### 屏幕旋转

监听 `orientationchange` 和 `resize` 事件，200ms 防抖后重新计算 viewport。

---

## 与原版的区别总结

### 功能差异

| 功能 | 桌面版 | Web 版 |
|---|---|---|
| 窗口控制按钮（最小化/最大化/关闭） | ✅ 显示 | ❌ 隐藏（浏览器自带） |
| 原生文件对话框 | ✅ 系统对话框 | ⚠️ HTML 文件选择器 + 后端暂存到 TempDir |
| 连接导入导出 | ✅ 原生文件对话框 | ✅ HTTP 文件上传/下载 |
| 登录认证 | ❌ 无 | ✅ 可选（环境变量控制） |
| 退出登录按钮 | ❌ 无 | ✅ 侧边栏底部 |
| Google Analytics | ✅ 启用 | ❌ 禁用（密钥为空） |
| Favicon | ❌ 无 | ✅ icon.png |
| 移动端适配 | ❌ 不适用 | ✅ 动态 viewport + 响应式登录页 |
| CORS/CSRF 防护 | ❌ 不需要 | ✅ Origin 验证 + Referer 检查 |
| 请求体大小限制 | ❌ 不需要 | ✅ 10MB 限制 |
| WebSocket 连接数限制 | ❌ 不需要 | ✅ 最大 50 连接 |
| CI/CD | ❌ 无 | ✅ GitHub Actions 自动构建推送 |

### 不影响的功能

以下功能在 Web 版中完全保留，行为与桌面版一致：
- Redis 连接管理（增删改查、分组、排序）
- 数据浏览（所有数据类型：String/Hash/List/Set/ZSet/Stream）
- CLI 终端（通过 WebSocket 实时交互）
- Monitor 监控
- Pubsub 订阅/发布
- 偏好设置（语言、主题、字体等）
- 慢日志、客户端列表
- 键导入/导出（CSV）
- 多语言支持（10 种语言）
- 命令历史记录

---

## 文件变更清单

### 原始文件修改（最小化改动）

| 文件 | 改动类型 | 改动量 |
|---|---|---|
| `main.go` | 添加 `//go:build !web` 构建标签 | 1 行 |
| `go.mod` / `go.sum` | 新增 `gin-gonic/gin` direct 依赖 + 相关 indirect 依赖 | ~30 行 |
| `backend/services/browser_service.go` | `runtime.EventsEmit` → `services.EventsEmit` 等 | ~15 处替换 |
| `backend/services/cli_service.go` | 同上 | ~5 处替换 |
| `backend/services/connection_service.go` | 同上（平台抽象替换） | ~10 处替换 |
| `backend/services/monitor_service.go` | 同上 | ~5 处替换 |
| `backend/services/pubsub_service.go` | 同上 | ~5 处替换 |
| `backend/services/preferences_service.go` | 同上 | ~3 处替换 |
| `backend/services/system_service.go` | 同上 | ~3 处替换 |
| `frontend/vite.config.js` | 条件别名（`VITE_WEB=true` 时启用）+ dev proxy | ~30 行 |
| `frontend/src/App.vue` | 新增登录认证门控 + viewport 管理 + WebSocket 重连 | 重写大部分逻辑 |
| `frontend/src/AppContent.vue` | Web 模式隐藏窗口控制按钮 + `isWeb()` 样式判断 + `100dvh` | ~10 行 |
| `frontend/src/components/sidebar/Ribbon.vue` | Web 模式新增退出登录按钮 | ~15 行 |
| `frontend/src/utils/platform.js` | 新增 `isWeb()` 函数（导入路径保持原版不变） | ~3 行 |
| `frontend/index.html` | 新增 favicon link | 1 行 |

### 新增文件

| 文件 | 行数（约） | 说明 |
|---|---|---|
| `main_web.go` | 85 | Web 入口，Gin 服务器 + 优雅关闭 |
| `backend/services/platform_desktop.go` | 75 | 桌面平台抽象层（Wails 封装） |
| `backend/services/platform_web.go` | 110 | Web 平台抽象层（回调 + Stub 类型） |
| `backend/services/connection_service_web.go` | 95 | Web 专用连接导入导出方法（`//go:build web`） |
| `backend/api/router.go` | 165 | Gin 路由器（安全头、CORS、CSRF、静态文件） |
| `backend/api/auth.go` | 260 | 登录认证系统（Token、限速、中间件） |
| `backend/api/websocket_hub.go` | 120 | WebSocket 连接管理 |
| `backend/api/event_bridge.go` | 10 | 事件桥接辅助 |
| `backend/api/connection_api.go` | 170 | 连接管理 API |
| `backend/api/browser_api.go` | ~600 | 数据浏览 API（最大文件） |
| `backend/api/cli_api.go` | 40 | CLI API |
| `backend/api/monitor_api.go` | 45 | Monitor API |
| `backend/api/pubsub_api.go` | 50 | Pubsub API |
| `backend/api/preferences_api.go` | 55 | 偏好设置 API |
| `backend/api/system_api.go` | 120 | 系统信息 + 文件上传下载 API |
| `frontend/src/utils/api.js` | ~440 | HTTP API 适配器（~80 个导出函数） |
| `frontend/src/utils/wails_runtime.js` | 80 | Wails Runtime Web 替代 |
| `frontend/src/utils/websocket.js` | 110 | WebSocket 客户端 |
| `frontend/src/components/LoginPage.vue` | 200 | 登录页面 |
| `frontend/src/components/icons/Logout.vue` | 35 | 退出图标 |
| `frontend/public/favicon.png` | — | 浏览器图标 |
| `Dockerfile` | 40 | Docker 三阶段构建 |
| `docker-compose.yml` | 18 | Docker Compose 配置 |
| `docker/entrypoint.sh` | 10 | 容器入口脚本 |
| `.dockerignore` | 10 | Docker 构建排除规则 |
| `.github/workflows/docker-publish.yml` | 45 | CI/CD 自动构建推送 |
| `DOCKER_WEB.md` | — | 本文档 |

---

## 逐文件 Diff 对比

以下是与原版 Tiny RDM v1.2.6 的逐文件详细对比。

### `main.go`

```diff
+//go:build !web
+
 package main
 // ... 其余完全不变
```

### `go.mod`

```diff
 require (
     // ... 原有依赖不变
+    github.com/gin-gonic/gin v1.10.1
 )

 require (
     // ... 原有 indirect 不变
+    github.com/bytedance/sonic v1.13.2 // indirect
+    github.com/bytedance/sonic/loader v0.2.4 // indirect
+    github.com/cloudwego/base64x v0.1.5 // indirect
+    github.com/cloudwego/iasm v0.2.0 // indirect
+    github.com/gabriel-vasile/mimetype v1.4.8 // indirect
+    github.com/gin-contrib/sse v1.0.0 // indirect
+    github.com/go-playground/locales v0.14.1 // indirect
+    github.com/go-playground/universal-translator v0.18.1 // indirect
+    github.com/go-playground/validator/v10 v10.26.0 // indirect
+    github.com/goccy/go-json v0.10.5 // indirect
+    github.com/json-iterator/go v1.1.12 // indirect
+    github.com/klauspost/cpuid/v2 v2.2.9 // indirect
+    github.com/leodido/go-urn v1.4.0 // indirect
+    github.com/modern-go/concurrent v0.0.0-20180228061459-e0a39a4cb421 // indirect
+    github.com/modern-go/reflect2 v1.0.2 // indirect
+    github.com/pelletier/go-toml/v2 v2.2.4 // indirect
+    github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
+    github.com/ugorji/go/codec v1.2.12 // indirect
+    golang.org/x/arch v0.15.0 // indirect
+    google.golang.org/protobuf v1.36.6 // indirect
 )
```

### `backend/services/browser_service.go`（示例 diff）

```diff
 import (
     // ...
-    "github.com/wailsapp/wails/v2/pkg/runtime"
 )

-    runtime.EventsEmit(b.ctx, "loading:xxx", data)
+    services.EventsEmit(b.ctx, "loading:xxx", data)

-    runtime.EventsOn(b.ctx, "event_name", func(data ...any) { ... })
+    services.EventsOn(b.ctx, "event_name", func(data ...any) { ... })
```

> 其他 6 个 service 文件的改动模式完全相同。

### `backend/services/connection_service.go`（平台抽象替换）

```diff
 import (
     // ...
-    "bytes"
-    "github.com/wailsapp/wails/v2/pkg/runtime"
+    // bytes 已移至 connection_service_web.go
 )

 // 原有方法中的 runtime.* 调用替换为 services.* （同上）
-    runtime.EventsEmit(...)
+    services.EventsEmit(...)

-// ExportConnectionsToBytes 和 ImportConnectionsFromBytes 已移至 connection_service_web.go
```

### `backend/services/connection_service_web.go`（新增文件）

```go
//go:build web

package services

import (
    "bytes"
    // ...
)

// Web 专用：连接导出为 zip 字节数组（HTTP 下载用）
func (c *connectionService) ExportConnectionsToBytes() ([]byte, string, error) { ... }

// Web 专用：从 zip 字节数组导入连接（HTTP 上传用）
func (c *connectionService) ImportConnectionsFromBytes(data []byte) (resp types.JSResp) { ... }
```

> 通过 `//go:build web` 构建标签，这两个方法仅在 Web 模式编译时存在，桌面模式完全不包含。

### `frontend/vite.config.js`

```diff
+const isWeb = process.env.VITE_WEB === 'true'
+
     resolve: {
         alias: {
             '@': rootPath + 'src',
             stores: rootPath + 'src/stores',
+            // Web mode only (VITE_WEB=true): redirect wailsjs imports
+            ...(isWeb ? {
+                'wailsjs/runtime/runtime.js': rootPath + 'src/utils/wails_runtime.js',
+                'wailsjs/go/services/connectionService.js': rootPath + 'src/utils/api.js',
+                'wailsjs/go/services/browserService.js': rootPath + 'src/utils/api.js',
+                'wailsjs/go/services/cliService.js': rootPath + 'src/utils/api.js',
+                'wailsjs/go/services/monitorService.js': rootPath + 'src/utils/api.js',
+                'wailsjs/go/services/pubsubService.js': rootPath + 'src/utils/api.js',
+                'wailsjs/go/services/preferencesService.js': rootPath + 'src/utils/api.js',
+                'wailsjs/go/services/systemService.js': rootPath + 'src/utils/api.js',
+            } : {}),
             wailsjs: rootPath + 'wailsjs',
         },
     },
     // ...
+    ...(isWeb ? {
+        server: {
+            proxy: {
+                '/api': { target: 'http://localhost:8088', changeOrigin: true },
+                '/ws': { target: 'ws://localhost:8088', ws: true },
+            },
+        },
+    } : {}),
 })
```

### `frontend/src/App.vue`

```diff
 <script setup>
 // ... 原有 import 不变
+import { ReconnectWebSocket } from 'wailsjs/runtime/runtime.js'
+import LoginPage from '@/components/LoginPage.vue'

+// Viewport 管理函数
+const setViewport = (mode) => { ... }
+
+// Auth 状态
+const authChecking = ref(true)
+const authenticated = ref(false)
+const authEnabled = ref(false)
+
+const checkAuth = async () => { ... }
+const onLogin = async () => { ... }
+const onUnauthorized = () => { ... }

-onMounted(async () => {
-    try {
-        initializing.value = true
-        await prefStore.loadFontList()
-        // ... 原始初始化逻辑
-    } finally {
-        initializing.value = false
-    }
-})
+const initApp = async () => {
+    try {
+        initializing.value = true
+        await prefStore.loadPreferences()          // 新增：先加载偏好
+        i18n.locale.value = prefStore.currentLanguage // 新增：同步语言
+        await prefStore.loadFontList()
+        // ... 原始初始化逻辑
+    } finally {
+        initializing.value = false
+    }
+}
+
+onMounted(async () => {
+    window.addEventListener('rdm:unauthorized', onUnauthorized)
+    window.addEventListener('orientationchange', onOrientationChange)
+    window.addEventListener('resize', onOrientationChange)
+    await checkAuth()
+    if (authenticated.value || !authEnabled.value) {
+        setViewport('desktop')
+        await initApp()
+    } else {
+        setViewport('mobile')
+    }
+})
 </script>

 <template>
     <n-config-provider ...>
-        <n-dialog-provider>
-            <app-content :loading="initializing" />
-            <!-- dialogs -->
-        </n-dialog-provider>
+        <template v-if="authChecking">
+            <div style="width: 100vw; height: 100vh"></div>
+        </template>
+        <template v-else-if="authEnabled && !authenticated">
+            <login-page @login="onLogin" />
+        </template>
+        <template v-else>
+            <n-dialog-provider>
+                <app-content :loading="initializing" />
+                <!-- dialogs -->
+            </n-dialog-provider>
+        </template>
     </n-config-provider>
 </template>
```

### `frontend/src/AppContent.vue`

```diff
-import { isMacOS, isWindows } from '@/utils/platform.js'
+import { isMacOS, isWindows, isWeb } from '@/utils/platform.js'

 const wrapperStyle = computed(() => {
-    if (isWindows()) {
+    if (isWindows() || isWeb()) {
         return {}
     }
     // ...
 })

 const spinStyle = computed(() => {
-    if (isWindows()) {
+    if (isWindows() || isWeb()) {
         return { backgroundColor: themeVars.value.bodyColor }
     }
     // ...
 })

 <!-- 窗口控制按钮 -->
-<toolbar-control-widget v-if="!isMacOS()" ... />
+<toolbar-control-widget v-if="!isMacOS() && !isWeb()" ... />

 <style>
 #app-content-wrapper {
     height: 100vh;
+    height: 100dvh;
     // ...
 }
 </style>
```

### `frontend/src/components/sidebar/Ribbon.vue`

```diff
 <script setup>
 // ... 原有 import
+import Logout from '@/components/icons/Logout.vue'
+import { isWeb } from '@/utils/platform.js'

+const handleLogout = async () => {
+    try {
+        await fetch('/api/auth/logout', { method: 'POST', credentials: 'same-origin' })
+    } catch {}
+    window.dispatchEvent(new Event('rdm:unauthorized'))
+}
 </script>

 <template>
     <!-- ... 原有内容 -->
     <div class="nav-menu-item flex-box-v">
         <!-- ... 原有按钮 -->
+        <icon-button
+            v-if="isWeb()"
+            :icon="Logout"
+            :size="iconSize"
+            :stroke-width="3"
+            :tooltip-delay="100"
+            t-tooltip="ribbon.logout"
+            @click="handleLogout" />
     </div>
 </template>
```

### `frontend/src/utils/platform.js`

```diff
-import { Environment } from 'wailsjs/runtime/runtime.js'
+import { Environment } from 'wailsjs/runtime/runtime.js'  // 保持原版导入路径，Web 模式通过 Vite 别名重定向

 // ... 原有函数不变

+export function isWeb() {
+    return os === 'web'
+}
```

### `frontend/index.html`

```diff
     <title>Tiny RDM</title>
+    <link rel="icon" type="image/png" href="/favicon.png" />
```

---

## 解决方案汇总

本项目在将 Wails 桌面应用改造为 Docker Web 应用的过程中，遇到并解决了以下核心问题：

### 1. Wails RPC 绑定 → HTTP REST API

**问题**：原版前端通过 Wails 生成的 JS 绑定直接调用 Go 方法（进程内 RPC），Web 模式下不可用。

**解决方案**：
- 后端：为每个 Service 创建对应的 `*_api.go` 文件，将所有方法暴露为 HTTP 端点
- 前端：创建 `api.js` 适配器，导出与 Wails 绑定完全相同签名的函数，内部改为 HTTP 请求
- 通过 Vite 别名将 `wailsjs/go/services/*.js` 重定向到 `api.js`，实现零修改 Store 层

### 2. Wails 事件系统 → WebSocket

**问题**：原版使用 `runtime.EventsEmit/EventsOn` 进行前后端事件通信，Web 模式下不可用。

**解决方案**：
- 后端：创建 `WSHub` WebSocket 连接池，`Emit()` 方法广播事件到所有客户端
- 前端：创建 `websocket.js` 客户端 + `wails_runtime.js` 适配器，将 `EventsOn/EventsEmit` 映射到 WebSocket
- 通过 Vite 别名将 `wailsjs/runtime/runtime.js` 重定向到 `wails_runtime.js`

### 3. 循环导入问题

**问题**：`api` 包导入 `services` 包调用业务方法，`services` 包需要调用 `api` 包的 WebSocket 广播 → 循环依赖。

**解决方案**：
- `platform_web.go` 中定义回调变量 `EmitEventFunc` 和 `RegisterHandlerFunc`
- `main_web.go` 启动时赋值：`services.EmitEventFunc = api.Hub().Emit`
- Service 层通过回调变量间接调用 API 层，打破循环

### 4. 桌面/Web 双模式共存

**问题**：需要一套代码同时支持 Wails 桌面和 Docker Web 两种模式。

**解决方案**：
- Go 构建标签：`//go:build !web`（桌面）和 `//go:build web`（Web）
- `platform_desktop.go` 封装 Wails Runtime，`platform_web.go` 提供相同签名的 Web 实现
- 所有 Service 文件调用 `services.EventsEmit()` 等平台抽象函数，编译时自动链接正确实现

### 5. 原生文件对话框 → Web 文件操作

**问题**：原版使用 `runtime.OpenFileDialog/SaveFileDialog` 打开系统文件对话框，Web 模式下不可用。

**解决方案**：
- `api.js` 中的 `SelectFile()` 使用 `<input type="file">` 让用户选择文件，上传到 `/api/system/select-file`，后端保存到 `os.TempDir()` 并返回路径
- `api.js` 中的 `SaveFile()` 通过 `/api/system/download` 触发浏览器下载
- 连接导入导出：新增 `connection_service_web.go`（`//go:build web`），包含 `ExportConnectionsToBytes()`/`ImportConnectionsFromBytes()` 方法 + `/api/connection/export-download`、`/api/connection/import-upload` 端点
- 安全防护：`sanitizeFilename()` 防路径遍历，`safeTempPath()` 限制下载范围

### 6. 窗口管理 → 浏览器适配

**问题**：原版有窗口最小化/最大化/关闭按钮和窗口状态管理，Web 模式下无意义。

**解决方案**：
- `wails_runtime.js` 中窗口管理函数全部返回 no-op 或默认值
- `AppContent.vue` 中通过 `isWeb()` 隐藏窗口控制按钮
- `AppContent.vue` 中 Web 模式使用与 Windows 相同的样式（无圆角边框）

### 7. 登录后语言不正确

**问题**：登录后进入主界面，i18n 语言未同步用户偏好设置。

**解决方案**：
- `initApp()` 开头新增 `await prefStore.loadPreferences()` + `i18n.locale.value = prefStore.currentLanguage`
- 确保在加载字体列表和连接之前先同步语言

### 8. WebSocket 认证时序

**问题**：WebSocket 在页面加载时立即连接，但此时可能尚未登录（无 Cookie），导致 401。

**解决方案**：
- `wails_runtime.js` 在模块加载时调用 `connectWebSocket()`（初始连接）
- 登录成功后调用 `ReconnectWebSocket()` 强制重连，此时 Cookie 已设置
- WebSocket 客户端支持 `waitForWebSocket()` Promise，确保连接就绪后再发送消息

### 9. 移动端浏览器适配

**问题**：桌面布局在手机浏览器上显示过小或布局错乱。

**解决方案**：
- 登录页使用标准响应式 viewport（`width=device-width`）
- 主界面根据屏幕方向和尺寸动态设置 viewport width（竖屏 680px、横屏 1280px、桌面 1024px）
- 允许用户手势缩放（`user-scalable=yes`）
- 使用 `100dvh` 替代 `100vh` 解决移动端地址栏遮挡

### 10. 退出登录流程

**问题**：桌面版通过关闭窗口退出，Web 版需要显式退出登录。

**解决方案**：
- `Ribbon.vue` 侧边栏底部新增退出登录按钮（仅 Web 模式显示）
- 点击后调用 `/api/auth/logout` 清除 Cookie
- 触发 `rdm:unauthorized` 自定义 DOM 事件
- `App.vue` 监听该事件，将 `authenticated` 设为 false，切换回登录页

### 11. 限速器内存泄漏

**问题**：分布式暴力破解可能产生大量不同 IP，导致 `rateLimiter.attempts` map 无限增长。

**解决方案**：
- 设置 `maxEntries=10000` 硬上限，超过后拒绝新 IP
- 每 5 分钟执行一次全量清理（`lastCleanup` 时间戳控制）
- 每次 `allow()` 调用时清理当前 IP 的过期记录

---

## 同步上游更新指南

当原始 Tiny RDM 发布新版本时：

### 1. Service 文件
对比新版 `backend/services/*.go`，确保所有 `runtime.EventsEmit` / `runtime.EventsOn` / `runtime.OpenFileDialog` 等调用替换为 `services.EventsEmit` / `services.EventsOn` / `services.OpenFileDialog`。

### 2. 新增 Service 方法
在对应的 `backend/api/*_api.go` 中添加新端点，在 `frontend/src/utils/api.js` 中添加对应的导出函数（函数签名必须与 Wails 生成的绑定一致）。

### 3. 新增 Service 文件
- 新建 `backend/api/xxx_api.go`
- 在 `router.go` 中注册路由
- 在 `vite.config.js` 中添加别名
- 在 `api.js` 中添加对应函数

### 4. platform 抽象层
如果新版引入了新的 `runtime.*` 调用（如新的窗口管理函数），需要在 `platform_desktop.go` 和 `platform_web.go` 中同时添加对应函数。

### 5. go.mod
合并上游依赖变更，保留 `gin-gonic/gin` 相关依赖。

### 6. vite.config.js
保留别名和 proxy 配置，合并上游其他变更。

### 7. 版本号
更新 `Dockerfile` 中 `-X main.version=x.x.x`。

### 8. 前端组件
如果新版修改了 `App.vue` 或 `AppContent.vue`，需要手动合并认证门控和 `isWeb()` 相关逻辑。

### 9. 语言文件
如果新版新增了语言文件或翻译键，确保 `LoginPage.vue` 的内置翻译字典和 `ribbon.logout` 键保持同步。
