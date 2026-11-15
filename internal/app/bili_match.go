package app

import (
	"strings"

	"netease-music-mygo/internal/netease"
)

// biliSongSearch is the site's song search, which tests replace.
var biliSongSearch = netease.Search

// biliFind returns the NetEase track that best names the song a line of a
// tracklist tells, or nil where nothing is close enough.
func biliFind(raw string) *netease.Track {
	var best *netease.Track
	bestScore := 0
	for _, q := range biliQueries(raw) {
		best, bestScore = biliBestOf(raw, q, best, bestScore)
		if bestScore >= 130 {
			break
		}
	}
	if bestScore < 50 {
		// Only where the line as a whole found nothing: the site may spell
		// the artist another way, and the title alone still finds the song.
		for _, q := range biliTitleOnly(raw) {
			best, bestScore = biliBestOf(raw, q, best, bestScore)
			if bestScore >= 130 {
				break
			}
		}
	}
	if bestScore < 50 {
		return nil
	}
	return best
}

// biliBestOf searches one query and keeps the track that scores highest.
func biliBestOf(raw, q string, best *netease.Track, bestScore int) (*netease.Track, int) {
	hits, err := biliSongSearch(q, 8)
	if err != nil {
		return best, bestScore
	}
	for j := range hits {
		s := biliScore(raw, &hits[j])
		if j == 0 && s > 0 {
			// The site's own first answer is usually the right one.
			s += 5
		}
		if s > bestScore {
			best, bestScore = &hits[j], s
		}
	}
	return best, bestScore
}

// biliTitleOnly are the searches for each side of a line on its own, the
// longer side first, which is the title more often than the artist.
func biliTitleOnly(raw string) []string {
	a, b, ok := biliTwoParts(raw)
	if !ok {
		return nil
	}
	out := []string{a, b}
	if len([]rune(b)) > len([]rune(a)) {
		out = []string{b, a}
	}
	return out
}

// biliQueries are the searches to try for a line, the line itself first,
// then its two sides in both orders: uploaders write "歌手 - 歌名" as
// often as "歌名 - 歌手".
func biliQueries(raw string) []string {
	q := strings.TrimSpace(raw)
	if q == "" {
		return nil
	}
	var out []string
	// A bracket often holds a translation, a year or a note, which the
	// site does not search by.
	if bare := biliClean(biliAsideRe.ReplaceAllString(q, "")); bare != "" && bare != q && len([]rune(bare))*3 >= len([]rune(q)) {
		out = append(out, bare)
	}
	out = append(out, q)
	if a, b, ok := biliTwoParts(q); ok {
		for _, s := range []string{a + " " + b, b + " " + a} {
			if s != q {
				out = append(out, s)
			}
		}
	}
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}

// biliScore says how strongly a track names the song a line tells: a title
// that matches counts most, the artist on the other side of the line
// confirms it, and an artist that plainly is not there rules it out.
func biliScore(raw string, t *netease.Track) int {
	names := []string{biliNorm(t.Name)}
	if short := biliNorm(biliAsideRe.ReplaceAllString(t.Name, "")); short != "" && short != names[0] {
		names = append(names, short)
	}
	for _, al := range t.Alias {
		if n := biliNorm(al); n != "" {
			names = append(names, n)
		}
	}
	best, bestPart := 0, ""
	for _, part := range biliTitleParts(raw) {
		// The brackets a line carries often hold a translation, a year or
		// a note, and the title is what is left.
		forms := []string{part}
		if bare := strings.TrimSpace(biliAsideRe.ReplaceAllString(part, "")); bare != part && bare != "" {
			forms = append(forms, bare)
		}
		for _, form := range forms {
			pn := biliNorm(form)
			if len([]rune(pn)) < 2 {
				continue
			}
			for _, n := range names {
				s := 0
				switch {
				case pn == n:
					s = 100
				case biliNear(pn, n):
					s = 60
				}
				if s > best {
					best, bestPart = s, part
				}
			}
		}
	}
	if best == 0 {
		// One title written in letters and the other in katakana name the
		// same song; only the artist beside it can confirm that.
		if a, b, ok := biliTwoParts(raw); ok {
			if (biliNamed(a, t) && biliSoundsLike(b, t)) || (biliNamed(b, t) && biliSoundsLike(a, t)) {
				return 60
			}
		}
		return 0
	}
	rn := biliNorm(raw)
	if a, b, ok := biliTwoParts(raw); ok && biliNorm(bestPart) != rn {
		other := a
		if biliNorm(a) == biliNorm(bestPart) {
			other = b
		}
		if biliNamed(other, t) {
			// The other side is the artist, written as the site has it.
			return best + 30
		}
		if biliHasCJK(other) || !biliHasCJK(t.ArtistNames()) {
			// The other side names someone who is not on the track.
			best -= 60
		}
		return best
	}
	// Without two sides, the artist only counts where both write it the
	// same way.
	if an := biliNorm(t.ArtistNames()); len([]rune(an)) >= 2 && strings.Contains(rn, an) {
		best += 30
	}
	return best
}

