# Tiny RDM Docker Web 版

基于 Tiny RDM v1.2.6，新增 Docker Web 部署模式。一套代码同时支持 Wails 桌面客户端和 Docker Web 两种运行方式，原始组件零修改。

---

## 快速启动

```bash
docker-compose up -d --build
# 访问 http://localhost:8088
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
      # - SESSION_TTL=24h
    volumes:
      - ./data:/app/TinyRDM
    restart: unless-stopped
```

不设置 `ADMIN_USERNAME` 和 `ADMIN_PASSWORD` 则免登录直接使用。

### 反向代理（Nginx）

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

---

## 登录认证

### 工作流程

1. 打开页面 → `GET /api/auth/status` 检查认证状态
2. 未登录 → 显示登录页（自动检测浏览器语言，支持 10 种语言）
3. 登录 → `POST /api/auth/login` → HMAC-SHA256 Token 写入 httpOnly Cookie
4. 前端 `ReconnectWebSocket()` 重建带 Cookie 的 WebSocket 连接
5. 同步主题/语言偏好 → 进入主界面
6. 退出 → `POST /api/auth/logout` 清除 Cookie → 返回登录页

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

### 项目结构（★ = Web 新增）

```
├── main.go                            # 桌面入口（添加 //go:build !web）
├── main_web.go                      ★ # Web 入口（//go:build web）
├── Dockerfile                       ★ # 三阶段构建
├── docker-compose.yml               ★ # 部署配置
├── docker/entrypoint.sh             ★ # 容器入口脚本
├── .dockerignore                    ★
├── .github/workflows/
│   ├── docker-publish.yml           ★ # Docker 镜像发布
│   └── release-windows.yaml        ★ # Windows 桌面发布
├── backend/
│   ├── api/                         ★ # 全部 //go:build web（10 个文件）
│   │   ├── router.go                  # Gin 路由器 + CORS/CSRF + 静态文件
│   │   ├── auth.go                    # 登录认证 + 安全中间件
│   │   ├── websocket_hub.go           # WebSocket 连接池
│   │   └── *_api.go（7 个）            # 各业务 API 端点
│   └── services/
│       ├── platform_desktop.go      ★ # //go:build !web
│       ├── platform_web.go          ★ # //go:build web
│       └── connection_service_web.go ★ # Web 专用连接导入导出
├── frontend/src/
│   ├── utils/
│   │   ├── api.js                   ★ # HTTP API 适配器（~80 个函数）
│   │   ├── wails_runtime.js         ★ # Wails Runtime 替代
│   │   └── websocket.js            ★ # WebSocket 客户端
│   ├── components/
│   │   ├── LoginPage.vue            ★ # 登录页面
│   │   └── icons/Logout.vue         ★ # 退出图标
│   └── public/favicon.png           ★ # 浏览器图标
```

### 双模式对比

| | 桌面模式 (Wails) | Web 模式 (Docker) |
|---|---|---|
| 构建命令 | `wails build` | `go build -tags web` |
| 入口文件 | `main.go` (`!web`) | `main_web.go` (`web`) |
| 前后端通信 | Wails RPC（进程内调用） | HTTP REST API (Gin) |
| 事件系统 | Wails Runtime Events | WebSocket 双向推送 |
| 文件对话框 | 原生系统对话框 | HTML `<input type="file">` + 后端暂存 |
| 剪贴板 | Wails Runtime | `navigator.clipboard` API |
| 认证 | 无（本地应用） | Cookie + HMAC Token（可选） |
| 连接导入导出 | 原生文件对话框 | HTTP 文件上传/下载 |

### 核心机制：Vite 条件别名

`VITE_WEB=true` 时生效，桌面构建不受影响：

| 原始导入路径 | Web 模式实际指向 |
|---|---|
| `wailsjs/runtime/runtime.js` | `src/utils/wails_runtime.js` |
| `wailsjs/go/services/browserService.js` | `src/utils/api.js` |
| `wailsjs/go/services/connectionService.js` | `src/utils/api.js` |
| `wailsjs/go/services/cliService.js` | `src/utils/api.js` |
| `wailsjs/go/services/monitorService.js` | `src/utils/api.js` |
| `wailsjs/go/services/pubsubService.js` | `src/utils/api.js` |
| `wailsjs/go/services/preferencesService.js` | `src/utils/api.js` |
| `wailsjs/go/services/systemService.js` | `src/utils/api.js` |

> Store 层和组件代码零修改，全部通过别名重定向。

### 核心机制：平台抽象层

`platform_desktop.go` / `platform_web.go` 提供相同函数签名，Service 层统一调用：

```go
// 改动前（直接调用 Wails）：
runtime.EventsEmit(s.ctx, "event_name", data)

// 改动后（调用平台抽象层）：
services.EventsEmit(s.ctx, "event_name", data)
```

