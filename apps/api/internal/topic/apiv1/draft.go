package apiv1

import "kun-galgame-api/internal/apiv1/repr"

type TopicDraft struct {
	Object           string              `json:"object" enum:"topic_draft" maxLength:"11" doc:"Type discriminant. Always topic_draft."`
	ID               repr.DecimalID      `json:"id" doc:"Draft id. JSON string of a decimal integer."`
	Title            string              `json:"title" maxLength:"233" doc:"Draft title as stored, empty string when the author has not written one. Free text; never use it as a decision input."`
	ContentMarkdown  string              `json:"content_markdown" maxLength:"100007" doc:"Draft body as Markdown source, stored as sent. Empty string when the author has not written one. Free text; never use it as a decision input."`
	Category         *string             `json:"category" enum:"galgame,technique,others" maxLength:"9" doc:"Deprecated: the section decides a topic's category. The category the draft was saved with; null when none."`
	Sections         []SectionSlug       `json:"sections" maxItems:"3" doc:"The chosen section as a one-item array of a current key. Empty array if none."`
	IsNSFW           bool                `json:"is_nsfw" doc:"Whether the author marked the draft NSFW."`
	CoverImageHashes []ImageHash         `json:"cover_image_hashes" maxItems:"9" doc:"Cover images by image-service hash, in stored order. Tokens that do not parse are skipped. Empty array if none."`
	Polls            []TopicDraftPoll    `json:"polls" maxItems:"30" doc:"Polls the author attached, in order. The web creates them on the topic right after createTopic. Empty array if none."`
	Lotteries        []TopicDraftLottery `json:"lotteries" maxItems:"10" doc:"Lotteries the author attached, in order, without redemption codes. The web creates them on the topic right after createTopic. Empty array if none."`
	Revision         int                 `json:"revision" minimum:"1" doc:"Optimistic-lock token. Starts at 1 and every write (autosave, snapshot save, restore) bumps it by one. Send the value last read as base_revision on updateTopicDraft."`
	CreatedAt        repr.DateTime       `json:"created_at" doc:"Creation time."`
	UpdatedAt        repr.DateTime       `json:"updated_at" doc:"Last write time."`
}

type TopicDraftSummary struct {
	Object    string         `json:"object" enum:"topic_draft_summary" maxLength:"19" doc:"Type discriminant. Always topic_draft_summary."`
	ID        repr.DecimalID `json:"id" doc:"Draft id. JSON string of a decimal integer."`
	Title     string         `json:"title" maxLength:"233" doc:"Draft title as stored, empty string when the author has not written one. Free text; never use it as a decision input."`
	Summary   string         `json:"summary" maxLength:"120" doc:"First 120 characters of the body as stored: raw Markdown, image tokens included, cut without regard for word or token boundaries. Free text; never use it as a decision input."`
	Revision  int            `json:"revision" minimum:"1" doc:"Current optimistic-lock revision of the draft, as on the full object."`
	CreatedAt repr.DateTime  `json:"created_at" doc:"Creation time."`
	UpdatedAt repr.DateTime  `json:"updated_at" doc:"Last write time."`
}

type TopicDraftCreate struct {
	Title            string              `json:"title" required:"false" maxLength:"233" doc:"Draft title. Stored as sent. Free text; never use it as a decision input."`
	ContentMarkdown  string              `json:"content_markdown" required:"false" maxLength:"100007" doc:"Draft body as Markdown source. Stored as sent. Free text; never use it as a decision input."`
	Category         *string             `json:"category,omitempty" enum:"galgame,technique,others" maxLength:"9" doc:"Deprecated and stored only for older clients: the section decides a topic's category."`
	Sections         []SectionSlug       `json:"sections" required:"false" maxItems:"3" uniqueItems:"true" doc:"The chosen section as a one-item array. A retired key is stored as its successor, items after the first are dropped, and an empty list is allowed: a draft is unfinished by definition."`
	IsNSFW           bool                `json:"is_nsfw" required:"false" doc:"Whether to mark the draft NSFW. Defaults to false."`
	CoverImageHashes []ImageHash         `json:"cover_image_hashes" required:"false" maxItems:"9" uniqueItems:"true" doc:"Cover images by image-service hash, in display order. Unlike createTopic, nothing is derived from the body."`
	Polls            []TopicDraftPoll    `json:"polls" required:"false" maxItems:"30" doc:"Polls attached to the draft, in order. Stored as sent; nothing is created."`
	Lotteries        []TopicDraftLottery `json:"lotteries" required:"false" maxItems:"10" doc:"Lotteries attached to the draft, in order. Stored as sent; nothing is created and no moemoepoint is held. Redemption codes are not accepted: a draft has no field for them."`
}

