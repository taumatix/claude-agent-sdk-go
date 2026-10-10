package sessions

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

var sessionIDRE = regexp.MustCompile(`^(?i:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

// ErrInvalidSessionID is returned when a session ID is not a UUID.
var ErrInvalidSessionID = errors.New("sessions: invalid session id")

// RenameSessionViaStore renames a session by appending a custom-title entry to store, the
// counterpart of upstream's rename_session_via_store. Surrounding whitespace is trimmed and
// the title must not be empty after that. The new title shows in ListSessionsFromStore and
// GetSessionInfoFromStore as soon as the append returns.
func RenameSessionViaStore(ctx context.Context, store SessionStore, projectKey, sessionID, title string) error {
	if !sessionIDRE.MatchString(sessionID) {
		return fmt.Errorf("%w: %q", ErrInvalidSessionID, sessionID)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("sessions: title must be non-empty")
	}
	return appendMeta(ctx, store, projectKey, sessionID, "custom-title", "customTitle", title)
}

// TagSessionViaStore tags a session by appending a tag entry to store; a nil tag clears it.
// The tag is stripped of format, private-use and unassigned characters and trimmed, and must
// not be empty after that. Upstream also applies NFKC normalisation first; this does not,
// because it would need golang.org/x/text, so a compatibility form such as a full-width
// letter is stored as given.
func TagSessionViaStore(ctx context.Context, store SessionStore, projectKey, sessionID string, tag *string) error {
	if !sessionIDRE.MatchString(sessionID) {
		return fmt.Errorf("%w: %q", ErrInvalidSessionID, sessionID)
	}
	value := ""
	if tag != nil {
		value = strings.TrimSpace(sanitizeUnicode(*tag))
		if value == "" {
			return errors.New("sessions: tag must be non-empty (pass nil to clear it)")
		}
	}
	return appendMeta(ctx, store, projectKey, sessionID, "tag", "tag", value)
}

// DeleteSessionViaStore deletes a session, and with it the store's sub-transcripts of that
// session if its Delete does so. A store that is not a SessionDeleter (an append-only one)
// is left alone and the call succeeds, as upstream's does; the second result reports
// whether anything was asked of the store.
func DeleteSessionViaStore(ctx context.Context, store SessionStore, projectKey, sessionID string) (bool, error) {
	if !sessionIDRE.MatchString(sessionID) {
		return false, fmt.Errorf("%w: %q", ErrInvalidSessionID, sessionID)
	}
	d, ok := store.(SessionDeleter)
	if !ok {
		return false, nil
	}
	return true, d.Delete(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionID})
}

func appendMeta(ctx context.Context, store SessionStore, projectKey, sessionID, typ, field, value string) error {
	return store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionID}, []SessionStoreEntry{{
		"type":      typ,
		field:       value,
		"sessionId": sessionID,
		"uuid":      newUUID(),
		"timestamp": time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
	}})
}

// sanitizeUnicode drops the characters upstream strips from a tag: format (Cf), private-use
// (Co) and unassigned code points, plus the explicit bidi and zero-width ranges.
func sanitizeUnicode(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case unicode.Is(unicode.Cf, r), unicode.Is(unicode.Co, r),
			!unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S, unicode.Z, unicode.Cc, unicode.Cs),
			r >= 0x200b && r <= 0x200f, r >= 0x202a && r <= 0x202e,
			r >= 0x2066 && r <= 0x2069, r == 0xfeff:
			return -1
		}
		return r
	}, s)
}
