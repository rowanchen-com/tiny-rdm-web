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
- [数据持久化](#数据持久化)
- [与原版的区别总结](#与原版的区别总结)
- [文件变更清单](#文件变更清单)
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
2. 如果认证已启用且未登录 → 显示登录页面
3. 用户输入用户名密码 → `POST /api/auth/login` 验证
4. 验证通过 → 后端生成 HMAC-SHA256 Token，写入 httpOnly Cookie
5. 后续所有 API 请求和 WebSocket 连接自动携带 Cookie 认证
6. 退出登录 → `POST /api/auth/logout` 清除 Cookie，返回登录页

### 登录页面特性

- 自动检测浏览器语言，支持 10 种语言（zh/tw/en/ja/ko/es/fr/ru/pt/tr）
- 契合 Tiny RDM UI 风格（NaiveUI 组件、相同配色）
- 页脚显示动态版本号 + GitHub 项目链接
- 副标题为 "Redis Web Manager"（区别于桌面版的 "Redis Desktop Manager"）

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
| WebSocket 认证 | WebSocket 连接同样需要 Cookie 认证，未认证返回 401 |

### HTTP 安全响应头

| 响应头 | 值 | 作用 |
|---|---|---|
| `X-Content-Type-Options` | `nosniff` | 防 MIME 类型嗅探 |
| `X-Frame-Options` | `SAMEORIGIN` | 防点击劫持 |
| `X-XSS-Protection` | `1; mode=block` | 浏览器 XSS 过滤 |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | 控制 Referer 泄露 |
| `Content-Security-Policy` | `default-src 'self'; ...` | 限制资源加载来源 |

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
| 文件对话框 | 原生系统对话框 | HTML `<input type="file">` + 后端暂存 |
| 剪贴板 | Wails Runtime | `navigator.clipboard` API |
| 认证 | 无（本地应用） | Cookie + HMAC Token |
| 退出 | 关闭窗口 | 侧边栏退出登录按钮 |
| 浏览器标题 | "Tiny RDM" | "Tiny RDM WEB" |

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

Service 层代码调用这些函数时无需关心底层是 Wails 还是 WebSocket。

### 核心机制：Vite 别名重定向

前端通过 `vite.config.js` 中的 7 个别名，将所有 `wailsjs/*` 导入重定向到 Web 适配器：

```javascript
// 原始 Store 代码（不修改）：
import { OpenConnection } from 'wailsjs/go/services/browserService.js'

// 桌面模式：解析到 wailsjs/go/services/browserService.js（Wails 生成的 RPC 绑定）
// Web 模式：通过 Vite 别名解析到 src/utils/api.js（HTTP 请求）
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
github.com/gin-gonic/gin v1.10.1   // HTTP 框架
github.com/gorilla/websocket v1.5.3 // WebSocket（已是 indirect 依赖，升级为 direct）
```

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

### 新增文件

#### 平台抽象层（构建标签切换）

| 文件 | 构建标签 | 说明 |
|---|---|---|
| `backend/services/platform_desktop.go` | `!web` | 封装 Wails Runtime 函数，类型别名指向 Wails 类型 |
| `backend/services/platform_web.go` | `web` | 回调变量桥接到 WebSocket，Stub 类型替代 Wails 类型 |

#### HTTP API 层（全部 `//go:build web` 或无标签）

| 文件 | 说明 |
|---|---|
| `backend/api/router.go` | Gin 路由器：安全头、CORS、公开路由、认证中间件、静态文件服务 |
| `backend/api/auth.go` | 登录认证：Token 生成/验证、限速、安全中间件（`//go:build web`） |
| `backend/api/websocket_hub.go` | WebSocket 连接池管理、事件广播、消息分发 |
| `backend/api/event_bridge.go` | 事件桥接辅助函数 |
| `backend/api/connection_api.go` | 连接管理 REST API |
| `backend/api/browser_api.go` | 数据浏览 REST API（约 50 个端点） |
| `backend/api/cli_api.go` | CLI 会话 REST API |
| `backend/api/monitor_api.go` | Monitor REST API |
| `backend/api/pubsub_api.go` | Pubsub REST API |
| `backend/api/preferences_api.go` | 偏好设置 REST API |
| `backend/api/system_api.go` | 系统信息 + 文件上传下载 REST API |

#### Web 入口

| 文件 | 说明 |
|---|---|
| `main_web.go` | `//go:build web`，Gin HTTP 服务器、回调连接、服务初始化、优雅关闭 |

---

## 前端改动详解

### 原始文件修改

#### 1. `frontend/vite.config.js` — 别名 + 代理
- 新增 7 个 `wailsjs/*` → `src/utils/` 别名（见上方别名映射表）
- 新增 dev server proxy 配置（`/api` → HTTP，`/ws` → WebSocket）

#### 2. `frontend/src/App.vue` — 登录认证门控
- 新增 auth gate：`authChecking` → `LoginPage` → `AppContent`
- `checkAuth()` 在 `onMounted` 时调用 `/api/auth/status`
- `onLogin()` 触发 `initApp()` 加载偏好设置并设置 i18n 语言
- 监听 `rdm:unauthorized` 事件，401 时自动跳回登录页
- `initApp()` 开头加载偏好设置并同步语言，修复登录后语言不正确的 bug

#### 3. `frontend/src/AppContent.vue` — 隐藏窗口控制按钮
- 右上角最小化/最大化/关闭按钮在 Web 模式下隐藏（`v-if="!isMacOS() && !isWeb()"`)

#### 4. `frontend/src/components/sidebar/Ribbon.vue` — 退出登录按钮
- 左下角侧边栏新增退出登录图标按钮（仅 Web 模式显示）
- 点击调用 `/api/auth/logout` 清除 Cookie，触发 `rdm:unauthorized` 返回登录页

#### 5. `frontend/src/utils/platform.js` — 新增 Web 平台检测
- 新增 `isWeb()` 函数，`Environment()` 返回 `platform: 'web'` 时为 true

#### 6. `frontend/index.html` — 标题和图标
- 标题改为 "Tiny RDM WEB"
- 新增 favicon：`<link rel="icon" type="image/png" href="/favicon.png" />`

#### 7. 10 个语言文件 — 新增 `ribbon.logout` 翻译键
- `zh-cn.json`：退出登录
- `zh-tw.json`：登出
- `en-us.json`：Sign Out
- `ja-jp.json`：ログアウト
- `ko-kr.json`：로그아웃
- 以及 es/fr/ru/pt/tr

### 新增文件

| 文件 | 说明 |
|---|---|
| `frontend/src/utils/api.js` | HTTP API 适配器，所有函数签名与 Wails 绑定一致，401 时触发 `rdm:unauthorized` |
| `frontend/src/utils/wails_runtime.js` | Wails Runtime 的 Web 替代：事件→WebSocket、剪贴板→navigator API、窗口管理→no-op |
| `frontend/src/utils/websocket.js` | WebSocket 客户端，自动重连（3 秒间隔），事件监听/分发 |
| `frontend/src/components/LoginPage.vue` | 登录页面，NaiveUI 组件，10 语言自动检测，版本号 + GitHub 页脚 |
| `frontend/src/components/icons/Logout.vue` | 退出登录 SVG 图标，风格与现有图标一致 |
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

# 阶段 3：运行时（Alpine 3.19，约 30MB）
FROM alpine:3.19
# 仅包含 tinyrdm 二进制 + entrypoint.sh
```

### 关键构建参数

- `-tags web`：激活 Web 模式构建标签
- `-ldflags "-s -w -X main.version=1.2.6"`：去除调试信息、注入版本号
- `CGO_ENABLED=0`：纯静态编译，无 C 依赖
- `GOPROXY=https://goproxy.cn,https://goproxy.io,direct`：Go 模块代理（中国网络）
- `XDG_CONFIG_HOME=/app`：配置文件存储在 `/app/TinyRDM/`

### 相关文件

| 文件 | 说明 |
|---|---|
| `Dockerfile` | 三阶段构建定义 |
| `docker-compose.yml` | 一键部署：端口映射、环境变量、数据卷 |
| `docker/entrypoint.sh` | 入口脚本，透传环境变量 |
| `.dockerignore` | 排除 node_modules、.git、docs 等 |

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

## 与原版的区别总结

### 功能差异

| 功能 | 桌面版 | Web 版 |
|---|---|---|
| 窗口控制按钮（最小化/最大化/关闭） | ✅ 显示 | ❌ 隐藏（浏览器自带） |
| 原生文件对话框 | ✅ 系统对话框 | ⚠️ HTML 文件选择器 + 后端暂存 |
| 登录认证 | ❌ 无 | ✅ 可选（环境变量控制） |
| 退出登录按钮 | ❌ 无 | ✅ 侧边栏底部 |
| Google Analytics | ✅ 启用 | ❌ 禁用（密钥为空） |
| 浏览器标题 | "Tiny RDM" | "Tiny RDM WEB" |
| 登录页副标题 | — | "Redis Web Manager" |
| Favicon | — | ✅ icon.png |

### 不影响的功能

以下功能在 Web 版中完全保留，行为与桌面版一致：
- Redis 连接管理（增删改查、分组、排序）
- 数据浏览（所有数据类型：String/Hash/List/Set/ZSet/Stream）
- CLI 终端
- Monitor 监控
- Pubsub 订阅/发布
- 偏好设置（语言、主题、字体等）
- 慢日志、客户端列表
- 键导入/导出
- 多语言支持（10 种语言）

---

## 文件变更清单

### 原始文件修改（最小化改动）

| 文件 | 改动 |
|---|---|
| `main.go` | 添加 `//go:build !web` 构建标签（1 行） |
| `go.mod` | 新增 `gin-gonic/gin`、`gorilla/websocket` 依赖 |
| `backend/services/browser_service.go` | `runtime.EventsEmit` → `services.EventsEmit` 等 |
| `backend/services/cli_service.go` | 同上 |
| `backend/services/connection_service.go` | 同上 |
| `backend/services/monitor_service.go` | 同上 |
| `backend/services/pubsub_service.go` | 同上 |
| `backend/services/preferences_service.go` | 同上 |
| `backend/services/system_service.go` | 同上 |
| `frontend/vite.config.js` | 新增 7 个别名 + dev proxy |
| `frontend/src/App.vue` | 新增登录认证门控 |
| `frontend/src/AppContent.vue` | Web 模式隐藏窗口控制按钮 |
| `frontend/src/components/sidebar/Ribbon.vue` | Web 模式新增退出登录按钮 |
| `frontend/src/utils/platform.js` | 新增 `isWeb()` 函数 |
| `frontend/index.html` | 标题改为 "Tiny RDM WEB"，新增 favicon |
| `frontend/src/langs/*.json`（10 个） | 新增 `ribbon.logout` 翻译键 |

### 新增文件

| 文件 | 说明 |
|---|---|
| `main_web.go` | Web 入口，Gin 服务器 + 优雅关闭 |
| `backend/services/platform_desktop.go` | 桌面平台抽象层 |
| `backend/services/platform_web.go` | Web 平台抽象层 |
| `backend/api/router.go` | Gin 路由器 |
| `backend/api/auth.go` | 登录认证系统 |
| `backend/api/websocket_hub.go` | WebSocket 连接管理 |
| `backend/api/event_bridge.go` | 事件桥接 |
| `backend/api/connection_api.go` | 连接管理 API |
| `backend/api/browser_api.go` | 数据浏览 API |
| `backend/api/cli_api.go` | CLI API |
| `backend/api/monitor_api.go` | Monitor API |
| `backend/api/pubsub_api.go` | Pubsub API |
| `backend/api/preferences_api.go` | 偏好设置 API |
| `backend/api/system_api.go` | 系统信息 API |
| `frontend/src/utils/api.js` | HTTP API 适配器 |
| `frontend/src/utils/wails_runtime.js` | Wails Runtime Web 替代 |
| `frontend/src/utils/websocket.js` | WebSocket 客户端 |
| `frontend/src/components/LoginPage.vue` | 登录页面 |
| `frontend/src/components/icons/Logout.vue` | 退出图标 |
| `frontend/public/favicon.png` | 浏览器图标 |
| `Dockerfile` | Docker 构建文件 |
| `docker-compose.yml` | Docker Compose 配置 |
| `docker/entrypoint.sh` | 容器入口脚本 |
| `.dockerignore` | Docker 构建排除规则 |
| `DOCKER_WEB.md` | 本文档 |

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

### 4. go.mod
合并上游依赖变更，保留 `gin-gonic/gin` 和 `gorilla/websocket` 相关依赖。

### 5. vite.config.js
保留别名和 proxy 配置，合并上游其他变更。

### 6. 版本号
更新 `Dockerfile` 中 `-X main.version=x.x.x`。
