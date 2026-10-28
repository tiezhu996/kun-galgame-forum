package app

import (
	"net/http"
	"strconv"
	"sync"
	"testing"

	"kun-galgame-api/internal/moemoepoint"
	"kun-galgame-api/pkg/problem"
)

func TestV1CreateQuizAnswerSelfForbidden(t *testing.T) {
	f := newQuizFix(t, nil)
	resp, got := f.qz(t, http.MethodPost, "/api/v1/quizzes/"+idStr(g2QJudge)+"/answers",
		"/quizzes/{quiz_id}/answers", "sess-grant", keyUUID(20),
		map[string]any{"choice_indexes": []int{}, "is_statement_true": true})
	wantCode(t, resp, got, http.StatusForbidden, problem.CodeSelfAnswerForbidden)
	if n := f.scalar(t, `SELECT COUNT(*) FROM galgame_quiz_answer WHERE quiz_id = ? AND role = 'answerer'`, g2QJudge); n != 0 {
		t.Error("author answerer row written")
	}
}

func TestV1CreateQuizAnswerRepeatPractice(t *testing.T) {
	f := newQuizFix(t, nil)
	// Bob's seeded first attempt on g2QMain was wrong ({"value":0}); the seed
	// quiz has 4 answerers and 1 correct.
	correct := func(body map[string]any) {
		t.Helper()
		ans, _ := body["answer"].(map[string]any)
		if ans == nil || ans["is_correct"] != true {
			t.Fatalf("answer %+v", ans)
		}
	}

	resp, got := f.qz(t, http.MethodPost, "/api/v1/quizzes/"+idStr(g2QMain)+"/answers",
		"/quizzes/{quiz_id}/answers", "sess-bob", keyUUID(21),
		map[string]any{"choice_indexes": []int{1}, "is_statement_true": nil})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("re-practice %d %+v", resp.StatusCode, got)
	}
	if got["is_first_attempt"] != false {
		t.Errorf("is_first_attempt %v", got["is_first_attempt"])
	}
	correct(got)
	ans, _ := got["answer"].(map[string]any)
	best, _ := ans["best"].(map[string]any)
	if best == nil || best["is_correct"] != true {
		t.Errorf("best %+v", ans["best"])
	}
	if ans["last_answered_at"] == nil {
		t.Error("missing last_answered_at")
	}
	if n := f.scalar(t, `SELECT COUNT(*) FROM galgame_quiz_answer WHERE quiz_id = ? AND user_id = ? AND role = 'answerer'`,
		g2QMain, w3UserBob); n != 1 {
		t.Errorf("answerer rows %d", n)
	}
	if n := f.scalar(t, `SELECT answer_count FROM galgame_quiz WHERE id = ?`, g2QMain); n != 4 {
		t.Errorf("answer_count moved on re-practice: %d", n)
	}
	if n := f.scalar(t, `SELECT correct_count FROM galgame_quiz WHERE id = ?`, g2QMain); n != 1 {
		t.Errorf("correct_count moved on re-practice: %d", n)
	}
	if n := f.scalar(t, `SELECT COUNT(*) FROM message WHERE sender_id = ? AND receiver_id = ? AND type = 'quiz-answered'
		AND link = ?`, w3UserBob, w3UserAlice, "/galgame-quiz/"+idStr(g2QMain)); n != 0 {
		t.Errorf("repeat answer notified the author: %d", n)
	}

	// A quality rating must survive later re-practice.
	if r, b := f.qz(t, http.MethodPut, "/api/v1/quizzes/"+idStr(g2QMain)+"/quality-rating",
		"/quizzes/{quiz_id}/quality-rating", "sess-bob", "", map[string]any{"rating": 8}); r.StatusCode != http.StatusOK {
		t.Fatalf("rate %d %+v", r.StatusCode, b)
	}

	// Wrong again: the latest grade flips, the best grade stays sticky.
	resp, got = f.qz(t, http.MethodPost, "/api/v1/quizzes/"+idStr(g2QMain)+"/answers",
		"/quizzes/{quiz_id}/answers", "sess-bob", keyUUID(210),
		map[string]any{"choice_indexes": []int{0}, "is_statement_true": nil})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second re-practice %d %+v", resp.StatusCode, got)
	}
	ans, _ = got["answer"].(map[string]any)
	if ans["is_correct"] != false {
		t.Errorf("latest grade %v", ans["is_correct"])
	}
	best, _ = ans["best"].(map[string]any)
	if best["is_correct"] != true {
		t.Errorf("sticky best %v", best)
	}
	if n := f.scalar(t, `SELECT answer_count * 100 + correct_count FROM galgame_quiz WHERE id = ?`, g2QMain); n != 401 {
		t.Error("public counts moved during re-practice")
	}
	if n := f.scalar(t, `SELECT quality_rating FROM galgame_quiz_answer WHERE quiz_id = ? AND user_id = ?`, g2QMain, w3UserBob); n != 8 {
		t.Errorf("quality rating lost on re-practice: %d", n)
	}

	resp, body := f.qz(t, http.MethodGet, "/api/v1/quizzes/"+idStr(g2QMain), "/quizzes/{quiz_id}", "sess-bob", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("detail %d", resp.StatusCode)
	}
	qv, _ := body["viewer"].(map[string]any)
	if qv["quality_rating"].(float64) != 8 {
		t.Errorf("viewer quality %v", qv["quality_rating"])
	}
	self, _ := qv["answer"].(map[string]any)
	if self["is_correct"] != false {
		t.Errorf("detail latest %v", self["is_correct"])
	}
	selfBest, _ := self["best"].(map[string]any)
	if selfBest["is_correct"] != true {
		t.Errorf("detail best %v", selfBest)
	}

	// The answered list marks the quiz by latest grade and sticky best grade.
	_, body = f.qz(t, http.MethodGet, "/api/v1/me/answered-quizzes?limit=100",
		"/me/answered-quizzes", "sess-bob", "", nil)
	for _, raw := range body["items"].([]any) {
		it, _ := raw.(map[string]any)
		if strID(it["id"]) != idStr(g2QMain) {
			continue
		}
		v, _ := it["viewer"].(map[string]any)
		if v["has_answered"] != true || v["is_correct"] != false || v["has_correct_attempt"] != true {
			t.Errorf("answered-list viewer %+v", v)
		}
		return
	}
	t.Error("g2QMain missing from bob's answered list")
}

