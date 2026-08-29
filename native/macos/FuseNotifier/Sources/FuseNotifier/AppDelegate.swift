import AppKit
@preconcurrency import UserNotifications
import FuseNotifierCore
import FuseNotifierProtocol

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate, @preconcurrency UNUserNotificationCenterDelegate {
    private let notificationCenter = MacNotificationCenter()
    private let transport: any HelperTransport
    private lazy var controller = NotificationController(
        center: notificationCenter,
        sink: transport,
        terminate: {
            DispatchQueue.main.async {
                NSApplication.shared.terminate(nil)
            }
        }
    )

    init(transport: any HelperTransport) {
        self.transport = transport
    }

    func applicationWillFinishLaunching(_ notification: Notification) {
        notificationCenter.center.delegate = self
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        controller.requestAuthorization()
        let controller = controller
        transport.start(
            onCommand: { command in
                controller.handle(command)
            },
            onEOF: {
                controller.inputDidReachEOF()
            }
        )
    }

    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse,
        withCompletionHandler completionHandler: @escaping () -> Void
    ) {
        let id = response.notification.request.content.userInfo["pending_id"] as? String
        let isDefaultAction = response.actionIdentifier == UNNotificationDefaultActionIdentifier
        completionHandler()
        if isDefaultAction, let id {
            controller.activate(id: id)
        }
    }

    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        completionHandler([.banner, .list])
    }
}
