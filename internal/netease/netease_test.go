package netease

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type streamTransport func(*http.Request) (*http.Response, error)

func (f streamTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// API failures used to become unavailable songs, triggering VIP prompts and automatic skips.
func TestSongURLResponse(t *testing.T) {
	old := http.DefaultTransport
	defer func() { http.DefaultTransport = old }()
	deviceOnce.Do(func() { deviceID = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123" })
	for _, tc := range []struct {
		name, body          string
		failed, unavailable bool
		code, httpStatus    int
	}{
		{"risk control", `{"code":-460,"message":"检测到您的网络环境存在风险，请稍后再试"}`, true, false, -460, 200},
		{"HTTP risk control", `{"code":-460}`, true, false, -460, 400},
		{"song risk control", `{"code":200,"data":[{"code":-460,"fee":1}]}`, true, false, -460, 200},
		{"signed out", `{"code":301}`, true, false, 301, 200},
		{"rate limited", `{"code":429}`, true, false, 429, 200},
		{"server error", `{"code":500}`, true, false, 500, 200},
		{"HTTP server error", `<html>Unavailable</html>`, true, false, 0, 503},
		{"no stream", `{"code":200,"data":[]}`, true, false, 0, 200},
		{"no URL", `{"code":200,"data":[{"code":200,"url":null}]}`, true, false, 0, 200},
		{"VIP denied", `{"code":200,"data":[{"code":-110,"fee":1,"payed":0}]}`, true, true, 0, 200},
		{"purchase required", `{"code":200,"data":[{"code":-110,"fee":4,"payed":0}]}`, true, true, 0, 200},
		{"not found", `{"code":200,"data":[{"code":404}]}`, true, true, 0, 200},
		{"unavailable", `{"code":200,"data":[{"code":-110}]}`, true, true, 0, 200},
		{"quality trial", `{"code":200,"data":[{"url":"https://example.com/preview.mp3","payed":1,"freeTrialInfo":{"start":0,"end":30}}]}`, true, false, 0, 200},
		{"preview", `{"code":200,"data":[{"url":"https://example.com/preview.mp3","freeTrialInfo":{"start":0,"end":30}}]}`, true, true, 0, 200},
		{"unsupported format", `{"code":200,"data":[{"url":"https://example.com/full.mp4","type":"av3a"}]}`, true, false, 0, 200},
		{"full stream", `{"code":200,"data":[{"url":"http://example.com/full.flac","type":"FLAC","level":"lossless","freeTrialInfo":null}]}`, false, false, 0, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			http.DefaultTransport = streamTransport(func(r *http.Request) (*http.Response, error) {
				// A PC request with an iPhone User-Agent triggered -460 on the player gateway.
				if !strings.Contains(r.UserAgent(), "Windows NT") || !strings.Contains(r.UserAgent(), "NeteaseMusicDesktop/"+eapiClient["appver"]) {
					t.Errorf("User-Agent does not match the PC client: %q", r.UserAgent())
				}
				return &http.Response{StatusCode: tc.httpStatus, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{}}, nil
			})
			st, err := SongURL(1, "lossless")
			if (err != nil) != tc.failed || errors.Is(err, ErrUnavailable) != tc.unavailable {
				t.Fatalf("stream %+v, error %v", st, err)
			}
			var api *apiError
			if tc.code != 0 && (!errors.As(err, &api) || api.Code != tc.code) {
				t.Fatalf("lost API cause %d: %v", tc.code, err)
			}
			if !tc.failed && (st == nil || st.URL != "https://example.com/full.flac" || st.Type != "flac" || st.Level != "lossless") {
				t.Fatalf("full stream: %+v", st)
			}
		})
	}
}

func TestPlayableFollowsPrivilege(t *testing.T) {
	tracks := []Track{{ID: 1, Fee: 1}, {ID: 2, Fee: 8}, {ID: 3, Fee: 8}, {ID: 4, Fee: 1}}
	if tracks[0].Playable() || !tracks[1].Playable() {
		t.Fatal("without privileges, VIP songs do not play and free ones do")
	}
	withPrivileges(tracks, []Privilege{
		{ID: 1, Pl: 320000},           // VIP song, the listener subscribes
		{ID: 2, Pl: 0},                // nothing to play
		{ID: 3, St: -200, Pl: 128000}, // taken down
	})
	want := []bool{true, false, false, false}
	for i, tr := range tracks {
		if tr.Playable() != want[i] {
			t.Errorf("track %d: playable %v", tr.ID, tr.Playable())
		}
	}
}
