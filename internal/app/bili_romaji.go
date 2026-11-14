package app

import "strings"

// biliKana is how each katakana sounds in letters, so a title the site
// writes in katakana can be compared with the same title an uploader wrote
// in letters.
var biliKana = map[rune]string{
	'ア': "a", 'イ': "i", 'ウ': "u", 'エ': "e", 'オ': "o",
	'カ': "ka", 'キ': "ki", 'ク': "ku", 'ケ': "ke", 'コ': "ko",
	'ガ': "ga", 'ギ': "gi", 'グ': "gu", 'ゲ': "ge", 'ゴ': "go",
	'サ': "sa", 'シ': "shi", 'ス': "su", 'セ': "se", 'ソ': "so",
	'ザ': "za", 'ジ': "ji", 'ズ': "zu", 'ゼ': "ze", 'ゾ': "zo",
	'タ': "ta", 'チ': "chi", 'ツ': "tsu", 'テ': "te", 'ト': "to",
	'ダ': "da", 'ヂ': "ji", 'ヅ': "zu", 'デ': "de", 'ド': "do",
	'ナ': "na", 'ニ': "ni", 'ヌ': "nu", 'ネ': "ne", 'ノ': "no",
	'ハ': "ha", 'ヒ': "hi", 'フ': "fu", 'ヘ': "he", 'ホ': "ho",
	'バ': "ba", 'ビ': "bi", 'ブ': "bu", 'ベ': "be", 'ボ': "bo",
	'パ': "pa", 'ピ': "pi", 'プ': "pu", 'ペ': "pe", 'ポ': "po",
	'マ': "ma", 'ミ': "mi", 'ム': "mu", 'メ': "me", 'モ': "mo",
	'ヤ': "ya", 'ユ': "yu", 'ヨ': "yo",
	'ラ': "ra", 'リ': "ri", 'ル': "ru", 'レ': "re", 'ロ': "ro",
	'ワ': "wa", 'ヲ': "o", 'ン': "n", 'ヴ': "vu",
	'ァ': "a", 'ィ': "i", 'ゥ': "u", 'ェ': "e", 'ォ': "o",
	'ャ': "ya", 'ュ': "yu", 'ョ': "yo", 'ヮ': "wa",
	'ヵ': "ka", 'ヶ': "ke", 'ッ': "", 'ー': "", '・': "",
}

// biliKanaPair is a sound written with two katakana.
type biliKanaPair struct{ kana, latin string }

// biliKanaPairs are the two-letter katakana, longest first.
var biliKanaPairs = []biliKanaPair{
	{"キャ", "kya"}, {"キュ", "kyu"}, {"キョ", "kyo"},
	{"ギャ", "gya"}, {"ギュ", "gyu"}, {"ギョ", "gyo"},
	{"シャ", "sha"}, {"シュ", "shu"}, {"ショ", "sho"}, {"シェ", "she"},
	{"ジャ", "ja"}, {"ジュ", "ju"}, {"ジョ", "jo"}, {"ジェ", "je"},
	{"チャ", "cha"}, {"チュ", "chu"}, {"チョ", "cho"}, {"チェ", "che"},
	{"ニャ", "nya"}, {"ニュ", "nyu"}, {"ニョ", "nyo"},
	{"ヒャ", "hya"}, {"ヒュ", "hyu"}, {"ヒョ", "hyo"},
	{"ビャ", "bya"}, {"ビュ", "byu"}, {"ビョ", "byo"},
	{"ピャ", "pya"}, {"ピュ", "pyu"}, {"ピョ", "pyo"},
	{"ミャ", "mya"}, {"ミュ", "myu"}, {"ミョ", "myo"},
	{"リャ", "rya"}, {"リュ", "ryu"}, {"リョ", "ryo"},
	{"ファ", "fa"}, {"フィ", "fi"}, {"フェ", "fe"}, {"フォ", "fo"},
	{"ティ", "ti"}, {"ディ", "di"}, {"デュ", "du"},
	{"トゥ", "tu"}, {"ドゥ", "du"},
	{"ウィ", "wi"}, {"ウェ", "we"}, {"ウォ", "wo"},
	{"ツァ", "tsa"}, {"ツィ", "tsi"}, {"ツェ", "tse"}, {"ツォ", "tso"},
	{"ヴァ", "va"}, {"ヴィ", "vi"}, {"ヴェ", "ve"}, {"ヴォ", "vo"},
}

// biliKanaLatin writes the kana of a text in letters, and leaves the rest
// of it as it is.
func biliKanaLatin(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r >= 'ぁ' && r <= 'ゖ' {
			r += 'ァ' - 'ぁ'
		}
		if i+1 < len(rs) {
			two := string(r) + string(mustKana(rs[i+1]))
			found := false
			for _, p := range biliKanaPairs {
				if p.kana == two {
					b.WriteString(p.latin)
					i++
					found = true
					break
				}
			}
			if found {
				continue
			}
		}
		if latin, ok := biliKana[r]; ok {
			b.WriteString(latin)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// mustKana is the katakana form of a rune, given as it is when it is not
// hiragana.
func mustKana(r rune) rune {
	if r >= 'ぁ' && r <= 'ゖ' {
		return r + 'ァ' - 'ぁ'
	}
	return r
}

// biliSoundKey is how a title sounds, with the vowels, the repeats and the
// difference between r and l dropped: "No Reply" and "ノー・リプライ"
// come out alike, while two songs that only look alike do not.
func biliSoundKey(s string) string {
	var b strings.Builder
	var last rune
	for _, r := range strings.ToLower(biliKanaLatin(s)) {
		switch r {
		case 'l':
			r = 'r'
		case 'a', 'e', 'i', 'o', 'u':
			continue
		}
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= '\u4e00' && r <= '\u9fff')) {
			continue
		}
		if r == last {
			continue
		}
		b.WriteRune(r)
		last = r
	}
	return b.String()
}

// biliLCSRatio is how much of the longer of two keys the same order of
// their characters covers, from 0 to 1.
func biliLCSRatio(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 || len(rb) == 0 {
		return 0
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			switch {
			case ra[i-1] == rb[j-1]:
				cur[j] = prev[j-1] + 1
			case prev[j] >= cur[j-1]:
				cur[j] = prev[j]
			default:
				cur[j] = cur[j-1]
			}
		}
		prev, cur = cur, prev
		for j := range cur {
			cur[j] = 0
		}
	}
	long := len(ra)
	if len(rb) > long {
		long = len(rb)
	}
	return float64(prev[len(rb)]) / float64(long)
}

// biliHasKatakana reports whether a name is written in katakana, which the
// site spells as the song sounds rather than as its name reads.
func biliHasKatakana(s string) bool {
	for _, r := range s {
		if (r >= 'ァ' && r <= 'ヿ') || r == 'ー' {
			return true
		}
	}
	return false
}
