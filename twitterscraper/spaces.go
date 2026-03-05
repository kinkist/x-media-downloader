package twitterscraper

import (
	"errors"
	"net/url"
	"time"
)

func (s *Scraper) GetSpace(id string) (*Space, error) {
	if !s.isLogged {
		return nil, errors.New("scraper is not logged in")
	}

	req, err := s.newRequest("GET", "https://x.com/i/api/graphql/d03OdorPdZ_sH9V3D1_yWQ/AudioSpaceById")
	if err != nil {
		return nil, err
	}

	variables := map[string]interface{}{
		"id":              id,
		"isMetatagsQuery": false,
		"withReplays":     true,
		"withListeners":   true,
	}

	features := map[string]interface{}{
		"spaces_2022_h2_spaces_communities":                                       true,
		"spaces_2022_h2_clipping":                                                 true,
		"creator_subscriptions_tweet_preview_api_enabled":                         true,
		"rweb_tipjar_consumption_enabled":                                         true,
		"responsive_web_graphql_exclude_directive_enabled":                        true,
		"verified_phone_label_enabled":                                            false,
		"communities_web_enable_tweet_community_results_fetch":                    true,
		"c9s_tweet_anatomy_moderator_badge_enabled":                               true,
		"articles_preview_enabled":                                                true,
		"responsive_web_graphql_skip_user_profile_image_extensions_enabled":       false,
		"tweetypie_unmention_optimization_enabled":                                true,
		"responsive_web_edit_tweet_api_enabled":                                   true,
		"graphql_is_translatable_rweb_tweet_is_translatable_enabled":              true,
		"view_counts_everywhere_api_enabled":                                      true,
		"longform_notetweets_consumption_enabled":                                 true,
		"responsive_web_twitter_article_tweet_consumption_enabled":                true,
		"tweet_awards_web_tipping_enabled":                                        false,
		"creator_subscriptions_quote_tweet_preview_enabled":                       false,
		"freedom_of_speech_not_reach_fetch_enabled":                               true,
		"standardized_nudges_misinfo":                                             true,
		"tweet_with_visibility_results_prefer_gql_limited_actions_policy_enabled": true,
		"rweb_video_timestamps_enabled":                                           true,
		"longform_notetweets_rich_text_read_enabled":                              true,
		"longform_notetweets_inline_media_enabled":                                true,
		"responsive_web_graphql_timeline_navigation_enabled":                      true,
		"responsive_web_enhance_cards_enabled":                                    false,
	}

	query := url.Values{}
	query.Set("variables", mapToJSONString(variables))
	query.Set("features", mapToJSONString(features))
	req.URL.RawQuery = query.Encode()

	var spaceData space
	err = s.RequestAPI(req, &spaceData)
	if err != nil {
		return nil, err
	}

	space := spaceData.parse()

	if space.ID == "" {
		return nil, errors.New("some erorr happend")
	}

	return space, nil
}

type Topic struct {
	ID    string
	Title string
}

type SpaceUser struct {
	UserID      string
	Username    string
	Name        string
	Avatar      string
	IsVerified  bool
	ConnectedAt time.Time
}

type SpaceParticipants struct {
	TotalCount   int
	CurrentCount int
	Admins       []*SpaceUser
	Speakers     []*SpaceUser
	Listeners    []*SpaceUser
}

type Space struct {
	ID             string
	State          string
	Title          string
	ContentType    string
	Topics         []Topic
	Participants   SpaceParticipants
	CreatedAt      time.Time
	ScheduledStart time.Time
	StartedAt      time.Time
	UpdatedAt      time.Time
}

type spaceUser struct {
	PeriscopeUserID   string `json:"periscope_user_id"`
	Start             int64  `json:"start"`
	TwitterScreenName string `json:"twitter_screen_name"`
	DisplayName       string `json:"display_name"`
	AvatarURL         string `json:"avatar_url"`
	IsVerified        bool   `json:"is_verified"`
	IsMutedByAdmin    bool   `json:"is_muted_by_admin"`
	IsMutedByGuest    bool   `json:"is_muted_by_guest"`
	UserResults       struct {
		RestID string `json:"rest_id"`
		Result struct {
			Typename                              string `json:"__typename"`
			IdentityProfileLabelsHighlightedLabel struct {
			} `json:"identity_profile_labels_highlighted_label"`
			IsBlueVerified bool `json:"is_blue_verified"`
			Legacy         struct {
			} `json:"legacy"`
		} `json:"result"`
	} `json:"user_results"`
}

