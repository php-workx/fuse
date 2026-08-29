import Foundation
import FuseNotifierProtocol

public enum NotificationAuthorization: Equatable, Sendable { case notDetermined, denied, authorized }
public enum NotificationSetting: Equatable, Sendable { case enabled, disabled }
public struct NotificationSettings: Equatable, Sendable {
    public let authorization: NotificationAuthorization
    public let alerts: NotificationSetting
    public let sound: NotificationSetting
    public init(authorization: NotificationAuthorization, alerts: NotificationSetting, sound: NotificationSetting) { self.authorization = authorization; self.alerts = alerts; self.sound = sound }
    public static var notDetermined: NotificationSettings { .init(authorization: .notDetermined, alerts: .disabled, sound: .disabled) }
    public static func authorized(alerts: NotificationSetting, sound: NotificationSetting) -> NotificationSettings { .init(authorization: .authorized, alerts: alerts, sound: sound) }
}

public struct NotificationRequest: Equatable, Sendable {
    public let id: String
    public let title: String
    public let body: String
    public let threadIdentifier: String
    public let playsSound: Bool
    public init(id: String, title: String, body: String, threadIdentifier: String, playsSound: Bool) { self.id = id; self.title = title; self.body = body; self.threadIdentifier = threadIdentifier; self.playsSound = playsSound }
}

public struct PresentationOptions: OptionSet, Equatable, Sendable {
    public let rawValue: UInt
    public init(rawValue: UInt) { self.rawValue = rawValue }
    public static let banner = PresentationOptions(rawValue: 1 << 0)
    public static let list = PresentationOptions(rawValue: 1 << 1)
    public static let sound = PresentationOptions(rawValue: 1 << 2)
}

public protocol NotificationCenterClient: AnyObject, Sendable {
    func getSettings(completion: @escaping @Sendable (NotificationSettings) -> Void)
    func requestAuthorization(completion: @escaping @Sendable (Bool, Error?) -> Void)
    func post(_ request: NotificationRequest, completion: @escaping @Sendable (Error?) -> Void)
    func removeNotifications(ids: [String], completion: @escaping @Sendable () -> Void)
    func deliveredNotificationIDs(completion: @escaping @Sendable ([String]) -> Void)
}

public final class NotificationController: @unchecked Sendable {
    public static let genericTitle = "Fuse approval required"
    public static let genericBody = "Open Fuse to review this request."
    private struct DesiredNotice { let requestID: UInt64; let notice: Notice }

    private let center: any NotificationCenterClient
    private let sink: any EventSink
    private let terminate: @Sendable () -> Void
    private let lock = NSLock()
    private var highestRequestID: UInt64 = 0
    private var desired: [String: DesiredNotice] = [:]
    private var authorizationInFlight = false
    private var soundEnabled = false
    private var soundSettingEnabled = false
    private var inFlightPostCount = 0
    private var shutdownCleanupFinished = false
    private var shutdownCompleted = false
    private var isClosing = false

    public init(center: any NotificationCenterClient, sink: any EventSink, terminate: @escaping @Sendable () -> Void) { self.center = center; self.sink = sink; self.terminate = terminate }

    public func handle(_ command: HelperCommand) {
        guard accept(command.requestID) else { emit(.error(requestID: command.requestID, operation: command.operation, id: commandID(command), code: "non_monotonic_request_id", message: "")); return }
        switch command {
        case let .hello(requestID, _):
            center.getSettings { [weak self] settings in self?.emitReady(requestID: requestID, settings: settings) }
        case let .post(requestID, notice): beginPost(requestID: requestID, notice: notice)
        case let .remove(requestID, id): remove(requestID: requestID, id: id)
        case let .reconcile(requestID, currentIDs): reconcile(requestID: requestID, currentIDs: currentIDs)
        case let .focus(requestID): emit(.focusResult(requestID: requestID, mode: "unavailable", detail: "terminal focus is unavailable"))
        case let .removeAll(requestID): removeAll(requestID: requestID)
        case .shutdown: close()
        }
    }

    public func activate(id: String) {
        lock.lock(); let active = !isClosing && desired[id] != nil; lock.unlock()
        if active { emit(.activated(id: id)) }
    }

    public func didReceiveNotificationAction(identifier: String, pendingID: String?, completion: @escaping () -> Void) {
        completion()
        guard identifier == "default", let pendingID else { return }
        activate(id: pendingID)
    }

    public func willPresent(completion: @escaping (PresentationOptions) -> Void) {
        lock.lock(); let options: PresentationOptions = soundEnabled ? [.banner, .list, .sound] : [.banner, .list]; lock.unlock()
        completion(options)
    }

    public func inputDidReachEOF() { close() }

    private func beginPost(requestID: UInt64, notice: Notice) {
        lock.lock()
        let closing = isClosing
        let duplicate = desired[notice.id] != nil
        if !closing && !duplicate { desired[notice.id] = DesiredNotice(requestID: requestID, notice: notice) }
        lock.unlock()
        if closing { emit(.error(requestID: requestID, operation: "post", id: notice.id, code: "shutting_down", message: "")); return }
        if duplicate { emit(.error(requestID: requestID, operation: "post", id: notice.id, code: "duplicate_id", message: "")); return }
        emit(.result(requestID: requestID, operation: "post", id: notice.id, status: "accepted"))
        evaluate(id: notice.id)
    }

    private func evaluate(id: String) {
        center.getSettings { [weak self] settings in self?.apply(settings: settings, to: id) }
    }