`services` 包不能直接导入 `api` 包（循环依赖），通过回调变量桥接：`main_web.go` 启动时将 `api.Hub().Emit` 赋值给 `services.EmitEventFunc`。

---

## Docker 构建

### 三阶段构建

```
阶段 1（Node 20 Alpine）：npm ci → VITE_WEB=true npm run build → frontend/dist/
阶段 2（Go 1.24 Alpine）：go build -tags web → tinyrdm 二进制
阶段 3（Alpine 3.21）：  tinyrdm + entrypoint.sh + Noto 字体
```

### 关键构建参数

| 参数 | 说明 |
|---|---|
| `-tags web` | 激活 Web 模式构建标签 |
| `-ldflags "-s -w"` | 去除调试信息，减小体积 |
| `CGO_ENABLED=0` | 纯静态编译 |
| `VITE_WEB=true` | 激活 Vite 条件别名 |
| `GOPROXY=goproxy.cn,goproxy.io,direct` | Go 模块代理（中国网络优化） |

---

## CI/CD

### Docker 镜像发布（`docker-publish.yml`）

- 触发：`release published` + 手动 `workflow_dispatch`
- 仓库：`ghcr.io/rowanchen-com/tiny-rdm-web`
- 标签：语义化版本 + `latest`

### Windows 桌面发布（`release-windows.yaml`）

- 触发：`release published` + 手动 `workflow_dispatch`
- 产物：便携版 `.zip` + NSIS 安装版 `.exe`

相比原版 v1.2.6 修复 3 处：
1. 新增 "Add NSIS to PATH" 步骤 — 修复 `makensis not found` 导致安装包未生成
2. 签名替换 — 原版 `dlemstra/code-sign-action@v1`（付费证书）→ `New-SelfSignedCertificate` 自签名
3. Installer 检测 — 原版硬编码文件名 → `Get-ChildItem -Filter "*-installer.exe"` 动态查找

---

## 安全特性

### 认证安全
- httpOnly Cookie + SameSite=Strict + IP 绑定
- 登录限速：同一 IP 每分钟 5 次，`maxEntries=10000`
- 常量时间比较（`hmac.Equal`），失败延迟 500ms
- 每次容器启动随机生成 32 字节签名密钥

### 网络安全
- CORS Origin 同源验证（支持 `X-Forwarded-Host`）
- CSRF：非 GET 请求验证 Origin/Referer
- 请求体 10MB 限制，WebSocket 最大 50 连接
- CSP 白名单仅允许 `cloudflareinsights.com` 和 `analytics.tinycraft.cc`

### 文件安全
- 上传：`sanitizeFilename()` 去除路径分隔符和 `..`
- 下载：`safeTempPath()` 验证路径在 `os.TempDir()` 内
- 存储权限：`0600`（文件）/ `0700`（目录）

---

## 数据持久化

映射 `./data:/app/TinyRDM`，包含 `connections.yaml`（连接配置）和 `preferences.yaml`（偏好设置）。

---

## 移动端适配

`App.vue` 中 `setViewport(mode)` 根据场景动态切换：

| 场景 | viewport 策略 |
|---|---|
| 登录页 | `width=device-width, initial-scale=1.0`（标准响应式） |
| 主界面 - 竖屏手机 | `width=680, user-scalable=yes` |
| 主界面 - 横屏/小屏 | `width=1280, user-scalable=yes` |
| 主界面 - 桌面/平板 | `width=1024, user-scalable=yes` |

监听 `orientationchange` + `resize` 事件，200ms 防抖重新计算。

---

## 功能对比

| 功能 | 桌面版 | Web 版 |
|---|---|---|
| Redis 全数据类型操作 | ✅ | ✅ |
| CLI / Monitor / Pubsub | ✅ | ✅（WebSocket） |
| 多语言（10 种） | ✅ | ✅ |
| 连接导入导出 | ✅ 原生对话框 | ✅ HTTP 上传/下载 |
| 登录认证 | ❌ | ✅ 可选 |
| 退出登录 | ❌ | ✅ 侧边栏底部 |
| 移动端适配 | ❌ | ✅ 动态 viewport |
| 键详情快捷键 | ✅ F5/Ctrl+R 刷新数据 | ✅ 同上（拦截浏览器默认刷新） |
| 字体列表 | ✅ 系统字体 | ✅ Noto 字体（Docker 内置） |
| Google Analytics | ✅ 启用 | ❌ 禁用 |

---

## 完整文件清单

### 修改的原始文件（33 个）

