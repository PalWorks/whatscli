package messages

import (
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"google.golang.org/protobuf/proto"
)

func waCommonKey(id string) *waCommon.MessageKey {
	return &waCommon.MessageKey{ID: proto.String(id), FromMe: proto.Bool(true)}
}

func TestRegistry_MessageCommands(t *testing.T) {
	for _, name := range []string{"download", "open", "show", "url", "revoke", "backlog", "info"} {
		if _, ok := commandRegistry[name]; !ok {
			t.Errorf("command %q is not registered", name)
		}
	}
}

func TestCmdRevoke(t *testing.T) {
	tests := []struct {
		name    string
		msg     *Message
		params  []string
		wantErr string // substring of the reported error, "" for usage text
	}{
		{"no params prints usage", nil, nil, ""},
		{"unknown id", nil, []string{"NOPE"}, "not found"},
		{"others' message rejected", &Message{Id: "THEIRS", ChatId: "c@s.whatsapp.net", FromMe: false, Text: "hi"}, []string{"THEIRS"}, ErrRevokeNotOwn.Error()},
		{"others' message in lid chat rejected", &Message{Id: "LID1", ChatId: "123@lid", FromMe: false, Text: "hi"}, []string{"LID1"}, ErrRevokeNotOwn.Error()},
		{"already revoked", &Message{Id: "GONE", ChatId: "c@s.whatsapp.net", FromMe: true, Text: RevokedText}, []string{"GONE"}, "already revoked"},
		{"own message but offline", &Message{Id: "MINE", ChatId: "c@s.whatsapp.net", FromMe: true, Text: "hi"}, []string{"MINE"}, "not connected"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, mock := newTestSessionManager(t)
			if tt.msg != nil {
				addTestMsg(t, sm.db, *tt.msg)
			}
			cmdRevoke(sm, nil, "revoke", tt.params)
			if tt.wantErr == "" {
				if len(mock.Errors) != 0 || len(mock.Texts) == 0 || !strings.Contains(mock.Texts[0], "Usage") {
					t.Fatalf("expected usage text, got texts=%v errors=%v", mock.Texts, mock.Errors)
				}
				return
			}
			if len(mock.Errors) != 1 || !strings.Contains(mock.Errors[0].Error(), tt.wantErr) {
				t.Fatalf("errors = %v, want one containing %q", mock.Errors, tt.wantErr)
			}
			if tt.msg != nil {
				got, _, _ := sm.db.GetMessage(tt.msg.Id)
				if got.Text != tt.msg.Text {
					t.Errorf("message text changed to %q", got.Text)
				}
			}
		})
	}
}

func TestCmdDownloadValidation(t *testing.T) {
	doc := docMsg(t, "DOC1", "a.pdf")
	tests := []struct {
		name    string
		cmd     string
		msg     *Message
		params  []string
		wantErr string
	}{
		{"text message is not downloadable", "download", &Message{Id: "TXT", ChatId: "c@s.whatsapp.net", Text: "hi"}, []string{"TXT"}, ErrNotDownloadable.Error()},
		{"show requires image", "show", &doc, []string{"DOC1"}, "only works for image"},
		{"offline", "open", &doc, []string{"DOC1"}, "not connected"},
		{"unknown id", "download", nil, []string{"NOPE"}, "not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, mock := newTestSessionManager(t)
			if tt.msg != nil {
				addTestMsg(t, sm.db, *tt.msg)
			}
			commandRegistry[tt.cmd](sm, nil, tt.cmd, tt.params)
			if len(mock.Errors) != 1 || !strings.Contains(mock.Errors[0].Error(), tt.wantErr) {
				t.Fatalf("errors = %v, want one containing %q", mock.Errors, tt.wantErr)
			}
			if len(mock.Files) != 0 {
				t.Errorf("nothing should be opened, got %v", mock.Files)
			}
		})
	}
}

