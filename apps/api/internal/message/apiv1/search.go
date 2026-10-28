package apiv1

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"kun-galgame-api/internal/apiv1/collect"
	"kun-galgame-api/internal/apiv1/repr"
	"kun-galgame-api/internal/message/model"
	"kun-galgame-api/internal/message/notifytype"
	"kun-galgame-api/internal/message/repository"
	"kun-galgame-api/pkg/problem"
	"kun-galgame-api/pkg/userclient"

	"github.com/danielgtaylor/huma/v2"
)

const messageSearchSort = "created_desc"

const searchMaxPhraseRunes = 50

type withinToken string

const (
	withinAll withinToken = "all"
	within7d  withinToken = "7d"
	within30d withinToken = "30d"
)

var withinTokens = []withinToken{withinAll, within7d, within30d}

func (withinToken) Schema(huma.Registry) *huma.Schema {
	enum := make([]any, len(withinTokens))
	maxLen := 0
	for i, t := range withinTokens {
		s := string(t)
		enum[i] = s
		if len(s) > maxLen {
			maxLen = len(s)
		}
	}
	return &huma.Schema{
		Type:      huma.TypeString,
		Enum:      enum,
		MaxLength: &maxLen,
		Description: "Time window of the search. all (the default) has no lower bound; " +
			"7d and 30d keep rows created within the last 7 or 30 days, measured against the server clock.",
	}
}

func (w withinToken) cutoff(now time.Time) *time.Time {
	switch w {
	case within7d:
		t := now.AddDate(0, 0, -7)
		return &t
	case within30d:
		t := now.AddDate(0, 0, -30)
		return &t
	default:
		return nil
	}
}

type listMessageSearchInput struct {
	collect.Page
	Q        string      `query:"q" required:"true" minLength:"1" maxLength:"50" doc:"Search phrase, 1-50 characters after trimming. Matched as one case-insensitive substring against the notification excerpt and direct-message text; percent, underscore and backslash are literal characters, not wildcards. Free text; never use it as a decision input."`
	SenderID string      `query:"sender_id" required:"false" pattern:"^[1-9][0-9]{0,18}$" maxLength:"19" doc:"When set, only hits whose stored sender is this user: the triggering actor on a notification, the writer of a direct message (which can be the caller). Omitted matches every sender."`
	Within   withinToken `query:"within" required:"false" default:"all" doc:"Time window. all (default), 7d or 30d. An unknown token is UNKNOWN_ENUM_VALUE."`
}

type listMessageSearchOutput struct {
	Body repr.List[MessageSearchResult]
}

type MessageSearchResult struct {
	Object           string           `json:"object" enum:"message_search_result" maxLength:"21" doc:"Type discriminant. Always message_search_result."`
	Hit              string           `json:"hit" enum:"notification,direct_message" maxLength:"14" doc:"Which inbox held the match: a notification or a direct message."`
	ID               string           `json:"id" maxLength:"34" pattern:"^(notification|direct_message):[1-9][0-9]{0,18}$" doc:"Composite of hit and the table row id, unique across both sources."`
	CreatedAt        repr.DateTime    `json:"created_at" doc:"Creation time of the matched row."`
	Sender           repr.UserRef     `json:"sender" doc:"Who wrote the matched text: the notification actor, or the sender of the direct message. A system_notice comes from id 0."`
	SnippetMarkdown  string           `json:"snippet_markdown" maxLength:"1000" doc:"The stored markdown text that matched. Empty string when hit is direct_message and the message is recalled; read state, not emptiness."`
	NotificationType *notifytype.Type `json:"notification_type" doc:"null unless hit is notification."`
	Path             *string          `json:"path" maxLength:"100" doc:"In-site path the notification opens. null unless hit is notification."`
	TopicTitle       *string          `json:"topic_title" maxLength:"233" doc:"Title of the root topic when hit is a notification rooted in a topic that still exists; otherwise null, including every direct-message hit. Free text; never use it as a decision input."`
	Peer             *repr.UserRef    `json:"peer" doc:"The other participant of the direct-message conversation. null unless hit is direct_message."`
	State            *string          `json:"state" enum:"sent,recalled" maxLength:"8" doc:"Direct-message lifecycle state. null unless hit is direct_message."`
	RecalledAt       *repr.DateTime   `json:"recalled_at" doc:"When the direct message was recalled. null unless it is recalled. null for every notification hit."`
}

