package twitterscraper

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type scheduleTweet struct {
	RestID         string `json:"rest_id"`
	SchedulingInfo struct {
		ExecuteAt int64  `json:"execute_at"`
		State     string `json:"state"`
	} `json:"scheduling_info"`
	TweetCreateRequest struct {
		Type                      string        `json:"type"`
		Status                    string        `json:"status"`
		ExcludeReplyUserIds       []interface{} `json:"exclude_reply_user_ids"`
		MediaIds                  []interface{} `json:"media_ids"`
		AutoPopulateReplyMetadata bool          `json:"auto_populate_reply_metadata"`
	} `json:"tweet_create_request"`
	MediaEntities []struct {
		MediaKey  string `json:"media_key"`
		MediaInfo struct {
			Typename          string `json:"__typename"`
			OriginalImgURL    string `json:"original_img_url"`
			OriginalImgWidth  int    `json:"original_img_width"`
			OriginalImgHeight int    `json:"original_img_height"`
			DurationMillis    int    `json:"duration_millis"`
			Variants          []struct {
				ContentType string `json:"content_type"`
				Bitrate     int    `json:"bit_rate,omitempty"`
				URL         string `json:"url"`
			} `json:"variants"`
			AspectRatio struct {
				Numerator   int `json:"numerator"`
				Denominator int `json:"denominator"`
			} `json:"aspect_ratio"`
			PreviewImage struct {
				OriginalImgURL    string `json:"original_img_url"`
				OriginalImgWidth  int    `json:"original_img_width"`
				OriginalImgHeight int    `json:"original_img_height"`
			} `json:"preview_image"`
		} `json:"media_info"`
	} `json:"media_entities,omitempty"`
}

func (result *scheduleTweet) parse() *ScheduledTweet {
	tweet := &ScheduledTweet{
		ID:        result.RestID,
		State:     result.SchedulingInfo.State,
		ExecuteAt: time.Unix(result.SchedulingInfo.ExecuteAt/1000, 0),
		Text:      result.TweetCreateRequest.Status,
	}

	for _, media := range result.MediaEntities {
		k := strings.Split(media.MediaKey, "_")
		key := k[len(k)-1]

		if media.MediaInfo.Typename == "ApiVideo" {
			video := Video{
				ID:      key,
				Preview: media.MediaInfo.PreviewImage.OriginalImgURL,
			}

			maxBitrate := 0
			for _, variant := range media.MediaInfo.Variants {
				if variant.Bitrate > maxBitrate {
					video.URL = strings.TrimSuffix(variant.URL, "?tag=10")
					maxBitrate = variant.Bitrate
				}
			}

			tweet.Videos = append(tweet.Videos, video)
		} else if media.MediaInfo.Typename == "ApiGif" {
			gif := GIF{
				ID:      key,
				Preview: media.MediaInfo.PreviewImage.OriginalImgURL,
			}

			maxBitrate := 0
			for _, variant := range media.MediaInfo.Variants {
				if variant.Bitrate >= maxBitrate {
					gif.URL = variant.URL
					maxBitrate = variant.Bitrate
				}
			}
			tweet.GIFs = append(tweet.GIFs, gif)
		} else if media.MediaInfo.Typename == "ApiImage" {
			tweet.Photos = append(tweet.Photos, Photo{
				ID:  key,
				URL: media.MediaInfo.OriginalImgURL,
			})
		}
	}

	return tweet
}

type scheduleTweets struct {
	Data struct {
		Viewer struct {
			ScheduledTweetList []scheduleTweet `json:"scheduled_tweet_list"`
		} `json:"viewer"`
	} `json:"data"`
}

type TweetSchedule struct {
	Text   string
	Date   time.Time
	Medias []*Media
}

func (timeline *scheduleTweets) parseTweets() []*ScheduledTweet {
	var tweets []*ScheduledTweet

	for _, entry := range timeline.Data.Viewer.ScheduledTweetList {
		if tweet := entry.parse(); tweet != nil {
			tweets = append(tweets, tweet)
		}
	}
	return tweets
}

// FetchScheduledTweets gets scheduled tweets via the Twitter frontend GraphQL API.
func (s *Scraper) FetchScheduledTweets() ([]*ScheduledTweet, error) {
	req, err := s.newRequest("GET", "https://x.com/i/api/graphql/ITtjAzvlZni2wWXwf295Qg/FetchScheduledTweets")
	if err != nil {
		return nil, err
	}

	variables := map[string]interface{}{
		"ascending": true,
	}

	query := url.Values{}
	query.Set("variables", mapToJSONString(variables))
	req.URL.RawQuery = query.Encode()

	var timeline scheduleTweets
	err = s.RequestAPI(req, &timeline)
	if err != nil {
		return nil, err
	}

	tweets := timeline.parseTweets()
	return tweets, nil
}

// DeleteScheduledTweet removes tweet from scheduled.
func (s *Scraper) DeleteScheduledTweet(id string) error {
	req, err := s.newRequest("POST", "https://x.com/i/api/graphql/CTOVqej0JBXAZSwkp1US0g/DeleteScheduledTweet")
	if err != nil {
		return err
	}

	req.Header.Set("content-type", "application/json")

	variables := map[string]interface{}{
		"scheduled_tweet_id": id,
	}

	body := map[string]interface{}{
		"variables": variables,
		"queryId":   "CTOVqej0JBXAZSwkp1US0g",
	}

	b, _ := json.Marshal(body)
	req.Body = io.NopCloser(bytes.NewReader(b))

	var response struct {
		Data struct {
			Status string `json:"scheduledtweet_delete"`
		} `json:"data"`
	}

	err = s.RequestAPI(req, &response)
	if err != nil {
		return err
	}

	if response.Data.Status == "Done" {
		return nil
	}

	return errors.New("scheduled tweet wasn't removed")
}

// CreateScheduledTweet schedule new tweet.
func (s *Scraper) CreateScheduledTweet(schedule TweetSchedule) (string, error) {
	if schedule.Date.Unix() <= time.Now().Unix() {
		return "", errors.New("date can't be in past")
	}

	req, err := s.newRequest("POST", "https://x.com/i/api/graphql/LCVzRQGxOaGnOnYH01NQXg/CreateScheduledTweet")
	if err != nil {
		return "", err
	}

	req.Header.Set("content-type", "application/json")

	post_tweet_request := map[string]interface{}{
		"auto_populate_reply_metadata": false,
		"status":                       schedule.Text,
		"exclude_reply_user_ids":       []string{},
		"media_ids":                    []string{},
	}

	if len(schedule.Medias) > 0 {
		var media_ids []string

		for _, media := range schedule.Medias {
			media_ids = append(media_ids, strconv.Itoa(media.ID))
		}

		post_tweet_request["media_ids"] = media_ids
	}

	variables := map[string]interface{}{
		"post_tweet_request": post_tweet_request,
		"execute_at":         schedule.Date.Unix(),
	}

	body := map[string]interface{}{
		"variables": variables,
		"queryId":   "LCVzRQGxOaGnOnYH01NQXg",
	}

	b, _ := json.Marshal(body)
	req.Body = io.NopCloser(bytes.NewReader(b))

	var response struct {
		Data struct {
			Tweet struct {
				ID string `json:"rest_id"`
			} `json:"tweet"`
		} `json:"data"`
	}

	err = s.RequestAPI(req, &response)
	if err != nil {
		return "", err
	}

	if response.Data.Tweet.ID != "" {
		return response.Data.Tweet.ID, nil
	}

	return "", errors.New("tweet wasn't scheduled")
}
