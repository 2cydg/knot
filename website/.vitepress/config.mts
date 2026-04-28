import { defineConfig } from 'vitepress'

const enSidebar = [
  {
    text: 'Start',
    items: [
      { text: 'Overview', link: '/' },
      { text: 'Install and Quick Start', link: '/guide/getting-started' }
    ]
  },
  {
    text: 'Features',
    items: [
      { text: 'SSH Connections', link: '/features/ssh' },
      { text: 'SFTP and File Copy', link: '/features/sftp' },
      { text: 'Remote Exec', link: '/features/exec' },
      { text: 'Proxy', link: '/features/proxy' },
      { text: 'Port Forwarding', link: '/features/forward' }
    ]
  },
  {
    text: 'Configuration and Management',
    items: [
      { text: 'Servers and Global Config', link: '/features/config' },
      { text: 'SSH Keys', link: '/features/keys' },
      { text: 'Daemon and Status', link: '/reference/daemon' },
      { text: 'Shell Completion and Version', link: '/reference/cli' }
    ]
  }
]

const zhSidebar = [
  {
    text: '开始',
    items: [
      { text: '概览', link: '/zh/' },
      { text: '安装与快速上手', link: '/zh/guide/getting-started' }
    ]
  },
  {
    text: '功能文档',
    items: [
      { text: 'SSH 连接', link: '/zh/features/ssh' },
      { text: 'SFTP 与文件复制', link: '/zh/features/sftp' },
      { text: '远程执行', link: '/zh/features/exec' },
      { text: 'Proxy 代理', link: '/zh/features/proxy' },
      { text: '端口转发', link: '/zh/features/forward' }
    ]
  },
  {
    text: '配置与管理',
    items: [
      { text: '服务器与全局配置', link: '/zh/features/config' },
      { text: 'SSH 密钥', link: '/zh/features/keys' },
      { text: 'Daemon 与状态', link: '/zh/reference/daemon' },
      { text: 'Shell 补全与版本', link: '/zh/reference/cli' }
    ]
  }
]

export default defineConfig({
  title: 'Knot',
  description: 'Native-terminal SSH and SFTP workflow',
  cleanUrls: true,
  lastUpdated: true,
  appearance: true,
  markdown: {
    lineNumbers: false
  },
  head: [
    ['link', { rel: 'alternate', hreflang: 'en', href: '/' }],
    ['link', { rel: 'alternate', hreflang: 'zh-CN', href: '/zh/' }]
  ],
  themeConfig: {
    logo: '/logo.svg',
    siteTitle: 'Knot Docs',
    search: {
      provider: 'local'
    }
  },
  locales: {
    root: {
      label: 'English',
      lang: 'en-US',
      title: 'Knot',
      description: 'Native-terminal SSH and SFTP workflow',
      themeConfig: {
        nav: [
          { text: 'Start', link: '/' },
          { text: 'Features', link: '/features/ssh' },
          { text: 'Config', link: '/features/config' },
          { text: 'GitHub', link: 'https://github.com/2cydg/knot' }
        ],
        sidebar: enSidebar,
        outline: {
          level: [2, 3],
          label: 'On this page'
        },
        docFooter: {
          prev: 'Previous',
          next: 'Next'
        },
        lastUpdated: {
          text: 'Last updated',
          formatOptions: {
            dateStyle: 'medium',
            timeStyle: 'short'
          }
        },
        langMenuLabel: 'Language',
        darkModeSwitchLabel: 'Appearance',
        sidebarMenuLabel: 'Menu',
        returnToTopLabel: 'Return to top'
      }
    },
    zh: {
      label: '简体中文',
      lang: 'zh-CN',
      title: 'Knot',
      description: '面向原生终端的 SSH 与 SFTP 工作流',
      themeConfig: {
        nav: [
          { text: '开始', link: '/zh/' },
          { text: '功能', link: '/zh/features/ssh' },
          { text: '配置', link: '/zh/features/config' },
          { text: 'GitHub', link: 'https://github.com/2cydg/knot' }
        ],
        sidebar: zhSidebar,
        outline: {
          level: [2, 3],
          label: '本页目录'
        },
        docFooter: {
          prev: '上一页',
          next: '下一页'
        },
        lastUpdated: {
          text: '最后更新',
          formatOptions: {
            dateStyle: 'medium',
            timeStyle: 'short'
          }
        },
        langMenuLabel: '语言',
        darkModeSwitchLabel: '外观',
        sidebarMenuLabel: '菜单',
        returnToTopLabel: '回到顶部'
      }
    }
  }
})
