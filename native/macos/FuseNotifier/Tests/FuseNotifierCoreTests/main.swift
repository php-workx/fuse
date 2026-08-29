import Foundation
import FuseNotifierCore
import FuseNotifierProtocol

func expect(_ condition: @autoclosure () -> Bool, _ message: String) { if !condition() { fatalError(message) } }
func post(_ requestID: UInt64, _ id: String, sound: Bool = false) -> HelperCommand { .post(requestID: requestID, notice: Notice(id: id, title: "untrusted title", subtitle: nil, body: "untrusted body", sound: sound)) }

enum FakeError: Error { case failed }
final class BoolBox: @unchecked Sendable { var value = false }
final class PresentationBox: @unchecked Sendable { var value: PresentationOptions? }
final class FakeNotificationCenter: NotificationCenterClient, @unchecked Sendable {
    var settings: NotificationSettings; var authorizationRequests = 0; var posted: [NotificationRequest] = []; var removed: [String] = []; var deliveredIDs: [String] = []; var postError: Error?; var defersPosts = false; private var postCompletions: [(@Sendable (Error?) -> Void)] = []; private var authorizationCompletion: ((Bool, Error?) -> Void)?
    init(settings: NotificationSettings) { self.settings = settings }
    func getSettings(completion: @escaping @Sendable (NotificationSettings) -> Void) { completion(settings) }
    func requestAuthorization(completion: @escaping @Sendable (Bool, Error?) -> Void) { authorizationRequests += 1; authorizationCompletion = completion }
    func post(_ request: NotificationRequest, completion: @escaping @Sendable (Error?) -> Void) { posted.append(request); if defersPosts { postCompletions.append(completion) } else { completion(postError) } }
    func removeNotifications(ids: [String], completion: @escaping @Sendable () -> Void) { removed.append(contentsOf: ids); completion() }
    func deliveredNotificationIDs(completion: @escaping @Sendable ([String]) -> Void) { completion(deliveredIDs) }
    func resolveAuthorization(granted: Bool) { authorizationCompletion?(granted, nil) }
    func completePosts() { let completions = postCompletions; postCompletions.removeAll(); for completion in completions { completion(postError) } }
    func completeNextPost() { guard !postCompletions.isEmpty else { return }; let completion = postCompletions.removeFirst(); completion(postError) }
}
final class RecordingSink: EventSink, @unchecked Sendable {
    var events: [HelperEvent] = []; let observe: (HelperEvent) -> Void; let shouldThrow: Bool
    init(shouldThrow: Bool = false, observe: @escaping (HelperEvent) -> Void = { _ in }) { self.shouldThrow = shouldThrow; self.observe = observe }
    func send(_ event: HelperEvent) throws { observe(event); if shouldThrow { throw FakeError.failed }; events.append(event) }
}

let deferredCenter = FakeNotificationCenter(settings: .notDetermined)
let deferredSink = RecordingSink()
let deferred = NotificationController(center: deferredCenter, sink: deferredSink, terminate: {})
deferred.handle(post(2, "approval-1"))
expect(deferredCenter.authorizationRequests == 1 && deferredCenter.posted.isEmpty, "Post did not defer while authorization was undecided")
deferred.handle(.remove(requestID: 3, id: "approval-1"))
deferredCenter.settings = .authorized(alerts: .enabled, sound: .enabled)
deferredCenter.resolveAuthorization(granted: true)
expect(deferredCenter.posted.isEmpty && deferredCenter.removed == ["approval-1"], "Remove during permission prompt posted a stale notification")

let queuedCenter = FakeNotificationCenter(settings: .notDetermined)
let queued = NotificationController(center: queuedCenter, sink: RecordingSink(), terminate: {})
queued.handle(post(2, "first")); queued.handle(post(3, "second"))
queuedCenter.settings = .authorized(alerts: .enabled, sound: .enabled)
queuedCenter.resolveAuthorization(granted: true)
expect(Set(queuedCenter.posted.map(\.id)) == Set(["first", "second"]), "Posts received during authorization were discarded")

