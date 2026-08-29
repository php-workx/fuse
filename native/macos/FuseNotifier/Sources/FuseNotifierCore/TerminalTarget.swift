import AppKit
import Foundation

public struct TerminalTargetMetadata: Equatable, Sendable {
    public let terminalProgram: String
    public let terminalProgramVersion: String
    public let iTermSessionID: String?
    public let terminalTTY: String?
    public let ghosttyTitleMarker: String?
    public let hasExactTargetingDegradation: Bool

    public init(terminalProgram: String, terminalProgramVersion: String, iTermSessionID: String?, terminalTTY: String?, ghosttyTitleMarker: String?, hasExactTargetingDegradation: Bool) {
        self.terminalProgram = terminalProgram
        self.terminalProgramVersion = terminalProgramVersion
        self.iTermSessionID = iTermSessionID
        self.terminalTTY = terminalTTY
        self.ghosttyTitleMarker = ghosttyTitleMarker
        self.hasExactTargetingDegradation = hasExactTargetingDegradation
    }
}

public enum TerminalTargetDegradation: Equatable, Sendable {
    case degradedMetadata
    case ghosttyUnsupported
    case ghosttyProbeFailed
    case ghosttyMarkerMissing
    case ghosttyTargetMissing
    case iTermDispatchFailed
    case terminalTTYMissing
    case terminalTargetMissing
    case unsupportedTerminal
}

public enum TerminalTargetResult: Equatable, Sendable {
    case focused
    case dispatched
    case dispatchedUnverified
    case applicationFallback(TerminalTargetDegradation)
}

public protocol WorkspaceOpening: Sendable {
    func open(_ url: URL) -> Bool
    func activateApplication(bundleIdentifier: String) -> Bool
}

public struct SystemWorkspace: WorkspaceOpening {
    public init() {}

    public func open(_ url: URL) -> Bool { NSWorkspace.shared.open(url) }

    public func activateApplication(bundleIdentifier: String) -> Bool {
        guard let url = NSWorkspace.shared.urlForApplication(withBundleIdentifier: bundleIdentifier) else {
            return false
        }
        let configuration = NSWorkspace.OpenConfiguration()
        configuration.activates = true
        NSWorkspace.shared.openApplication(at: url, configuration: configuration)
        return true
    }
}

public final class TerminalTarget {
    private static let osascript = "/usr/bin/osascript"
    private static let focusTimeout: TimeInterval = 3
    private static let ghosttyProbeScript = """
    tell application id "com.mitchellh.ghostty"
        return version
    end tell
    """
    private static let ghosttyLookupScript = """
    on run argv
        set marker to item 1 of argv
        tell application id "com.mitchellh.ghostty"
            set matchingTerminals to every terminal whose name is marker
            if (count of matchingTerminals) is not 1 then error "expected exactly one matching terminal"
            return id of item 1 of matchingTerminals
        end tell
    end run
    """
    private static let ghosttyFocusScript = """
    on run argv
        set terminalID to item 1 of argv
        tell application id "com.mitchellh.ghostty"
            set targetTerminal to first terminal whose id is terminalID
            focus targetTerminal
        end tell
    end run
    """
    private static let terminalFocusScript = """
    on run argv
        set requestedTTY to item 1 of argv
        tell application id "com.apple.Terminal"
            set matchingTabs to every tab of every window whose tty is requestedTTY
            if (count of matchingTabs) is not 1 then error "expected exactly one matching tab"
            set targetTab to item 1 of matchingTabs
            set targetWindow to first window whose tabs contains targetTab
            set selected tab of targetWindow to targetTab
            set miniaturized of targetWindow to false
            set index of targetWindow to 1
            activate
        end tell
    end run
    """

    private let runner: any ProcessRunning
    private let workspace: any WorkspaceOpening
    private let sleep: @Sendable (TimeInterval) -> Void
    private var ghosttyTerminalID: String?

    public init(runner: any ProcessRunning = SystemProcessRunner(), workspace: any WorkspaceOpening = SystemWorkspace(), sleep: @escaping @Sendable (TimeInterval) -> Void = { Thread.sleep(forTimeInterval: $0) }) {
        self.runner = runner
        self.workspace = workspace
        self.sleep = sleep
    }

