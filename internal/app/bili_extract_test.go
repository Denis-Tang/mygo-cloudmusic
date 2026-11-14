package app

import (
	"testing"

	"netease-music-mygo/internal/bilibili"
)

// A video's parts are read before anything else, one song each, with a
// date or index taken off the title.
func TestExtractParts(t *testing.T) {
	v := bilibili.Video{Desc: "00:00 不该用 - 别人", Pages: []bilibili.Page{
		{Page: 1, Title: "01 晴天 - 周杰伦"},
		{Page: 2, Title: "2013-10-01 夜曲 - 周杰伦"},
	}}
	got := biliExtract(v, "")
	if len(got) != 2 {
		t.Fatalf("cands %+v, want two parts", got)
	}
	if got[0].Name != "晴天" || got[0].Artist != "周杰伦" {
		t.Fatalf("first %+v", got[0])
	}
	if got[1].Name != "夜曲" || got[1].Artist != "周杰伦" {
		t.Fatalf("second %+v", got[1])
	}
}

// Timestamps under a video come first without parts, and carry where each
// song starts.
func TestExtractStamps(t *testing.T) {
	v := bilibili.Video{Desc: "本视频收录：\n00:00 晴天 - 周杰伦\n03:20 夜曲 - 周杰伦\n1:02:03 稻香 - 周杰伦\n求三连"}
	got := biliExtract(v, "")
	if len(got) != 3 {
		t.Fatalf("cands %+v, want three songs", got)
	}
	if got[1].StartMs != 200000 || got[2].StartMs != 3723000 {
		t.Fatalf("starts %+v", got)
	}
}

// Without timestamps a numbered list is read, then the book quotes, then
// the uploader's own comment.
func TestExtractListQuotesAndComment(t *testing.T) {
	list := bilibili.Video{Desc: "曲目：\n1. 晴天 - 周杰伦\n2、夜曲 - 周杰伦"}
	if got := biliExtract(list, ""); len(got) != 2 || got[1].Name != "夜曲" {
		t.Fatalf("list %+v", got)
	}
	quotes := bilibili.Video{Desc: "这次用了《晴天》和《夜曲》，还有一首《稻香》。"}
	if got := biliExtract(quotes, ""); len(got) != 3 {
		t.Fatalf("quotes %+v", got)
	}
	comment := bilibili.Video{Desc: "感谢收听"}
	got := biliExtract(comment, "歌单：\n00:00 晴天\n04:00 夜曲")
	if len(got) != 2 || got[0].StartMs != 0 {
		t.Fatalf("comment %+v", got)
	}
}

// The repeats of one song are kept once, and the site's own wording is not
// taken for a song.
func TestExtractDedupesAndSkipsNoise(t *testing.T) {
	v := bilibili.Video{Desc: "00:00 晴天 - 周杰伦\n00:00 晴天 - 周杰伦\nhttps://example.com\n关注我"}
	got := biliExtract(v, "")
	if len(got) != 1 || got[0].Name != "晴天" {
		t.Fatalf("cands %+v", got)
	}
}