func TestV1CreateQuizAnswerFirstAttemptCountsOnly(t *testing.T) {
	f := newQuizFix(t, nil)
	// First attempt wrong, re-practice correct: the author's correct_count
	// keeps following the first attempt (0 correct of 1 answerer).
	resp, got := f.qz(t, http.MethodPost, "/api/v1/quizzes/"+idStr(g2QJudge)+"/answers",
		"/quizzes/{quiz_id}/answers", "sess-alice", keyUUID(23),
		map[string]any{"choice_indexes": []int{}, "is_statement_true": false})
	if resp.StatusCode != http.StatusCreated || got["is_first_attempt"] != true {
		t.Fatalf("first %d %+v", resp.StatusCode, got)
	}
	if n := f.scalar(t, `SELECT answer_count * 100 + correct_count FROM galgame_quiz WHERE id = ?`, g2QJudge); n != 100 {
		t.Error("first attempt did not count once")
	}
	if n := f.scalar(t, `SELECT COUNT(*) FROM message WHERE sender_id = ? AND receiver_id = ? AND type = 'quiz-answered'
		AND link = ?`, w3UserAlice, w3UserGrant, "/galgame-quiz/"+idStr(g2QJudge)); n != 1 {
		t.Error("first attempt did not notify the author once")
	}
	resp, got = f.qz(t, http.MethodPost, "/api/v1/quizzes/"+idStr(g2QJudge)+"/answers",
		"/quizzes/{quiz_id}/answers", "sess-alice", keyUUID(24),
		map[string]any{"choice_indexes": []int{}, "is_statement_true": true})
	if resp.StatusCode != http.StatusOK || got["is_first_attempt"] != false {
		t.Fatalf("repeat %d %+v", resp.StatusCode, got)
	}
	if n := f.scalar(t, `SELECT answer_count * 100 + correct_count FROM galgame_quiz WHERE id = ?`, g2QJudge); n != 100 {
		t.Errorf("public counts moved on a later correct re-practice: %d", n)
	}
	if n := f.scalar(t, `SELECT COUNT(*) FROM message WHERE sender_id = ? AND receiver_id = ? AND type = 'quiz-answered'
		AND link = ?`, w3UserAlice, w3UserGrant, "/galgame-quiz/"+idStr(g2QJudge)); n != 1 {
		t.Error("re-practice notified the author again")
	}
}

