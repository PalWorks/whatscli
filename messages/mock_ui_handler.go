// SPDX-License-Identifier: AGPL-3.0-only

package messages

// MockUiHandler implements UiMessageHandler for testing.
// It captures all calls so tests can assert on them.
type MockUiHandler struct {
	Texts     []string
	Errors    []error
	Files     []string
	QRCodes   []string
	Statuses  []SessionStatus
	ChatLists [][]*Conversation
	ChatSets  [][]Chat
	Screens   [][]Message
	// ScreenTexts holds the status printed with each NewScreenWithStatus.
	ScreenTexts []string
	Messages    []Message
	HelpCalled  int
	ClearCalled int
	QuitCalled  int
	ColorCalled int
	CmdsCalled  int
	// OpenErr is returned by OpenFile.
	OpenErr error
}

func NewMockUiHandler() *MockUiHandler {
	return &MockUiHandler{}
}

func (m *MockUiHandler) NewMessage(msg Message)           { m.Messages = append(m.Messages, msg) }
func (m *MockUiHandler) NewScreen(msgs []Message)         { m.Screens = append(m.Screens, msgs) }
func (m *MockUiHandler) SetChats(chats []Chat)            { m.ChatSets = append(m.ChatSets, chats) }
func (m *MockUiHandler) UpdateChatList(c []*Conversation) { m.ChatLists = append(m.ChatLists, c) }
func (m *MockUiHandler) PrintError(err error)             { m.Errors = append(m.Errors, err) }
func (m *MockUiHandler) PrintText(text string)            { m.Texts = append(m.Texts, text) }
func (m *MockUiHandler) PrintFile(path string)            { m.Files = append(m.Files, path) }
func (m *MockUiHandler) PrintQR(qr string)                { m.QRCodes = append(m.QRCodes, qr) }
func (m *MockUiHandler) SetStatus(s SessionStatus)        { m.Statuses = append(m.Statuses, s) }
func (m *MockUiHandler) ShowColorList()                   { m.ColorCalled++ }
func (m *MockUiHandler) Clear()                           { m.ClearCalled++ }
func (m *MockUiHandler) ReplaceText(text string) {
	m.ClearCalled++
	m.Texts = append(m.Texts, text)
}
func (m *MockUiHandler) UpdateQR(_ string, _ int, _ int) {}
func (m *MockUiHandler) PrintCommands()                  { m.CmdsCalled++ }
func (m *MockUiHandler) PrintHelp()                      { m.HelpCalled++ }
func (m *MockUiHandler) Quit()                           { m.QuitCalled++ }

// OpenFile records path and returns OpenErr.
func (m *MockUiHandler) OpenFile(path string) error {
	m.Files = append(m.Files, path)
	return m.OpenErr
}

// NewScreenWithStatus records the screen, its status and any non-nil errors.
func (m *MockUiHandler) NewScreenWithStatus(msgs []Message, status string, errs ...error) {
	m.Screens = append(m.Screens, msgs)
	m.ScreenTexts = append(m.ScreenTexts, status)
	for _, err := range errs {
		if err != nil {
			m.Errors = append(m.Errors, err)
		}
	}
}
