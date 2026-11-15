package bilibili

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// The offline tests play the API back from testdata: a test server
// stands in for every host, and the fixed responses say what the parser
// must read.

// routeAll sends every request to the test server, whatever host it was
// for, so CDN and short-link addresses land there too.
type routeAll struct{ base string }

func (r *routeAll) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(r.base, "http://")
	return http.DefaultTransport.RoundTrip(clone)
}

// testAPI makes the package call the test server for everything, and
// records what each API was asked.
func testAPI(t *testing.T, mux *http.ServeMux) *recorded {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	oldBase, oldWWW, oldClient := apiBase, wwwBase, httpClient
	wbiCache.mu.Lock()
	oldImg, oldSub, oldAt := wbiCache.imgKey, wbiCache.subKey, wbiCache.at
	wbiCache.imgKey, wbiCache.subKey, wbiCache.at = "", "", time.Time{}
	wbiCache.mu.Unlock()
	httpClient = &http.Client{Timeout: 15 * time.Second, Transport: &routeAll{base: srv.URL}}
	apiBase = srv.URL
	wwwBase = "https://www.bilibili.com"
	t.Cleanup(func() {
		apiBase, wwwBase, httpClient = oldBase, oldWWW, oldClient
		wbiCache.mu.Lock()
		wbiCache.imgKey, wbiCache.subKey, wbiCache.at = oldImg, oldSub, oldAt
		wbiCache.mu.Unlock()
	})
	return &recorded{queries: make(map[string]url.Values)}
}

// recorded keeps the query each API path was last called with.
type recorded struct {
	mu      sync.Mutex
	queries map[string]url.Values
}

func (r *recorded) handle(path, file string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.queries[path] = req.URL.Query()
		r.mu.Unlock()
		b, err := os.ReadFile("testdata/" + file)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(b)
	}
}

func (r *recorded) query(path string) url.Values {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.queries[path]
}

// mux builds the standard test API: every endpoint answers with its
// testdata response.
func mux(t *testing.T) (*http.ServeMux, *recorded) {
	t.Helper()
	m := http.NewServeMux()
	rec := &recorded{queries: make(map[string]url.Values)}
	m.HandleFunc("/x/web-interface/nav", rec.handle("/x/web-interface/nav", "nav.json"))
	m.HandleFunc("/x/web-interface/wbi/search/type", rec.handle("/x/web-interface/wbi/search/type", "search.json"))
	m.HandleFunc("/x/web-interface/view", rec.handle("/x/web-interface/view", "view.json"))
	m.HandleFunc("/x/player/wbi/playurl", rec.handle("/x/player/wbi/playurl", "playurl.json"))
	m.HandleFunc("/x/web-interface/ranking/v2", rec.handle("/x/web-interface/ranking/v2", "ranking.json"))
	m.HandleFunc("/x/player/wbi/v2", rec.handle("/x/player/wbi/v2", "player_v2.json"))
	m.HandleFunc("/bfs/subtitle/zh.json", rec.handle("/bfs/subtitle/zh.json", "subtitle.json"))
	return m, rec
}

// The keys the nav testdata hands out, and the signature they make of a
// fixed call, as B 站's reference code signs it.
const (
	testImgKey = "7cd084941338484aae1ad9425b84077c"
	testSubKey = "4932caff0ff746eab6f01bf08b70ac45"
)

func TestWbiSign(t *testing.T) {
	if got := mixinKey(testImgKey, testSubKey); got != "ea1db124af3c7062474693fa704f4ff8" {
		t.Fatalf("mixin key %q", got)
	}
	params := url.Values{
		"search_type": {"video"},
		"keyword":     {"周杰伦"},
		"page":        {"1"},
		"wts":         {"1794568000"},
	}
	if got := wbiRid(params, testImgKey, testSubKey); got != "39aad60827ddcde6d9e5ee83b9ee682c" {
		t.Fatalf("w_rid %q", got)
	}
	// The characters the signature leaves out of values.
	filtered := url.Values{"keyword": {"a!b'c(d)e*f g"}, "wts": {"1794568000"}}
	if got := wbiRid(filtered, testImgKey, testSubKey); got != "5fe51d3d9999498524f3f280cd61635c" {
		t.Fatalf("filtered w_rid %q", got)
	}
}

