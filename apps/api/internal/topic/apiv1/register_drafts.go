package apiv1

import (
	"net/http"

	v1 "kun-galgame-api/internal/apiv1"

	"github.com/danielgtaylor/huma/v2"
)

const draftPrivacy = "A draft is private to its author. Another author's draft is NOT_FOUND, never 403: a stranger must not learn that the id exists."

func RegisterDrafts(d *Drafts) func(huma.API) {
	return func(api huma.API) {
		huma.Register(api, v1.Required(huma.Operation{
			OperationID: "listTopicDrafts",
			Method:      http.MethodGet,
			Path:        "/me/topic-drafts",
			Summary:     "List the caller's topic drafts",
			Description: "Lists the caller's own topic drafts, most recently written first, ties broken by descending id. " +
				"There is one sort and no sort parameter. " + draftPrivacy,
			Tags: []string{"topics"},
		}), d.listTopicDrafts)

		huma.Register(api, v1.Required(huma.Operation{
			OperationID: "getTopicDraft",
			Method:      http.MethodGet,
			Path:        "/me/topic-drafts/{draft_id}",
			Summary:     "Get a topic draft",
			Description: "Returns one of the caller's own drafts, with the body as stored. " + draftPrivacy,
			Tags:        []string{"topics"},
			Responses: problemResponses(map[int]string{
				404: "NOT_FOUND when the draft does not exist or belongs to another author.",
			}),
		}), d.getTopicDraft)

		huma.Register(api, v1.IdempotencyRequired(v1.Required(huma.Operation{
			OperationID:   "createTopicDraft",
			Method:        http.MethodPost,
			Path:          "/me/topic-drafts",
			Summary:       "Save a topic draft",
			DefaultStatus: http.StatusCreated,
			Description: "Saves the editor's current buffer as a new draft and returns it at revision 1; " +
				"every new draft is born with one pinned revision. " +
				"Every field is optional because a draft is unfinished by definition; sections are not checked against category. " +
				"This is only the FIRST save of a new post: continued saves go to updateTopicDraft, which rewrites one draft instead of minting rows. " +
				"Deleting is the only way to make room once the cap is reached.",
			Tags: []string{"topics"},
			Responses: problemResponses(map[int]string{
				409: "DRAFT_LIMIT_REACHED when the caller already holds 30 drafts.",
				422: "VALIDATION_FAILED when the title and the body are both blank after trimming whitespace.",
			}),
		})), d.createTopicDraft)

		huma.Register(api, v1.Required(huma.Operation{
			OperationID: "updateTopicDraft",
			Method:      http.MethodPatch,
			Path:        "/me/topic-drafts/{draft_id}",
			Summary:     "Continue saving one topic draft",
			Description: "Rewrites the caller's own draft and bumps its revision by one. " +
				"Autosave sends is_manual_save=false and leaves no revision; an explicit save sends is_manual_save=true and pins a 'manual' revision the author can diff and restore. " +
				"base_revision must equal the stored revision. When it does not, nothing is written and DRAFT_REVISION_CONFLICT comes back with current_revision: another window or device saved first. " +
				"is_force_overwrite=true wins the conflict by pinning that stored content as a 'checkpoint' revision before overwriting it.",
			Tags: []string{"topics"},
			Responses: problemResponses(map[int]string{
				404: "NOT_FOUND when the draft does not exist or belongs to another author.",
				409: "DRAFT_REVISION_CONFLICT when base_revision differs from the stored revision and force is not set.",
				422: "VALIDATION_FAILED when the title and the body are both blank after trimming whitespace.",
			}),
		}), d.updateTopicDraft)

		huma.Register(api, v1.Required(huma.Operation{
			OperationID: "listTopicDraftRevisions",
			Method:      http.MethodGet,
			Path:        "/me/topic-drafts/{draft_id}/revisions",
			Summary:     "List a topic draft's pinned revisions",
			Description: "Lists the draft's pinned revisions, newest first, at most 50: " +
				"'manual' revisions from explicit saves (plus revision 1 at creation) and 'checkpoint' revisions kept automatically before a force overwrite or a restore. " +
				draftPrivacy,
			Tags: []string{"topics"},
			Responses: problemResponses(map[int]string{
				404: "NOT_FOUND when the draft does not exist or belongs to another author.",
			}),
		}), d.listTopicDraftRevisions)

		huma.Register(api, v1.Required(huma.Operation{
			OperationID: "getTopicDraftRevision",
			Method:      http.MethodGet,
			Path:        "/me/topic-drafts/{draft_id}/revisions/{revision_id}",
			Summary:     "Get one pinned topic draft revision",
			Description: "Returns one revision's full content, as stored. " + draftPrivacy,
			Tags:        []string{"topics"},
			Responses: problemResponses(map[int]string{
				404: "NOT_FOUND when the draft or revision does not exist or belongs to another author.",
			}),
		}), d.getTopicDraftRevision)

		huma.Register(api, v1.Required(huma.Operation{
			OperationID: "restoreTopicDraftRevision",
			Method:      http.MethodPost,
			Path:        "/me/topic-drafts/{draft_id}/revisions/{revision_id}/restore",
			Summary:     "Restore a pinned topic draft revision",
			Description: "Pins the draft's current working content as a 'checkpoint' revision, copies the chosen revision's content over the draft, bumps the revision, and returns the draft. " +
				"The current content is therefore never lost: it becomes the newest checkpoint. " +
				"base_revision must be current; a stale restore is refused with DRAFT_REVISION_CONFLICT like updateTopicDraft. " +
				draftPrivacy,
			Tags: []string{"topics"},
			Responses: problemResponses(map[int]string{
				404: "NOT_FOUND when the draft or revision does not exist or belongs to another author.",
				409: "DRAFT_REVISION_CONFLICT when base_revision differs from the stored revision.",
			}),
		}), d.restoreTopicDraftRevision)

		huma.Register(api, v1.Required(huma.Operation{
			OperationID:   "deleteTopicDraft",
			Method:        http.MethodDelete,
			Path:          "/me/topic-drafts/{draft_id}",
			Summary:       "Delete a topic draft",
			DefaultStatus: http.StatusNoContent,
			Description:   "Deletes one of the caller's own drafts. Deleting it again is NOT_FOUND. " + draftPrivacy,
			Tags:          []string{"topics"},
			Responses: problemResponses(map[int]string{
				404: "NOT_FOUND when the draft does not exist or belongs to another author.",
			}),
		}), d.deleteTopicDraft)
	}
}
