import Foundation
@preconcurrency import UserNotifications
import FuseNotifierCore

final class MacNotificationCenter: NSObject, NotificationCenterClient, @unchecked Sendable {
    let center: UNUserNotificationCenter
    init(center: UNUserNotificationCenter = .current()) { self.center = center }

    func getSettings(completion: @escaping @Sendable (NotificationSettings) -> Void) {
        center.getNotificationSettings { settings in
            let authorization: NotificationAuthorization
            switch settings.authorizationStatus {
            case .notDetermined: authorization = .notDetermined
            case .authorized, .provisional, .ephemeral: authorization = .authorized
            default: authorization = .denied
            }
            completion(NotificationSettings(authorization: authorization, alerts: settings.alertSetting == .enabled ? .enabled : .disabled, sound: settings.soundSetting == .enabled ? .enabled : .disabled))
        }
    }

    func requestAuthorization(completion: @escaping @Sendable (Bool, Error?) -> Void) {
        center.requestAuthorization(options: [.alert, .sound], completionHandler: completion)
    }

    func post(_ request: NotificationRequest, completion: @escaping @Sendable (Error?) -> Void) {
        let content = UNMutableNotificationContent()
        content.title = request.title
        content.body = request.body
        content.threadIdentifier = request.threadIdentifier
        content.userInfo = ["pending_id": request.id]
        if request.playsSound { content.sound = .default }
        center.add(UNNotificationRequest(identifier: request.id, content: content, trigger: nil), withCompletionHandler: completion)
    }

    func removeNotifications(ids: [String], completion: @escaping @Sendable () -> Void) {
        center.removePendingNotificationRequests(withIdentifiers: ids)
        center.removeDeliveredNotifications(withIdentifiers: ids)
        completion()
    }

    func deliveredNotificationIDs(completion: @escaping @Sendable ([String]) -> Void) {
        center.getDeliveredNotifications { completion($0.map(\.request.identifier)) }
    }
}