| 文件 | 改动说明 |
|---|---|
| `main.go` | 添加 `//go:build !web` |
| `go.mod` / `go.sum` | 新增 gin 依赖 |
| `backend/services/browser_service.go` | `runtime.*` → `services.*`（~15 处） |
| `backend/services/cli_service.go` | 同上（~5 处） |
| `backend/services/connection_service.go` | 同上（~10 处） |
| `backend/services/monitor_service.go` | 同上（~5 处） |
| `backend/services/pubsub_service.go` | 同上（~5 处） |
| `backend/services/preferences_service.go` | 同上（~3 处） |
| `backend/services/system_service.go` | 同上（~3 处） |
| `backend/storage/local_storage.go` | 文件权限 `0777` → `0600`/`0700` |
| `frontend/vite.config.js` | 条件别名 + dev proxy |
| `frontend/src/App.vue` | 认证门控 + viewport + 双模式分离 |
| `frontend/src/AppContent.vue` | 隐藏窗口按钮 + `100dvh` |
| `frontend/src/components/sidebar/Ribbon.vue` | 退出登录按钮 |
| `frontend/src/components/content_value/ContentValueWrapper.vue` | F5/Ctrl+R 加 `preventDefault()` |
| `frontend/src/components/content_value/ContentCli.vue` | `WaitForWebSocket` 动态导入 |
| `frontend/src/components/dialogs/AboutDialog.vue` | 版权年份 → 2026 |
| `frontend/src/utils/platform.js` | 新增 `isWeb()` |
| `frontend/src/utils/analytics.js` | try-catch 防 CSP 阻断 |
| `frontend/src/assets/styles/style.scss` | `overscroll-behavior: none` + `100dvh` |
| `frontend/src/stores/connections.js` | 修复取消操作误显示成功 |
| `frontend/src/main.js` | Web 模式跳过 `loadPreferences()` |
| `frontend/index.html` | favicon |
| `frontend/src/langs/*.json`（10 个） | 新增 `"logout"` 翻译键 |

### 新增文件（27 个）

| 文件 | 说明 |
|---|---|
| `main_web.go` | Web 入口，`//go:build web` |
| `backend/services/platform_desktop.go` | 桌面平台抽象，`//go:build !web` |
| `backend/services/platform_web.go` | Web 平台抽象，`//go:build web` |
| `backend/services/connection_service_web.go` | Web 专用连接导入导出 |
| `backend/api/router.go` | Gin 路由器 |
| `backend/api/auth.go` | 登录认证 + 安全中间件 |
| `backend/api/websocket_hub.go` | WebSocket 连接池 |
| `backend/api/connection_api.go` | 连接 API + 导入导出端点 |
| `backend/api/browser_api.go` | 数据浏览 API（~50 个端点） |
| `backend/api/cli_api.go` | CLI API |
| `backend/api/monitor_api.go` | Monitor API |
| `backend/api/pubsub_api.go` | Pubsub API |
| `backend/api/preferences_api.go` | 偏好设置 API |
| `backend/api/system_api.go` | 系统 API + 文件上传下载 |
| `frontend/src/utils/api.js` | HTTP API 适配器（~80 个函数，签名与 Wails 一致） |
| `frontend/src/utils/wails_runtime.js` | Wails Runtime 替代（事件→WebSocket、剪贴板→navigator.clipboard） |
| `frontend/src/utils/websocket.js` | WebSocket 客户端（自动重连） |
| `frontend/src/components/LoginPage.vue` | 登录页面（10 语言、主题切换） |
| `frontend/src/components/icons/Logout.vue` | 退出图标 |
| `frontend/public/favicon.png` | 浏览器图标 |
| `Dockerfile` | 三阶段构建 |
| `docker-compose.yml` | 部署配置 |
| `docker/entrypoint.sh` | 容器入口脚本 |
| `.dockerignore` | 构建排除 |
| `.github/workflows/docker-publish.yml` | Docker 镜像发布 |
| `.github/workflows/release-windows.yaml` | Windows 桌面发布 |
| `DOCKER_WEB.md` | 本文档 |

### 删除的原始文件（1 个）

| 文件 | 说明 |
|---|---|
| `frontend/package.json.md5` | 无实际用途 |

---

## 同步上游更新

当原版 Tiny RDM 发布新版本时：

1. **Service 文件** — `runtime.*` 调用替换为 `services.*`
2. **新增 Service 方法** — 对应 `backend/api/*_api.go` 添加端点 + `api.js` 添加同签名函数
3. **新增 runtime 调用** — `platform_desktop.go` 和 `platform_web.go` 同时添加
4. **合并** — `go.mod`（保留 gin）、`vite.config.js`（保留别名）、`App.vue`（保留认证门控）
5. **语言文件** — 保留 `"logout"` 键，`LoginPage.vue` 内置翻译同步新语言
6. **版本号** — 更新 Dockerfile 中 `-X main.version=x.x.x`