func TestV1CreateQuizAnswerConcurrentUnique(t *testing.T) {
	f := newQuizFix(t, nil)
	body := map[string]any{"choice_indexes": []int{}, "is_statement_true": false}
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		got []string
	)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			resp, out := f.qz(t, http.MethodPost, "/api/v1/quizzes/"+idStr(g2QJudge)+"/answers",
				"/quizzes/{quiz_id}/answers", "sess-bob", keyUUID(30+i), body)
			first, _ := out["is_first_attempt"].(bool)
			mu.Lock()
			got = append(got, strconv.Itoa(resp.StatusCode)+":"+strconv.FormatBool(first))
			mu.Unlock()
		}()
	}
	wg.Wait()
	created, repeated := 0, 0
	for _, g := range got {
		switch g {
		case "201:true":
			created++
		case "200:false":
			repeated++
		}
	}
	if created != 1 || repeated != 1 {
		t.Errorf("concurrent answers %v", got)
	}
	if n := f.scalar(t, `SELECT COUNT(*) FROM galgame_quiz_answer WHERE quiz_id = ? AND role = 'answerer'`, g2QJudge); n != 1 {
		t.Errorf("answerer rows %d", n)
	}
	if n := f.scalar(t, `SELECT answer_count FROM galgame_quiz WHERE id = ?`, g2QJudge); n != 1 {
		t.Errorf("answer_count %d", n)
	}
	if n := f.scalar(t, `SELECT COUNT(*) FROM message WHERE sender_id = ? AND receiver_id = ? AND type = 'quiz-answered'`,
		w3UserBob, w3UserGrant); n != 1 {
		t.Errorf("notification count %d", n)
	}
}

func TestV1CreateQuizAnswerOK(t *testing.T) {
	f := newQuizFix(t, nil)
	resp, got := f.qz(t, http.MethodPost, "/api/v1/quizzes/"+idStr(g2QJudge)+"/answers",
		"/quizzes/{quiz_id}/answers", "sess-bob", keyUUID(22),
		map[string]any{"choice_indexes": []int{}, "is_statement_true": true})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("answer %d %+v", resp.StatusCode, got)
	}
	if resp.Header.Get("Location") != "/api/v1/quizzes/"+idStr(g2QJudge) {
		t.Errorf("location %s", resp.Header.Get("Location"))
	}
	if got["object"] != "quiz_answer_result" {
		t.Errorf("object %v", got["object"])
	}
	ans, _ := got["answer"].(map[string]any)
	if ans == nil || ans["is_correct"] != true {
		t.Errorf("answer %+v", ans)
	}
	if got["solution"] == nil {
		t.Error("missing solution")
	}
	if n := f.scalar(t, `SELECT answer_count FROM galgame_quiz WHERE id = ?`, g2QJudge); n != 1 {
		t.Errorf("answer_count %d", n)
	}
	if n := f.scalar(t, `SELECT COUNT(*) FROM message WHERE sender_id = ? AND receiver_id = ? AND type = 'quiz-answered'
		AND link = ? AND content = '选择「正确」，回答正确'`, w3UserBob, w3UserGrant, "/galgame-quiz/"+idStr(g2QJudge)); n != 1 {
		t.Errorf("the author was not told about the answer: %d quiz-answered rows", n)
	}
	answerID := f.scalar(t, `SELECT id FROM galgame_quiz_answer WHERE quiz_id = ? AND user_id = ?`, g2QJudge, w3UserBob)
	wantNotifRefs(t, f.db, []string{notifRef("quiz-answered", "quiz_answer", answerID, "quiz", g2QJudge)},
		"sender_id = ? AND receiver_id = ? AND type = 'quiz-answered'", w3UserBob, w3UserGrant)
}

