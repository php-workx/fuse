import FuseNotifierCore
import FuseNotifierProtocol

protocol HelperTransport: EventSink {
    func start(onCommand: @escaping @Sendable (HelperCommand) -> Void, onEOF: @escaping @Sendable () -> Void)
}
