# panel — 转发管理面板

基于 [go-gost/gost](https://github.com/go-gost/gost) 与 [go-gost/x](https://github.com/go-gost/x) 构建的流量转发管理面板，fork 自 [bqlpfy/flux-panel](https://github.com/bqlpfy/flux-panel) 并加入了若干新功能。

## 新增功能

- **协议链接导入**：在转发面板的导入对话框中，支持直接粘贴代理协议链接（vmess、vless、trojan、ss、hysteria2、tuic 等），自动识别并填充落地 IP 与端口，无需手动输入。
- **协议链接导出**：导出时支持"协议格式"选项，将转发规则重新生成为带有修改后入口端口的完整协议链接，方便直接分发。

## 特性

- 支持按 **隧道/账号级别** 管理转发配额，适用于多用户场景
- 支持 **TCP** 与 **UDP** 协议转发
- 两种转发模式：**端口转发** 与 **隧道转发**
- 针对指定用户的指定隧道进行 **限速** 设置
- 灵活的 **单向/双向流量计费** 配置
- 实时流量统计与历史记录
- 服务暂停 / 恢复
- 多协议支持（SOCKS4/5、HTTP、TLS、QUIC、WebSocket、Shadowsocks 等）
- 拖拽排序、连通性诊断
- 移动端适配（Android / iOS App）
- 深色 / 浅色主题

## 部署

### Docker Compose 快速部署

**面板端（稳定版）：**
```bash
curl -L https://raw.githubusercontent.com/ctsunny/panel/refs/heads/main/panel_install.sh -o panel_install.sh && chmod +x panel_install.sh && ./panel_install.sh
```

**节点端（稳定版）：**
```bash
curl -L https://raw.githubusercontent.com/ctsunny/panel/refs/heads/main/install.sh -o install.sh && chmod +x install.sh && ./install.sh
```

### 默认管理员账号

| 字段 | 值 |
|------|-----|
| 账号 | `admin_user` |
| 密码 | `admin_user` |

> ⚠️ 首次登录后请立即修改默认密码！

## 技术栈

| 层次 | 技术 |
|------|------|
| 前端 | React 18 + TypeScript + Vite + HeroUI + Tailwind CSS |
| 后端 | Spring Boot 2.7 + MyBatis Plus + MySQL 5.7 |
| 节点引擎 | Go 1.23 + go-gost/gost + go-gost/x |
| 部署 | Docker Compose + Nginx |

## 免责声明

本项目仅供个人学习与研究使用，基于开源项目进行二次开发。  
使用本项目产生的任何风险（包括但不限于配置错误、服务异常、法律责任等）均由使用者自行承担。  
请确保在合法、合规的前提下使用本项目，禁止用于任何违法或未经授权的行为。  
作者对因使用本项目造成的任何直接或间接损失概不负责。
