package messages

import "sync"

// MockUiHandler implements UiMessageHandler for testing.
// It captures all calls so tests can assert on them.
//
// The embedded Mutex makes every method safe to call concurrently, matching
// how the real UiMessageHandler is actually used in production (command
// handlers routinely call it from goroutines — e.g. cmdSyncGroups,
// cmdBacklog, cmd_message.go's download/open/show/revoke). A test that
// exercises one of those async paths must take the same lock before reading
// the public fields below from a different goroutine than the one driving
// the command, e.g.:
//
//	mock.Lock()
//	n := len(mock.Errors)
//	mock.Unlock()
//
// Tests that only touch the mock from a single goroutine (the common case)
// can keep reading the fields directly without locking, exactly as before.
type MockUiHandler struct {
	sync.Mutex

	Texts       []string
	Errors      []error
	Files       []string
	QRCodes     []string
	Statuses    []SessionStatus
	ChatLists   [][]*Conversation
	Screens     [][]Message
	Messages    []Message
	HelpCalled  int
	ClearCalled int
	QuitCalled  int
	ColorCalled int
	CmdsCalled  int
}

func NewMockUiHandler() *MockUiHandler {
	return &MockUiHandler{}
}

func (m *MockUiHandler) NewMessage(msg Message) {
	m.Lock()
	defer m.Unlock()
	m.Messages = append(m.Messages, msg)
}

func (m *MockUiHandler) NewScreen(msgs []Message) {
	m.Lock()
	defer m.Unlock()
	m.Screens = append(m.Screens, msgs)
}

func (m *MockUiHandler) UpdateChatList(c []*Conversation) {
	m.Lock()
	defer m.Unlock()
	m.ChatLists = append(m.ChatLists, c)
}

func (m *MockUiHandler) PrintError(err error) {
	m.Lock()
	defer m.Unlock()
	m.Errors = append(m.Errors, err)
}

func (m *MockUiHandler) PrintText(text string) {
	m.Lock()
	defer m.Unlock()
	m.Texts = append(m.Texts, text)
}

func (m *MockUiHandler) PrintFile(path string) {
	m.Lock()
	defer m.Unlock()
	m.Files = append(m.Files, path)
}

func (m *MockUiHandler) PrintQR(qr string) {
	m.Lock()
	defer m.Unlock()
	m.QRCodes = append(m.QRCodes, qr)
}

func (m *MockUiHandler) SetStatus(s SessionStatus) {
	m.Lock()
	defer m.Unlock()
	m.Statuses = append(m.Statuses, s)
}

func (m *MockUiHandler) OpenFile(path string) {
	m.Lock()
	defer m.Unlock()
	m.Files = append(m.Files, path)
}

func (m *MockUiHandler) ShowColorList() {
	m.Lock()
	defer m.Unlock()
	m.ColorCalled++
}

func (m *MockUiHandler) Clear() {
	m.Lock()
	defer m.Unlock()
	m.ClearCalled++
}

func (m *MockUiHandler) UpdateQR(_ string, _ int, _ int) {}

func (m *MockUiHandler) PrintCommands() {
	m.Lock()
	defer m.Unlock()
	m.CmdsCalled++
}

func (m *MockUiHandler) PrintHelp() {
	m.Lock()
	defer m.Unlock()
	m.HelpCalled++
}

func (m *MockUiHandler) Quit() {
	m.Lock()
	defer m.Unlock()
	m.QuitCalled++
}
