package sessions

import "context"

var transcriptEntryTypes = map[string]bool{"user": true, "assistant": true, "progress": true, "system": true, "attachment": true}

// GetSessionMessagesFromStore reads a session's conversation from a SessionStore: the
// user and assistant messages on the main chain, oldest first.
//
// The chain is the one the CLI would resume. It is found the way upstream's
// get_session_messages_from_store finds it: the transcript entries with a uuid are indexed,
// the entries nothing names as a parent are the terminals, the newest terminal that is
// (or leads back to) a user or assistant entry outside a sidechain, team or meta entry is
// the leaf, and the chain is that leaf's parentUuid links back to the root. Edits and
// rewinds leave abandoned branches in the transcript, and those are not returned. Meta,
// sidechain and team entries are left out; compact summaries are kept, being the only copy
// of what they summarise.
//
// limit <= 0 means no limit, offset <= 0 means none. A session that is absent or has no
// visible message gives an empty result and no error; a Load error is returned.
func GetSessionMessagesFromStore(ctx context.Context, store SessionStore, projectKey, sessionID string, limit, offset int) ([]SessionMessage, error) {
	entries, err := store.Load(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionID})
	if err != nil {
		return nil, err
	}
	var chain []SessionStoreEntry
	for _, e := range conversationChain(entries) {
		if !visibleMessage(e) {
			continue
		}
		chain = append(chain, e)
	}
	var out []SessionMessage
	for _, e := range paginate(chain, limit, offset) {
		typ, _ := e["type"].(string)
		uuid, _ := e["uuid"].(string)
		sid, _ := e["sessionId"].(string)
		out = append(out, SessionMessage{Type: typ, UUID: uuid, SessionID: sid, Message: e["message"]})
	}
	return out, nil
}

func visibleMessage(e SessionStoreEntry) bool {
	if t, _ := e["type"].(string); t != "user" && t != "assistant" {
		return false
	}
	if e["isMeta"] == true || e["isSidechain"] == true {
		return false
	}
	team, _ := e["teamName"].(string)
	return team == ""
}

func mainChainEntry(e SessionStoreEntry) bool {
	team, _ := e["teamName"].(string)
	return e["isSidechain"] != true && team == "" && e["isMeta"] != true
}

// conversationChain returns the root-to-leaf chain of transcript entries, or nil.
func conversationChain(all []SessionStoreEntry) []SessionStoreEntry {
	var entries []SessionStoreEntry
	for _, e := range all {
		typ, _ := e["type"].(string)
		if _, ok := e["uuid"].(string); ok && transcriptEntryTypes[typ] {
			entries = append(entries, e)
		}
	}
	if len(entries) == 0 {
		return nil
	}
	byUUID := make(map[string]SessionStoreEntry, len(entries))
	index := make(map[string]int, len(entries))
	for i, e := range entries {
		id := e["uuid"].(string)
		byUUID[id], index[id] = e, i
	}
	parents := map[string]bool{}
	for _, e := range entries {
		if p, _ := e["parentUuid"].(string); p != "" {
			parents[p] = true
		}
	}
	parentOf := func(e SessionStoreEntry) SessionStoreEntry {
		p, _ := e["parentUuid"].(string)
		if p == "" {
			return nil
		}
		return byUUID[p]
	}
	var leaves []SessionStoreEntry
	for _, e := range entries {
		if parents[e["uuid"].(string)] {
			continue
		}
		seen := map[string]bool{}
		for cur := e; cur != nil; cur = parentOf(cur) {
			id := cur["uuid"].(string)
			if seen[id] {
				break
			}
			seen[id] = true
			if t, _ := cur["type"].(string); t == "user" || t == "assistant" {
				leaves = append(leaves, cur)
				break
			}
		}
	}
	if len(leaves) == 0 {
		return nil
	}
	best := func(c []SessionStoreEntry) SessionStoreEntry {
		b := c[0]
		for _, e := range c[1:] {
			if index[e["uuid"].(string)] > index[b["uuid"].(string)] {
				b = e
			}
		}
		return b
	}
	var main []SessionStoreEntry
	for _, l := range leaves {
		if mainChainEntry(l) {
			main = append(main, l)
		}
	}
	leaf := best(leaves)
	if len(main) > 0 {
		leaf = best(main)
	}
	var chain []SessionStoreEntry
	seen := map[string]bool{}
	for cur := leaf; cur != nil; cur = parentOf(cur) {
		id := cur["uuid"].(string)
		if seen[id] {
			break
		}
		seen[id] = true
		chain = append(chain, cur)
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}
