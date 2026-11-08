# MyGo 云音乐

> 一款精简的第三方网易云音乐桌面客户端，让听歌更专注。

[**下载安装**](https://github.com/Denis-Tang/mygo-cloudmusic/releases/latest) · [**更新记录**](CHANGELOG.md) · [**问题反馈**](https://github.com/Denis-Tang/mygo-cloudmusic/issues)

## Overview | 产品介绍

MyGo 云音乐是一款面向 Windows 和 Linux 的第三方网易云音乐客户端。如果你主要用网易云音乐听歌，想保留日推、搜索、歌单和歌词，同时减少不常用的功能入口，可以用它专注听歌。

它默认按发行日期筛选每日推荐和私人 FM，减少遇到新近发行的 AI 歌曲的机会；你也可以自己选择筛选范围。播放时使用网易云音乐提供的音源，支持从标准到超清母带等音质选择。

## Preview | 产品预览

![MyGo 云音乐 0.1.3 播放页：封面铺满窗口，歌名、歌手和进度条使用封面同色系](docs/images/now-playing.png)

播放页在播放器窗口内展开封面。宽屏下，封面从左侧的清晰画面逐渐过渡到右侧的虚化画面；歌名、歌手和进度条随封面取色。

## Features | 核心功能

### 1. 自选发行日期过滤范围

默认过滤每日推荐和私人 FM 中，专辑发行日期在 **2026 年 2 月 27 日及以后**的歌曲。日期按北京时间计算；无法获取专辑发行日期时，歌曲会暂时保留。

在设置的“发行日期过滤”中，分别勾选每日推荐、私人 FM 和其他播放（搜索、歌单等）。搜索和歌单中的手动播放默认不受这个日期限制。

这是按日期筛选，并不识别歌曲是否由 AI 生成：符合日期条件的非 AI 歌曲也会被过滤，较早发行的 AI 歌曲仍可能出现。

### 2. 随封面变色的播放页

歌名、歌手、进度条和拖动圆点使用封面中的同色系颜色，降低饱和度并提高亮度，切歌时平滑过渡。文字后方的渐变压暗背景，让浅色封面上的信息也容易看清。

点击封面切换歌词显示，在“更多”面板中查看待播清单和评论。面板支持折叠、拖动和调整分区高度。

### 3. 搜索、收藏与日常听歌

搜索提供“综合”、单曲、歌单、专辑、歌手和用户分类，也支持在歌单内查找歌曲。喜欢的歌曲可以收藏到歌单，歌单和专辑可以收藏，收藏的专辑可在“我的收藏”中查看。

界面保留每日推荐、私人 FM、歌词和播放队列等常用功能，支持歌词翻译、浅色和深色主题、侧栏收起，以及退出后恢复播放进度。

### 4. 音质选择与播放

可在设置中选择标准、较高、极高、无损、Hi-Res 和超清母带音质。实际音质取决于歌曲提供的音源和账号权限；部分音质需要会员。播放器会对非 48 kHz 音源进行采样率转换，因此不能保证与官方客户端的输出完全相同。

在线歌曲先缓冲再播放，断流后积累缓冲再继续；听过的歌曲可缓存到本机。点选歌曲加载失败时，播放条会保留选中的歌曲并提示原因。较早的歌单或 FM 加载请求也不会覆盖你后来点选的歌曲。

## Quick Start | 快速开始

### 环境要求

- Windows 或 Linux，支持 amd64 和 arm64 架构。
- 使用网易云音乐账号扫码登录，会员曲目和音质按账号权限提供。

### 安装

前往 [GitHub Releases](https://github.com/Denis-Tang/mygo-cloudmusic/releases/latest)，下载适用于 Windows 或 Linux 的安装包，按提示安装并启动。

## Usage | 使用指南

### 开始听歌

1. 使用网易云音乐账号扫码登录。
2. 在首页查看每日推荐，或搜索歌曲、打开自己的歌单。
3. 打开设置，选择需要的音质和发行日期过滤范围。
4. 点击歌曲开始播放，展开播放页查看封面、歌词和待播清单。

### 收藏与更新

在底部播放条或播放页把当前歌曲添加到歌单；在歌单、专辑详情页点击收藏。打开“我的收藏”查看收藏的专辑。

在设置中点击“检查更新”，或开启自动检查更新和自动下载安装。也可以从 [GitHub Releases](https://github.com/Denis-Tang/mygo-cloudmusic/releases/latest) 下载最新安装包。

## Architecture | 技术架构

使用 Go 和 [MyGo](https://github.com/egoist/mygo) 构建原生桌面界面，不使用 WebView。播放器负责音频解码、缓存和播放；网易云音乐接口提供登录、歌曲和推荐等数据。安装包和签名更新包通过 GitHub Releases 分发。

## Project Structure | 项目结构

```text
main.go             程序入口
internal/app/       界面与交互
internal/netease/   网易云音乐接口
internal/player/    播放、解码与缓存
internal/lyrics/    歌词解析
docs/               界面预览与说明
website/            项目网站
```

## Configuration | 配置说明

日常使用在应用设置中调整，无需编辑配置文件。

| 设置项 | 作用 |
| --- | --- |
| 发行日期过滤 | 分别勾选每日推荐、私人 FM 和其他播放；默认勾选前两项 |
| 音质 | 选择标准、较高、极高、无损、Hi-Res 或超清母带 |
| 音量均衡 | 按歌曲响度调整播放音量 |
| 歌曲缓存 | 选择本机缓存大小，或清除已缓存的歌曲 |
| 外观 | 跟随系统，或选择浅色、深色主题 |
| 更新 | 检查新版本，调整自动检查和下载安装设置 |

## Development | 开发指南

构建源码需要 Go 1.27.3 或更高版本。

```bash
git clone https://github.com/Denis-Tang/mygo-cloudmusic.git
cd mygo-cloudmusic
```

在仓库目录运行：

```bash
go tool mygo dev       # 开发模式运行
go tool mygo build     # 打包到 build/
go test ./internal/... # 运行项目测试
go vet ./...           # 检查 Go 代码
```

持续集成会运行代码检查和测试，跳过刘海播放器相关测试，避免依赖真实音频设备。

## FAQ | 常见问题

**为什么搜索到的新歌仍然可以播放？**

搜索和歌单等播放入口默认不启用日期过滤。要在这些入口过滤，勾选设置中的“其他播放（搜索、歌单等）”；取消勾选“每日推荐”或“私人 FM”则会关闭对应范围的过滤。

**为什么点歌后没有开始播放？**

先查看歌曲是否被标为不可播放，以及播放条中的提示。平台临时限制、登录失效、会员或购买权限、试听、网络和解码错误会分别提示原因。连续播放会跳过明确无完整播放权限或命中日期过滤的歌曲；平台和网络异常会保留选中歌曲并停止连续跳过。

**为什么选择了高音质，播放时仍可能不是该音质？**

歌曲的可用音源和账号权限各不相同，网易云音乐可能返回较低的音质。

**支持 macOS 吗？**

项目面向 Windows 和 Linux；后续不再开发或适配 macOS 版本。

## Contributing | 参与贡献

欢迎通过 [Issue](https://github.com/Denis-Tang/mygo-cloudmusic/issues) 反馈问题，或提交 Pull Request 改进项目。

## License | 开源协议

本项目采用 [AGPL-3.0](LICENSE) 许可证。

## Acknowledgements | 致谢

- [egoist/mygo-cloudmusic](https://github.com/egoist/mygo-cloudmusic)：本项目 fork 自这个上游仓库，界面与功能都从它而来；感谢原作者 EGOIST 与所有贡献者
- [MyGo](https://github.com/egoist/mygo)：原生 UI 与应用框架
- [Lucide](https://lucide.dev)：菜单的复制、链接与编辑图标，许可见 [lucide-license.txt](icon/lucide-license.txt)
- [NeteaseCloudMusicApi](https://github.com/Binaryify/NeteaseCloudMusicApi)：eapi 加密与登录流程的参考
- [folia-major](https://github.com/chthollyphile/folia-major)：歌词动画效果的参考
- [sherpa-onnx](https://github.com/k2-fsa/sherpa-onnx)、[Qwen3-ASR](https://github.com/QwenLM/Qwen3-ASR) 与 [Silero VAD](https://github.com/snakers4/silero-vad)：本机语音识别

本项目与网易云音乐官方无关，音乐版权归各权利人所有。
