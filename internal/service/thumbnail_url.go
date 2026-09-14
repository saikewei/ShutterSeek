package service

import "strconv"

// ThumbnailURL is the public path of a photo's thumbnail.
//
// Kept in one place because the route is authenticated now: if the prefix
// drifts in any one of the emitters, that caller starts handing out URLs that
// 404 (or, worse, a stale public path).
func ThumbnailURL(id int64) string {
	return "/api/v1/thumbnails/" + strconv.FormatInt(id, 10) + ".webp"
}