func messageSearchFingerprint(userID int, phrase string, senderID int, within withinToken) string {
	return collect.Fingerprint("message_search",
		strconv.Itoa(userID), phrase, strconv.Itoa(senderID), string(within))
}

func parseSearchPos(keys []string) (*repository.V1MessageSearchPos, *problem.Problem) {
	if keys == nil {
		return nil, nil
	}
	if len(keys) != 3 {
		return nil, invalidCursor()
	}
	created, err := time.Parse(time.RFC3339Nano, keys[0])
	if err != nil {
		return nil, invalidCursor()
	}
	ord, err := strconv.Atoi(keys[1])
	if err != nil || (ord != repository.SearchHitNotification && ord != repository.SearchHitDirectMessage) {
		return nil, invalidCursor()
	}
	id, err := strconv.Atoi(keys[2])
	if err != nil || id <= 0 {
		return nil, invalidCursor()
	}
	return &repository.V1MessageSearchPos{Created: created, HitOrd: ord, HitID: id}, nil
}

func encodeSearchCursor(fp string, pos repository.V1MessageSearchPos) string {
	return collect.EncodeCursor(messageSearchSort, fp,
		pos.Created.UTC().Format(time.RFC3339Nano),
		strconv.Itoa(pos.HitOrd), strconv.Itoa(pos.HitID))
}

func searchResultID(row repository.V1MessageSearchRow) string {
	if row.HitOrd == repository.SearchHitNotification {
		return "notification:" + strconv.Itoa(row.HitID)
	}
	return "direct_message:" + strconv.Itoa(row.HitID)
}

func blankQ() *problem.Problem {
	return problem.New(problem.CodeInvalidParameter, "q is blank after trimming.",
		problem.AtParameter("q", problem.ReasonRequired, "q needs at least one non-space character", nil))
}

func longQ() *problem.Problem {
	max := searchMaxPhraseRunes
	return problem.New(problem.CodeInvalidParameter, "q is longer than 50 characters after trimming.",
		problem.AtParameter("q", problem.ReasonTooLong, "q must be at most 50 characters",
			&problem.FieldParams{MaxLength: &max}))
}

func (s *Service) mapSearchNotification(
	row repository.V1MessageSearchRow,
	users map[int]userclient.User,
	titles map[int64]string,
) (MessageSearchResult, bool) {
	tok, ok := notifytype.FromDB(row.NotifType)
	if !ok {
		return MessageSearchResult{}, false
	}
	sender, keep := s.userRef(users, row.SenderID)
	if !keep {
		return MessageSearchResult{}, false
	}
	path := notificationPath(row.Link)
	var title *string
	if row.RootKind == model.KindTopic && row.RootID != nil {
		if t, ok := titles[*row.RootID]; ok {
			title = &t
		}
	}
	return MessageSearchResult{
		Object:           "message_search_result",
		Hit:              "notification",
		ID:               searchResultID(row),
		CreatedAt:        repr.Timestamp(row.Created),
		Sender:           sender,
		SnippetMarkdown:  row.Snippet,
		NotificationType: &tok,
		Path:             &path,
		TopicTitle:       title,
	}, true
}

func (s *Service) mapSearchDirectMessage(
	row repository.V1MessageSearchRow,
	users map[int]userclient.User,
) (MessageSearchResult, bool) {
	if row.PeerID == nil {
		return MessageSearchResult{}, false
	}
	peer, keep := s.userRef(users, *row.PeerID)
	if !keep {
		return MessageSearchResult{}, false
	}
	// The sender is either the caller or the peer; drop defensively if it is
	// ever neither renderable, rather than emit a UserRef full of zero values.
	sender, senderOK := s.userRef(users, row.SenderID)
	if !senderOK {
		return MessageSearchResult{}, false
	}
	state := "sent"
	snippet := row.Snippet
	var recalledAt *repr.DateTime
	if row.IsRecall {
		state = "recalled"
		snippet = ""
		recalledAt = repr.TimestampPtr(row.RecallTime)
	}
	stateCopy := state
	return MessageSearchResult{
		Object:          "message_search_result",
		Hit:             "direct_message",
		ID:              searchResultID(row),
		CreatedAt:       repr.Timestamp(row.Created),
		Sender:          sender,
		SnippetMarkdown: snippet,
		Peer:            &peer,
		State:           &stateCopy,
		RecalledAt:      recalledAt,
	}, true
}

