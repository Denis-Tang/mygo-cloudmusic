# 网易云音乐 · MyGo

用 Go 和 [MyGo](https://github.com/egoist/mygo) 写的第三方网易云音乐桌面客户端，原生界面，无 WebView。

## 功能

- 扫码登录，登录态保存在系统凭据存储中
- 每日推荐、推荐歌单、排行榜、我的歌单
- 搜索单曲、歌手、专辑、用户与播客，输入时显示搜索建议，保留搜索历史
- 歌手页的热门与最新歌曲、专辑与动态，关注歌手；专辑页与用户页
- 播客：浏览与收听节目，订阅播客；在本机把节目转写成字幕（Qwen3-ASR），像歌词一样随播放滚动
- 私人 FM（推荐 / 熟悉 / 探索）与心动模式
- 边下边播，支持标准 / 较高 / 极高 / 无损（FLAC）/ Hi-Res / 超清母带音质
- 听过的歌曲缓存在本机，再次播放无需等待
- 音量均衡，暂停与继续时淡出淡入，拖动进度无杂音
- 退出后再打开，从上次的播放队列与进度继续
- 逐字歌词与翻译，正在播放页的「律动」视图与歌曲评论
- 刘海播放器：在屏幕顶部刘海处显示迷你播放器，悬停展开（在设置中开启）
- 喜欢歌曲、播放队列、随机与循环播放；歌曲列表的「…」和右键共用操作菜单，添加到歌单使用可滚动弹窗
- 浅色 / 深色 / 跟随系统
- 设置面板固定在主界面中央，点击外部关闭；名称与控件对齐、说明单独换行
- 播放页“更多”面板可拖动，待播清单与评论可分别折叠，分割线可拖动调节高度

## 构建

需要 Go 1.27+。

```bash
go tool mygo dev     # 开发模式运行
go tool mygo build   # 打包到 build/
go test ./...        # 运行测试
```

音乐播放的采样率转换在 `internal/player/resample.go`：非 48 kHz 音源先经过保留双声道的带限转换，再交给 MyGo 的 48 kHz 混音器；48 kHz 音源直接通过。字幕与频谱仍使用原始采样率，定位会补读滤波器需要的前后采样，保持播放时间不变。

封面加载在 `internal/app/artwork.go`：最多同时下载 8 张，不把旧页面的图片排队到新页面前面；失败的封面在 10 秒后允许重新加载。播放页先用已加载的普通封面显示，再换成高清封面。首页的推荐歌单与每日歌曲分别加载，推荐歌单无需等待专辑日期查询。

Logo 源文件为 `icon/note.svg`，颜色为 `#F3D99B`。程序和开发版使用 `resources/icon.png`、`resources/icon-dev.png`，托盘使用 `internal/app/tray/` 下的 PNG，网站使用 `website/public/favicon.svg` 和 `icon.png`；位图由同一 SVG 导出，Windows 可执行文件图标由 MyGo 打包时嵌入。

## 致谢

- [egoist/mygo-cloudmusic](https://github.com/egoist/mygo-cloudmusic)：本项目 fork 自这个上游仓库，界面与功能都从它而来；感谢原作者 EGOIST 与所有贡献者
- [MyGo](https://github.com/egoist/mygo)：原生 UI 与应用框架
- [Lucide](https://lucide.dev)：菜单的复制、链接与编辑图标，许可见 [lucide-license.txt](icon/lucide-license.txt)
- [NeteaseCloudMusicApi](https://github.com/Binaryify/NeteaseCloudMusicApi)：eapi 加密与登录流程的参考
- [folia-major](https://github.com/chthollyphile/folia-major)：歌词动画效果的参考
- [sherpa-onnx](https://github.com/k2-fsa/sherpa-onnx)、[Qwen3-ASR](https://github.com/QwenLM/Qwen3-ASR) 与 [Silero VAD](https://github.com/snakers4/silero-vad)：本机语音识别

本项目与网易云音乐官方无关，音乐版权归各权利人所有。

## 许可证

[AGPL-3.0](LICENSE)
