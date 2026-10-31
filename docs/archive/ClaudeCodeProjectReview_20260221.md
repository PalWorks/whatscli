# Claude Code Project Review - 2026-02-21

**Analyzer:** OpenClaw (Gemini 3 Pro Preview)
**Date:** 2026-02-21
**Target:** `github.com/normen/whatscli` (Local Fork)

## 1. Executive Summary

`whatscli` is a Go-based terminal user interface (TUI) for WhatsApp, leveraging `whatsmeow` for protocol handling and `tview` for rendering. The project is functional but demonstrates signs of aging architecture, specifically regarding **concurrency safety** in the UI layer and **dependency management** (vendored/local `whatsmeow`).

**Health Score:** 🟡 **Moderate Risk**
- Core logic is sound.
- Critical race conditions exist in the login flow.
- Global state usage (`ctx` variable) makes the codebase brittle and hard to test.

---

## 2. Critical Issues (Must Fix)

### 🚨 2.1 UI Concurrency Violation (QR Code)
**Severity:** Critical
**Location:** `messages/session_manager.go` (Login Flow) & `ui_handler.go`

**The Bug:**
The login process runs in a background goroutine (`runManager`). When it receives a QR code event:
1. It generates the QR string.
2. It calls `sm.uiHandler.UpdateQR`.
3. `UpdateQR` spawns a *new* goroutine to call `ctx.App.QueueUpdateDraw`.

While `QueueUpdateDraw` is thread-safe, the interaction involves multiple context switches. More importantly, the previous implementation in `session_manager.go` (lines 252-254) was writing *directly* to `ANSIWriter` without a lock, which causes race conditions and likely invisible/broken QR codes.

**Remediation:**
- Ensure `UpdateQR` encapsulates the entire redraw logic (Clear -> Print Help -> Print QR) within a single `QueueUpdateDraw` closure.
- Remove any direct usage of `tview.ANSIWriter` from background threads in `session_manager.go`.

### 🚨 2.2 Global State (`ctx`)
**Severity:** High
**Location:** `main.go`, `ui_helpers.go`, `ui_handler.go`

**The Issue:**
The application relies on a package-level global variable `ctx *AppContext` (defined in `main.go`).
- `ui_helpers.go` functions (`PrintText`, `PrintHelp`) directly access `ctx.TextView`.
- `ui_handler.go` methods directly access `ctx`.

**Consequences:**
- **Untestable Code:** You cannot easily write unit tests for UI helpers because you cannot mock `ctx` or run parallel tests.
- **Tight Coupling:** The UI logic is inseparable from the main application bootstrap.

**Remediation (Long Term):**
- Refactor `UiHandler` to hold a reference to the necessary UI components (`TextView`, `App`) instead of reaching out to a global `ctx`.

---

## 3. Architecture & Code Quality

### 3.1 Dependency Management
- **Observation:** The project appears to use a local copy of `whatsmeow` rather than a module dependency.
- **Risk:** Missing out on critical protocol updates, security fixes, and new features from the upstream library.
- **Recommendation:** Switch to `go.mod` dependency (`go get go.mau.fi/whatsmeow`) unless there are specific patches in the local copy.

### 3.2 Error Handling
- **Observation:** `messages/session_manager.go` ignores the error from `client.GetQRChannel` in the original code (fixed in analysis).
- **Risk:** If the client is already logged in or the store is corrupt, the app hangs indefinitely on a nil channel.
- **Recommendation:** Explicitly handle all errors from `whatsmeow` methods, especially during connection/login.

### 3.3 Code Style
- **Strengths:** Clear separation of concerns between `messages` (backend) and `main` (UI).
- **Weaknesses:** Inconsistent usage of "Command Registry" vs direct method calls.

---

## 4. Action Plan (Prioritized)

1.  **[IMMEDIATE] Fix Login Flow:**
    - Patch `messages/session_manager.go` to safely handle `GetQRChannel` errors.
    - Patch `ui_handler.go` / `session_manager.go` to ensure atomic UI updates for the QR code.

2.  **[SHORT TERM] Run Test Suite:**
    - Execute `go test ./...` to baseline current stability.

3.  **[MEDIUM TERM] Refactor Globals:**
    - Create a proper `UI` struct that encapsulates `tview.Application` and `TextView`.
    - Pass this struct to `SessionManager` instead of relying on `ctx`.

4.  **[LONG TERM] Dependency Cleanup:**
    - Evaluate diff between local `whatsmeow` and upstream.
    - Migrate to upstream if possible.

---

## 5. Development Environment
- **Go Version:** System default (verify `go.mod` matches).
- **Build Command:** `go build` or `make`.
