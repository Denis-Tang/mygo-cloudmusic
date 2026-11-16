package bilibili

// The sign-in of the site's web player. The listener scans a code with the
// B 站 app, and the cookie it leaves is kept on this machine alone: the
// account is read, never written to.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// passportBase is where the site signs a listener in.
var passportBase = "https://passport.bilibili.com"

// session is the cookie signed-in calls carry, if any.
var session struct {
	mu     sync.Mutex
	cookie string
}

// SetSession sets the cookie signed-in calls carry, or clears it with "".
func SetSession(cookie string) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.cookie = cookie
}

// SessionCookie is the cookie signed-in calls carry, or "".
func SessionCookie() string {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.cookie
}

// A Login is a sign-in waiting to be scanned: the code's address as the
// site draws it, and the key that asks whether it was scanned.
type Login struct {
	URL string
	Key string
}

// QRCode starts a sign-in.
func QRCode(ctx context.Context) (Login, error) {
	body, _, err := passportCall(ctx, "/x/passport-login/web/qrcode/generate", nil)
	if err != nil {
		return Login{}, err
	}
	var res struct {
		Data struct {
			URL string `json:"url"`
			Key string `json:"qrcode_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return Login{}, err
	}
	if res.Data.URL == "" || res.Data.Key == "" {
		return Login{}, errors.New("B 站没有给出登录二维码")
	}
	return Login{URL: res.Data.URL, Key: res.Data.Key}, nil
}

// A QRCodeState is how far a sign-in has come.
type QRCodeState int

const (
	QRWaiting QRCodeState = iota
	QRScanned
	QRConfirmed
	QRExpired
)

// PollLogin asks whether the code was scanned. On QRConfirmed it returns
// the cookie the account signs in with.
func PollLogin(ctx context.Context, key string) (QRCodeState, string, error) {
	body, h, err := passportCall(ctx, "/x/passport-login/web/qrcode/poll", url.Values{"qrcode_key": {key}})
	if err != nil {
		return QRWaiting, "", err
	}
	var res struct {
		Data struct {
			URL  string `json:"url"`
			Code int    `json:"code"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return QRWaiting, "", err
	}
	switch res.Data.Code {
	case 0:
		cookie := sessionFrom(h, res.Data.URL)
		if cookie == "" {
			return QRConfirmed, "", errors.New("登录成功但没有拿到 B 站的凭据")
		}
		return QRConfirmed, cookie, nil
	case 86090:
		return QRScanned, "", nil
	case 86038:
		return QRExpired, "", nil
	default:
		// 86101: not scanned yet.
		return QRWaiting, "", nil
	}
}

// passportCall calls the sign-in API, with the headers the site wants,
// and returns the body and its headers as they came.
func passportCall(ctx context.Context, apiPath string, query url.Values) ([]byte, http.Header, error) {
	u := passportBase + apiPath
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Referer", wwwBase+"/")
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, res.Header, fmt.Errorf("B 站登录接口异常（HTTP %d）", res.StatusCode)
	}
	return body, res.Header, nil
}

