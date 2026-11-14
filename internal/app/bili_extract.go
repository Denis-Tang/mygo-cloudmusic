package app

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"netease-music-mygo/internal/bilibili"
	"netease-music-mygo/internal/netease"
)

// A biliCand is a song guessed from a video: its name, the artist if the
// text says, and where in the video it starts when the text says.
type biliCand struct {
	Name    string
	Artist  string
	StartMs int64
	// Track is what the song was matched to on NetEase, nil when nothing.
	Track *netease.Track
}

// biliCandMost is the most songs one video is taken to hold.
const biliCandMost = 500

var (
	// A line of a tracklist: a timestamp, then the song.
	biliStampRe = regexp.MustCompile(`(?m)^\s*(?:(\d{1,2}):)?(\d{1,2}):(\d{2})\s*[-–—:：|]?\s*(.+?)\s*$`)
	// A numbered or bulleted line of a list.
	biliListRe = regexp.MustCompile(`(?m)^\s*(?:\d{1,3}[.、)）]\s*|[-•·]\s+)(.+?)\s*$`)
	// A title in book quotes.
	biliQuoteRe = regexp.MustCompile(`《([^》]{1,40})》`)
	// A date or an index at the start of a part's title.
	biliLeadRe = regexp.MustCompile(`^\s*(?:\d{4}[-./年]\d{1,2}[-./月]\d{1,2}日?\s*|\d{1,3}\s+|\d{1,3}[.、)）]\s*|P\d{1,3}\s*[:：.-]?\s*)`)
	// Lines that say nothing about a song.
	biliNoiseRe = regexp.MustCompile(`(?i)^(https?://|@|#|本视频|视频|仅供|版权|如有侵权|关注|点赞|投币|收藏|三连|求|感谢|声明|简介|更新|订阅|收听|播放|歌曲|曲目|歌单|playlist|tracklist|setlist|track ?list)`)
)

// biliExtract guesses the songs a video holds, from the most reliable
// source to the least: its parts, the timestamps and lists under it, what
// it mentions in quotes, and the uploader's own comment.
func biliExtract(v bilibili.Video, comment string) []biliCand {
	var out []biliCand
	if len(v.Pages) >= 2 {
		for _, p := range v.Pages {
			if c, ok := biliCandFromTitle(p.Title, time.Duration(0)); ok {
				out = append(out, c)
			}
		}
	}
	if len(out) == 0 {
		out = biliFromStamps(v.Desc)
	}
	if len(out) == 0 {
		out = biliFromList(v.Desc)
	}
	if len(out) == 0 {
		out = biliFromQuotes(v.Desc)
	}
	if len(out) == 0 && comment != "" {
		if cs := biliFromStamps(comment); len(cs) > 0 {
			out = cs
		} else {
			out = biliFromList(comment)
		}
	}
	return biliUnique(out)
}

// biliCandFromTitle reads a part's title, as "歌名 - 歌手" or a date and a
// name.
func biliCandFromTitle(title string, dur time.Duration) (biliCand, bool) {
	t := biliLeadRe.ReplaceAllString(strings.TrimSpace(title), "")
	t = strings.TrimSpace(t)
	if t == "" || biliNoiseRe.MatchString(t) {
		return biliCand{}, false
	}
	name, artist := biliSplit(t)
	return biliCand{Name: name, Artist: artist}, true
}

// biliSplit splits "name - artist" on the last dash, which is how most
// uploaders write it.
func biliSplit(s string) (name, artist string) {
	s = strings.TrimSpace(s)
	for _, sep := range []string{" - ", " – ", " — ", "-", "–", "—"} {
		if i := strings.LastIndex(s, sep); i > 0 {
			name, artist = strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+len(sep):])
			if name != "" && artist != "" && len(artist) <= 40 {
				return biliClean(name), biliClean(artist)
			}
		}
	}
	return biliClean(s), ""
}

// biliFromStamps reads the timestamped lines of a text.
func biliFromStamps(text string) []biliCand {
	var out []biliCand
	for _, m := range biliStampRe.FindAllStringSubmatch(text, -1) {
		h, _ := strconv.Atoi(m[1])
		min, _ := strconv.Atoi(m[2])
		sec, _ := strconv.Atoi(m[3])
		rest := m[4]
		if biliNoiseRe.MatchString(rest) || len(rest) < 1 {
			continue
		}
		name, artist := biliSplit(rest)
		if name == "" {
			continue
		}
		out = append(out, biliCand{Name: name, Artist: artist, StartMs: int64(h)*3600000 + int64(min)*60000 + int64(sec)*1000})
	}
	return out
}

// biliFromList reads a numbered or bulleted list.
func biliFromList(text string) []biliCand {
	var out []biliCand
	for _, m := range biliListRe.FindAllStringSubmatch(text, -1) {
		line := strings.TrimSpace(m[1])
		if line == "" || biliNoiseRe.MatchString(line) || strings.Contains(line, "http") {
			continue
		}
		name, artist := biliSplit(line)
		if name == "" {
			continue
		}
		out = append(out, biliCand{Name: name, Artist: artist})
	}
	return out
}

// biliFromQuotes reads the names in book quotes.
func biliFromQuotes(text string) []biliCand {
	var out []biliCand
	for _, m := range biliQuoteRe.FindAllStringSubmatch(text, -1) {
		name := biliClean(m[1])
		if name == "" {
			continue
		}
		out = append(out, biliCand{Name: name})
	}
	return out
}

// biliClean drops what a name carries besides the name.
func biliClean(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "《》\"“”'‘’[]【】()（）")
	s = strings.TrimSpace(s)
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

// biliUnique drops the repeats, keeping the order.
func biliUnique(in []biliCand) []biliCand {
	seen := map[string]bool{}
	var out []biliCand
	for _, c := range in {
		key := strings.ToLower(strings.TrimSpace(c.Name + "|" + c.Artist))
		if key == "|" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
		if len(out) >= biliCandMost {
			break
		}
	}
	return out
}