func TestSearch(t *testing.T) {
	m, rec := mux(t)
	testAPI(t, m)
	videos, err := Search(context.Background(), "周杰伦", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) != 3 {
		t.Fatalf("3 results, got %d", len(videos))
	}
	first := videos[0]
	if first.Bvid != "BV1q4FTeBEMj" || first.Aid != 113911642789603 {
		t.Fatalf("ids %+v", first)
	}
	if strings.Contains(first.Title, "<") || !strings.Contains(first.Title, "周杰伦") {
		t.Fatalf("title %q", first.Title)
	}
	if first.Up != "无损音乐合集" || first.UpMid != 1346923466 {
		t.Fatalf("up %q %d", first.Up, first.UpMid)
	}
	if want := 419*time.Minute + 2*time.Second; first.Duration != want {
		t.Fatalf("duration %v", first.Duration)
	}
	if first.PubDate != 1738326240 {
		t.Fatalf("pubdate %d", first.PubDate)
	}
	if !strings.HasPrefix(first.Cover, "https://i") {
		t.Fatalf("cover %q", first.Cover)
	}
	// The call was wbi-signed: its w_rid signs its own params.
	q := rec.query("/x/web-interface/wbi/search/type")
	if q.Get("search_type") != "video" || q.Get("order") != "totalrank" || q.Get("page") != "1" {
		t.Fatalf("query %v", q)
	}
	wrid := q.Get("w_rid")
	q.Del("w_rid")
	if wrid == "" || wrid != wbiRid(q, testImgKey, testSubKey) {
		t.Fatalf("w_rid %q", wrid)
	}
}

func TestSearchRiskControl(t *testing.T) {
	m := http.NewServeMux()
	m.HandleFunc("/x/web-interface/nav", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`{"code":-101,"data":{"wbi_img":{"img_url":"https://i0.hdslb.com/bfs/wbi/7cd084941338484aae1ad9425b84077c.png","sub_url":"https://i0.hdslb.com/bfs/wbi/4932caff0ff746eab6f01bf08b70ac45.png"}}}`))
	})
	m.HandleFunc("/x/web-interface/wbi/search/type", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`{"code":-412,"message":"请求被拦截","data":null}`))
	})
	testAPI(t, m)
	_, err := Search(context.Background(), "周杰伦", 1)
	if err == nil || !strings.Contains(err.Error(), "拦截") {
		t.Fatalf("err %v", err)
	}
	if !errors.Is(err, ErrRiskControl) {
		t.Fatalf("err %v is not ErrRiskControl", err)
	}
}

func TestResolve(t *testing.T) {
	m, rec := mux(t)
	testAPI(t, m)
	ctx := context.Background()

	v, err := Resolve(ctx, "BV1Gf4y1y7wc")
	if err != nil {
		t.Fatal(err)
	}
	if v.Bvid != "BV1Gf4y1y7wc" || v.Aid != 288571926 || v.CID != 281031471 {
		t.Fatalf("%+v", v)
	}
	if v.Title != "1000集 TED-ED 【中英CC字幕】| 认真刷完英语水平直接起飞！" {
		t.Fatalf("title %q", v.Title)
	}
	if v.UpMid != 319176130 || v.Up == "" {
		t.Fatalf("up %q %d", v.Up, v.UpMid)
	}
	if v.PubDate != 1610354644 {
		t.Fatalf("pubdate %d", v.PubDate)
	}

	// A page link picks that page's CID.
	v, err = Resolve(ctx, "https://www.bilibili.com/video/BV1Gf4y1y7wc?p=2")
	if err != nil {
		t.Fatal(err)
	}
	if v.CID != 281031531 {
		t.Fatalf("p2 cid %d", v.CID)
	}

	// An av link works the same, the page chosen on it.
	v, err = Resolve(ctx, "https://www.bilibili.com/video/av288571926?p=3")
	if err != nil {
		t.Fatal(err)
	}
	if v.CID != 281031579 || v.Bvid != "BV1Gf4y1y7wc" {
		t.Fatalf("%+v", v)
	}

	// A bare av id, and a bare numeric one.
	if v, err = Resolve(ctx, "av288571926"); err != nil || v.Aid != 288571926 {
		t.Fatalf("%+v %v", v, err)
	}
	if v, err = Resolve(ctx, "288571926"); err != nil || v.Aid != 288571926 {
		t.Fatalf("%+v %v", v, err)
	}

	// A page past the last says so.
	if _, err = Resolve(ctx, "https://www.bilibili.com/video/BV1Gf4y1y7wc?p=999"); err == nil || !strings.Contains(err.Error(), "999") {
		t.Fatalf("err %v", err)
	}

	// What is not a video link says so.
	if _, err = Resolve(ctx, "https://www.bilibili.com/bangumi/play/ep374717"); err == nil || !strings.Contains(err.Error(), "仅支持") {
		t.Fatalf("err %v", err)
	}
	if _, err = Resolve(ctx, "不是链接"); err == nil || !strings.Contains(err.Error(), "未识别") {
		t.Fatalf("err %v", err)
	}

	// The view call names the video by bvid.
	if q := rec.query("/x/web-interface/view"); q.Get("bvid") != "BV1Gf4y1y7wc" {
		t.Fatalf("view query %v", q)
	}
}