    private func apply(settings: NotificationSettings, to id: String) {
        lock.lock()
        soundSettingEnabled = settings.sound == .enabled
        updateSoundEnabledLocked()
        let desiredNotice = desired[id]
        let closing = isClosing
        let shouldRequestAuthorization = settings.authorization == .notDetermined && !authorizationInFlight && desiredNotice != nil && !closing
        let waitingForAuthorization = settings.authorization == .notDetermined && authorizationInFlight && desiredNotice != nil && !closing
        if shouldRequestAuthorization { authorizationInFlight = true }
        lock.unlock()
        if shouldRequestAuthorization {
            center.requestAuthorization { [weak self] _, _ in self?.authorizationResolved() }
            return
        }
        if waitingForAuthorization { return }
        guard !closing, let desiredNotice else { return }
        guard settings.authorization == .authorized, settings.alerts == .enabled else {
            lock.lock(); desired.removeValue(forKey: id); updateSoundEnabledLocked(); lock.unlock()
            emit(.error(requestID: desiredNotice.requestID, operation: "post", id: id, code: "notifications_not_allowed", message: settings.alerts == .disabled ? "alerts are disabled" : "authorization is denied"))
            return
        }
        let request = NotificationRequest(id: id, title: Self.genericTitle, body: Self.genericBody, threadIdentifier: "fuse.approvals", playsSound: desiredNotice.notice.sound && settings.sound == .enabled)
        lock.lock(); inFlightPostCount += 1; lock.unlock()
        center.post(request) { [weak self] error in self?.postCompleted(id: id, requestID: desiredNotice.requestID, error: error) }
    }

    private func authorizationResolved() {
        lock.lock(); authorizationInFlight = false; let ids = Array(desired.keys); let closing = isClosing; lock.unlock()
        guard !closing else { return }
        for id in ids { evaluate(id: id) }
    }

    private func postCompleted(id: String, requestID: UInt64, error: Error?) {
        lock.lock()
        inFlightPostCount -= 1
        let active = !isClosing && desired[id] != nil
        lock.unlock()
        guard active else {
            if error == nil {
                center.removeNotifications(ids: [id]) { [weak self] in self?.finishShutdownIfReady() }
            } else {
                finishShutdownIfReady()
            }
            return
        }
        if error == nil {
            emit(.posted(requestID: requestID, id: id))
        } else {
            lock.lock(); desired.removeValue(forKey: id); updateSoundEnabledLocked(); lock.unlock()
            emit(.error(requestID: requestID, operation: "post", id: id, code: "notification_post_failed", message: ""))
        }
    }

    private func remove(requestID: UInt64, id: String) {
        lock.lock(); desired.removeValue(forKey: id); updateSoundEnabledLocked(); lock.unlock()
        center.removeNotifications(ids: [id]) { [weak self] in self?.emit(.result(requestID: requestID, operation: "remove", id: id, status: "ok")) }
    }

    private func removeAll(requestID: UInt64) {
        lock.lock(); let ids = Array(desired.keys); desired.removeAll(); updateSoundEnabledLocked(); lock.unlock()
        center.removeNotifications(ids: ids) { [weak self] in self?.emit(.result(requestID: requestID, operation: "remove_all", id: "", status: "ok")) }
    }

    private func reconcile(requestID: UInt64, currentIDs: [String]) {
        center.deliveredNotificationIDs { [weak self] delivered in
            guard let self else { return }
            let current = Set(currentIDs); let stale = delivered.filter { !current.contains($0) }
            self.center.removeNotifications(ids: stale) { [weak self] in self?.emit(.reconciled(requestID: requestID, deliveredCurrentIDs: delivered.filter { current.contains($0) })) }
        }
    }

    private func emitReady(requestID: UInt64, settings: NotificationSettings) { emit(.ready(requestID: requestID, helperVersion: "0.0.0-stage3", protocolMin: 1, protocolMax: 1, authorization: authorizationName(settings.authorization), alertSetting: settingName(settings.alerts), soundSetting: settingName(settings.sound))) }
    private func accept(_ requestID: UInt64) -> Bool { lock.lock(); defer { lock.unlock() }; guard !isClosing, requestID > highestRequestID else { return false }; highestRequestID = requestID; return true }
    private func commandID(_ command: HelperCommand) -> String { switch command { case let .post(_, notice): notice.id; case let .remove(_, id): id; default: "" } }
    private func authorizationName(_ value: NotificationAuthorization) -> String { switch value { case .notDetermined: "not_determined"; case .denied: "denied"; case .authorized: "authorized" } }
    private func settingName(_ value: NotificationSetting) -> String { value == .enabled ? "enabled" : "disabled" }

    private func updateSoundEnabledLocked() { soundEnabled = soundSettingEnabled && desired.values.contains(where: { $0.notice.sound }) }
    private func emit(_ event: HelperEvent) { do { try sink.send(event) } catch { close() } }
    private func close() {
        lock.lock()
        guard !isClosing else { lock.unlock(); return }
        isClosing = true
        let ids = Array(desired.keys)
        desired.removeAll()
        updateSoundEnabledLocked()
        lock.unlock()
        center.removeNotifications(ids: ids) { [weak self] in
            guard let self else { return }
            self.lock.lock()
            self.shutdownCleanupFinished = true
            self.lock.unlock()
            self.finishShutdownIfReady()
        }
    }

    private func finishShutdownIfReady() {
        lock.lock()
        guard isClosing, shutdownCleanupFinished, inFlightPostCount == 0, !shutdownCompleted else { lock.unlock(); return }
        shutdownCompleted = true
        lock.unlock()
        do { try sink.send(.shutdownComplete) } catch {}
        terminate()
    }
}
