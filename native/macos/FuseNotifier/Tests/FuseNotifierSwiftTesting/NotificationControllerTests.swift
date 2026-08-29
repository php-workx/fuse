#if canImport(Testing)
import Testing
@testable import FuseNotifierCore
@testable import FuseNotifierProtocol

@Test func notificationControllerStage3Boundaries() {
    let center = SwiftTestingCenter(settings: .notDetermined)
    let sink = SwiftTestingSink()
    let controller = NotificationController(center: center, sink: sink, terminate: {})
    controller.handle(.post(requestID: 2, notice: .init(id: "first", title: "x", subtitle: nil, body: "x", sound: false)))
    controller.handle(.post(requestID: 3, notice: .init(id: "second", title: "x", subtitle: nil, body: "x", sound: false)))
    #expect(center.authorizationRequests == 1)
    center.settings = .authorized(alerts: .enabled, sound: .enabled)
    center.resolveAuthorization()
    #expect(Set(center.posted.map(\.id)) == Set(["first", "second"]) && center.posted.count == 2)
    #expect(sink.events.contains(.result(requestID: 2, operation: "post", id: "first", status: "accepted")))

    let delayedCenter = SwiftTestingCenter(settings: .authorized(alerts: .enabled, sound: .enabled), deferPosts: true)
    let delayedSink = SwiftTestingSink()
    let termination = SwiftTestingFlag()
    let delayed = NotificationController(center: delayedCenter, sink: delayedSink, terminate: { termination.value = true })
    delayed.handle(.post(requestID: 2, notice: .init(id: "late", title: "x", subtitle: nil, body: "x", sound: false)))
    delayed.inputDidReachEOF()
    #expect(!termination.value)
    delayedCenter.completePosts()
    #expect(termination.value && delayedSink.events.last == .shutdownComplete)
}

private final class SwiftTestingFlag: @unchecked Sendable { var value = false }
private final class SwiftTestingCenter: NotificationCenterClient, @unchecked Sendable {
    var settings: NotificationSettings; var authorizationRequests = 0; var posted: [NotificationRequest] = []; var delayed: [@Sendable (Error?) -> Void] = []; let deferPosts: Bool; var authorization: (@Sendable (Bool, Error?) -> Void)?
    init(settings: NotificationSettings, deferPosts: Bool = false) { self.settings = settings; self.deferPosts = deferPosts }
    func getSettings(completion: @escaping @Sendable (NotificationSettings) -> Void) { completion(settings) }
    func requestAuthorization(completion: @escaping @Sendable (Bool, Error?) -> Void) { authorizationRequests += 1; authorization = completion }
    func post(_ request: NotificationRequest, completion: @escaping @Sendable (Error?) -> Void) { posted.append(request); if deferPosts { delayed.append(completion) } else { completion(nil) } }
    func removeNotifications(ids: [String], completion: @escaping @Sendable () -> Void) { completion() }
    func deliveredNotificationIDs(completion: @escaping @Sendable ([String]) -> Void) { completion([]) }
    func resolveAuthorization() { authorization?(true, nil) }
    func completePosts() { let pending = delayed; delayed.removeAll(); pending.forEach { $0(nil) } }
}
private final class SwiftTestingSink: EventSink, @unchecked Sendable { var events: [HelperEvent] = []; func send(_ event: HelperEvent) throws { events.append(event) } }
#endif
