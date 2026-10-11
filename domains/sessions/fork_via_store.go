package sessions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrSessionNotFound is returned when a session is not in the store.
var ErrSessionNotFound = errors.New("sessions: session not found")

// ForkOptions refine ForkSessionViaStore. The zero value forks the whole transcript and
// derives the title.
type ForkOptions struct {
	// UpToMessageID, if set, ends the copy at the entry with this uuid, inclusive.
	UpToMessageID string
	// Title names the fork. Empty derives it from the source (custom title, AI title or
	// first prompt) with " (fork)" appended.
	Title string
}

var forkTranscriptTypes = map[string]bool{"user": true, "assistant": true, "attachment": true, "system": true, "progress": true}

// ForkSessionViaStore copies a session into a new one in the same store, the counterpart of
// upstream's fork_session_via_store, and returns the new session's id. Every uuid is
// remapped, parentUuid chains are kept (skipping progress entries, which are not copied),
// sessionId is rewritten, each entry records forkedFrom, sidechain entries are left out, and
// a custom-title entry names the fork. The transform runs in this process: copying the
// objects inside the storage layer would keep the old uuids and session id.
func ForkSessionViaStore(ctx context.Context, store SessionStore, projectKey, sessionID string, opts ForkOptions) (string, error) {
	if !sessionIDRE.MatchString(sessionID) {
		return "", fmt.Errorf("%w: %q", ErrInvalidSessionID, sessionID)
	}
	if opts.UpToMessageID != "" && !sessionIDRE.MatchString(opts.UpToMessageID) {
		return "", fmt.Errorf("%w: up to message %q", ErrInvalidSessionID, opts.UpToMessageID)
	}
	loaded, err := store.Load(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionID})
	if err != nil {
		return "", err
	}
	if len(loaded) == 0 {
		return "", fmt.Errorf("%w: %s", ErrSessionNotFound, sessionID)
	}

	var transcript []SessionStoreEntry
	var replacements []any
	for _, e := range loaded {
		typ, _ := e["type"].(string)
		if _, ok := e["uuid"].(string); ok && forkTranscriptTypes[typ] {
			if side, _ := e["isSidechain"].(bool); !side {
				transcript = append(transcript, e)
			}
		} else if typ == "content-replacement" && e["sessionId"] == sessionID {
			if r, ok := e["replacements"].([]any); ok {
				replacements = append(replacements, r...)
			}
		}
	}
	if len(transcript) == 0 {
		return "", fmt.Errorf("sessions: session %s has no messages to fork", sessionID)
	}
	if opts.UpToMessageID != "" {
		cut := -1
		for i, e := range transcript {
			if e["uuid"] == opts.UpToMessageID {
				cut = i
				break
			}
		}
		if cut < 0 {
			return "", fmt.Errorf("sessions: message %s not found in session %s", opts.UpToMessageID, sessionID)
		}
		transcript = transcript[:cut+1]
	}

	mapping := make(map[string]string, len(transcript))
	byUUID := make(map[string]SessionStoreEntry, len(transcript))
	var writable []SessionStoreEntry
	for _, e := range transcript {
		u := e["uuid"].(string)
		mapping[u] = newUUID()
		byUUID[u] = e
		if e["type"] != "progress" {
			writable = append(writable, e)
		}
	}
	if len(writable) == 0 {
		return "", fmt.Errorf("sessions: session %s has no messages to fork", sessionID)
	}

	forkID := newUUID()
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	out := make([]SessionStoreEntry, 0, len(writable)+2)
	for i, orig := range writable {
		var parent any
		for pid, _ := orig["parentUuid"].(string); pid != ""; {
			p, ok := byUUID[pid]
			if !ok {
				break
			}
			if p["type"] != "progress" {
				parent = mapping[pid]
				break
			}
			pid, _ = p["parentUuid"].(string)
		}
		forked := make(SessionStoreEntry, len(orig)+4)
		for k, v := range orig {
			forked[k] = v
		}
		ts := orig["timestamp"]
		if i == len(writable)-1 || ts == nil {
			ts = now
		}
		forked["uuid"] = mapping[orig["uuid"].(string)]
		forked["parentUuid"] = parent
		if lp, ok := orig["logicalParentUuid"].(string); ok && lp != "" {
			if m, found := mapping[lp]; found {
				forked["logicalParentUuid"] = m
			} else {
				forked["logicalParentUuid"] = nil
			}
		}
		forked["sessionId"] = forkID
		forked["timestamp"] = ts
		forked["isSidechain"] = false
		forked["forkedFrom"] = map[string]any{"sessionId": sessionID, "messageUuid": orig["uuid"]}
		for _, k := range []string{"teamName", "agentName", "slug", "sourceToolAssistantUUID"} {
			delete(forked, k)
		}
		out = append(out, forked)
	}
	if len(replacements) > 0 {
		out = append(out, SessionStoreEntry{
			"type": "content-replacement", "sessionId": forkID, "replacements": replacements,
			"uuid": newUUID(), "timestamp": now,
		})
	}

	title := strings.TrimSpace(opts.Title)
	if title == "" {
		derived := "Forked session"
		sum := FoldSessionSummary(nil, SessionKey{ProjectKey: projectKey, SessionID: sessionID}, loaded)
		if info, ok := SummaryToSessionInfo(sum, ""); ok {
			if derived = info.CustomTitle; derived == "" {
				derived = info.FirstPrompt
			}
			if derived == "" {
				derived = "Forked session"
			}
		}
		title = derived + " (fork)"
	}
	out = append(out, SessionStoreEntry{
		"type": "custom-title", "sessionId": forkID, "customTitle": title,
		"uuid": newUUID(), "timestamp": now,
	})

	if err := store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: forkID}, out); err != nil {
		return "", err
	}
	return forkID, nil
}