func TestResolveShortLink(t *testing.T) {
	m, _ := mux(t)
	m.HandleFunc("/short", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "https://www.bilibili.com/video/BV1Gf4y1y7wc?p=2", http.StatusFound)
	})
	// One that forwards in a page of script instead of redirecting.
	m.HandleFunc("/scripted", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`<html><script>location.replace("https://www.bilibili.com/video/av288571926")</script></html>`))
	})
	testAPI(t, m)
	v, err := Resolve(context.Background(), "https://b23.tv/short")
	if err != nil {
		t.Fatal(err)
	}
	if v.CID != 281031531 {
		t.Fatalf("cid %d", v.CID)
	}
	v, err = Resolve(context.Background(), "https://b23.tv/scripted")
	if err != nil {
		t.Fatal(err)
	}
	if v.Aid != 288571926 {
		t.Fatalf("aid %d", v.Aid)
	}
}

func TestPlayURL(t *testing.T) {
	m, rec := mux(t)
	testAPI(t, m)
	audio, err := PlayURL(context.Background(), Video{Bvid: "BV1GJ411x7h7", CID: 137649199, Duration: 212 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	// The best AAC track of the three.
	if !strings.Contains(audio.URL, "/30280.m4s") {
		t.Fatalf("url %q", audio.URL)
	}
	if audio.Type != "m4a" || audio.Duration != 213*time.Second {
		t.Fatalf("%+v", audio)
	}
	if audio.Headers["Referer"] != "https://www.bilibili.com/" || audio.Headers["User-Agent"] != UserAgent {
		t.Fatalf("headers %v", audio.Headers)
	}
	q := rec.query("/x/player/wbi/playurl")
	if q.Get("fnval") != "4048" || q.Get("cid") != "137649199" {
		t.Fatalf("query %v", q)
	}
	wrid := q.Get("w_rid")
	q.Del("w_rid")
	if wrid == "" || wrid != wbiRid(q, testImgKey, testSubKey) {
		t.Fatalf("w_rid %q", wrid)
	}
}

// TestPlayURLDashChoice plays the choice of track out against a list
// where the biggest is not AAC: the biggest AAC wins.
func TestPlayURLDashChoice(t *testing.T) {
	m := http.NewServeMux()
	m.HandleFunc("/x/web-interface/nav", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`{"code":-101,"data":{"wbi_img":{"img_url":"https://i0.hdslb.com/bfs/wbi/7cd084941338484aae1ad9425b84077c.png","sub_url":"https://i0.hdslb.com/bfs/wbi/4932caff0ff746eab6f01bf08b70ac45.png"}}}`))
	})
	m.HandleFunc("/x/player/wbi/playurl", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`{"code":0,"data":{"dash":{"duration":10,"audio":[
			{"id":1,"baseUrl":"https://xy.example.com/a1.m4s","bandwidth":96000,"mimeType":"audio/mp4","codecid":0},
			{"id":2,"baseUrl":"https://xy.example.com/a2.m4s","bandwidth":192000,"mimeType":"audio/webm","codecid":12},
			{"id":3,"baseUrl":"https://xy.example.com/a3.m4s","bandwidth":128000,"mimeType":"audio/mp4","codecid":0}
		]}}}`))
	})
	testAPI(t, m)
	audio, err := PlayURL(context.Background(), Video{Bvid: "BV1GJ411x7h7", CID: 1, Duration: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(audio.URL, "/a3.m4s") {
		t.Fatalf("url %q", audio.URL)
	}
	if audio.Duration != 10*time.Second {
		t.Fatalf("duration %v", audio.Duration)
	}
}

// TestPlayURLDurl plays an old video whose only stream is a whole durl.
func TestPlayURLDurl(t *testing.T) {
	m := http.NewServeMux()
	m.HandleFunc("/x/web-interface/nav", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`{"code":-101,"data":{"wbi_img":{"img_url":"https://i0.hdslb.com/bfs/wbi/7cd084941338484aae1ad9425b84077c.png","sub_url":"https://i0.hdslb.com/bfs/wbi/4932caff0ff746eab6f01bf08b70ac45.png"}}}`))
	})
	m.HandleFunc("/x/player/wbi/playurl", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`{"code":0,"data":{"timelength":53000,"durl":[{"order":1,"length":53000,"size":123456,"url":"http://cn-jsnj3.bilivideo.com/upgcxcode/1.mp3"}]}}`))
	})
	testAPI(t, m)
	audio, err := PlayURL(context.Background(), Video{Bvid: "BV1xx411c7mD", CID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if audio.Size != 123456 || audio.Type != "mp3" || audio.Duration != 53*time.Second {
		t.Fatalf("%+v", audio)
	}
	if !strings.HasPrefix(audio.URL, "https://") {
		t.Fatalf("url %q", audio.URL)
	}
}

func TestPlayURLGone(t *testing.T) {
	m := http.NewServeMux()
	m.HandleFunc("/x/web-interface/nav", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`{"code":-101,"data":{"wbi_img":{"img_url":"https://i0.hdslb.com/bfs/wbi/7cd084941338484aae1ad9425b84077c.png","sub_url":"https://i0.hdslb.com/bfs/wbi/4932caff0ff746eab6f01bf08b70ac45.png"}}}`))
	})
	m.HandleFunc("/x/player/wbi/playurl", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`{"code":-404,"message":"啥都木有","data":null}`))
	})
	testAPI(t, m)
	if _, err := PlayURL(context.Background(), Video{Bvid: "BV1GJ411x7h7", CID: 1}); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("err %v", err)
	}
}

