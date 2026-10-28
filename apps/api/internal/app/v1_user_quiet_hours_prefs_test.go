package app

import (
	"fmt"
	"net/http"
	"testing"
)

func (f *meFix) putQuiet(t *testing.T, session string, quiet *bool) (*http.Response, map[string]any) {
	t.Helper()
	body := map[string]any{"muted_types": []string{}}
	if quiet != nil {
		body["is_quiet_hours_enabled"] = *quiet
	}
	return f.call(t, http.MethodPut, mePath+"/notification-preferences", "/me/notification-preferences", session, "", nil, body)
}

func TestV1QuietHoursDefaultsOn(t *testing.T) {
	f := newMeFix(t)
	resp, body := f.getMuted(t, "sess-alice")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get prefs %d %+v", resp.StatusCode, body)
	}
	if body["is_quiet_hours_enabled"] != true {
		t.Fatalf("is_quiet_hours_enabled %v, want true", body["is_quiet_hours_enabled"])
	}
}

func TestV1QuietHoursDefaultsOnWithoutStateRow(t *testing.T) {
	f := newMeFix(t)
	f.execSQL(t, `DELETE FROM kungal_user_state WHERE user_id = ?`, w3UserAlice)
	resp, body := f.getMuted(t, "sess-alice")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get prefs %d %+v", resp.StatusCode, body)
	}
	if body["is_quiet_hours_enabled"] != true {
		t.Fatalf("is_quiet_hours_enabled %v, want true", body["is_quiet_hours_enabled"])
	}
}

func TestV1QuietHoursToggleRoundTrip(t *testing.T) {
	f := newMeFix(t)
	t.Cleanup(func() {
		f.db.Exec(`UPDATE kungal_user_state SET quiet_hours_enabled = true, muted_notification_types = '[]' WHERE user_id = ?`, w3UserAlice)
	})

	off := false
	resp, body := f.putQuiet(t, "sess-alice", &off)
	if resp.StatusCode != http.StatusOK || body["is_quiet_hours_enabled"] != false {
		t.Fatalf("turn off %d %+v", resp.StatusCode, body)
	}
	resp, body = f.getMuted(t, "sess-alice")
	if resp.StatusCode != http.StatusOK || body["is_quiet_hours_enabled"] != false {
		t.Fatalf("read back off %d %+v", resp.StatusCode, body)
	}
	var stored bool
	if err := f.db.Raw(`SELECT quiet_hours_enabled FROM kungal_user_state WHERE user_id = ?`, w3UserAlice).Scan(&stored).Error; err != nil {
		t.Fatalf("stored quiet: %v", err)
	}
	if stored {
		t.Fatalf("stored is_quiet_hours_enabled = true, want false")
	}

	on := true
	resp, body = f.putQuiet(t, "sess-alice", &on)
	if resp.StatusCode != http.StatusOK || body["is_quiet_hours_enabled"] != true {
		t.Fatalf("turn on %d %+v", resp.StatusCode, body)
	}
}

func TestV1QuietHoursAbsentFieldKeepsStored(t *testing.T) {
	f := newMeFix(t)
	t.Cleanup(func() {
		f.db.Exec(`UPDATE kungal_user_state SET quiet_hours_enabled = true, muted_notification_types = '[]' WHERE user_id = ?`, w3UserAlice)
	})

	off := false
	if resp, body := f.putQuiet(t, "sess-alice", &off); resp.StatusCode != http.StatusOK {
		t.Fatalf("turn off %d %+v", resp.StatusCode, body)
	}
	// Old-client request: muted_types only, no is_quiet_hours_enabled.
	resp, body := f.putQuiet(t, "sess-alice", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("legacy put %d %+v", resp.StatusCode, body)
	}
	if body["is_quiet_hours_enabled"] != false {
		t.Fatalf("is_quiet_hours_enabled %v, want false (preserved)", body["is_quiet_hours_enabled"])
	}
	if got := fmt.Sprint(body["muted_types"]); got != "[]" {
		t.Fatalf("muted_types %v, want []", got)
	}
}

func TestV1SingleMuteEndpointReturnsQuietFlag(t *testing.T) {
	f := newMeFix(t)
	t.Cleanup(func() {
		f.db.Exec(`UPDATE kungal_user_state SET quiet_hours_enabled = true, muted_notification_types = '[]' WHERE user_id = ?`, w3UserAlice)
	})
	off := false
	if resp, body := f.putQuiet(t, "sess-alice", &off); resp.StatusCode != http.StatusOK {
		t.Fatalf("turn off %d %+v", resp.StatusCode, body)
	}
	resp, body := f.setMuted(t, http.MethodPut, "sess-alice", "system_notice")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("mute %d %+v", resp.StatusCode, body)
	}
	if body["is_quiet_hours_enabled"] != false {
		t.Fatalf("is_quiet_hours_enabled %v, want false echoed", body["is_quiet_hours_enabled"])
	}
	if fmt.Sprint(body["muted_types"]) != "[system_notice]" {
		t.Fatalf("muted_types %v", body["muted_types"])
	}
}