// sessionFrom is the cookie a signed-in account needs, from the headers
// the site set, or from the address it answered with.
func sessionFrom(h http.Header, u string) string {
	want := []string{"SESSDATA", "bili_jct", "DedeUserID", "DedeUserID__ckMd5", "sid"}
	seen := map[string]bool{}
	var parts []string
	for _, c := range h.Values("Set-Cookie") {
		k, v, ok := strings.Cut(c, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if !wanted(k, want) || seen[k] {
			continue
		}
		v, _, _ = strings.Cut(v, ";")
		parts = append(parts, k+"="+v)
		seen[k] = true
	}
	if len(parts) == 0 && u != "" {
		if pu, err := url.Parse(u); err == nil {
			q := pu.Query()
			for _, k := range want {
				if v := q.Get(k); v != "" {
					parts = append(parts, k+"="+v)
				}
			}
		}
	}
	return strings.Join(parts, "; ")
}

func wanted(k string, list []string) bool {
	for _, w := range list {
		if k == w {
			return true
		}
	}
	return false
}

// An Account is the listener signed in, as the site names them.
type Account struct {
	Mid  int64
	Name string
	Face string
}

// Nav returns the account the session belongs to.
func Nav(ctx context.Context) (Account, error) {
	if SessionCookie() == "" {
		return Account{}, errors.New("还没有登录 B 站")
	}
	var res struct {
		Data struct {
			IsLogin bool   `json:"isLogin"`
			Mid     int64  `json:"mid"`
			Name    string `json:"uname"`
			Face    string `json:"face"`
		} `json:"data"`
	}
	if err := api(ctx, "/x/web-interface/nav", nil, &res); err != nil {
		return Account{}, err
	}
	if !res.Data.IsLogin {
		return Account{}, errors.New("B 站登录已失效，请重新扫码")
	}
	return Account{Mid: res.Data.Mid, Name: res.Data.Name, Face: picURL(res.Data.Face)}, nil
}

// A FavFolder is one of the account's collections of videos.
type FavFolder struct {
	ID    int64
	Name  string
	Count int
	Cover string
}

// FavFolders returns the collections the account made.
func FavFolders(ctx context.Context, mid int64) ([]FavFolder, error) {
	var res struct {
		Data struct {
			List []struct {
				ID         int64  `json:"id"`
				Title      string `json:"title"`
				Cover      string `json:"cover"`
				MediaCount int    `json:"media_count"`
			} `json:"list"`
		} `json:"data"`
	}
	q := url.Values{"up_mid": {strconv.FormatInt(mid, 10)}}
	if err := api(ctx, "/x/v3/fav/folder/created/list-all", q, &res); err != nil {
		return nil, err
	}
	out := make([]FavFolder, 0, len(res.Data.List))
	for _, f := range res.Data.List {
		out = append(out, FavFolder{ID: f.ID, Name: f.Title, Count: f.MediaCount, Cover: picURL(f.Cover)})
	}
	return out, nil
}

// A FavVideo is one video of a collection.
type FavVideo struct {
	Bvid     string
	Aid      int64
	Title    string
	Cover    string
	UpMid    int64
	Up       string
	Duration time.Duration
	PubDate  int64
}

// FavItems returns one page of a collection's videos, and whether more
// follow.
func FavItems(ctx context.Context, mediaID int64, page int) ([]FavVideo, bool, error) {
	q := url.Values{
		"media_id": {strconv.FormatInt(mediaID, 10)},
		"pn":       {strconv.Itoa(page)},
		"ps":       {"20"},
		"platform": {"web"},
	}
	var res struct {
		Data struct {
			Medias []struct {
				ID       int64  `json:"id"`
				Title    string `json:"title"`
				Cover    string `json:"cover"`
				Bvid     string `json:"bvid"`
				Duration int64  `json:"duration"`
				PubTime  int64  `json:"pubtime"`
				Attr     int    `json:"attr"`
				Upper    struct {
					Mid  int64  `json:"mid"`
					Name string `json:"name"`
				} `json:"upper"`
			} `json:"medias"`
			HasMore bool `json:"has_more"`
		} `json:"data"`
	}
	if err := api(ctx, "/x/v3/fav/resource/list", q, &res); err != nil {
		return nil, false, err
	}
	out := make([]FavVideo, 0, len(res.Data.Medias))
	for _, m := range res.Data.Medias {
		// A video gone or private has no BV number, and cannot play.
		if m.Bvid == "" || m.Attr != 0 {
			continue
		}
		out = append(out, FavVideo{
			Bvid:     m.Bvid,
			Aid:      m.ID,
			Title:    m.Title,
			Cover:    picURL(m.Cover),
			UpMid:    m.Upper.Mid,
			Up:       m.Upper.Name,
			Duration: time.Duration(max(m.Duration, 0)) * time.Second,
			PubDate:  m.PubTime,
		})
	}
	return out, res.Data.HasMore, nil
}

// Followings returns one page of the uploaders the account follows, and
// whether more follow.
func Followings(ctx context.Context, mid int64, page int) ([]User, bool, error) {
	q := url.Values{
		"vmid":  {strconv.FormatInt(mid, 10)},
		"pn":    {strconv.Itoa(page)},
		"ps":    {"50"},
		"order": {"desc"},
	}
	var res struct {
		Data struct {
			List []struct {
				Mid  int64  `json:"mid"`
				Name string `json:"uname"`
				Face string `json:"face"`
				Sign string `json:"sign"`
			} `json:"list"`
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := api(ctx, "/x/relation/followings", q, &res); err != nil {
		return nil, false, err
	}
	out := make([]User, 0, len(res.Data.List))
	for _, u := range res.Data.List {
		out = append(out, User{Mid: u.Mid, Name: u.Name, Face: picURL(u.Face), Sign: u.Sign})
	}
	return out, len(out) >= 50, nil
}
