<div align="center">

<img src="resources/icon.png" width="112" alt="">

# Clash Traffic

See where your Clash Verge traffic goes — by app, site, proxy node, subscription and device.

**English** · [简体中文](README.zh-CN.md)

<picture><source media="(prefers-color-scheme: dark)" srcset="docs/screenshots/overview-dark.png"><img alt="Overview: seven days of traffic across three computers" src="docs/screenshots/overview-light.png"></picture>

</div>

## Features

- **Every app and site** — upload, download and connections per app and host. Helper processes count toward their app; subdomains can merge into their domain.
- **Proxy or direct** — one switch shows only the traffic your proxy plan counts.
- **Subscriptions** — what each subscription carried, beside the usage and expiry its provider reports.
- **All your computers, no setup** — copies running in your Tailscale tailnet find each other and merge their numbers. A shared folder or WebDAV works too.
- **Access log and live view** — every connection with its app, target, node and rule; live speed and open connections.
- **Stays out of the way** — finds Clash Verge by itself, lives in the menu bar, starts at login, updates itself, exports CSV, follows light and dark mode.

Your data stays on your computers, which exchange it directly. The app speaks English, 简体中文, 繁體中文, 日本語, 한국어 and Русский, following your system or the language you pick in Settings.

## Screenshots

<table>
<tr><td width="50%" valign="top"><picture><source media="(prefers-color-scheme: dark)" srcset="docs/screenshots/apps-dark.png"><img alt="Apps" src="docs/screenshots/apps-light.png"></picture><br><sub><b>Apps</b> — and the sites one of them visits</sub></td><td width="50%" valign="top"><picture><source media="(prefers-color-scheme: dark)" srcset="docs/screenshots/subscriptions-dark.png"><img alt="Subscriptions" src="docs/screenshots/subscriptions-light.png"></picture><br><sub><b>Subscriptions</b> — provider usage beside your own count</sub></td></tr>
<tr><td width="50%" valign="top"><picture><source media="(prefers-color-scheme: dark)" srcset="docs/screenshots/live-dark.png"><img alt="Live connections" src="docs/screenshots/live-light.png"></picture><br><sub><b>Live</b> — speed and open connections</sub></td><td width="50%" valign="top"><picture><source media="(prefers-color-scheme: dark)" srcset="docs/screenshots/records-dark.png"><img alt="Access log" src="docs/screenshots/records-light.png"></picture><br><sub><b>Access log</b> — every connection, searchable</sub></td></tr>
<tr><td width="50%" valign="top"><picture><source media="(prefers-color-scheme: dark)" srcset="docs/screenshots/devices-dark.png"><img alt="Devices" src="docs/screenshots/devices-light.png"></picture><br><sub><b>Devices</b> — found over Tailscale</sub></td><td width="50%"></td></tr>
</table>

## Install

Download the latest build from [Releases](https://github.com/lufficc/clash-traffic/releases): a universal `.dmg` for macOS 13 or later (Apple silicon and Intel), an installer for Windows (x64 or ARM64), or a `.deb` / `.tar.gz` for Linux.

The builds are not signed yet. On macOS, right-click the app and choose **Open** the first time; on Windows, choose **More info → Run anyway**.

It works with Clash Verge Rev, FlClash (turn on its external controller; subscription names and usage come along), or any mihomo core whose controller it can reach. To tell apps apart it sets the core's `find-process-mode` to `always`; add `find-process-mode: always` to Clash Verge's merge config to keep it after a reload.

## Build

With Go 1.27+ and Bun:

```sh
bun install
bun run dev        # run with live reload
bun run build      # package for this machine into build/
go test ./...      # and bun run test:web
```

Built with [MyGo](https://mygo.egoist.dev/).
