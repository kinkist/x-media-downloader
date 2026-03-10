package twitterscraper

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// GetLikes returns a channel with the authenticated user's liked tweets.
func (s *Scraper) GetLikes(ctx context.Context, maxTweetsNbr int) <-chan *TweetResult {
	return getTweetTimeline(ctx, "", maxTweetsNbr, func(unused string, maxTweetsNbr int, cursor string) ([]*Tweet, string, error) {
		return s.FetchLikes(maxTweetsNbr, cursor)
	})
}

// FetchLikes gets liked tweets via the Twitter frontend GraphQL API.
// userId is extracted automatically from the `twid` cookie set during login.
func (s *Scraper) FetchLikes(maxTweetsNbr int, cursor string) ([]*Tweet, string, error) {
	if maxTweetsNbr > 200 {
		maxTweetsNbr = 200
	}

	// Extract userId from the `twid` cookie.
	// Value may be "u=12345" or URL-encoded "u%3D12345" — decode first, then strip prefix.
	userId := ""
	for _, c := range s.client.Jar.Cookies(twURL) {
		if c.Name == "twid" {
			decodedValue, err := url.QueryUnescape(c.Value)
			if err != nil {
				userId = strings.TrimPrefix(c.Value, "u=")
			} else {
				userId = strings.TrimPrefix(decodedValue, "u=")
			}
			break
		}
	}
	if userId == "" {
		return nil, "", fmt.Errorf("likes: userId not found in cookies (twid cookie missing or empty)")
	}

	req, err := s.newRequest("GET", "https://x.com/i/api/graphql/j-O2fOmYBTqofGfn6LMb8g/Likes")
	if err != nil {
		return nil, "", err
	}

	// variables from HAR: {"userId":"281817835","count":20,"includePromotedContent":false,
	//                      "withClientEventToken":false,"withBirdwatchNotes":false,"withVoice":true}
	variables := map[string]interface{}{
		"userId":                  userId,
		"count":                   maxTweetsNbr,
		"includePromotedContent":  false,
		"withClientEventToken":    false,
		"withBirdwatchNotes":      false,
		"withVoice":               true,
	}
	// features from HAR capture (x.com-202603041556.har)
	features := map[string]interface{}{
		"rweb_video_screen_enabled":                                               false,
		"profile_label_improvements_pcf_label_in_post_enabled":                   true,
		"responsive_web_profile_redirect_enabled":                                 false,
		"rweb_tipjar_consumption_enabled":                                         false,
		"verified_phone_label_enabled":                                            false,
		"creator_subscriptions_tweet_preview_api_enabled":                         true,
		"responsive_web_graphql_timeline_navigation_enabled":                      true,
		"responsive_web_graphql_skip_user_profile_image_extensions_enabled":       false,
		"premium_content_api_read_enabled":                                        false,
		"communities_web_enable_tweet_community_results_fetch":                    true,
		"c9s_tweet_anatomy_moderator_badge_enabled":                               true,
		"responsive_web_grok_analyze_button_fetch_trends_enabled":                 false,
		"responsive_web_grok_analyze_post_followups_enabled":                      true,
		"responsive_web_jetfuel_frame":                                            true,
		"responsive_web_grok_share_attachment_enabled":                            true,
		"responsive_web_grok_annotations_enabled":                                 true,
		"articles_preview_enabled":                                                true,
		"responsive_web_edit_tweet_api_enabled":                                   true,
		"graphql_is_translatable_rweb_tweet_is_translatable_enabled":              true,
		"view_counts_everywhere_api_enabled":                                      true,
		"longform_notetweets_consumption_enabled":                                 true,
		"responsive_web_twitter_article_tweet_consumption_enabled":                true,
		"tweet_awards_web_tipping_enabled":                                        false,
		"content_disclosure_indicator_enabled":                                    true,
		"content_disclosure_ai_generated_indicator_enabled":                       true,
		"responsive_web_grok_show_grok_translated_post":                           false,
		"responsive_web_grok_analysis_button_from_backend":                        true,
		"post_ctas_fetch_enabled":                                                 false,
		"freedom_of_speech_not_reach_fetch_enabled":                               true,
		"standardized_nudges_misinfo":                                             true,
		"tweet_with_visibility_results_prefer_gql_limited_actions_policy_enabled": true,
		"longform_notetweets_rich_text_read_enabled":                              true,
		"longform_notetweets_inline_media_enabled":                                false,
		"responsive_web_grok_image_annotation_enabled":                            true,
		"responsive_web_grok_imagine_annotation_enabled":                          true,
		"responsive_web_grok_community_note_auto_translation_is_enabled":          false,
		"responsive_web_enhance_cards_enabled":                                    false,
	}
	fieldToggles := map[string]interface{}{
		"withArticlePlainText": false,
	}

	if cursor != "" {
		variables["cursor"] = cursor
	}

	query := url.Values{}
	query.Set("variables", mapToJSONString(variables))
	query.Set("features", mapToJSONString(features))
	query.Set("fieldToggles", mapToJSONString(fieldToggles))
	req.URL.RawQuery = query.Encode()

	var timeline likesTimelineV2
	err = s.RequestAPI(req, &timeline)
	if err != nil {
		return nil, "", err
	}

	tweets, nextCursor := timeline.parseTweets()
	return tweets, nextCursor, nil
}