type space struct {
	Data struct {
		AudioSpace struct {
			Metadata struct {
				RestID         string `json:"rest_id"`
				State          string `json:"state"`
				Title          string `json:"title"`
				MediaKey       string `json:"media_key"`
				CreatedAt      int64  `json:"created_at"`
				ScheduledStart int64  `json:"scheduled_start"`
				StartedAt      int64  `json:"started_at"`
				UpdatedAt      int64  `json:"updated_at"`
				ContentType    string `json:"content_type"`
				CreatorResults struct {
					Result struct {
						Typename           string     `json:"__typename"`
						ID                 string     `json:"id"`
						RestID             string     `json:"rest_id"`
						HasGraduatedAccess bool       `json:"has_graduated_access"`
						IsBlueVerified     bool       `json:"is_blue_verified"`
						ProfileImageShape  string     `json:"profile_image_shape"`
						Legacy             legacyUser `json:"legacy"`
					} `json:"result"`
				} `json:"creator_results"`
				ConversationControls        int  `json:"conversation_controls"`
				DisallowJoin                bool `json:"disallow_join"`
				IsEmployeeOnly              bool `json:"is_employee_only"`
				IsLocked                    bool `json:"is_locked"`
				IsMuted                     bool `json:"is_muted"`
				IsSpaceAvailableForClipping bool `json:"is_space_available_for_clipping"`
				IsSpaceAvailableForReplay   bool `json:"is_space_available_for_replay"`
				MentionedUsers              []struct {
					RestID string `json:"rest_id"`
				} `json:"mentioned_users"`
				NarrowCastSpaceType int  `json:"narrow_cast_space_type"`
				NoIncognito         bool `json:"no_incognito"`
				TotalReplayWatched  int  `json:"total_replay_watched"`
				TotalLiveListeners  int  `json:"total_live_listeners"`
				Topics              []struct {
					Topic struct {
						TopicID string `json:"topic_id"`
						Name    string `json:"name"`
					} `json:"topic"`
				} `json:"topics"`
				TweetResults struct {
					Result tweet `json:"result"`
				} `json:"tweet_results"`
				MaxGuestSessions int `json:"max_guest_sessions"`
				MaxAdminCapacity int `json:"max_admin_capacity"`
			} `json:"metadata"`

			IsSubscribed bool `json:"is_subscribed"`
			Participants struct {
				Total     int         `json:"total"`
				Admins    []spaceUser `json:"admins"`
				Speakers  []spaceUser `json:"speakers"`
				Listeners []spaceUser `json:"listeners"`
			} `json:"participants"`
			Sharings struct {
				Items []struct {
					SharingID   string `json:"sharing_id"`
					CreatedAtMs int64  `json:"created_at_ms"`
					UpdatedAtMs int64  `json:"updated_at_ms"`
					SharedItem  struct {
						Typename     string `json:"__typename"`
						TweetResults struct {
							Result struct {
								Typename string `json:"__typename"`
								RestID   string `json:"rest_id"`
								Core     tweet  `json:"result"`
							} `json:"tweet_results"`
						} `json:"shared_item"`
						UserResults struct {
							Result struct {
								Typename                   string `json:"__typename"`
								ID                         string `json:"id"`
								RestID                     string `json:"rest_id"`
								AffiliatesHighlightedLabel struct {
								} `json:"affiliates_highlighted_label"`
								HasGraduatedAccess bool       `json:"has_graduated_access"`
								IsBlueVerified     bool       `json:"is_blue_verified"`
								ProfileImageShape  string     `json:"profile_image_shape"`
								Legacy             legacyUser `json:"legacy"`
								TipjarSettings     struct {
									IsEnabled     bool   `json:"is_enabled"`
									CashAppHandle string `json:"cash_app_handle"`
								} `json:"tipjar_settings"`
							} `json:"result"`
						} `json:"user_results"`
					} `json:"shared_item"`
				} `json:"items"`
			} `json:"sharings"`
		} `json:"audioSpace"`
	} `json:"data"`
}

func (user *spaceUser) parse() *SpaceUser {
	result := &SpaceUser{
		UserID:      user.UserResults.RestID,
		Username:    user.TwitterScreenName,
		Name:        user.DisplayName,
		Avatar:      user.AvatarURL,
		IsVerified:  user.IsVerified,
		ConnectedAt: time.Unix(user.Start/1000, 0),
	}

	return result
}

func (space *space) parse() *Space {
	result := &Space{
		ID:          space.Data.AudioSpace.Metadata.RestID,
		State:       space.Data.AudioSpace.Metadata.State,
		Title:       space.Data.AudioSpace.Metadata.Title,
		ContentType: space.Data.AudioSpace.Metadata.ContentType,
		Topics:      []Topic{},
		Participants: SpaceParticipants{
			TotalCount:   space.Data.AudioSpace.Metadata.TotalLiveListeners,
			CurrentCount: space.Data.AudioSpace.Participants.Total,
		},
		CreatedAt:      time.Unix(space.Data.AudioSpace.Metadata.CreatedAt/1000, 0),
		ScheduledStart: time.Unix(space.Data.AudioSpace.Metadata.ScheduledStart/1000, 0),
		StartedAt:      time.Unix(space.Data.AudioSpace.Metadata.StartedAt/1000, 0),
		UpdatedAt:      time.Unix(space.Data.AudioSpace.Metadata.UpdatedAt/1000, 0),
	}

	for _, topic := range space.Data.AudioSpace.Metadata.Topics {
		result.Topics = append(result.Topics, Topic{
			ID:    topic.Topic.TopicID,
			Title: topic.Topic.Name,
		})
	}

	for _, admin := range space.Data.AudioSpace.Participants.Admins {
		result.Participants.Admins = append(result.Participants.Admins, admin.parse())
	}

	for _, speaker := range space.Data.AudioSpace.Participants.Speakers {
		result.Participants.Speakers = append(result.Participants.Speakers, speaker.parse())
	}

	for _, listener := range space.Data.AudioSpace.Participants.Listeners {
		result.Participants.Listeners = append(result.Participants.Listeners, listener.parse())

	}

	return result
}