// biliNamed reports whether a name a line carries is on the track at all:
// its artist, its album or its name. Japanese names are often written with
// another form of the same character, so most of the characters count.
func biliNamed(name string, t *netease.Track) bool {
	n := biliNorm(name)
	if n == "" {
		return true
	}
	hay := biliNorm(t.ArtistNames() + " " + t.Album.Name + " " + t.Name)
	if hay == "" {
		return false
	}
	if strings.Contains(hay, n) {
		return true
	}
	if !biliHasCJK(n) {
		for _, w := range strings.FieldsFunc(name, func(r rune) bool {
			return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
		}) {
			w = biliNorm(w)
			if len([]rune(w)) >= 4 && !biliCommonWord(w) && strings.Contains(hay, w) {
				return true
			}
		}
		return false
	}
	// Only the characters the name is written in count: letters that
	// happen to come up in another name say nothing.
	common, total := 0, 0
	for _, r := range n {
		if !biliIsCJK(r) {
			continue
		}
		total++
		if strings.ContainsRune(hay, r) {
			common++
		}
	}
	return total >= 2 && common*10 >= total*6
}

// biliHasCJK reports whether a name is written in Chinese or Japanese
// characters, which the site spells as the uploader did.
func biliHasCJK(s string) bool {
	for _, r := range s {
		if (r >= 0x4e00 && r <= 0x9fff) || (r >= 0x3040 && r <= 0x30ff) {
			return true
		}
	}
	return false
}

// biliTitleParts are the parts of a line a title may be written as: the
// whole line, and its two sides where it has a separator.
func biliTitleParts(raw string) []string {
	out := []string{raw}
	if a, b, ok := biliTwoParts(raw); ok {
		out = append(out, a, b)
	}
	return out
}

// biliNear reports whether two normalized names are close enough to be
// the same title: one holds the other, and not as a small part of it.
func biliNear(a, b string) bool {
	if a == b {
		return true
	}
	short, long := a, b
	if len([]rune(short)) > len([]rune(long)) {
		short, long = long, short
	}
	sn := len([]rune(short))
	if sn < 3 || !strings.Contains(long, short) {
		return false
	}
	// Most of the name has to be the same one, not a short word inside a
	// longer title.
	return sn*10 >= len([]rune(long))*7
}

// biliSoundsLike reports whether a title sounds like the track's, where one
// is written in kana and the other in letters, as "No Reply" and
// "ノー・リプライ" are.
func biliSoundsLike(title string, t *netease.Track) bool {
	key := biliSoundKey(title)
	if len([]rune(key)) < 3 {
		return false
	}
	names := append([]string{t.Name}, t.Alias...)
	for _, n := range names {
		if biliHasKatakana(n) == biliHasKatakana(title) {
			continue
		}
		k := biliSoundKey(n)
		if len([]rune(k)) < 3 {
			continue
		}
		if biliLCSRatio(key, k) >= 0.7 {
			return true
		}
	}
	return false
}

// biliNorm keeps the letters and digits of a name, lowercased and written
// in simplified characters, so punctuation, spacing and the way a title is
// written do not decide a match.
func biliNorm(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if simple, ok := biliT2S[r]; ok {
			r = simple
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= '\u4e00' && r <= '\u9fff') ||
			(r >= '\u3040' && r <= '\u30ff') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// biliCommonWord reports whether a word says nothing about who made a
// song, so that "The Dukes" is not taken for "Jay and the Americans".
func biliCommonWord(w string) bool {
	switch w {
	case "the", "and", "for", "with", "from", "feat", "this", "that", "your", "you", "his", "her", "its", "are", "was", "live", "version", "remaster", "remastered", "edit", "mix":
		return true
	}
	return false
}

// biliIsCJK reports whether a rune is written in Chinese or Japanese
// characters.
func biliIsCJK(r rune) bool {
	return (r >= 0x4e00 && r <= 0x9fff) || (r >= 0x3040 && r <= 0x30ff)
}
