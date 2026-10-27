package netease

import (
	"strconv"
)

// AlbumPublishTime returns the album's publish time, in Unix milliseconds:
// the /api/v1/album/{id} detail, reduced to the time.
func AlbumPublishTime(id int64) (int64, error) {
	var res struct {
		Album struct {
			PublishTime int64 `json:"publishTime"`
		} `json:"album"`
	}
	if err := get("/api/v1/album/"+strconv.FormatInt(id, 10), nil, &res); err != nil {
		return 0, err
	}
	return res.Album.PublishTime, nil
}
