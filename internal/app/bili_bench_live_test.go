package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"netease-music-mygo/internal/bilibili"
)

// biliSample is one video the rules are tried on: what its uploader does
// with a tracklist, and what a person expects from it.
type biliSample struct {
	Bvid  string
	Label string
	// Want is how many songs the sample really holds; 0 where the text
	// holds no list at all.
	Want int
}

// biliSamples are real videos of uploaders who post tracklists, from the
// timestamps-and-genre-dash kind to plain lists and ones with no list.
var biliSamples = []biliSample{
	{"BV1AEHg64EH4", "CityPopRadio C157 歌手 - 歌名（年）」", 10},
	{"BV1JF476nE7w", "CityPopRadio C159 同格式 2026", 10},
	{"BV1vakRYZExU", "CityPopRadio 年度歌单 2个分P，简介分P列表", 20},
	{"BV17VLJzgEAm", "CityPopRadio 年度英文 2个分P", 20},
	{"BV14rY96SEqW", "CityPopRadio Neo C150", 10},
	{"BV13SPKeBExw", "LorryDriverRadio 歌名 - 歌手 无时间戳", 10},
	{"BV1ykEQznEaY", "星期五13th 编号 歌手 - 歌名", 17},
	{"BV1rkxp6AEsD", "北岸盐仔 歌名 - 英文歌手", 8},
	{"BV1oLxf64Egx", "DSVIDEO 时间 歌名 / 英文名 无歌手", 15},
	{"BV18XWB69Eiy", "詹姆斯的歌单 时间 歌名 (英文) 无歌手", 9},
	{"BV1PuWD6kEuP", "音浪群岛FM 无曲目表", 0},
	{"BV1tLHxz9EJj", "士多啤梨留声机 无曲目表", 0},
	{"BV1924m68E5c", "发条小饭团 评论里是 36 首的曲目表", 36},
}

// TestBiliSamplesLive tries the rules on real videos and prints how many
// songs each one yields and matches, so a change can be measured.
// LIVE=1 and a network that reaches B 站 and NetEase.
func TestBiliSamplesLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("LIVE=1 to call B 站 and NetEase")
	}
	if only := os.Getenv("BV"); only != "" {
		var keep []biliSample
		for _, s := range biliSamples {
			if strings.Contains(only, s.Bvid) {
				keep = append(keep, s)
			}
		}
		biliSamples = keep
	}
	totalGot, totalMatch := 0, 0
	for _, s := range biliSamples {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		v, err := bilibili.Resolve(ctx, s.Bvid)
		if err != nil {
			cancel()
			t.Logf("%s\t%s\tresolve: %v", s.Bvid, s.Label, err)
			continue
		}
		if biliAISong(v.Desc) {
			cancel()
			t.Logf("%s	%s	ai=true", s.Bvid, s.Label)
			continue
		}
		comment, _ := bilibili.UpperComments(ctx, v)
		chapters, _ := bilibili.Chapters(ctx, v)
		cands := biliExtract(v, comment, chapters)
		biliMatch(cands)
		matched := 0
		var rows []string
		for _, c := range cands {
			if c.Track != nil {
				matched++
				rows = append(rows, fmt.Sprintf("  ok   %s -> %s · %s", c.Raw, c.Track.Name, c.Track.ArtistNames()))
			} else {
				rows = append(rows, fmt.Sprintf("  none %s", c.Raw))
			}
		}
		cancel()
		totalGot += len(cands)
		totalMatch += matched
		t.Logf("%s\t%s\tchapters=%d extracted=%d match=%d want=%d\n%s", s.Bvid, s.Label, len(chapters), len(cands), matched, s.Want, strings.Join(rows, "\n"))
	}
	t.Logf("TOTAL extracted=%d matched=%d", totalGot, totalMatch)
}

// A music video's title names the song the site has, so the pages can say
// the site has a source for it rather than guess.
func TestBiliProbeLive(t *testing.T) {
	if os.Getenv("LIVE") == "" {
		t.Skip("LIVE=1 to call NetEase")
	}
	for _, c := range []struct{ title, want string }{
		{"夜 话｜当山瞳 - 《Darling》 1985", "Darling"},
		{"太超越时代了｜大橋純子 - 《I LOVE YOU SO》 1984", "I LOVE YOU SO"},
		{"慵懒公路情歌｜杏里 - 《MORNING HIGHWAY》1984", "MORNING HIGHWAY"},
	} {
		got := biliFind(biliProbeQuery(c.title))
		if got == nil {
			t.Errorf("%q, want the song the site has", c.title)
			continue
		}
		a, b := biliNorm(got.Name), biliNorm(c.want)
		if !strings.Contains(a, b) && !strings.Contains(b, a) {
			t.Errorf("%q found %q · %s, want %q", c.title, got.Name, got.ArtistNames(), c.want)
		}
	}
}
