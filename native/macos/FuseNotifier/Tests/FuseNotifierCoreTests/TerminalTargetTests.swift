import Foundation
import FuseNotifierCore

private func terminalExpect(_ condition: @autoclosure () -> Bool, _ message: String) {
    if !condition() { fatalError(message) }
}

private final class RecordingProcessRunner: ProcessRunning, @unchecked Sendable {
    var invocations: [ProcessInvocation] = []
    var results: [ProcessResult]

    init(results: [ProcessResult]) { self.results = results }

    func run(_ invocation: ProcessInvocation) -> ProcessResult {
        invocations.append(invocation)
        return results.isEmpty ? .failure : results.removeFirst()
    }
}

private final class RecordingWorkspace: WorkspaceOpening, @unchecked Sendable {
    var openedURLs: [URL] = []
    var activatedBundles: [String] = []
    var openResult = true
    var activateResult = true

    func open(_ url: URL) -> Bool { openedURLs.append(url); return openResult }
    func activateApplication(bundleIdentifier: String) -> Bool { activatedBundles.append(bundleIdentifier); return activateResult }
}

func runTerminalTargetTests() {
    let ghosttyMetadata = TerminalTargetMetadata(
        terminalProgram: "Ghostty",
        terminalProgramVersion: "1.3.1",
        iTermSessionID: nil,
        terminalTTY: "/dev/ttys004",
        ghosttyTitleMarker: "fuse-marker; return 1",
        hasExactTargetingDegradation: false
    )
    let ghosttyRunner = RecordingProcessRunner(results: [.success(output: "1.3.1\n"), .success(output: "42\n"), .success(output: "")])
    let ghosttyWorkspace = RecordingWorkspace()
    let ghosttyTarget = TerminalTarget(runner: ghosttyRunner, workspace: ghosttyWorkspace, sleep: { _ in })
    terminalExpect(ghosttyTarget.focus(ghosttyMetadata) == .focused, "Ghostty did not focus its exact terminal")
    terminalExpect(ghosttyRunner.invocations == [
        ProcessInvocation(executable: "/usr/bin/osascript", arguments: ["-e", "tell application id \"com.mitchellh.ghostty\"\n    return version\nend tell"], timeout: 3),
        ProcessInvocation(executable: "/usr/bin/osascript", arguments: ["-e", "on run argv\n    set marker to item 1 of argv\n    tell application id \"com.mitchellh.ghostty\"\n        set matchingTerminals to every terminal whose name is marker\n        if (count of matchingTerminals) is not 1 then error \"expected exactly one matching terminal\"\n        return id of item 1 of matchingTerminals\n    end tell\nend run", "fuse-marker; return 1"], timeout: 3),
        ProcessInvocation(executable: "/usr/bin/osascript", arguments: ["-e", "on run argv\n    set terminalID to item 1 of argv\n    tell application id \"com.mitchellh.ghostty\"\n        set targetTerminal to first terminal whose id is terminalID\n        focus targetTerminal\n    end tell\nend run", "42"], timeout: 3),
    ], "Ghostty scripts, argument boundary, or timeouts changed")
    terminalExpect(!ghosttyRunner.invocations[1].arguments[1].contains("fuse-marker"), "Ghostty marker was interpolated into AppleScript")

    let terminalRunner = RecordingProcessRunner(results: [.success(output: "")])
    let terminalWorkspace = RecordingWorkspace()
    let terminalTarget = TerminalTarget(runner: terminalRunner, workspace: terminalWorkspace, sleep: { _ in })
    let terminalMetadata = TerminalTargetMetadata(terminalProgram: "Apple_Terminal", terminalProgramVersion: "2.14", iTermSessionID: nil, terminalTTY: "/dev/ttys004", ghosttyTitleMarker: nil, hasExactTargetingDegradation: false)
    terminalExpect(terminalTarget.focus(terminalMetadata) == .focused, "Terminal.app did not select its exact tab")
    terminalExpect(terminalRunner.invocations == [
        ProcessInvocation(executable: "/usr/bin/osascript", arguments: ["-e", "on run argv\n    set requestedTTY to item 1 of argv\n    tell application id \"com.apple.Terminal\"\n        set matchingTabs to every tab of every window whose tty is requestedTTY\n        if (count of matchingTabs) is not 1 then error \"expected exactly one matching tab\"\n        set targetTab to item 1 of matchingTabs\n        set targetWindow to first window whose tabs contains targetTab\n        set selected tab of targetWindow to targetTab\n        set miniaturized of targetWindow to false\n        set index of targetWindow to 1\n        activate\n    end tell\nend run", "/dev/ttys004"], timeout: 3),
    ], "Terminal.app script, argument boundary, or timeout changed")

    let iTermWorkspace = RecordingWorkspace()
    let iTermTarget = TerminalTarget(runner: RecordingProcessRunner(results: []), workspace: iTermWorkspace, sleep: { _ in })
    let iTermMetadata = TerminalTargetMetadata(terminalProgram: "iTerm.app", terminalProgramVersion: "3.5", iTermSessionID: "w0t0p0:A&B C", terminalTTY: nil, ghosttyTitleMarker: nil, hasExactTargetingDegradation: false)
    terminalExpect(iTermTarget.focus(iTermMetadata) == .dispatched, "iTerm URL dispatch was not reported as dispatched")
    terminalExpect(iTermWorkspace.openedURLs.map(\.absoluteString) == ["iterm2:///reveal?sessionid=w0t0p0:A%26B%20C"], "iTerm session identifier was not percent encoded")

    let unverifiedITermWorkspace = RecordingWorkspace()
    let unverifiedITermTarget = TerminalTarget(runner: RecordingProcessRunner(results: []), workspace: unverifiedITermWorkspace, sleep: { _ in })
    let unverifiedITermMetadata = TerminalTargetMetadata(terminalProgram: "iTerm.app", terminalProgramVersion: "3.5", iTermSessionID: "w0t0p0:tmux", terminalTTY: nil, ghosttyTitleMarker: nil, hasExactTargetingDegradation: true)
    terminalExpect(unverifiedITermTarget.focus(unverifiedITermMetadata) == .dispatchedUnverified, "iTerm dispatch through a degraded topology was not labeled unverified")

    let fallbackWorkspace = RecordingWorkspace()
    fallbackWorkspace.openResult = false
    let fallbackTarget = TerminalTarget(runner: RecordingProcessRunner(results: []), workspace: fallbackWorkspace, sleep: { _ in })
    terminalExpect(fallbackTarget.focus(iTermMetadata) == .applicationFallback(.iTermDispatchFailed), "Failed iTerm URL dispatch did not report application fallback")
    terminalExpect(fallbackWorkspace.activatedBundles == ["com.googlecode.iterm2"], "Failed iTerm URL dispatch did not activate iTerm")

    let degradedWorkspace = RecordingWorkspace()
    let degradedTarget = TerminalTarget(runner: RecordingProcessRunner(results: []), workspace: degradedWorkspace, sleep: { _ in })
    let degradedMetadata = TerminalTargetMetadata(terminalProgram: "Ghostty", terminalProgramVersion: "1.3.1", iTermSessionID: nil, terminalTTY: nil, ghosttyTitleMarker: "marker", hasExactTargetingDegradation: true)
    terminalExpect(degradedTarget.focus(degradedMetadata) == .applicationFallback(.degradedMetadata), "Degraded target metadata claimed exact focus")
    terminalExpect(degradedWorkspace.activatedBundles == ["com.mitchellh.ghostty"], "Degraded Ghostty metadata did not activate Ghostty")
}