func TestV1PutDeleteQuizFavorite(t *testing.T) {
	f := newQuizFix(t, nil)
	resp, got := f.qz(t, http.MethodPut, "/api/v1/quizzes/"+idStr(g2QMain)+"/favorite",
		"/quizzes/{quiz_id}/favorite", "sess-bob", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fav %d %+v", resp.StatusCode, got)
	}
	v, _ := got["viewer"].(map[string]any)
	if v == nil || v["has_favorited"] != true || asInt(got["favorite_count"]) != 1 {
		t.Errorf("fav body %+v", got)
	}
	aw := f.snapshotAwards()
	if len(aw) != 1 || aw[0].delta != 1 || aw[0].reason != moemoepoint.ReasonLiked {
		t.Errorf("fav award %+v", aw)
	}
	resp, got = f.qz(t, http.MethodPut, "/api/v1/quizzes/"+idStr(g2QMain)+"/favorite",
		"/quizzes/{quiz_id}/favorite", "sess-bob", "", nil)
	if resp.StatusCode != http.StatusOK || asInt(got["favorite_count"]) != 1 {
		t.Errorf("idempotent put %d %+v", resp.StatusCode, got)
	}
	if len(f.snapshotAwards()) != 1 {
		t.Errorf("second put awarded again %+v", f.snapshotAwards())
	}
	resp, got = f.qz(t, http.MethodDelete, "/api/v1/quizzes/"+idStr(g2QMain)+"/favorite",
		"/quizzes/{quiz_id}/favorite", "sess-bob", "", nil)
	if resp.StatusCode != http.StatusOK || asInt(got["favorite_count"]) != 0 {
		t.Errorf("unfav %d %+v", resp.StatusCode, got)
	}
	resp, got = f.qz(t, http.MethodDelete, "/api/v1/quizzes/"+idStr(g2QMain)+"/favorite",
		"/quizzes/{quiz_id}/favorite", "sess-bob", "", nil)
	if resp.StatusCode != http.StatusOK || asInt(got["favorite_count"]) != 0 {
		t.Errorf("idempotent delete %d %+v", resp.StatusCode, got)
	}
	before := len(f.snapshotAwards())
	resp, got = f.qz(t, http.MethodPut, "/api/v1/quizzes/"+idStr(g2QMain)+"/favorite",
		"/quizzes/{quiz_id}/favorite", "sess-alice", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("self fav %d %+v", resp.StatusCode, got)
	}
	if len(f.snapshotAwards()) != before {
		t.Errorf("self-favorite awarded: %+v", f.snapshotAwards()[before:])
	}
}