let duplicateCenter = FakeNotificationCenter(settings: .authorized(alerts: .enabled, sound: .enabled))
let duplicateSink = RecordingSink()
let duplicate = NotificationController(center: duplicateCenter, sink: duplicateSink, terminate: {})
duplicate.handle(post(2, "approval-1")); duplicate.handle(post(3, "approval-1"))
expect(duplicateCenter.posted.map(\.id) == ["approval-1"], "Duplicate ID was posted twice")
expect(duplicateSink.events.contains(.error(requestID: 3, operation: "post", id: "approval-1", code: "duplicate_id", message: "")), "Duplicate ID was not rejected")
expect(duplicateCenter.posted.first?.title == NotificationController.genericTitle && duplicateCenter.posted.first?.body == NotificationController.genericBody, "Untrusted notification copy escaped the controller")

let acceptedCenter = FakeNotificationCenter(settings: .authorized(alerts: .enabled, sound: .enabled)); acceptedCenter.defersPosts = true
let acceptedSink = RecordingSink(); let accepted = NotificationController(center: acceptedCenter, sink: acceptedSink, terminate: {})
accepted.handle(post(2, "approval-accepted"))
expect(acceptedSink.events.contains(.result(requestID: 2, operation: "post", id: "approval-accepted", status: "accepted")), "Post acceptance was not correlated before scheduling")
expect(!acceptedSink.events.contains(.posted(requestID: 2, id: "approval-accepted")), "Post completion was reported before Notification Center accepted it")
acceptedCenter.completePosts()
expect(acceptedSink.events.contains(.posted(requestID: 2, id: "approval-accepted")), "Posted event did not follow successful scheduling")

let disabledCenter = FakeNotificationCenter(settings: .notDetermined)
let disabledSink = RecordingSink()
let disabled = NotificationController(center: disabledCenter, sink: disabledSink, terminate: {})
disabled.handle(post(2, "approval-1")); disabledCenter.settings = .authorized(alerts: .disabled, sound: .enabled); disabledCenter.resolveAuthorization(granted: true)
expect(disabledCenter.posted.isEmpty, "Disabled alerts still posted")
expect(disabledSink.events.contains(.error(requestID: 2, operation: "post", id: "approval-1", code: "notifications_not_allowed", message: "alerts are disabled")), "Disabled alerts were not reported")

let failureCenter = FakeNotificationCenter(settings: .authorized(alerts: .enabled, sound: .enabled)); failureCenter.postError = FakeError.failed
let failureSink = RecordingSink(); let failure = NotificationController(center: failureCenter, sink: failureSink, terminate: {})
failure.handle(post(2, "approval-1")); failure.activate(id: "approval-1")
expect(failureSink.events.contains(.error(requestID: 2, operation: "post", id: "approval-1", code: "notification_post_failed", message: "")), "Post failure was not reported")
expect(!failureSink.events.contains(.activated(id: "approval-1")), "Failed notification activation was routed")

let reconcileCenter = FakeNotificationCenter(settings: .authorized(alerts: .enabled, sound: .enabled)); reconcileCenter.deliveredIDs = ["live", "stale"]
let reconcileSink = RecordingSink(); let reconcile = NotificationController(center: reconcileCenter, sink: reconcileSink, terminate: {})
reconcile.handle(.reconcile(requestID: 2, currentIDs: ["live"]))
expect(reconcileCenter.removed == ["stale"], "Reconcile did not remove only stale delivered notifications")
expect(reconcileSink.events.contains(.reconciled(requestID: 2, deliveredCurrentIDs: ["live"])), "Reconcile did not report remaining delivered IDs")

