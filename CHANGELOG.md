# 更新日志

每个版本的说明也会显示在应用的更新窗口中。

## 0.1.5

- **FlClash**: Subscriptions now shows FlClash's subscriptions, with their names and usage, instead of "Unknown subscription"; traffic already counted there keeps that name. Turn on FlClash's external controller and the app finds it by itself.
  **支持 FlClash**：「订阅」页会显示 FlClash 的订阅名称和用量，不再显示「未知订阅」；已记入「未知订阅」的历史流量不变。开启 FlClash 的外部控制后，应用可自动找到它。
- This has been tested against FlClash 0.9.2's data format, but not yet with FlClash itself on a real computer. Please report anything amiss in issue #3.
  此功能已按 FlClash 0.9.2 的数据格式测试，但尚未在真机上配合 FlClash 实际运行。如有问题，请在 issue #3 反馈。

## 0.1.4

- Exporting CSV from Apps or Hosts keeps the search: the file holds the lines the page shows.
  在「应用」「访问地址」页导出 CSV 时会应用搜索词，导出内容与页面所见一致。
- With a device picked, Subscriptions splits each subscription's traffic by that device only.
  在「订阅」页选择设备后，各订阅下的设备分项只显示所选设备。
- Pages no longer dim every few seconds as they refresh; chart tooltips show dates in the app's language.
  页面定时刷新时不再每隔几秒变暗一次；图表提示中的日期按所选语言显示。
- On Windows, Clash Verge's core is found by its running pipe too, as in service mode; Settings shows the order the app searches in.
  Windows 上也会查找运行中内核的管道（例如服务模式）；设置页中的查找顺序说明已更新。
- A settings file that cannot be read is kept as settings.json.unreadable-…, instead of being replaced by the defaults.
  无法读取的设置文件会另存为 settings.json.unreadable-…，不再被默认设置覆盖。
- Files are on disk before they replace the old ones, so a power cut leaves no empty file; queries of long ranges no longer hold up counting.
  文件先写入磁盘再替换旧文件，断电后不会留下空文件；查询较长时间范围时不再阻塞流量统计。
- The macOS app says it needs macOS 13 or later, as its code does; it runs on Apple silicon and Intel alike.
  macOS 版本标明需要 macOS 13 及以上（与程序实际要求一致），Apple 芯片和 Intel 均可运行。

## 0.1.3

- **Languages**: English, 简体中文, 繁體中文, 日本語, 한국어 and Русский. The app follows the system; choose another in Settings → This computer → Language. The menus, the tray and the messages follow too.
  **多语言**：支持简体中文、繁體中文、English、日本語、한국어、Русский，默认跟随系统，可在「设置 → 本机 → 语言」中切换；菜单、菜单栏和提示信息同步切换。
- Names the app gives unknown traffic, such as "Unidentified process", show in your language and can be searched in it.
  “未识别进程”等名称按所选语言显示，也能用该语言搜索。
- Card headers wrap their tools under the title when space runs short; the macOS menu bar menus are translated.
  卡片标题栏空间不够时工具按钮自动换行；macOS 菜单栏菜单已翻译。

## 0.1.2

- **自动更新**：每天检查一次新版本，显示更新说明，一键下载安装并重启；也可以在「设置 → 软件更新」中改为自动安装，或从菜单栏图标、应用菜单「检查更新…」手动检查。更新包经过签名校验，通常只需下载有变化的部分。
- 开启 Clash Verge 系统代理时，检查和下载更新会经由 Clash 的代理端口访问 GitHub。
- 菜单栏图标重新绘制：更宽的猫脸、更粗的箭头，小尺寸下更清晰。
- macOS 窗口左上角：红绿灯按钮下移，与 Logo、页面标题之间的间距更协调；全屏时 Logo 和标题不再贴着顶部。

## 0.1.1

- 新图标：呼应 Clash Verge 的猫，眼睛是上传、下载两个箭头。
- 修复实时页“最近 5 分钟”速度曲线只显示填充、不显示线条。

## 0.1.0

- 首个版本：按应用、访问地址、节点、代理/直连、订阅、设备统计 Clash Verge 的流量；访问记录与实时连接；通过 Tailscale 自动合并多台电脑的统计，也可用共享文件夹或 WebDAV 同步。
