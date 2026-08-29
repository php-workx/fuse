import FuseNotifierProtocol

public protocol EventSink: AnyObject, Sendable {
    func send(_ event: HelperEvent)
}
