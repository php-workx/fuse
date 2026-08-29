import Foundation
import FuseNotifierProtocol

func expect(_ condition: @autoclosure () -> Bool, _ message: String) {
    if !condition() {
        fatalError(message)
    }
}

func expectProtocolError(_ expected: ProtocolError, _ operation: () throws -> Void) {
    do {
        try operation()
        fatalError("Expected protocol error \(expected)")
    } catch let actual as ProtocolError {
        expect(actual == expected, "Expected \(expected), received \(actual)")
    } catch {
        fatalError("Expected ProtocolError, received \(error)")
    }
}

do {
    let hello = try JSONLineCodec.decodeCommand(Data(#"{"version":1,"type":"hello","request_id":1,"client_version":"test"}"#.utf8))
    expect(hello == .hello(requestID: 1, clientVersion: "test"), "Hello command did not decode")

    let oversized = Data(repeating: 0x20, count: JSONLineCodec.maximumLineBytes + 1)
    expectProtocolError(.lineTooLong) {
        _ = try JSONLineCodec.decodeCommand(oversized)
    }

    let identifier = String(repeating: "a", count: JSONLineCodec.maximumDisplayScalars + 1)
    let post = try JSONSerialization.data(withJSONObject: [
        "version": 1,
        "type": "post",
        "request_id": 2,
        "id": identifier,
    ])
    expectProtocolError(.fieldTooLong("id")) {
        _ = try JSONLineCodec.decodeCommand(post)
    }

    let encoded = try JSONLineCodec.encode(.ready(requestID: 1, helperVersion: "0.0.0"))
    expect(encoded.last == 0x0A, "Events must end with a newline")
    expect(encoded.count <= JSONLineCodec.maximumLineBytes, "Encoded event exceeded the line bound")
} catch {
    fatalError("Unexpected protocol test failure: \(error)")
}
