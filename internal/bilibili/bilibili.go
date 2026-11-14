// Package bilibili is B 站's web player API, as it serves a listener who
// is not signed in: searching videos, resolving their pages, and where
// their audio streams from. Videos are never downloaded: the API gives
// the player the audio track's URL.
package bilibili

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The public B 站 web API, as the web player calls it, without an
// account: what anyone may watch and hear.

var (
	apiBase = "https://api.bilibili.com"
	wwwBase = "https://www.bilibili.com"
)

// UserAgent is the desktop Chrome the web player says it is, which B 站
// expects with every call, as the Referer of the site itself.
const UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// httpClient times each call out, and never follows short links: their
// hop is walked by hand, so a b23.tv page is read before its target.
var httpClient = &http.Client{Timeout: 15 * time.Second}

// Video is a B 站 video: its ids, who made it, and what it is. Only the
// audio of its CID plays, as the video's P if chosen.
type Video struct {
	Bvid     string
	Aid      int64
	CID      int64
	Title    string
	Up       string
	UpMid    int64
	Cover    string
	Duration time.Duration
	PubDate  int64
}

// URL is the video's page on the B 站 website.
func (v Video) URL() string {
	if v.Bvid != "" {
		return wwwBase + "/video/" + v.Bvid
	}
	return wwwBase + "/video/av" + strconv.FormatInt(v.Aid, 10)
}

// Audio is where a video's audio track streams from, as the player asks
// for it: the URL alone answers to nobody, Headers must come with it.
type Audio struct {
	URL      string
	Size     int64 // in bytes; 0 where B 站 says none
	Duration time.Duration
	Type     string
	// Headers are what the stream's CDN requires: the site's Referer and
	// the User-Agent, so the player sends them with the URL.
	Headers map[string]string
}

// ErrRiskControl is what B 站 says when it holds the client back for
// asking too fast or too strangely: the pages say to try again later.
var ErrRiskControl = errors.New("B 站风控限制")

// apiError is what a B 站 API says when it refuses a call.
type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Msg     string `json:"msg"`
}

func (e *apiError) Error() string {
	switch e.Code {
	case -352:
		return "B 站风控校验失败，请稍后再试（-352）"
	case -412:
		return "B 站请求被拦截，请更换网络或稍后再试（-412）"
	case -403:
		return "B 站拒绝了当前请求：该内容可能仅限部分地区或需要登录（-403）"
	case -404:
		return "视频不存在或已被删除"
	case 62002:
		return "视频不可见（62002）"
	case 62004:
		return "视频正在审核中（62004）"
	case 86090:
		return "该内容需要大会员或登录，暂无法播放"
	case 10004001, 10004002, 10004003, 10004004:
		return fmt.Sprintf("B 站安全校验未通过，请稍后再试（%d）", e.Code)
	}
	msg := e.Message
	if msg == "" {
		msg = e.Msg
	}
	if msg != "" && msg != "0" {
		return fmt.Sprintf("B 站返回：%s（%d）", msg, e.Code)
	}
	return fmt.Sprintf("B 站返回未知错误（%d）", e.Code)
}

// Unwrap marks the risk-control codes as ErrRiskControl, so the pages
// say to try again later rather than show the raw code.
func (e *apiError) Unwrap() error {
	switch e.Code {
	case -352, -412, 10004001, 10004002, 10004003, 10004004:
		return ErrRiskControl
	}
	return nil
}

// request sends req with the headers B 站 wants, retrying once on a
// network error or a 5xx, and returns the body and the status.
func request(req *http.Request) ([]byte, int, error) {
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Referer", wwwBase+"/")
	if c := anonCookie(); c != "" {
		req.Header.Set("Cookie", c)
	}
	var res *http.Response
	var err error
	for attempt := 0; ; attempt++ {
		res, err = httpClient.Do(req)
		if err == nil && res.StatusCode < http.StatusInternalServerError {
			break
		}
		if attempt == 1 || req.Context().Err() != nil {
			break
		}
		if res != nil {
			res.Body.Close()
		}
	}
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, res.StatusCode, err
	}
	return body, res.StatusCode, nil
}

