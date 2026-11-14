package app

import (
	"strings"
	"testing"

	"netease-music-mygo/internal/bilibili"
)

// A video's parts are read before anything else, one song each, with a
// date or index taken off the title, and their start from the parts before.
func TestExtractParts(t *testing.T) {
	v := bilibili.Video{Desc: "00:00 不该用 - 别人", Pages: []bilibili.Page{
		{Page: 1, Title: "01 晴天 - 周杰伦", Duration: 269 * 1000000000},
		{Page: 2, Title: "2013-10-01 夜曲 - 周杰伦"},
	}}
	got := biliExtract(v, "", nil)
	if len(got) != 2 {
		t.Fatalf("cands %+v, want two parts", got)
	}
	if got[0].Raw != "晴天 - 周杰伦" || got[1].Raw != "夜曲 - 周杰伦" {
		t.Fatalf("first %+v second %+v", got[0], got[1])
	}
	if got[0].StartMs != 0 || got[1].StartMs != 269000 {
		t.Fatalf("starts %+v, want each part after the one before", got)
	}
}

// The timestamps under a video are read in every shape uploaders write
// them: a bar after the stamp, a bracket around it, an hour, and a year in
// brackets that is not part of the song.
func TestExtractStamps(t *testing.T) {
	v := bilibili.Video{Desc: "私信up：获取歌单\n" +
		"00:00｜ICE - Dreams To Remember(1995)\n" +
		"06:06 | Hi-Fi Set - Destiny（1979）\n" +
		"[11:08] WINDY - EVERLASTIN' LOVE（1986）\n" +
		"1:02:03 稻香\n" +
		"求三连"}
	got := biliExtract(v, "", nil)
	if len(got) != 4 {
		t.Fatalf("cands %+v, want four songs", got)
	}
	want := []string{"ICE - Dreams To Remember", "Hi-Fi Set - Destiny", "WINDY - EVERLASTIN' LOVE", "稻香"}
	for i, w := range want {
		if got[i].Raw != w {
			t.Fatalf("line %d %q, want %q", i, got[i].Raw, w)
		}
	}
	if got[0].StartMs != 0 || got[1].StartMs != 366000 || got[3].StartMs != 3723000 {
		t.Fatalf("starts %+v", got)
	}
}

// A video whose parts name sections falls back to the list under it, so a
// year's mix is not read as two songs called P1 and P2.
func TestExtractSectionPagesFallBack(t *testing.T) {
	v := bilibili.Video{
		Desc: "P1\n00:00 杏里 - Last Summer Whisper\n04:58 竹内まりや - PLASTIC LOVE",
		Pages: []bilibili.Page{
			{Page: 1, Title: "2024 年度歌单 P1"},
			{Page: 2, Title: "2024 年度歌单 P2"},
		},
	}
	got := biliExtract(v, "", nil)
	if len(got) != 2 || got[0].Raw != "杏里 - Last Summer Whisper" {
		t.Fatalf("cands %+v", got)
	}
}

// A plain list under a heading is read by its separators, in either order,
// while a sentence that happens to hold a dash is not.
func TestExtractPlainList(t *testing.T) {
	v := bilibili.Video{Desc: "歌曲来源：\n" +
		"街のドルフィン - 濱田金吾\n" +
		"真夜中のドアStay With Me - 松原みき\n" +
		"DOWN TOWN - SUGAR BABE\n" +
		"Someday - Tatsuro Yamashita/Minako Yoshida\n" +
		"Nostalgic Japanese City Pop inspired by 1980s Tokyo — retro Showa-era vibes for work"}
	got := biliExtract(v, "", nil)
	if len(got) != 4 {
		t.Fatalf("cands %+v, want the four list lines", got)
	}
	if got[3].Raw != "Someday - Tatsuro Yamashita/Minako Yoshida" {
		t.Fatalf("last %+v", got[3])
	}
}

