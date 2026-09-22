package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/rohithgilla12/openmind/api/internal/api"
	"github.com/rohithgilla12/openmind/api/internal/store/db"
)

type settingsResp struct {
	KindleEmail            *string `json:"kindleEmail"`
	AiAssistedOrganisation *bool   `json:"aiAssistedOrganisation"`
	NotifyDigest           *string `json:"notifyDigest"`
	NotifyFeedRiver        *string `json:"notifyFeedRiver"`
	NotifyLifecycle        *string `json:"notifyLifecycle"`
	NotifyQuietHours       *string `json:"notifyQuietHours"`
	NotifyTimezone         *string `json:"notifyTimezone"`
	NotifyDailyCap         *int    `json:"notifyDailyCap"`
}

func getSettings(t *testing.T, url string) settingsResp {
	t.Helper()
	resp, err := http.Get(url + "/settings")
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get settings status = %d, want 200", resp.StatusCode)
	}
	var out settingsResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	return out
}

// TestSettingsRoundTrip covers the full lifecycle: empty by default, set via
// PATCH (echoed and persisted), then cleared via an explicit empty string.
func TestSettingsRoundTrip(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	empty := getSettings(t, srv.URL)
	if empty.KindleEmail != nil {
		t.Fatalf("initial kindleEmail = %v, want absent", *empty.KindleEmail)
	}

	patch := doJSON(t, http.MethodPatch, srv.URL+"/settings", `{"kindleEmail":"me@kindle.com"}`)
	if patch.StatusCode != http.StatusOK {
		t.Fatalf("patch status = %d, want 200", patch.StatusCode)
	}
	var patched settingsResp
	if err := json.NewDecoder(patch.Body).Decode(&patched); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	patch.Body.Close()
	if patched.KindleEmail == nil || *patched.KindleEmail != "me@kindle.com" {
		t.Fatalf("patched echo = %v, want me@kindle.com", patched.KindleEmail)
	}

	got := getSettings(t, srv.URL)
	if got.KindleEmail == nil || *got.KindleEmail != "me@kindle.com" {
		t.Fatalf("get after patch = %v, want me@kindle.com", got.KindleEmail)
	}

	clear := doJSON(t, http.MethodPatch, srv.URL+"/settings", `{"kindleEmail":""}`)
	if clear.StatusCode != http.StatusOK {
		t.Fatalf("clear status = %d, want 200", clear.StatusCode)
	}
	var cleared settingsResp
	if err := json.NewDecoder(clear.Body).Decode(&cleared); err != nil {
		t.Fatalf("decode clear: %v", err)
	}
	clear.Body.Close()
	if cleared.KindleEmail != nil {
		t.Fatalf("cleared echo = %v, want absent", *cleared.KindleEmail)
	}

	final := getSettings(t, srv.URL)
	if final.KindleEmail != nil {
		t.Fatalf("final kindleEmail = %v, want absent", *final.KindleEmail)
	}
}

// TestSettingsValidation asserts a malformed address is rejected, while
// leaving the field absent is a no-op that still returns 200.
func TestSettingsValidation(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	bad := doJSON(t, http.MethodPatch, srv.URL+"/settings", `{"kindleEmail":"notanemail"}`)
	defer bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Errorf("bad email status = %d, want 400", bad.StatusCode)
	}

	noop := doJSON(t, http.MethodPatch, srv.URL+"/settings", `{}`)
	defer noop.Body.Close()
	if noop.StatusCode != http.StatusOK {
		t.Errorf("absent field status = %d, want 200", noop.StatusCode)
	}

	// Address itself well-formed, but exceeds RFC 5321's 254-octet limit.
	longLocal := strings.Repeat("a", 250)
	tooLong := doJSON(t, http.MethodPatch, srv.URL+"/settings", `{"kindleEmail":"`+longLocal+`@example.com"}`)
	defer tooLong.Body.Close()
	if tooLong.StatusCode != http.StatusBadRequest {
		t.Errorf("over-length email status = %d, want 400", tooLong.StatusCode)
	}
}