    public func focus(_ metadata: TerminalTargetMetadata) -> TerminalTargetResult {
        switch metadata.terminalProgram {
        case "Ghostty":
            return focusGhostty(metadata)
        case "iTerm.app":
            return focusITerm(metadata)
        case "Apple_Terminal":
            return focusTerminal(metadata)
        default:
            return .applicationFallback(.unsupportedTerminal)
        }
    }

    private func focusGhostty(_ metadata: TerminalTargetMetadata) -> TerminalTargetResult {
        guard !metadata.hasExactTargetingDegradation else {
            return fallback(bundleIdentifier: "com.mitchellh.ghostty", reason: .degradedMetadata)
        }
        guard Self.versionIsAtLeast130(metadata.terminalProgramVersion) else {
            return fallback(bundleIdentifier: "com.mitchellh.ghostty", reason: .ghosttyUnsupported)
        }
        guard case let .success(output) = runAppleScript(Self.ghosttyProbeScript), Self.versionIsAtLeast130(output.trimmingCharacters(in: .whitespacesAndNewlines)) else {
            return fallback(bundleIdentifier: "com.mitchellh.ghostty", reason: .ghosttyProbeFailed)
        }
        guard let marker = metadata.ghosttyTitleMarker, !marker.isEmpty else {
            return fallback(bundleIdentifier: "com.mitchellh.ghostty", reason: .ghosttyMarkerMissing)
        }
        if ghosttyTerminalID == nil {
            for attempt in 0..<10 {
                if case let .success(output) = runAppleScript(Self.ghosttyLookupScript, arguments: [marker]) {
                    let id = output.trimmingCharacters(in: .whitespacesAndNewlines)
                    if !id.isEmpty {
                        ghosttyTerminalID = id
                        break
                    }
                }
                if attempt < 9 { sleep(0.05) }
            }
        }
        guard let id = ghosttyTerminalID, case .success = runAppleScript(Self.ghosttyFocusScript, arguments: [id]) else {
            ghosttyTerminalID = nil
            return fallback(bundleIdentifier: "com.mitchellh.ghostty", reason: .ghosttyTargetMissing)
        }
        return .focused
    }

    private func focusITerm(_ metadata: TerminalTargetMetadata) -> TerminalTargetResult {
        guard let sessionID = metadata.iTermSessionID, !sessionID.isEmpty else {
            return fallback(bundleIdentifier: "com.googlecode.iterm2", reason: .iTermDispatchFailed)
        }
        var components = URLComponents()
        components.scheme = "iterm2"
        components.host = ""
        components.path = "/reveal"
        components.queryItems = [URLQueryItem(name: "sessionid", value: sessionID)]
        guard let url = components.url, workspace.open(url) else {
            return fallback(bundleIdentifier: "com.googlecode.iterm2", reason: .iTermDispatchFailed)
        }
        return metadata.hasExactTargetingDegradation ? .dispatchedUnverified : .dispatched
    }

    private func focusTerminal(_ metadata: TerminalTargetMetadata) -> TerminalTargetResult {
        guard !metadata.hasExactTargetingDegradation else {
            return fallback(bundleIdentifier: "com.apple.Terminal", reason: .degradedMetadata)
        }
        guard let tty = metadata.terminalTTY, tty.hasPrefix("/dev/ttys") else {
            return fallback(bundleIdentifier: "com.apple.Terminal", reason: .terminalTTYMissing)
        }
        guard case .success = runAppleScript(Self.terminalFocusScript, arguments: [tty]) else {
            return fallback(bundleIdentifier: "com.apple.Terminal", reason: .terminalTargetMissing)
        }
        return .focused
    }

    private func runAppleScript(_ script: String, arguments: [String] = []) -> ProcessResult {
        runner.run(.init(executable: Self.osascript, arguments: ["-e", script] + arguments, timeout: Self.focusTimeout))
    }

    private func fallback(bundleIdentifier: String, reason: TerminalTargetDegradation) -> TerminalTargetResult {
        _ = workspace.activateApplication(bundleIdentifier: bundleIdentifier)
        return .applicationFallback(reason)
    }

    private static func versionIsAtLeast130(_ string: String) -> Bool {
        let components = string.split(separator: ".", maxSplits: 2, omittingEmptySubsequences: false).map { Int($0.prefix { $0.isNumber }) ?? 0 }
        guard components.count >= 2 else { return false }
        if components[0] != 1 { return components[0] > 1 }
        return components[1] >= 3
    }
}