func TestRanking(t *testing.T) {
	m, _ := mux(t)
	testAPI(t, m)
	videos, err := Ranking(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) != 3 {
		t.Fatalf("3 videos, got %d", len(videos))
	}
	first := videos[0]
	if first.Bvid != "BV17VZpY1ENA" || first.CID != 29167322421 || first.Aid != 114256984934434 {
		t.Fatalf("%+v", first)
	}
	if first.Duration != 15*time.Second || first.UpMid != 477676711 || first.Up == "" {
		t.Fatalf("%+v", first)
	}
	if !strings.HasPrefix(first.Cover, "https://i") {
		t.Fatalf("cover %q", first.Cover)
	}
}

func TestSubtitles(t *testing.T) {
	m, rec := mux(t)
	testAPI(t, m)
	lrc, err := Subtitles(context.Background(), Video{Bvid: "BV1Gf4y1y7wc", CID: 281031471})
	if err != nil {
		t.Fatal(err)
	}
	want := "[00:00.22]All I want to do is make you mine\n" +
		"[00:02.50]Just a little bit of your time\n" +
		"[01:00.00]I'll give you everything I've got\n" +
		"[01:01.50]Just to make you mine\n"
	if lrc != want {
		t.Fatalf("lrc %q", lrc)
	}
	// The Chinese track was asked for: the subtitle file of zh.json came.
	if q := rec.query("/bfs/subtitle/zh.json"); q == nil {
		t.Fatal("the zh subtitle file was not fetched")
	}
}