// TestSettingsScoped asserts one user's kindle_email setting is invisible to
// another user — settings are per-user like every other table. The HTTP
// harness only authenticates as the dev user, so user B's setting is seeded
// directly through the store (mirrors seedOtherUserItem in server_test.go);
// the dev user's GET must still only reflect their own value.
func TestSettingsScoped(t *testing.T) {
	s, rc, _ := testDeps(t)
	ctx := t.Context()
	other := uuid.MustParse("00000000-0000-0000-0000-0000000000ff")
	if err := s.Queries.EnsureUser(ctx, other); err != nil {
		t.Fatalf("ensure other user: %v", err)
	}
	if err := s.Queries.UpsertUserSetting(ctx, db.UpsertUserSettingParams{UserID: other, Key: "kindle_email", Value: "other@kindle.com"}); err != nil {
		t.Fatalf("seed other user setting: %v", err)
	}

	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	patch := doJSON(t, http.MethodPatch, srv.URL+"/settings", `{"kindleEmail":"dev@kindle.com"}`)
	patch.Body.Close()

	got := getSettings(t, srv.URL)
	if got.KindleEmail == nil || *got.KindleEmail != "dev@kindle.com" {
		t.Fatalf("dev settings = %v, want dev@kindle.com", got.KindleEmail)
	}

	otherVal, err := s.Queries.GetUserSetting(ctx, db.GetUserSettingParams{UserID: other, Key: "kindle_email"})
	if err != nil {
		t.Fatalf("get other user setting: %v", err)
	}
	if otherVal != "other@kindle.com" {
		t.Errorf("other user setting = %q, want unaffected other@kindle.com", otherVal)
	}
}

