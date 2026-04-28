# Knot 文档

Knot 是面向原生终端的 SSH/SFTP 管理工具。它不提供 TUI，也不替代你已经在用的终端模拟器；它把服务器配置、认证、跳板机、代理、文件传输、远程执行、端口转发和连接复用收进一个 CLI 工作流。

```sh
curl -fsSL https://knot.clay.li/i/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://knot.clay.li/i/install.ps1 | iex
```

## 日常路径

<div class="quick-grid">
  <div class="quick-card">
    <strong>SSH</strong>
    <p>保存服务器别名后，用 <code>knot ssh alias</code> 或快捷写法 <code>knot alias</code> 进入远程 shell。</p>
  </div>
  <div class="quick-card">
    <strong>SFTP</strong>
    <p>使用交互式 SFTP shell，或用 <code>alias:/path</code> 形式直接复制、列目录、删除和重命名。</p>
  </div>
  <div class="quick-card">
    <strong>Proxy 与跳板机</strong>
    <p>服务器配置可以引用托管 proxy，也可以设置逗号分隔的 jump host chain。</p>
  </div>
  <div class="quick-card">
    <strong>Daemon</strong>
    <p>后台 daemon 持有 SSH 连接池，让 SSH、exec、SFTP、cp 和 forward 复用已有连接。</p>
  </div>
</div>

## 快速示例

```sh
knot add web-prod --host 1.2.3.4 --user deploy --key deploy-key --tags prod
knot web-prod
knot exec web-prod "uptime" --json
knot cp ./dist/. web-prod:/var/www/html/
```

## 文档结构

- [安装与快速上手](/zh/guide/getting-started)：安装、添加服务器、连接和复制文件。
- [SSH 连接](/zh/features/ssh)：服务器配置、连接入口、快捷写法和跳板机。
- [SFTP 与文件复制](/zh/features/sftp)：交互式 SFTP、批处理 SFTP 子命令和 `knot cp`。
- [Proxy 代理](/zh/features/proxy)：托管 HTTP/SOCKS5 proxy 及服务器引用方式。
- [端口转发](/zh/features/forward)：本地、远程和动态 SOCKS5 转发。
- [服务器与全局配置](/zh/features/config)：服务器增删改查、全局设置和配置导入导出。