// The cookie the site gives a listener who is not signed in: some APIs,
// the uploader's videos among them, refuse a request without it.
var (
	cookieMu   sync.Mutex
	cookieText string
	cookieAt   time.Time
)

func anonCookie() string {
	cookieMu.Lock()
	defer cookieMu.Unlock()
	if cookieText != "" && time.Since(cookieAt) < 12*time.Hour {
		return cookieText
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/x/frontend/finger/spi", nil)
	if err != nil {
		return cookieText
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Referer", wwwBase+"/")
	res, err := httpClient.Do(req)
	if err != nil {
		return cookieText
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil || res.StatusCode != http.StatusOK {
		return cookieText
	}
	var finger struct {
		Data struct {
			B3 string `json:"b_3"`
			B4 string `json:"b_4"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &finger) != nil || finger.Data.B3 == "" {
		return cookieText
	}
	cookieText, cookieAt = "buvid3="+finger.Data.B3+"; buvid4="+finger.Data.B4, time.Now()
	return cookieText
}

// api calls the web API and reads out into out, failing on a B 站 error
// code, which says in Chinese what went wrong.
func api(ctx context.Context, apiPath string, query url.Values, out any) error {
	u := apiBase + apiPath
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	body, status, err := request(req)
	if err != nil {
		return err
	}
	var reply apiError
	if err := json.Unmarshal(body, &reply); err != nil {
		return fmt.Errorf("B 站接口异常（HTTP %d）", status)
	}
	if os.Getenv("BILI_DEBUG") != "" {
		log.Printf("bilibili %s: %s", apiPath, string(body))
	}
	if reply.Code != 0 && reply.Code != 200 {
		return &reply
	}
	return json.Unmarshal(body, out)
}

// fetchBytes reads what is at u, as the site's Referer asks for it.
func fetchBytes(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	body, status, err := request(req)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("下载失败（HTTP %d）", status)
	}
	return body, nil
}

// The wbi signing, as the web player does it: the nav API hands out two
// image names that make the mixin key, which signs each wbi call with a
// timestamp. The keys change rarely, so they are kept for an hour.

var mixinKeyEncTab = [64]int{
	46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35, 27, 43, 5, 49,
	33, 9, 42, 19, 29, 28, 14, 39, 12, 38, 41, 13, 37, 48, 7, 16, 24, 55, 40,
	61, 26, 17, 0, 1, 60, 51, 30, 4, 22, 25, 54, 21, 56, 59, 6, 63, 57, 62, 11,
	36, 20, 34, 44, 52,
}

// wbiCache holds the mixin key's halves, fetched once an hour.
var wbiCache struct {
	mu     sync.Mutex
	imgKey string
	subKey string
	at     time.Time
}

// wbiKeys returns the keys the nav API hands out, fetching them when the
// hour is up. The nav says -101 to a listener without an account, but the
// keys come with it, so only they are read.
func wbiKeys(ctx context.Context) (string, string, error) {
	wbiCache.mu.Lock()
	defer wbiCache.mu.Unlock()
	if wbiCache.imgKey != "" && time.Since(wbiCache.at) < time.Hour {
		return wbiCache.imgKey, wbiCache.subKey, nil
	}
	var res struct {
		Code int `json:"code"`
		Data struct {
			WbiImg struct {
				ImgURL string `json:"img_url"`
				SubURL string `json:"sub_url"`
			} `json:"wbi_img"`
		} `json:"data"`
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/x/web-interface/nav", nil)
	if err != nil {
		return "", "", err
	}
	body, _, err := request(req)
	if err != nil {
		return "", "", fmt.Errorf("B 站签名密钥请求失败：%w", err)
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return "", "", fmt.Errorf("B 站签名密钥响应异常：%w", err)
	}
	if res.Code == -352 || res.Code == -412 {
		return "", "", fmt.Errorf("%w：签名密钥请求被拦截（%d）", ErrRiskControl, res.Code)
	}
	imgKey, subKey := keyFromURL(res.Data.WbiImg.ImgURL), keyFromURL(res.Data.WbiImg.SubURL)
	if len(imgKey)+len(subKey) < 64 {
		return "", "", errors.New("B 站返回的签名密钥无效")
	}
	wbiCache.imgKey, wbiCache.subKey, wbiCache.at = imgKey, subKey, time.Now()
	return imgKey, subKey, nil
}

// keyFromURL is the image's file name without its extension.
func keyFromURL(u string) string {
	name := path.Base(strings.SplitN(u, "?", 2)[0])
	return strings.TrimSuffix(name, path.Ext(name))
}

// mixinKey mixes the two keys together, as B 站's table says.
func mixinKey(imgKey, subKey string) string {
	orig := imgKey + subKey
	key := make([]byte, 32)
	for i := range key {
		key[i] = orig[mixinKeyEncTab[i]]
	}
	return string(key)
}

// filterWbi drops the characters the signature leaves out of values.
func filterWbi(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune("!'()*", r) {
			return -1
		}
		return r
	}, s)
}

// wbiRid signs params with the mixin key of imgKey and subKey: the
// params, sorted with their values filtered and wts set, then MD5'd with
// the mixin key.
func wbiRid(params url.Values, imgKey, subKey string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(url.QueryEscape(filterWbi(params.Get(k))))
	}
	sum := md5.Sum([]byte(b.String() + mixinKey(imgKey, subKey)))
	return hex.EncodeToString(sum[:])
}

// wbiSigned stamps params with wts and w_rid, as a wbi call must carry.
func wbiSigned(ctx context.Context, params url.Values) (url.Values, error) {
	imgKey, subKey, err := wbiKeys(ctx)
	if err != nil {
		return nil, err
	}
	params.Set("wts", strconv.FormatInt(time.Now().Unix(), 10))
	for _, vs := range params {
		for i, v := range vs {
			vs[i] = filterWbi(v)
		}
	}
	params.Set("w_rid", wbiRid(params, imgKey, subKey))
	return params, nil
}

// Search returns the videos B 站 finds for query, by its overall
// ranking, one page of 20 at a time.
func Search(ctx context.Context, query string, page int) ([]Video, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("搜索词不能为空")
	}
	if page < 1 {
		page = 1
	}
	q := url.Values{
		"search_type": {"video"},
		"keyword":     {query},
		"order":       {"totalrank"},
		"page":        {strconv.Itoa(page)},
		"page_size":   {"20"},
	}
	signed, err := wbiSigned(ctx, q)
	if err != nil {
		return nil, err
	}
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Result []struct {
				Bvid     string `json:"bvid"`
				Aid      int64  `json:"aid"`
				Title    string `json:"title"`
				Author   string `json:"author"`
				Mid      int64  `json:"mid"`
				Pic      string `json:"pic"`
				Duration string `json:"duration"`
				Pubdate  int64  `json:"pubdate"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := api(ctx, "/x/web-interface/wbi/search/type", signed, &res); err != nil {
		return nil, err
	}
	out := make([]Video, 0, len(res.Data.Result))
	for _, r := range res.Data.Result {
		out = append(out, Video{
			Bvid:     r.Bvid,
			Aid:      r.Aid,
			Title:    stripHTML(r.Title),
			Up:       r.Author,
			UpMid:    r.Mid,
			Cover:    picURL(r.Pic),
			Duration: parseDuration(r.Duration),
			PubDate:  r.Pubdate,
		})
	}
	return out, nil
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// stripHTML takes the tags and entities out of a search title, which
// B 站 marks the keyword up in.
func stripHTML(s string) string {
	return html.UnescapeString(tagRe.ReplaceAllString(s, ""))
}

// parseDuration reads a duration as the API writes it, "mm:ss" with
// minutes past 59, or "h:mm:ss".
func parseDuration(s string) time.Duration {
	var secs int64
	for _, part := range strings.Split(strings.TrimSpace(s), ":") {
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return 0
		}
		secs = secs*60 + n
	}
	return time.Duration(secs) * time.Second
}

// picURL is a picture's address in full, as the image CDN serves it.
func picURL(p string) string {
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "//") {
		p = "https:" + p
	}
	return strings.Replace(p, "http://", "https://", 1)
}

var (
	bvRe   = regexp.MustCompile(`BV[0-9A-Za-z]{10}`)
	avRe   = regexp.MustCompile(`(?i)\bav(\d+)\b`)
	pageRe = regexp.MustCompile(`[?&]p=(\d+)`)
)

// Resolve returns the video input points at: a BV or av id, a video page
// link, or a b23.tv short link, followed to its page. A page of the
// video is chosen with ?p=, whose CID then plays.
func Resolve(ctx context.Context, input string) (Video, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Video{}, errors.New("请输入 BV 号、av 号或 B 站视频链接")
	}
	p := pageOf(input)
	target := input
	if strings.Contains(input, "b23.tv") {
		var err error
		if target, err = followShortLink(ctx, input); err != nil {
			return Video{}, err
		}
		if p == 0 {
			p = pageOf(target)
		}
	}
	bvid := bvRe.FindString(target)
	var aid int64
	if m := avRe.FindStringSubmatch(target); m != nil {
		aid, _ = strconv.ParseInt(m[1], 10, 64)
	}
	if bvid == "" && aid == 0 && !strings.Contains(target, "bilibili.com") {
		if n, err := strconv.ParseInt(target, 10, 64); err == nil && n > 0 {
			aid = n
		}
	}
	if bvid == "" && aid == 0 {
		if strings.Contains(target, "bilibili.com") {
			return Video{}, errors.New("暂不支持该链接（仅支持视频页）")
		}
		return Video{}, errors.New("未识别出 BV 号或 av 号")
	}
	q := url.Values{}
	if bvid != "" {
		q.Set("bvid", bvid)
	} else {
		q.Set("aid", strconv.FormatInt(aid, 10))
	}
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Bvid     string `json:"bvid"`
			Aid      int64  `json:"aid"`
			CID      int64  `json:"cid"`
			Title    string `json:"title"`
			Pic      string `json:"pic"`
			Duration int64  `json:"duration"`
			Pubdate  int64  `json:"pubdate"`
			Owner    struct {
				Mid  int64  `json:"mid"`
				Name string `json:"name"`
			} `json:"owner"`
			Pages []struct {
				CID  int64 `json:"cid"`
				Page int   `json:"page"`
			} `json:"pages"`
		} `json:"data"`
	}
	if err := api(ctx, "/x/web-interface/view", q, &res); err != nil {
		return Video{}, err
	}
	d := res.Data
	cid := d.CID
	if p > 0 {
		if p > len(d.Pages) {
			return Video{}, fmt.Errorf("该视频没有第 %d P（共 %d P）", p, len(d.Pages))
		}
		cid = d.Pages[p-1].CID
	}
	return Video{
		Bvid:     d.Bvid,
		Aid:      d.Aid,
		CID:      cid,
		Title:    d.Title,
		Up:       d.Owner.Name,
		UpMid:    d.Owner.Mid,
		Cover:    picURL(d.Pic),
		Duration: time.Duration(d.Duration) * time.Second,
		PubDate:  d.Pubdate,
	}, nil
}

// pageOf is the page a link's ?p= asks for, or none.
func pageOf(s string) int {
	if m := pageRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

var videoLinkRe = regexp.MustCompile(`(?:https?:)?//[a-z0-9.-]*bilibili\.com/video/(BV[0-9A-Za-z]{10}|[aA][vV]\d+)`)

// videoLink finds a video page link in text, as a short link's page may
// forward in script rather than answer with a redirect.
func videoLink(text string) string {
	if m := videoLinkRe.FindStringSubmatch(text); m != nil {
		return wwwBase + "/video/" + m[1]
	}
	return ""
}

// followShortLink walks a b23.tv link to the video page it points at.
func followShortLink(ctx context.Context, u string) (string, error) {
	client := *httpClient
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	cur := u
	for range 3 {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, cur, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", UserAgent)
		req.Header.Set("Referer", wwwBase+"/")
		res, err := client.Do(req)
		if err != nil {
			return "", err
		}
		loc := res.Header.Get("Location")
		if loc == "" {
			body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
			res.Body.Close()
			if target := videoLink(string(body)); target != "" {
				return target, nil
			}
			return "", errors.New("短链接没有跳转到视频页")
		}
		res.Body.Close()
		base, err := url.Parse(cur)
		if err != nil {
			return "", err
		}
		next, err := url.Parse(loc)
		if err != nil {
			return "", err
		}
		cur = base.ResolveReference(next).String()
		if strings.Contains(cur, "/video/") {
			return cur, nil
		}
	}
	return "", errors.New("短链接跳转次数过多")
}

// dashAudio is one audio track of the DASH stream.
type dashAudio struct {
	ID             int64    `json:"id"`
	BaseURL        string   `json:"baseUrl"`
	BaseURLSnake   string   `json:"base_url"`
	BackupURL      []string `json:"backupUrl"`
	BackupURLSnake []string `json:"backup_url"`
	Bandwidth      int64    `json:"bandwidth"`
	MimeType       string   `json:"mimeType"`
	MimeTypeSnake  string   `json:"mime_type"`
	Codecid        int      `json:"codecid"`
}

func (a dashAudio) url() string {
	if a.BaseURL != "" {
		return a.BaseURL
	}
	if a.BaseURLSnake != "" {
		return a.BaseURLSnake
	}
	if len(a.BackupURL) > 0 {
		return a.BackupURL[0]
	}
	if len(a.BackupURLSnake) > 0 {
		return a.BackupURLSnake[0]
	}
	return ""
}

func (a dashAudio) mime() string {
	if a.MimeType != "" {
		return a.MimeType
	}
	return a.MimeTypeSnake
}

// isAAC reports whether the track is AAC, which the player prefers.
func isAAC(mime string, codecid int) bool {
	if codecid == 0 {
		return true
	}
	m := strings.ToLower(mime)
	return strings.Contains(m, "mp4a") || strings.Contains(m, "aac") || strings.Contains(m, "m4a")
}

// audioType says what the track is: an .m4s stream is m4a, as the player
// calls it.
func audioType(mime, u string) string {
	m := strings.ToLower(mime)
	u = strings.ToLower(u)
	switch {
	case strings.Contains(m, "flac"):
		return "flac"
	case strings.Contains(m, "mp3") || strings.HasSuffix(u, ".mp3"):
		return "mp3"
	}
	return "m4a"
}

// durl is one whole stream of the old kind, audio and video together.
type durl struct {
	Order     int      `json:"order"`
	Length    int64    `json:"length"` // milliseconds
	Size      int64    `json:"size"`
	URL       string   `json:"url"`
	BackupURL []string `json:"backup_url"`
}

func (d durl) url() string {
	if d.URL != "" {
		return d.URL
	}
	if len(d.BackupURL) > 0 {
		return d.BackupURL[0]
	}
	return ""
}

// PlayURL returns where v's audio streams from: the DASH track of the
// highest bit rate in AAC, or the old stream whole. The CDN only answers
// to the headers Audio carries.
func PlayURL(ctx context.Context, v Video) (Audio, error) {
	if v.Bvid == "" && v.Aid == 0 {
		return Audio{}, errors.New("视频缺少 BV 号或 av 号")
	}
	// A search's results carry no CID: the video's page says it, with the
	// duration a ranking may leave out.
	if v.CID == 0 {
		id := v.Bvid
		if id == "" {
			id = strconv.FormatInt(v.Aid, 10)
		}
		got, err := Resolve(ctx, id)
		if err != nil {
			return Audio{}, err
		}
		v.CID = got.CID
		if v.Duration == 0 {
			v.Duration = got.Duration
		}
	}
	q := url.Values{
		"fnval": {"4048"},
		"fnver": {"0"},
		"fourk": {"1"},
		"cid":   {strconv.FormatInt(v.CID, 10)},
	}
	if v.Bvid != "" {
		q.Set("bvid", v.Bvid)
	} else {
		q.Set("aid", strconv.FormatInt(v.Aid, 10))
	}
	signed, err := wbiSigned(ctx, q)
	if err != nil {
		return Audio{}, err
	}
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Result     string `json:"result"`
			TimeLength int64  `json:"timelength"` // milliseconds
			Dash       struct {
				Duration int64       `json:"duration"` // seconds
				Audio    []dashAudio `json:"audio"`
			} `json:"dash"`
			Durl []durl `json:"durl"`
		} `json:"data"`
	}
	if err := api(ctx, "/x/player/wbi/playurl", signed, &res); err != nil {
		return Audio{}, err
	}
	if res.Data.Result == "lmt" {
		return Audio{}, errors.New("该视频受地区限制，当前网络无法播放")
	}
	headers := map[string]string{
		"Referer":    wwwBase + "/",
		"User-Agent": UserAgent,
	}
	if len(res.Data.Dash.Audio) > 0 {
		best, bestAAC := 0, -1
		for i, a := range res.Data.Dash.Audio {
			if a.Bandwidth > res.Data.Dash.Audio[best].Bandwidth {
				best = i
			}
			if isAAC(a.mime(), a.Codecid) && (bestAAC < 0 || a.Bandwidth > res.Data.Dash.Audio[bestAAC].Bandwidth) {
				bestAAC = i
			}
		}
		if bestAAC < 0 {
			bestAAC = best
		}
		a := res.Data.Dash.Audio[bestAAC]
		if u := a.url(); u != "" {
			duration := v.Duration
			if res.Data.Dash.Duration > 0 {
				duration = time.Duration(res.Data.Dash.Duration) * time.Second
			}
			return Audio{
				URL:      strings.Replace(u, "http://", "https://", 1),
				Duration: duration,
				Type:     audioType(a.mime(), u),
				Headers:  headers,
			}, nil
		}
	}
	if len(res.Data.Durl) > 0 {
		d := res.Data.Durl[0]
		if u := d.url(); u != "" {
			duration := v.Duration
			if d.Length > 0 {
				duration = time.Duration(d.Length) * time.Millisecond
			}
			return Audio{
				URL:      strings.Replace(u, "http://", "https://", 1),
				Size:     d.Size,
				Duration: duration,
				Type:     audioType("", u),
				Headers:  headers,
			}, nil
		}
	}
	return Audio{}, errors.New("B 站未返回该视频的音源，可能受地区限制或需要登录")
}

