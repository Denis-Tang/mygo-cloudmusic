# MyGo 云音乐

> 一款精简的第三方网易云音乐桌面客户端，让听歌更专注。

[**下载安装**](https://github.com/Denis-Tang/mygo-cloudmusic/releases/latest) · [**问题反馈**](https://github.com/Denis-Tang/mygo-cloudmusic/issues)

## Overview | 产品介绍

MyGo 云音乐保留了日推、搜索、歌单和歌词等常用功能，精简不常用的入口，让界面回归听歌本身。

如果你不想在每日推荐里遇到太多新近发行的 AI 歌曲，可以用它按发行日期筛选日推。播放时仍使用网易云音乐提供的音源，支持从标准到超清母带等音质选择。

## Features | 核心功能

### 1. 筛选每日推荐

日推会过滤专辑发行日期在 **2026 年 2 月 27 日及以后**的歌曲，减少遇到新近 AI 歌曲的机会。日期按北京时间计算；无法获取专辑发行日期时，歌曲会暂时保留。

这是按日期筛选，并不识别歌曲是否由 AI 生成：符合日期条件的非 AI 歌曲也会被过滤，较早发行的 AI 歌曲仍可能出现。

### 2. 更纯粹的听歌体验

精简功能入口，专注于搜索、每日推荐、歌单、私人 FM、歌词和播放队列。支持喜欢歌曲、收藏专辑、歌词翻译、浅色和深色主题，以及退出后恢复播放进度。

### 3. 使用网易云音乐音源

可在设置中选择标准、较高、极高、无损、Hi-Res 和超清母带音质。实际音质取决于歌曲提供的音源和账号权限；部分音质需要会员。播放器会对非 48 kHz 音源进行采样率转换，因此不能保证与官方客户端的输出完全相同。

## Quick Start | 快速开始

### 安装

前往 [GitHub Releases](https://github.com/Denis-Tang/mygo-cloudmusic/releases/latest)，下载适用于 Windows 或 Linux 的安装包，按提示安装并启动。

### 开始听歌

1. 使用网易云音乐账号扫码登录。
2. 在首页查看每日推荐，或搜索歌曲、打开自己的歌单。
3. 在设置中选择需要的音质，再开始播放歌曲。

## FAQ | 常见问题

**这个日期限制会影响其他歌单吗？**

每日推荐列表会按上述日期过滤。播放其他列表时，专辑发行日期已知且达到截止日期的歌曲也会被跳过；日期未知时会暂时放行并查询。

**为什么选择了高音质，播放时仍可能不是该音质？**

歌曲的可用音源和账号权限各不相同，网易云音乐可能返回较低的音质。

**支持哪些系统？**

项目面向 Windows 和 Linux；后续不再开发或适配 macOS 版本。

## Architecture | 技术架构

使用 Go 和 [MyGo](https://github.com/egoist/mygo) 构建原生桌面界面，不使用 WebView。播放器负责音频解码、缓存和播放；网易云音乐接口提供登录、歌曲和推荐等数据。

## Project Structure | 项目结构

```text
main.go             程序入口
internal/app/       界面与交互
internal/netease/   网易云音乐接口
internal/player/    播放、解码与缓存
internal/lyrics/    歌词解析
website/            项目网站
```

## Development | 开发指南

构建源码需要 Go 1.27.3 或更高版本。在仓库目录运行：

```bash
go tool mygo dev       # 开发模式运行
go tool mygo build     # 打包到 build/
go test ./internal/... # 运行项目测试
```

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