func TestV1PutQuizQualityRating(t *testing.T) {
	f := newQuizFix(t, nil)
	resp, got := f.qz(t, http.MethodPut, "/api/v1/quizzes/"+idStr(g2QMain)+"/quality-rating",
		"/quizzes/{quiz_id}/quality-rating", "sess-alice", "", map[string]any{"rating": 8})
	wantCode(t, resp, got, http.StatusForbidden, problem.CodeQuizAnswerRequired)
	resp, got = f.qz(t, http.MethodPut, "/api/v1/quizzes/"+idStr(g2QJudge)+"/quality-rating",
		"/quizzes/{quiz_id}/quality-rating", "sess-bob", "", map[string]any{"rating": 8})
	wantCode(t, resp, got, http.StatusForbidden, problem.CodeQuizAnswerRequired)
	resp, got = f.qz(t, http.MethodPut, "/api/v1/quizzes/"+idStr(g2QMain)+"/quality-rating",
		"/quizzes/{quiz_id}/quality-rating", "sess-bob", "", map[string]any{"rating": 8})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rate %d %+v", resp.StatusCode, got)
	}
	if asInt(got["quality_count"]) != 1 {
		t.Errorf("count %v", got["quality_count"])
	}
	v, _ := got["viewer"].(map[string]any)
	if v == nil || asInt(v["quality_rating"]) != 8 {
		t.Errorf("viewer %+v", v)
	}
	resp, got = f.qz(t, http.MethodPut, "/api/v1/quizzes/"+idStr(g2QMain)+"/quality-rating",
		"/quizzes/{quiz_id}/quality-rating", "sess-bob", "", map[string]any{"rating": 4})
	if resp.StatusCode != http.StatusOK || asInt(got["quality_count"]) != 1 {
		t.Errorf("replace %d %+v", resp.StatusCode, got)
	}
	sum := f.scalar(t, `SELECT quality_sum FROM galgame_quiz WHERE id = ?`, g2QMain)
	if sum != 4 {
		t.Errorf("sum %d", sum)
	}
}

func TestV1ListMyQuizStates(t *testing.T) {
	f := newQuizFix(t, nil)
	_, _ = f.qz(t, http.MethodPut, "/api/v1/quizzes/"+idStr(g2QMain)+"/favorite",
		"/quizzes/{quiz_id}/favorite", "sess-bob", "", nil)
	ids := idStr(g2QMain) + "," + idStr(g2QJudge) + "," + idStr(g2QTied+5) + "," + idStr(g2QGone)
	resp, body := f.qz(t, http.MethodGet, "/api/v1/me/quiz-states?quiz_ids="+ids,
		"/me/quiz-states", "sess-bob", "", nil)
	if resp.StatusCode != http.StatusOK || body["object"] != "list" {
		t.Fatalf("states %d %+v", resp.StatusCode, body)
	}
	items, _ := body["items"].([]any)
	missing, _ := body["missing"].([]any)
	if len(items) != 2 {
		t.Errorf("items %v", items)
	}
	if len(missing) != 2 {
		t.Errorf("missing %v", missing)
	}
	foundFav := false
	for _, raw := range items {
		it, _ := raw.(map[string]any)
		if strID(it["quiz_id"]) == idStr(g2QMain) && it["has_favorited"] == true {
			foundFav = true
		}
	}
	if !foundFav {
		t.Errorf("favorite not reported in items %+v", items)
	}
	resp, body = f.qz(t, http.MethodGet, "/api/v1/me/quiz-states", "/me/quiz-states", "sess-bob", "", nil)
	wantCode(t, resp, body, http.StatusBadRequest, problem.CodeInvalidParameter)
}

func TestV1PutQuizQualityRatingConcurrentRerates(t *testing.T) {
	f := newQuizFix(t, nil)
	var wg sync.WaitGroup
	for rating := 1; rating <= 8; rating++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, got := f.qz(t, http.MethodPut, "/api/v1/quizzes/"+idStr(g2QMain)+"/quality-rating",
				"/quizzes/{quiz_id}/quality-rating", "sess-bob", "", map[string]any{"rating": rating})
			if resp.StatusCode != http.StatusOK {
				t.Errorf("rate %d: %d %+v", rating, resp.StatusCode, got)
			}
		}()
	}
	wg.Wait()
	if n := f.scalar(t, `SELECT quality_count FROM galgame_quiz WHERE id = ?`, g2QMain); n != 1 {
		t.Errorf("quality_count %d after one answerer re-rated, want 1", n)
	}
	if n := f.scalar(t, `SELECT (q.quality_sum = a.quality_rating)::int FROM galgame_quiz q
		JOIN galgame_quiz_answer a ON a.quiz_id = q.id AND a.id = ? WHERE q.id = ?`, g2AnsBob, g2QMain); n != 1 {
		t.Error("quality_sum drifted from the only rating")
	}
}