// Ranking returns the videos of chart rid, the music region being 3, as
// B 站 ranks them today.
func Ranking(ctx context.Context, rid int) ([]Video, error) {
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			List []struct {
				Bvid     string `json:"bvid"`
				Aid      int64  `json:"aid"`
				CID      int64  `json:"cid"`
				Title    string `json:"title"`
				Pic      string `json:"pic"`
				Duration int64  `json:"duration"`
				Pubdate  int64  `json:"pubdate"`
				Owner    struct {
					Mid  int64  `json:"mid"`
					Name string `json:"name"`
				} `json:"owner"`
			} `json:"list"`
		} `json:"data"`
	}
	q := url.Values{"rid": {strconv.Itoa(rid)}, "type": {"all"}}
	if err := api(ctx, "/x/web-interface/ranking/v2", q, &res); err != nil {
		return nil, err
	}
	out := make([]Video, 0, len(res.Data.List))
	for _, l := range res.Data.List {
		out = append(out, Video{
			Bvid:     l.Bvid,
			Aid:      l.Aid,
			CID:      l.CID,
			Title:    l.Title,
			Up:       l.Owner.Name,
			UpMid:    l.Owner.Mid,
			Cover:    picURL(l.Pic),
			Duration: time.Duration(l.Duration) * time.Second,
			PubDate:  l.Pubdate,
		})
	}
	return out, nil
}