func TestCmdURL(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	addTestMsg(t, sm.db, Message{Id: "U1", ChatId: "c@s.whatsapp.net", Text: "look https://example.org/x."})
	addTestMsg(t, sm.db, Message{Id: "U2", ChatId: "c@s.whatsapp.net", Text: "no link"})

	cmdURL(sm, nil, "url", []string{"U1"})
	if len(mock.Files) != 1 || mock.Files[0] != "https://example.org/x" {
		t.Fatalf("opened = %v", mock.Files)
	}
	cmdURL(sm, nil, "url", []string{"U2"})
	if len(mock.Files) != 1 {
		t.Errorf("nothing should be opened for a message without URL, got %v", mock.Files)
	}
}

func TestBacklogAnchor(t *testing.T) {
	sm, _ := newTestSessionManager(t)
	if _, ok, err := sm.backlogAnchor("g@g.us"); ok || err != nil {
		t.Fatalf("empty chat: ok=%v err=%v", ok, err)
	}
	addTestMsg(t, sm.db, Message{Id: "NEW", ChatId: "g@g.us", ContactId: "111@lid", Timestamp: 200})
	addTestMsg(t, sm.db, Message{Id: "OLD", ChatId: "g@g.us", ContactId: "222@lid", Timestamp: 100, FromMe: true})
	info, ok, err := sm.backlogAnchor("g@g.us")
	if err != nil || !ok {
		t.Fatalf("anchor: ok=%v err=%v", ok, err)
	}
	if info.ID != "OLD" || !info.IsFromMe || !info.IsGroup || info.Chat.String() != "g@g.us" ||
		info.Sender.String() != "222@lid" || info.Timestamp.Unix() != 100 {
		t.Errorf("unexpected anchor %+v", info)
	}
}

func TestCmdBacklog_Guards(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	cmdBacklog(sm, nil, "backlog", nil)
	if len(mock.Texts) != 1 || !strings.Contains(mock.Texts[0], "only works in a chat") {
		t.Fatalf("no chat: texts=%v", mock.Texts)
	}
	sm.currentReceiver = "c@s.whatsapp.net"
	cmdBacklog(sm, nil, "backlog", nil)
	if len(mock.Errors) != 1 || !strings.Contains(mock.Errors[0].Error(), "not connected") {
		t.Fatalf("offline: errors=%v", mock.Errors)
	}
}

func TestWaitForNewMessages(t *testing.T) {
	db := newSingleConnDB(t)
	md := &MessageDatabase{}
	if err := md.InitWithDB(db); err != nil {
		t.Fatal(err)
	}
	sm := &SessionManager{db: md, uiHandler: NewMockUiHandler()}

	// Times out with no change.
	start := time.Now()
	n, err := sm.waitForNewMessages("c", 0, 50*time.Millisecond, 10*time.Millisecond)
	if err != nil || n != 0 || time.Since(start) < 50*time.Millisecond {
		t.Fatalf("timeout case: n=%d err=%v elapsed=%v", n, err, time.Since(start))
	}

	// Returns early when a row arrives.
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = md.AddMessage(Message{Id: "X", ChatId: "c", Timestamp: 1})
	}()
	start = time.Now()
	n, err = sm.waitForNewMessages("c", 0, 5*time.Second, 5*time.Millisecond)
	if err != nil || n != 1 {
		t.Fatalf("arrival case: n=%d err=%v", n, err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("did not return early: %v", time.Since(start))
	}
}

func TestApplyRevoke_UnknownIDIsNoop(t *testing.T) {
	sm, mock := newTestSessionManager(t)
	sm.currentReceiver = "c@s.whatsapp.net"
	sm.applyRevoke("c@s.whatsapp.net", "MISSING")
	if len(mock.Screens) != 0 || len(mock.Errors) != 0 {
		t.Errorf("unexpected UI activity: screens=%d errors=%v", len(mock.Screens), mock.Errors)
	}
}
