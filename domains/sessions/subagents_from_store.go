package sessions

import (
	"context"
	"errors"
	"strings"
)

// ErrSubkeysUnsupported is returned by ListSubagentsFromStore for a store that is not a
// SubkeyLister, and so cannot say which sub-transcripts a session has.
var ErrSubkeysUnsupported = errors.New("sessions: the store does not implement SubkeyLister, so it cannot list subagents")

// ListSubagentsFromStore lists the IDs of the subagents a session ran, from the
// "subagents/.../agent-<id>" sub-transcripts the store holds, in the order the store lists
// them and each ID once. A subagent nested under "subagents/workflows/<run>/" is included.
// An invalid session ID, or a session with no subagents, gives an empty result.
func ListSubagentsFromStore(ctx context.Context, store SessionStore, projectKey, sessionID string) ([]string, error) {
	if !sessionIDRE.MatchString(sessionID) {
		return []string{}, nil
	}
	lister, ok := store.(SubkeyLister)
	if !ok {
		return nil, ErrSubkeysUnsupported
	}
	subkeys, err := lister.ListSubkeys(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionID})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	ids := []string{}
	for _, sub := range subkeys {
		if !strings.HasPrefix(sub, "subagents/") {
			continue
		}
		last := sub[strings.LastIndex(sub, "/")+1:]
		id, ok := strings.CutPrefix(last, "agent-")
		if ok && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// GetSubagentMessagesFromStore reads one subagent's conversation from a SessionStore, oldest
// first. A store that is a SubkeyLister is searched for the sub-transcript, wherever under
// "subagents/" it sits; any other store is asked for "subagents/agent-<id>" directly.
//
// A subagent's transcript is linear, so the leaf is its last user or assistant entry and the
// messages are that leaf's parentUuid links back to the root. Every message carries the
// subagent's ParentToolUseID and ParentAgentID, taken from the last "agent_metadata" entry
// the store holds for it (that entry is the store's copy of the CLI's .meta.json sidecar and
// is not a message). limit <= 0 means no limit, offset <= 0 means none. A session or
// subagent that is absent gives an empty result and no error.
func GetSubagentMessagesFromStore(ctx context.Context, store SessionStore, projectKey, sessionID, agentID string, limit, offset int) ([]SessionMessage, error) {
	if !sessionIDRE.MatchString(sessionID) || agentID == "" {
		return nil, nil
	}
	subpath := "subagents/agent-" + agentID
	if lister, ok := store.(SubkeyLister); ok {
		subkeys, err := lister.ListSubkeys(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionID})
		if err != nil {
			return nil, err
		}
		found := ""
		for _, sub := range subkeys {
			if strings.HasPrefix(sub, "subagents/") && sub[strings.LastIndex(sub, "/")+1:] == "agent-"+agentID {
				found = sub
				break
			}
		}
		if found == "" {
			return nil, nil
		}
		subpath = found
	}
	entries, err := store.Load(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionID, Subpath: subpath})
	if err != nil {
		return nil, err
	}

	var meta SessionStoreEntry
	var transcript []SessionStoreEntry
	for _, e := range entries {
		if e["type"] == "agent_metadata" {
			meta = e
			continue
		}
		if t, _ := e["type"].(string); transcriptEntryTypes[t] {
			if _, ok := e["uuid"].(string); ok {
				transcript = append(transcript, e)
			}
		}
	}
	chain := subagentChain(transcript)
	var toolUseID, parentAgentID interface{}
	if s, ok := meta["toolUseId"].(string); ok {
		toolUseID = s
	}
	if s, ok := meta["parentAgentId"].(string); ok {
		parentAgentID = s
	}
	var out []SessionMessage
	for _, e := range paginate(chain, limit, offset) {
		typ, _ := e["type"].(string)
		uuid, _ := e["uuid"].(string)
		sid, _ := e["sessionId"].(string)
		out = append(out, SessionMessage{
			Type: typ, UUID: uuid, SessionID: sid, Message: e["message"],
			ParentToolUseID: toolUseID, ParentAgentID: parentAgentID,
		})
	}
	return out, nil
}

// subagentChain is the user and assistant entries from the last of them back along
// parentUuid to the root, oldest first. A parent cycle ends the walk.
func subagentChain(entries []SessionStoreEntry) []SessionStoreEntry {
	byUUID := map[string]SessionStoreEntry{}
	for _, e := range entries {
		byUUID[e["uuid"].(string)] = e
	}
	var leaf SessionStoreEntry
	for i := len(entries) - 1; i >= 0; i-- {
		if t := entries[i]["type"]; t == "user" || t == "assistant" {
			leaf = entries[i]
			break
		}
	}
	var chain []SessionStoreEntry
	seen := map[string]bool{}
	for cur := leaf; cur != nil; {
		uid := cur["uuid"].(string)
		if seen[uid] {
			break
		}
		seen[uid] = true
		if t := cur["type"]; t == "user" || t == "assistant" {
			chain = append(chain, cur)
		}
		parent, _ := cur["parentUuid"].(string)
		cur = byUUID[parent]
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}