// TestGetSettingsDoesNotPersistDefaults asserts that fetching settings for a
// user who has never set a preference does not write user_settings rows. An
// absent row is what lets the documented default (notify.digest=push, etc.)
// apply without a backfill; if GET silently wrote default rows on read,
// upgrading an existing install would no longer be free.
func TestGetSettingsDoesNotPersistDefaults(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	got := getSettings(t, srv.URL)
	if got.NotifyDigest != nil || got.NotifyFeedRiver != nil || got.NotifyLifecycle != nil ||
		got.NotifyQuietHours != nil || got.NotifyTimezone != nil || got.NotifyDailyCap != nil {
		t.Fatalf("fresh settings = %+v, want all notify fields absent", got)
	}

	rows, err := s.Queries.ListUserSettings(t.Context(), api.DevUserID)
	if err != nil {
		t.Fatalf("list user settings: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("user_settings rows after GET = %d, want 0 (GET must not persist defaults)", len(rows))
	}
}

// TestNotificationPrefsRoundTrip covers setting all six notify.* preferences
// in one PATCH, echoed back correctly, and reflected by a subsequent GET.
func TestNotificationPrefsRoundTrip(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	body := `{"notifyDigest":"both","notifyFeedRiver":"push","notifyQuietHours":"22:00-07:00","notifyTimezone":"Europe/London","notifyDailyCap":5}`
	patch := doJSON(t, http.MethodPatch, srv.URL+"/settings", body)
	defer patch.Body.Close()
	if patch.StatusCode != http.StatusOK {
		t.Fatalf("patch status = %d, want 200", patch.StatusCode)
	}
	var got settingsResp
	if err := json.NewDecoder(patch.Body).Decode(&got); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	assertPrefs(t, "patch echo", got, "both", "push", "22:00-07:00", "Europe/London", 5)

	got = getSettings(t, srv.URL)
	assertPrefs(t, "get after patch", got, "both", "push", "22:00-07:00", "Europe/London", 5)
}

func assertPrefs(t *testing.T, label string, got settingsResp, digest, feedRiver, quietHours, timezone string, dailyCap int) {
	t.Helper()
	if got.NotifyDigest == nil || *got.NotifyDigest != digest {
		t.Errorf("%s: NotifyDigest = %v, want %s", label, got.NotifyDigest, digest)
	}
	if got.NotifyFeedRiver == nil || *got.NotifyFeedRiver != feedRiver {
		t.Errorf("%s: NotifyFeedRiver = %v, want %s", label, got.NotifyFeedRiver, feedRiver)
	}
	if got.NotifyQuietHours == nil || *got.NotifyQuietHours != quietHours {
		t.Errorf("%s: NotifyQuietHours = %v, want %s", label, got.NotifyQuietHours, quietHours)
	}
	if got.NotifyTimezone == nil || *got.NotifyTimezone != timezone {
		t.Errorf("%s: NotifyTimezone = %v, want %s", label, got.NotifyTimezone, timezone)
	}
	if got.NotifyDailyCap == nil || *got.NotifyDailyCap != dailyCap {
		t.Errorf("%s: NotifyDailyCap = %v, want %d", label, got.NotifyDailyCap, dailyCap)
	}
}

// TestPatchSettingsRejectsBadQuietHours asserts a malformed quiet-hours
// string is rejected with 400 and never reaches the store.
func TestPatchSettingsRejectsBadQuietHours(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	resp := doJSON(t, http.MethodPatch, srv.URL+"/settings", `{"notifyQuietHours":"bedtime"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// TestPatchSettingsRejectsBadTimezone asserts an unknown IANA zone is
// rejected with 400.
func TestPatchSettingsRejectsBadTimezone(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	resp := doJSON(t, http.MethodPatch, srv.URL+"/settings", `{"notifyTimezone":"Mars/Olympus"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// TestPatchSettingsRejectsLocalTimezone is the regression test for M7:
// time.LoadLocation("Local") succeeds and resolves to the server process's
// own system timezone, not the user's, so it must be rejected explicitly
// rather than accepted as if it were a real IANA zone.
func TestPatchSettingsRejectsLocalTimezone(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	resp := doJSON(t, http.MethodPatch, srv.URL+"/settings", `{"notifyTimezone":"Local"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// TestPatchSettingsRejectsOutOfRangeDailyCap asserts a cap outside [0, 200]
// is rejected with 400 before any write happens.
func TestPatchSettingsRejectsOutOfRangeDailyCap(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	resp := doJSON(t, http.MethodPatch, srv.URL+"/settings", `{"notifyDailyCap":500}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

// TestPatchSettingsRejectsBeforeWritingAnyField asserts that when a request
// body mixes a valid field with an invalid one, nothing is written: the
// valid field (notifyDigest, processed earlier) must not have been persisted
// just because a later field (notifyTimezone) failed validation. Validating
// every field up front, before any store call, is what the global "no
// partially-applied PATCH" rule requires.
func TestPatchSettingsRejectsBeforeWritingAnyField(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	resp := doJSON(t, http.MethodPatch, srv.URL+"/settings", `{"notifyDigest":"both","notifyTimezone":"Mars/Olympus"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}

	got := getSettings(t, srv.URL)
	if got.NotifyDigest != nil {
		t.Errorf("notifyDigest = %v, want untouched (absent) since the whole PATCH was rejected", *got.NotifyDigest)
	}
}

// TestPatchSettingsOmittedFieldLeavesUntouched is the crux of the PATCH
// contract: a field absent from the request body must be left exactly as it
// was, while a field present as an explicit empty string must be cleared
// back to its default. A second PATCH that only sets notifyFeedRiver must
// not disturb notifyDigest or notifyTimezone set by the first; a third PATCH
// that explicitly clears notifyDigest must not disturb notifyTimezone.
func TestPatchSettingsOmittedFieldLeavesUntouched(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	first := doJSON(t, http.MethodPatch, srv.URL+"/settings", `{"notifyDigest":"email","notifyTimezone":"Europe/London"}`)
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first patch status = %d, want 200", first.StatusCode)
	}

	second := doJSON(t, http.MethodPatch, srv.URL+"/settings", `{"notifyFeedRiver":"push"}`)
	defer second.Body.Close()
	if second.StatusCode != http.StatusOK {
		t.Fatalf("second patch status = %d, want 200", second.StatusCode)
	}
	var afterSecond settingsResp
	if err := json.NewDecoder(second.Body).Decode(&afterSecond); err != nil {
		t.Fatalf("decode second patch: %v", err)
	}
	if afterSecond.NotifyDigest == nil || *afterSecond.NotifyDigest != "email" {
		t.Errorf("after omitting notifyDigest: got %v, want untouched email", afterSecond.NotifyDigest)
	}
	if afterSecond.NotifyTimezone == nil || *afterSecond.NotifyTimezone != "Europe/London" {
		t.Errorf("after omitting notifyTimezone: got %v, want untouched Europe/London", afterSecond.NotifyTimezone)
	}
	if afterSecond.NotifyFeedRiver == nil || *afterSecond.NotifyFeedRiver != "push" {
		t.Errorf("NotifyFeedRiver = %v, want push", afterSecond.NotifyFeedRiver)
	}

	third := doJSON(t, http.MethodPatch, srv.URL+"/settings", `{"notifyDigest":""}`)
	defer third.Body.Close()
	if third.StatusCode != http.StatusOK {
		t.Fatalf("third patch status = %d, want 200", third.StatusCode)
	}
	var afterThird settingsResp
	if err := json.NewDecoder(third.Body).Decode(&afterThird); err != nil {
		t.Fatalf("decode third patch: %v", err)
	}
	if afterThird.NotifyDigest != nil {
		t.Errorf("NotifyDigest after explicit clear = %v, want absent", *afterThird.NotifyDigest)
	}
	if afterThird.NotifyTimezone == nil || *afterThird.NotifyTimezone != "Europe/London" {
		t.Errorf("NotifyTimezone after unrelated clear = %v, want untouched Europe/London", afterThird.NotifyTimezone)
	}
}

// TestSettingsAIAssistedOrganisationRoundTrip covers the opt-in Jev gate:
// absent by default, set true via PATCH, cleared back to absent via false.
func TestSettingsAIAssistedOrganisationRoundTrip(t *testing.T) {
	s, rc, _ := testDeps(t)
	srv := httptest.NewServer(newSrv(t, s, rc, ""))
	t.Cleanup(srv.Close)

	empty := getSettings(t, srv.URL)
	if empty.AiAssistedOrganisation != nil && *empty.AiAssistedOrganisation {
		t.Fatalf("initial aiAssistedOrganisation = true, want absent/false")
	}

	on := doJSON(t, http.MethodPatch, srv.URL+"/settings", `{"aiAssistedOrganisation":true}`)
	defer on.Body.Close()
	if on.StatusCode != http.StatusOK {
		t.Fatalf("patch on status = %d, want 200", on.StatusCode)
	}
	var patched settingsResp
	if err := json.NewDecoder(on.Body).Decode(&patched); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if patched.AiAssistedOrganisation == nil || !*patched.AiAssistedOrganisation {
		t.Fatalf("patched = %v, want true", patched.AiAssistedOrganisation)
	}

	got := getSettings(t, srv.URL)
	if got.AiAssistedOrganisation == nil || !*got.AiAssistedOrganisation {
		t.Fatalf("get after on = %v, want true", got.AiAssistedOrganisation)
	}

	off := doJSON(t, http.MethodPatch, srv.URL+"/settings", `{"aiAssistedOrganisation":false}`)
	defer off.Body.Close()
	if off.StatusCode != http.StatusOK {
		t.Fatalf("patch off status = %d, want 200", off.StatusCode)
	}
	final := getSettings(t, srv.URL)
	if final.AiAssistedOrganisation != nil && *final.AiAssistedOrganisation {
		t.Fatalf("final = %v, want absent/false", final.AiAssistedOrganisation)
	}
}