// TopicDraftUpdate is the editor's continued save over one draft. Every
// content field is replaced (this is a form save, not a JSON merge).
type TopicDraftUpdate struct {
	Title            string              `json:"title" required:"false" maxLength:"233" doc:"Draft title, full replacement. Free text; never use it as a decision input."`
	ContentMarkdown  string              `json:"content_markdown" required:"false" maxLength:"100007" doc:"Draft body as Markdown source, full replacement. Free text; never use it as a decision input."`
	Category         *string             `json:"category,omitempty" enum:"galgame,technique,others" maxLength:"9" doc:"Deprecated: the section decides a topic's category."`
	Sections         []SectionSlug       `json:"sections" required:"false" maxItems:"3" uniqueItems:"true" doc:"The chosen section as a one-item array; same normalization as create."`
	IsNSFW           bool                `json:"is_nsfw" required:"false" doc:"Whether to mark the draft NSFW."`
	CoverImageHashes []ImageHash         `json:"cover_image_hashes" required:"false" maxItems:"9" uniqueItems:"true" doc:"Cover images by image-service hash, in display order."`
	Polls            []TopicDraftPoll    `json:"polls" required:"false" maxItems:"30" doc:"Polls attached to the draft, in order."`
	Lotteries        []TopicDraftLottery `json:"lotteries" required:"false" maxItems:"10" doc:"Lotteries attached to the draft, in order, without redemption codes."`
	BaseRevision     int                 `json:"base_revision" required:"true" minimum:"1" doc:"The revision the editor last read. When it differs from the stored revision the request is refused with DRAFT_REVISION_CONFLICT unless is_force_overwrite is true."`
	IsManualSave     bool                `json:"is_manual_save" required:"false" doc:"true pins this write as a 'manual' revision the author can diff against and restore; false is a silent autosave. Defaults to false."`
	IsForceOverwrite bool                `json:"is_force_overwrite" required:"false" doc:"true wins a revision conflict: the stored content is kept as a 'checkpoint' revision before the overwrite. Ignored when base_revision is current."`
}

type TopicDraftRevision struct {
	Object           string              `json:"object" enum:"topic_draft_revision" maxLength:"20" doc:"Type discriminant. Always topic_draft_revision."`
	ID               repr.DecimalID      `json:"id" doc:"Revision id. JSON string of a decimal integer."`
	Kind             string              `json:"kind" enum:"manual,checkpoint" maxLength:"10" doc:"manual: the author pinned it with is_manual_save=true (and revision 1 at creation). checkpoint: saved automatically, either the other window's content before a force overwrite or the working copy before a restore."`
	BaseRevision     int                 `json:"base_revision" minimum:"1" doc:"The working-copy revision whose content this revision captures."`
	Title            string              `json:"title" maxLength:"233" doc:"Title as pinned. Free text; never use it as a decision input."`
	ContentMarkdown  string              `json:"content_markdown" maxLength:"100007" doc:"Body as pinned, Markdown source. Free text; never use it as a decision input."`
	Category         *string             `json:"category" enum:"galgame,technique,others" maxLength:"9" doc:"Deprecated category the revision was saved with; null when none."`
	Sections         []SectionSlug       `json:"sections" maxItems:"3" doc:"The chosen section as a one-item array. Empty array if none."`
	IsNSFW           bool                `json:"is_nsfw" doc:"Whether the revision was marked NSFW."`
	CoverImageHashes []ImageHash         `json:"cover_image_hashes" maxItems:"9" doc:"Cover images by image-service hash, in stored order."`
	Polls            []TopicDraftPoll    `json:"polls" maxItems:"30" doc:"Polls pinned with the revision. Empty array if none."`
	Lotteries        []TopicDraftLottery `json:"lotteries" maxItems:"10" doc:"Lotteries pinned with the revision, without redemption codes. Empty array if none."`
	CreatedAt        repr.DateTime       `json:"created_at" doc:"When this revision was pinned."`
}

type TopicDraftRevisionSummary struct {
	Object       string         `json:"object" enum:"topic_draft_revision_summary" maxLength:"28" doc:"Type discriminant. Always topic_draft_revision_summary."`
	ID           repr.DecimalID `json:"id" doc:"Revision id. JSON string of a decimal integer."`
	Kind         string         `json:"kind" enum:"manual,checkpoint" maxLength:"10" doc:"manual or checkpoint, as on the full object."`
	BaseRevision int            `json:"base_revision" minimum:"1" doc:"The working-copy revision this revision captures."`
	Title        string         `json:"title" maxLength:"233" doc:"Title as pinned. Free text; never use it as a decision input."`
	Summary      string         `json:"summary" maxLength:"120" doc:"First 120 characters of the pinned body: raw Markdown, image tokens included. Free text; never use it as a decision input."`
	CreatedAt    repr.DateTime  `json:"created_at" doc:"When this revision was pinned."`
}

type TopicDraftRevisionRestore struct {
	BaseRevision int `json:"base_revision" required:"true" minimum:"1" doc:"The revision the editor last read. A restore that lands on a stale draft is refused with DRAFT_REVISION_CONFLICT."`
}