func TestSubtitlesNone(t *testing.T) {
	m := http.NewServeMux()
	m.HandleFunc("/x/web-interface/nav", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`{"code":-101,"data":{"wbi_img":{"img_url":"https://i0.hdslb.com/bfs/wbi/7cd084941338484aae1ad9425b84077c.png","sub_url":"https://i0.hdslb.com/bfs/wbi/4932caff0ff746eab6f01bf08b70ac45.png"}}}`))
	})
	m.HandleFunc("/x/player/wbi/v2", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte(`{"code":0,"data":{"subtitle":{"subtitles":[]}}}`))
	})
	testAPI(t, m)
	lrc, err := Subtitles(context.Background(), Video{Bvid: "BV1Gf4y1y7wc", CID: 281031471})
	if err != nil || lrc != "" {
		t.Fatalf("lrc %q err %v", lrc, err)
	}
}

func TestSubtitlesToLRC(t *testing.T) {
	data := []byte("{\"from\":0,\"to\":1,\"content\":\"start\"}\n" +
		"{\"from\":61.5,\"to\":63.1,\"content\":\"minute\"}\n" +
		"{\"from\":0.999,\"to\":2,\"content\":\"rounds up\"}\n" +
		"not json\n" +
		"{\"from\":3.456,\"to\":4,\"content\":\"centis\"}\n")
	got, err := subtitlesToLRC(data)
	if err != nil {
		t.Fatal(err)
	}
	want := "[00:00.00]start\n" +
		"[01:01.50]minute\n" +
		"[00:01.00]rounds up\n" +
		"[00:03.46]centis\n"
	if got != want {
		t.Fatalf("lrc %q", got)
	}
}

func TestParseDuration(t *testing.T) {
	for s, want := range map[string]time.Duration{
		"3:24":    3*time.Minute + 24*time.Second,
		"419:2":   419*time.Minute + 2*time.Second,
		"1:02:03": time.Hour + 2*time.Minute + 3*time.Second,
		"0:05":    5 * time.Second,
		"bad":     0,
	} {
		if got := parseDuration(s); got != want {
			t.Fatalf("%q: %v, want %v", s, got, want)
		}
	}
}

// An uploader search reads the users the answer lists, with the newest
// videos it carries for each.
func TestSearchUsers(t *testing.T) {
	m := http.NewServeMux()
	rec := &recorded{queries: make(map[string]url.Values)}
	m.HandleFunc("/x/web-interface/nav", rec.handle("/x/web-interface/nav", "nav.json"))
	m.HandleFunc("/x/web-interface/wbi/search/type", rec.handle("/x/web-interface/wbi/search/type", "user_search.json"))
	m.HandleFunc("/x/frontend/finger/spi", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":0,"data":{"b_3":"B3","b_4":"B4"}}`))
	})
	testAPI(t, m)
	users, err := SearchUsers(context.Background(), "音乐", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 {
		t.Fatalf("users: %d, want 2", len(users))
	}
	u := users[0]
	if u.Mid != 32708543 || u.Name != "大家的音乐机" || u.Fans != 1600637 {
		t.Fatalf("user %+v, want the first of the answer", u)
	}
	if len(u.Latest) != 1 || u.Latest[0].Bvid != "BV1dAWQ6vEDq" || u.Latest[0].Duration != 63*time.Minute+56*time.Second {
		t.Fatalf("latest %+v, want the video the answer carries", u.Latest)
	}
	if q := rec.query("/x/web-interface/wbi/search/type"); q.Get("search_type") != "bili_user" || q.Get("w_rid") == "" {
		t.Fatalf("search query %v, want a signed uploader search", q)
	}
}

// A comment's member id comes as a string in places, and reading it must
// not lose the whole comment: a tracklist often lives in one.
func TestReplyMidAsString(t *testing.T) {
	var r reply
	if err := json.Unmarshal([]byte(`{"member":{"mid":"12345"},"content":{"message":"晴天"}}`), &r); err != nil {
		t.Fatal(err)
	}
	if int64(r.Member.Mid) != 12345 || r.Content.Message != "晴天" {
		t.Fatalf("reply %+v, want the id and the words", r)
	}
}
