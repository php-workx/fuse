import Foundation
import FuseNotifierCore
import FuseNotifierProtocol

func expect(_ condition: @autoclosure () -> Bool, _ message: String) {
    if !condition() {
        fatalError(message)
    }
}

final class RecordingNotificationCenter: NotificationCenterClient {
    var postedIDs: [String] = []
    var removedIDs: [String] = []
    var lastTitle: String?
    var lastBody: String?

    func requestAuthorization() {}

    func postGenericNotification(id: String, title: String, body: String, completion: @escaping @Sendable (Error?) -> Void) {
        postedIDs.append(id)
        lastTitle = title
        lastBody = body
        completion(nil)
    }

    func removeNotifications(ids: [String], completion: @escaping @Sendable () -> Void) {
        removedIDs.append(contentsOf: ids)
        completion()
    }
}

final class RecordingEventSink: EventSink, @unchecked Sendable {
    var events: [HelperEvent] = []

    func send(_ event: HelperEvent) {
        events.append(event)
    }
}

final class TerminationFlag: @unchecked Sendable {
    private let lock = NSLock()
    private var terminated = false

    var value: Bool {
        lock.lock()
        defer { lock.unlock() }
        return terminated
    }

    func set() {
        lock.lock()
        terminated = true
        lock.unlock()
    }
}

let center = RecordingNotificationCenter()
let sink = RecordingEventSink()
let controller = NotificationController(center: center, sink: sink, terminate: {})

controller.handle(.post(requestID: 2, id: "approval-1"))
expect(center.postedIDs == ["approval-1"], "Post did not schedule the owned notification")
expect(center.lastTitle == "Fuse approval required", "Notification title is not generic")
expect(center.lastBody == "Open Fuse to review this request.", "Notification body is not generic")
expect(sink.events == [.result(requestID: 2, operation: "post", id: "approval-1", status: "accepted")], "Post was not acknowledged")

controller.activate(id: "approval-1")
expect(sink.events.last == .activated(id: "approval-1"), "Owned activation was not emitted")

let terminationFlag = TerminationFlag()
let eofCenter = RecordingNotificationCenter()
let eofSink = RecordingEventSink()
let eofController = NotificationController(center: eofCenter, sink: eofSink, terminate: { terminationFlag.set() })
eofController.handle(.post(requestID: 2, id: "approval-1"))
eofController.inputDidReachEOF()
expect(eofCenter.removedIDs == ["approval-1"], "EOF did not remove owned notifications")
expect(eofSink.events.last == .shutdownComplete, "EOF cleanup did not complete before termination")
expect(terminationFlag.value, "EOF did not terminate the helper")

let monotonicCenter = RecordingNotificationCenter()
let monotonicSink = RecordingEventSink()
let monotonicController = NotificationController(center: monotonicCenter, sink: monotonicSink, terminate: {})
monotonicController.handle(.hello(requestID: 2, clientVersion: "test"))
monotonicController.handle(.post(requestID: 2, id: "approval-1"))
expect(monotonicCenter.postedIDs.isEmpty, "Non-monotonic request posted a notification")
expect(monotonicSink.events.last == .error(requestID: 2, operation: "post", id: "approval-1", code: "non_monotonic_request_id"), "Non-monotonic request did not report a protocol error")

let socketLaunch = try LaunchConfiguration.parse([
    "--connect", "/tmp/fuse-notifier/socket", "--token", String(repeating: "a", count: 64),
])
expect(socketLaunch == .socket(path: "/tmp/fuse-notifier/socket", token: String(repeating: "a", count: 64)), "Socket launch arguments did not parse")

do {
    _ = try LaunchConfiguration.parse(["--connect", "/tmp/fuse-notifier/socket", "--token", "short"])
    fatalError("Non-256-bit socket token parsed")
} catch LaunchConfigurationError.invalidToken {
}