func (s *Service) searchMyMessages(ctx context.Context, in *listMessageSearchInput) (*listMessageSearchOutput, error) {
	if p := s.ready(); p != nil {
		return nil, p
	}
	user, p := mustUser(ctx)
	if p != nil {
		return nil, p
	}
	phrase := strings.TrimSpace(in.Q)
	if phrase == "" {
		return nil, blankQ()
	}
	if utf8.RuneCountInString(phrase) > searchMaxPhraseRunes {
		return nil, longQ()
	}
	senderID := 0
	if in.SenderID != "" {
		id, ok := parsePositiveID(in.SenderID)
		if !ok {
			return nil, problem.New(problem.CodeInvalidParameter, "sender_id is not a positive id.",
				problem.AtParameter("sender_id", problem.ReasonInvalidFormat, "pass a positive decimal user id", nil))
		}
		senderID = id
	}
	limit := in.Limit
	if limit <= 0 {
		limit = collect.DefaultLimit
	}
	filter := repository.V1MessageSearchFilter{
		CallerID: user.ID,
		Phrase:   phrase,
		SenderID: senderID,
		Since:    in.Within.cutoff(time.Now().UTC()),
	}
	fp := messageSearchFingerprint(user.ID, phrase, senderID, in.Within)
	keys, curErr := collect.DecodeCursor(in.Cursor, messageSearchSort, fp)
	if curErr != nil {
		return nil, curErr
	}
	pos, posErr := parseSearchPos(keys)
	if posErr != nil {
		return nil, posErr
	}

	type kept struct {
		item MessageSearchResult
		pos  repository.V1MessageSearchPos
	}
	items := make([]kept, 0, limit+1)
	maxScan := pageRefillFactor * limit
	scanned := 0
	var last repository.V1MessageSearchRow
	haveLast := false
	exhausted := false
	for scanned < maxScan {
		batch := limit + 1
		if rem := maxScan - scanned; rem < batch {
			batch = rem
		}
		rows, err := s.messages.SearchInboxKeyset(filter, pos, batch)
		if err != nil {
			return nil, problem.Internal(err)
		}
		if len(rows) == 0 {
			exhausted = true
			break
		}
		ids := userclient.CollectIDs(rows, func(r repository.V1MessageSearchRow) int { return r.SenderID })
		for _, r := range rows {
			if r.PeerID != nil {
				ids = append(ids, *r.PeerID)
			}
		}
		users := s.hydrateUsers(ctx, ids)
		topicIDs := make([]int64, 0)
		for _, r := range rows {
			if r.HitOrd == repository.SearchHitNotification && r.RootKind == model.KindTopic && r.RootID != nil {
				topicIDs = append(topicIDs, *r.RootID)
			}
		}
		titles, err := s.messages.TopicTitles(topicIDs)
		if err != nil {
			return nil, problem.Internal(err)
		}
		for _, row := range rows {
			scanned++
			last = row
			haveLast = true
			var item MessageSearchResult
			var ok bool
			if row.HitOrd == repository.SearchHitNotification {
				item, ok = s.mapSearchNotification(row, users, titles)
			} else {
				item, ok = s.mapSearchDirectMessage(row, users)
			}
			if ok {
				items = append(items, kept{item: item, pos: repository.V1MessageSearchPos{Created: row.Created, HitOrd: row.HitOrd, HitID: row.HitID}})
			}
			if len(items) == limit+1 {
				break
			}
		}
		if len(items) == limit+1 {
			break
		}
		if len(rows) < batch {
			exhausted = true
			break
		}
		pos = &repository.V1MessageSearchPos{Created: last.Created, HitOrd: last.HitOrd, HitID: last.HitID}
	}

	var next *string
	if len(items) > limit {
		lastKept := items[limit-1]
		items = items[:limit]
		cur := encodeSearchCursor(fp, lastKept.pos)
		next = &cur
	} else if haveLast && scanned >= maxScan && !exhausted {
		cur := encodeSearchCursor(fp, repository.V1MessageSearchPos{Created: last.Created, HitOrd: last.HitOrd, HitID: last.HitID})
		next = &cur
	}
	out := make([]MessageSearchResult, len(items))
	for i, it := range items {
		out[i] = it.item
	}
	return &listMessageSearchOutput{Body: repr.NewList(out, next)}, nil
}