// The chapters a video's timeline names are read before the text under it,
// each carrying where it starts.
func TestExtractChapters(t *testing.T) {
	v := bilibili.Video{Desc: "感谢收听，歌单见评论"}
	chapters := []bilibili.Chapter{
		{Title: "ICE - Dreams To Remember(1995)", From: 0},
		{Title: "Hi-Fi Set - Destiny（1979）", From: 366000000000},
		{Title: "Repeat", From: 600000000000},
	}
	got := biliExtract(v, "", chapters)
	if len(got) != 2 {
		t.Fatalf("cands %+v, want two chapters", got)
	}
	if got[1].Raw != "Hi-Fi Set - Destiny" || got[1].StartMs != 366000 {
		t.Fatalf("second %+v", got[1])
	}
}

// Without a list the songs a video names in book quotes are read, then the
// uploader's own comment.
func TestExtractQuotesAndComment(t *testing.T) {
	quotes := bilibili.Video{Desc: "这次用了《晴天》和《夜曲》，还有一首《稻香》。"}
	if got := biliExtract(quotes, "", nil); len(got) != 3 {
		t.Fatalf("quotes %+v", got)
	}
	comment := bilibili.Video{Desc: "感谢收听"}
	got := biliExtract(comment, "歌单：\n00:00 晴天\n04:00 夜曲", nil)
	if len(got) != 2 || got[0].StartMs != 0 {
		t.Fatalf("comment %+v", got)
	}
}

// A real tracklist of City Pop Radio reads as its ten songs, with the year,
// the translation and the bar between the stamp and the song taken off.
func TestExtractCityPopRadioList(t *testing.T) {
	lines := []string{
		"私信up：获取 封面、歌单！！网易云&Spotify：City_Pop_Radio",
		"00:00｜ICE - Dreams To Remember(1995)",
		"06:06｜Hi-Fi Set - Destiny（1979）",
		"11:08｜WINDY - EVERLASTIN' LOVE（1986）",
		"16:12｜MIQ - 風のデュエット（風の二重奏）1984",
		"20:20｜中原めいこ - Fortune-銀の月夜のハネムーン-（1991）",
		"25:12｜鮒川麻弥 - 風のNo Reply（1984）",
		"28:40｜丸山圭子 - 揺れてふ・た・り（1980）",
		"32:28｜伊東ゆかり - 再会レストラン（1982）",
		"36:07｜須藤薫 - 雨の中の噴水（1985）",
		"40:12｜松下誠 - First Light (1981)",
	}
	v := bilibili.Video{Desc: strings.Join(lines, "\n")}
	got := biliExtract(v, "", nil)
	if len(got) != 10 {
		t.Fatalf("cands %d %+v, want ten songs", len(got), got)
	}
	if got[0].Raw != "ICE - Dreams To Remember" || got[3].Raw != "MIQ - 風のデュエット（風の二重奏）" {
		t.Fatalf("first %q fourth %q", got[0].Raw, got[3].Raw)
	}
	if got[9].StartMs != 2412000 {
		t.Fatalf("last starts %d, want 40:12", got[9].StartMs)
	}
}

// The repeats of one song are kept once, and the site's own wording is not
// taken for a song.
func TestExtractDedupesAndSkipsNoise(t *testing.T) {
	v := bilibili.Video{Desc: "00:00 晴天 - 周杰伦\n00:00 晴天 - 周杰伦\nhttps://example.com\n关注我\n00:10 Repeat"}
	got := biliExtract(v, "", nil)
	if len(got) != 1 || got[0].Raw != "晴天 - 周杰伦" {
		t.Fatalf("cands %+v", got)
	}
}

// An uploader who says a machine made the music is taken to say so, and
// one who only mentions AI for a repair is not.
func TestAISong(t *testing.T) {
	for _, text := range []string{
		"音乐由suno AI生成",
		"動画内の音楽・画像は商用利用可能なAI生成ツールを活用し",
		"All tracks are AI-generated.",
	} {
		if !biliAISong(text) {
			t.Errorf("%q, want AI", text)
		}
	}
	for _, text := range []string{"AI 修复版 1985", "City Pop 精选", "TRACK LIST"} {
		if biliAISong(text) {
			t.Errorf("%q, want no AI", text)
		}
	}
}
