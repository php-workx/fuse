import Foundation
import FuseNotifierProtocol

public protocol NotificationCenterClient: AnyObject {
    func requestAuthorization()
    func postGenericNotification(id: String, title: String, body: String, completion: @escaping @Sendable (Error?) -> Void)
    func removeNotifications(ids: [String], completion: @escaping @Sendable () -> Void)
}

public final class NotificationController: @unchecked Sendable {
    public static let genericTitle = "Fuse approval required"
    public static let genericBody = "Open Fuse to review this request."

    private let center: NotificationCenterClient
    private let sink: EventSink
    private let terminate: @Sendable () -> Void
    private let lock = NSLock()
    private var highestRequestID: UInt64 = 0
    private var ownedIDs = Set<String>()
    private var isClosing = false

    public init(center: NotificationCenterClient, sink: EventSink, terminate: @escaping @Sendable () -> Void) {
        self.center = center
        self.sink = sink
        self.terminate = terminate
    }

    public func requestAuthorization() {
        center.requestAuthorization()
    }

    public func handle(_ command: HelperCommand) {
        guard accept(requestID: command.requestID) else {
            switch command {
            case let .post(requestID, id):
                sink.send(.error(requestID: requestID, operation: "post", id: id, code: "non_monotonic_request_id"))
            case let .hello(requestID, _):
                sink.send(.error(requestID: requestID, operation: "hello", id: "", code: "non_monotonic_request_id"))
            }
            return
        }

        switch command {
        case let .hello(requestID, _):
            sink.send(.ready(requestID: requestID, helperVersion: "0.0.0-stage0"))
        case let .post(requestID, id):
            lock.lock()
            let closing = isClosing
            if !closing {
                ownedIDs.insert(id)
            }
            lock.unlock()
            guard !closing else {
                sink.send(.error(requestID: requestID, operation: "post", id: id, code: "shutting_down"))
                return
            }

            center.postGenericNotification(id: id, title: Self.genericTitle, body: Self.genericBody) { [weak self] error in
                guard let self else { return }
                if error == nil {
                    self.sink.send(.result(requestID: requestID, operation: "post", id: id, status: "accepted"))
                } else {
                    self.removeOwnedID(id)
                    self.sink.send(.error(requestID: requestID, operation: "post", id: id, code: "notification_post_failed"))
                }
            }
        }
    }

    public func activate(id: String) {
        lock.lock()
        let isOwned = ownedIDs.contains(id) && !isClosing
        lock.unlock()
        guard isOwned else { return }
        sink.send(.activated(id: id))
    }

    public func inputDidReachEOF() {
        close()
    }

    private func accept(requestID: UInt64) -> Bool {
        lock.lock()
        defer { lock.unlock() }
        guard !isClosing, requestID > highestRequestID else { return false }
        highestRequestID = requestID
        return true
    }

    private func removeOwnedID(_ id: String) {
        lock.lock()
        ownedIDs.remove(id)
        lock.unlock()
    }

    private func close() {
        lock.lock()
        guard !isClosing else {
            lock.unlock()
            return
        }
        isClosing = true
        let ids = Array(ownedIDs)
        ownedIDs.removeAll()
        lock.unlock()

        center.removeNotifications(ids: ids) { [sink, terminate] in
            sink.send(.shutdownComplete)
            terminate()
        }
    }
}