// subtitle is one subtitle track the player offers.
type subtitle struct {
	Lan         string `json:"lan"`
	LanDoc      string `json:"lan_doc"`
	SubtitleURL string `json:"subtitle_url"`
}

// Subtitles returns v's subtitles as LRC, the Chinese track preferred,
// and nothing where B 站 has none.
func Subtitles(ctx context.Context, v Video) (string, error) {
	if v.Bvid == "" && v.Aid == 0 {
		return "", errors.New("视频缺少 BV 号或 av 号")
	}
	if v.CID == 0 {
		return "", errors.New("视频缺少 CID")
	}
	q := url.Values{"cid": {strconv.FormatInt(v.CID, 10)}}
	if v.Bvid != "" {
		q.Set("bvid", v.Bvid)
	} else {
		q.Set("aid", strconv.FormatInt(v.Aid, 10))
	}
	signed, err := wbiSigned(ctx, q)
	if err != nil {
		return "", err
	}
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Subtitle struct {
				Subtitles []subtitle `json:"subtitles"`
			} `json:"subtitle"`
		} `json:"data"`
	}
	if err := api(ctx, "/x/player/wbi/v2", signed, &res); err != nil {
		return "", err
	}
	subs := res.Data.Subtitle.Subtitles
	// The Chinese track first, then any, where B 站 gives its address.
	pick := func(zh bool) string {
		for _, s := range subs {
			if s.SubtitleURL == "" {
				continue
			}
			if zh && !strings.HasPrefix(s.Lan, "zh") {
				continue
			}
			return s.SubtitleURL
		}
		return ""
	}
	u := pick(true)
	if u == "" {
		u = pick(false)
	}
	if u == "" {
		return "", nil
	}
	if strings.HasPrefix(u, "//") {
		u = "https:" + u
	}
	body, err := fetchBytes(ctx, u)
	if err != nil {
		return "", err
	}
	return subtitlesToLRC(body)
}

