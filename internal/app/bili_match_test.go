package app

import (
	"testing"

	"netease-music-mygo/internal/netease"
)

func biliTrackOf(name, artist string, alias ...string) *netease.Track {
	return &netease.Track{Name: name, Artists: []netease.Artist{{Name: artist}}, Alias: alias}
}

// A title the site spells with a remaster aside still names the song the
// uploader wrote, and the artist on the line confirms it.
func TestScoreRemasterAndArtist(t *testing.T) {
	got := biliScore("松下誠 - First Light", biliTrackOf("First Light (2018 Remaster)", "松下誠"))
	if got < 130 {
		t.Fatalf("score %d, want the title and the artist to count", got)
	}
}

// A title that only looks alike, on a track by someone else, is not the
// song: the artist on the line is not on the track.
func TestScoreRejectsAnotherArtist(t *testing.T) {
	if got := biliScore("松下誠 - First Light", biliTrackOf("First Light", "Lana Del Rey")); got >= 50 {
		t.Fatalf("score %d, want the other artist to rule it out", got)
	}
	if got := biliScore("秋本奈緒美 - TELEPATHY", biliTrackOf("Telepathy", "Jai Wolf")); got >= 50 {
		t.Fatalf("score %d, want the other artist to rule it out", got)
	}
}

// An artist written with another form of the same character is the same
// artist, as 山口百惠 and 山口百恵 are.
func TestScoreAcceptsAnotherForm(t *testing.T) {
	got := biliScore("山口百惠 - さよならの向う側", biliTrackOf("さよならの向う側", "山口百恵"))
	if got < 130 {
		t.Fatalf("score %d, want the name written the site's way", got)
	}
}

// A title that only shares its ending with the track's is not the song.
func TestScoreRejectsLooseEnding(t *testing.T) {
	if got := biliScore("ウインカーの合図 / Turn Signal", biliTrackOf("ラテンライトの合図", "Nocturn Echo")); got != 0 {
		t.Fatalf("score %d, want no match for a shared ending", got)
	}
}

// A title the uploader wrote in letters and the site wrote in katakana is
// the same song, once the artist beside it is on the track.
func TestScoreKatakanaTitle(t *testing.T) {
	got := biliScore("鮎川麻弥 - 風のNo Reply", biliTrackOf("風のノー・リプライ", "鮎川麻弥"))
	if got < 60 {
		t.Fatalf("score %d, want the title heard as the same", got)
	}
	if got := biliScore("鮎川麻弥 - 風のbelieve", biliTrackOf("風のノー・リプライ", "鮎川麻弥")); got != 0 {
		t.Fatalf("score %d, want another title of the same artist to miss", got)
	}
}

// A title written in traditional characters is the one the site writes in
// simplified ones.
func TestScoreTraditionalTitle(t *testing.T) {
	got := biliScore("事後 - 劉美君", biliTrackOf("事后", "刘美君"))
	if got < 130 {
		t.Fatalf("score %d, want the same title in the site's characters", got)
	}
	if a, b := biliNorm("遙遠的樂園"), biliNorm("遥远的乐园"); a != b {
		t.Fatalf("%q, want %q", a, b)
	}
}

// An artist written in letters under a name the site spells in Japanese is
// not proof of another song: the title still decides.
func TestScoreKeepsRomanizedArtist(t *testing.T) {
	got := biliScore("Someday - Tatsuro Yamashita", biliTrackOf("いつか（SOMEDAY）", "山下達郎"))
	if got < 60 {
		t.Fatalf("score %d, want the title to carry it", got)
	}
}

// The name beside a title confirms the artist only by the characters it is
// written in: letters that also come up in another name do not.
func TestScoreIgnoresLettersInArtist(t *testing.T) {
	got := biliScore("夜 话 - 当山瞳 - 《Darling》", biliTrackOf("夜話 (City Lights) (Feat. TAEYONG of NCT)", "东方神起"))
	if got >= 50 {
		t.Fatalf("score %d, want the other artist to rule it out", got)
	}
	// The same title, with its artist on the track, is the song.
	got = biliScore("Darling - 当山瞳", biliTrackOf("Darling (2023 Remaster)", "当山ひとみ"))
	if got < 130 {
		t.Fatalf("score %d, want the title and the artist to count", got)
	}
}
