import Foundation
@preconcurrency import UserNotifications
import FuseNotifierCore

final class MacNotificationCenter: NSObject, NotificationCenterClient, @unchecked Sendable {
    let center: UNUserNotificationCenter

    init(center: UNUserNotificationCenter = .current()) {
        self.center = center
    }

    func requestAuthorization() {
        center.requestAuthorization(options: [.alert]) { _, _ in }
    }

    func postGenericNotification(id: String, title: String, body: String, completion: @escaping @Sendable (Error?) -> Void) {
        let content = UNMutableNotificationContent()
        content.title = title
        content.body = body
        content.threadIdentifier = "fuse.approvals"
        content.userInfo = ["pending_id": id]

        let request = UNNotificationRequest(identifier: id, content: content, trigger: nil)
        center.add(request, withCompletionHandler: completion)
    }

    func removeNotifications(ids: [String], completion: @escaping @Sendable () -> Void) {
        center.removePendingNotificationRequests(withIdentifiers: ids)
        center.removeDeliveredNotifications(withIdentifiers: ids)
        completion()
    }
}