let actionCenter = FakeNotificationCenter(settings: .authorized(alerts: .enabled, sound: .enabled)); let completed = BoolBox()
let actionSink = RecordingSink(observe: { event in if case .activated = event { expect(completed.value, "Output preceded notification delegate completion") } })
let action = NotificationController(center: actionCenter, sink: actionSink, terminate: {})
action.handle(post(2, "approval-1")); action.didReceiveNotificationAction(identifier: "default", pendingID: "approval-1") { completed.value = true }; action.didReceiveNotificationAction(identifier: "default", pendingID: "unknown") {}
expect(actionSink.events.contains(.activated(id: "approval-1")), "Default action was not routed")
expect(actionSink.events.filter { if case .activated = $0 { return true }; return false }.count == 1, "Unknown action routed activation")
let presentation = PresentationBox(); action.willPresent { presentation.value = $0 }
expect(presentation.value == [.banner, .list], "Foreground presentation did not complete")

let soundCenter = FakeNotificationCenter(settings: .authorized(alerts: .enabled, sound: .enabled)); let sound = NotificationController(center: soundCenter, sink: RecordingSink(), terminate: {})
sound.handle(post(2, "approval-1", sound: true)); let soundPresentation = PresentationBox(); sound.willPresent { soundPresentation.value = $0 }
expect(soundPresentation.value == [.banner, .list, .sound], "Foreground sound was not gated by effective setting")

let brokenCenter = FakeNotificationCenter(settings: .authorized(alerts: .enabled, sound: .enabled)); let terminated = BoolBox(); let broken = NotificationController(center: brokenCenter, sink: RecordingSink(shouldThrow: true), terminate: { terminated.value = true })
broken.handle(post(2, "approval-1"))
expect(brokenCenter.removed == ["approval-1"] && terminated.value, "Broken output did not shut down cleanly")

let delayedCenter = FakeNotificationCenter(settings: .authorized(alerts: .enabled, sound: .enabled)); delayedCenter.defersPosts = true
let delayedSink = RecordingSink(); let delayedTermination = BoolBox(); let delayed = NotificationController(center: delayedCenter, sink: delayedSink, terminate: { delayedTermination.value = true })
delayed.handle(post(2, "approval-late")); delayed.inputDidReachEOF()
expect(!delayedTermination.value && !delayedSink.events.contains(.shutdownComplete), "Shutdown completed while a notification post remained in flight")
delayedCenter.completePosts()
expect(delayedCenter.removed == ["approval-late", "approval-late"] && delayedSink.events.last == .shutdownComplete && delayedTermination.value, "Late notification delivery was not removed before shutdown completed")

let generationCenter = FakeNotificationCenter(settings: .authorized(alerts: .enabled, sound: .enabled)); generationCenter.defersPosts = true
let generationSink = RecordingSink(); let generationTermination = BoolBox(); let generation = NotificationController(center: generationCenter, sink: generationSink, terminate: { generationTermination.value = true })
generation.handle(post(2, "same")); generation.handle(.remove(requestID: 3, id: "same")); generation.handle(post(4, "same")); generation.inputDidReachEOF()
generationCenter.completeNextPost()
expect(!generationTermination.value && !generationSink.events.contains(.shutdownComplete), "First generation completion made the second in-flight post invisible to shutdown")
generationCenter.completeNextPost()
expect(generationTermination.value && generationSink.events.last == .shutdownComplete, "Shutdown did not wait for every scheduled post generation")

let eofCenter = FakeNotificationCenter(settings: .authorized(alerts: .enabled, sound: .enabled)); let eofSink = RecordingSink(); let eofTerminated = BoolBox(); let eof = NotificationController(center: eofCenter, sink: eofSink, terminate: { eofTerminated.value = true })
eof.handle(post(2, "approval-1")); eof.inputDidReachEOF()
expect(eofCenter.removed == ["approval-1"] && eofSink.events.last == .shutdownComplete && eofTerminated.value, "EOF did not clean up before termination")

do {
    let token = String(repeating: "a", count: 64)
    let launch = try LaunchConfiguration.parse(["--connect", "/tmp/fuse-notifier/socket", "--token", token])
    expect(launch == .socket(path: "/tmp/fuse-notifier/socket", token: token), "Socket launch configuration changed")
} catch {
    fatalError("Socket launch configuration failed: \(error)")
}

runTerminalTargetTests()
