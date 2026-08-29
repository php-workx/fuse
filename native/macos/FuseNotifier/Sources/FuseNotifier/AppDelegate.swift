import AppKit
@preconcurrency import UserNotifications
import FuseNotifierCore
import FuseNotifierProtocol

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate, @preconcurrency UNUserNotificationCenterDelegate {
    private let notificationCenter = MacNotificationCenter()
    private let transport: any HelperTransport
    private lazy var controller = NotificationController(center: notificationCenter, sink: transport, terminate: { DispatchQueue.main.async { NSApplication.shared.terminate(nil) } })

    init(transport: any HelperTransport) { self.transport = transport }
    func applicationWillFinishLaunching(_ notification: Notification) { notificationCenter.center.delegate = self }
    func applicationDidFinishLaunching(_ notification: Notification) {
        let controller = controller
        transport.start(onCommand: { controller.handle($0) }, onEOF: { controller.inputDidReachEOF() })
    }

    func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse, withCompletionHandler completionHandler: @escaping () -> Void) {
        controller.didReceiveNotificationAction(identifier: response.actionIdentifier == UNNotificationDefaultActionIdentifier ? "default" : response.actionIdentifier, pendingID: response.notification.request.content.userInfo["pending_id"] as? String, completion: completionHandler)
    }

    func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification, withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void) {
        controller.willPresent { options in
            var presentation: UNNotificationPresentationOptions = [.banner, .list]
            if options.contains(.sound) { presentation.insert(.sound) }
            completionHandler(presentation)
        }
    }
}