// subLine is one line of B 站's subtitle file, a JSON object per line.
type subLine struct {
	From    float64 `json:"from"` // seconds
	Content string  `json:"content"`
}

// subtitlesToLRC turns B 站's JSON lines into LRC, one timestamped line
// each, to the hundredth of a second.
func subtitlesToLRC(data []byte) (string, error) {
	var out strings.Builder
	for _, raw := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var l subLine
		if err := json.Unmarshal([]byte(raw), &l); err != nil {
			continue
		}
		ms := int(l.From*1000 + 0.5)
		cs := ((ms % 1000) + 5) / 10
		total := ms/1000 + cs/100
		cs %= 100
		fmt.Fprintf(&out, "[%02d:%02d.%02d]%s\n", total/60, total%60, cs, l.Content)
	}
	return out.String(), nil
}

// A User is an uploader of the site's, as a search finds them.
type User struct {
	Mid  int64
	Name string
	Face string
	Sign string
	Fans int64
	// Latest are the newest videos the search's answer carries, up to
	// three: the site lists no uploader's videos to a listener who is not
	// signed in, and this is what it does show.
	Latest []Video
}

// SearchUsers searches the site's uploaders by name.
func SearchUsers(ctx context.Context, query string, page int) ([]User, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("搜索词不能为空")
	}
	if page < 1 {
		page = 1
	}
	q := url.Values{
		"search_type": {"bili_user"},
		"keyword":     {query},
		"page":        {strconv.Itoa(page)},
		"page_size":   {"20"},
	}
	signed, err := wbiSigned(ctx, q)
	if err != nil {
		return nil, err
	}
	var res struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Result []struct {
				Mid   int64  `json:"mid"`
				Uname string `json:"uname"`
				Uface string `json:"upic"`
				Usign string `json:"usign"`
				Fans  int64  `json:"fans"`
				Res   []struct {
					Aid      int64  `json:"aid"`
					Bvid     string `json:"bvid"`
					Title    string `json:"title"`
					Pic      string `json:"pic"`
					Pubdate  int64  `json:"pubdate"`
					Duration string `json:"duration"`
				} `json:"res"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := api(ctx, "/x/web-interface/wbi/search/type", signed, &res); err != nil {
		return nil, err
	}
	out := make([]User, 0, len(res.Data.Result))
	for _, r := range res.Data.Result {
		u := User{
			Mid:  r.Mid,
			Name: stripHTML(r.Uname),
			Face: picURL(r.Uface),
			Sign: stripHTML(r.Usign),
			Fans: r.Fans,
		}
		for _, v := range r.Res {
			u.Latest = append(u.Latest, Video{
				Bvid:     v.Bvid,
				Aid:      v.Aid,
				Title:    stripHTML(v.Title),
				Up:       u.Name,
				UpMid:    u.Mid,
				Cover:    picURL(v.Pic),
				Duration: parseDuration(v.Duration),
				PubDate:  v.Pubdate,
			})
		}
		out = append(out, u)
	}
	return out, nil
}

